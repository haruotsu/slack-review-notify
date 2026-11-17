package handlers

import (
	"log"
	"net/http"
	"slack-review-notify/i18n"
	"slack-review-notify/models"
	"slack-review-notify/services"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// openMenuModal opens the menu via views.open. The trigger_id expires a few
// seconds after Slack issues it, so the API call runs in the background and the
// caller answers Slack immediately.
func openMenuModal(db *gorm.DB, triggerID, channelID, userID string) {
	if channelID == "" {
		// A menu without its channel opens fine and then ignores every click.
		log.Printf("menu not opened: no channel in the request (user=%s)", userID)
		return
	}
	view := menuModalView(db, channelID, userID)

	runAsync(func() {
		if err := services.OpenView(triggerID, view); err != nil {
			log.Printf("menu views.open failed: %v", err)
		}
	})
}

func menuModalView(db *gorm.DB, channelID, userID string) map[string]any {
	lang := pickModalLanguage(loadChannelConfigs(db, channelID), "")
	return services.BuildMenuModalView(services.MenuModalMetadata{ChannelID: channelID, UserID: userID}, lang)
}

func handleOpenMenu(c *gin.Context, db *gorm.DB, payload SlackActionPayload) {
	openMenuModal(db, payload.TriggerID, payload.Container.ChannelID, payload.User.ID)
	c.Status(http.StatusOK)
}

// handleMenuAction pushes the view for the menu button that was clicked on top
// of the menu, so closing or submitting it lands the user back on the menu.
func handleMenuAction(c *gin.Context, db *gorm.DB, payload SlackActionPayload) {
	view, ok := menuActionView(db, payload)
	if !ok {
		c.Status(http.StatusOK)
		return
	}
	triggerID := payload.TriggerID

	runAsync(func() {
		if err := services.PushView(triggerID, view); err != nil {
			log.Printf("menu views.push failed: %v", err)
		}
	})

	c.Status(http.StatusOK)
}

// menuContext recovers the channel the menu was opened from. ok is false when
// the payload carries no usable view: without the channel no operation can be
// scoped, and guessing one would read or change another channel's settings.
func menuContext(payload SlackActionPayload) (services.MenuModalMetadata, bool) {
	if payload.View == nil {
		return services.MenuModalMetadata{}, false
	}
	meta, err := services.DecodeMenuModalMetadata(payload.View.PrivateMetadata)
	if err != nil || meta.ChannelID == "" {
		log.Printf("menu action has unusable private_metadata: %q (err=%v)", payload.View.PrivateMetadata, err)
		return services.MenuModalMetadata{}, false
	}
	// The clicker is authoritative; the metadata's user is whoever opened the menu.
	meta.UserID = payload.User.ID
	return meta, true
}

// menuActionView builds the view a menu button opens. ok is false for an
// unknown operation or a payload without the menu's context.
func menuActionView(db *gorm.DB, payload SlackActionPayload) (map[string]any, bool) {
	meta, ok := menuContext(payload)
	if !ok || len(payload.Actions) == 0 {
		return nil, false
	}
	op := strings.TrimPrefix(payload.Actions[0].ActionID, services.MenuActionPrefix)

	configs := loadChannelConfigs(db, meta.ChannelID)
	lang := pickModalLanguage(configs, "")
	result := func(texts ...string) (map[string]any, bool) {
		return services.BuildMenuResultView(meta, lang, services.MenuOpLabel(op, lang), texts), true
	}
	perLabel := func(text func(db *gorm.DB, channelID, labelName, lang string) string) (map[string]any, bool) {
		if len(configs) == 0 {
			return result(i18n.TWithLang(lang, "cmd.show.no_config"))
		}
		texts := make([]string, 0, len(configs))
		for _, cfg := range configs {
			texts = append(texts, text(db, meta.ChannelID, cfg.LabelName, lang))
		}
		return result(texts...)
	}

	switch op {
	case services.MenuOpShowLabels:
		return result(allLabelsText(db, meta.ChannelID, lang))
	case services.MenuOpShowLabelDetails:
		return perLabel(configText)
	case services.MenuOpShowReviewers:
		return perLabel(reviewersText)
	case services.MenuOpShowMyReviews:
		return result(assignedReviewsText(db, meta.UserID, lang))
	case services.MenuOpShowUserMappings:
		return result(userMappingsText(db, lang))
	case services.MenuOpShowAvailability:
		return menuAvailabilityView(db, meta, lang), true
	case services.MenuOpEditSettings:
		return settingsModalView(db, meta.ChannelID, meta.UserID, menuDefaultLabel(configs)), true
	case services.MenuOpNewLabel:
		return settingsModalView(db, meta.ChannelID, meta.UserID, services.CreateNewLabelSentinel), true
	case services.MenuOpAway:
		return newAwayModalView(db, meta.ChannelID, meta.UserID, services.DefaultAwayModalPrefill()), true
	case services.MenuOpUserMapping:
		return userMappingModalView(db, meta.ChannelID, meta.UserID), true
	}

	log.Printf("unknown menu operation: %q", op)
	return nil, false
}

// With no configs at all nothing here has to choose create-new: the settings
// modal falls back to it by itself for a label it cannot find.
func menuDefaultLabel(configs []*models.ChannelConfig) string {
	for _, cfg := range configs {
		if cfg.LabelName == defaultLabelName {
			return defaultLabelName
		}
	}
	if len(configs) > 0 {
		return configs[0].LabelName
	}
	return defaultLabelName
}

func menuAvailabilityView(db *gorm.DB, meta services.MenuModalMetadata, lang string) map[string]any {
	loc := resolveAwayTimezone(db, meta.ChannelID)
	now := time.Now()

	records := activeAvailabilityRecords(db, now)
	rows := make([]services.MenuAvailabilityRow, 0, len(records))
	for _, r := range records {
		rows = append(rows, services.MenuAvailabilityRow{ID: r.ID, Text: availabilityLine(r, loc, now, lang)})
	}
	return services.BuildMenuAvailabilityView(meta, lang, rows)
}

// Posted to the channel rather than ephemerally, so teammates see the leave was cleared.
func menuUnsetAwayNotice(db *gorm.DB, channelID string, record models.ReviewerAvailability, lang string) string {
	t := i18n.L(lang)
	loc := resolveAwayTimezone(db, channelID)
	period := formatDateRange(models.InLocation(record.AwayFrom, loc), models.InLocation(record.AwayUntil, loc), t)
	return t("modal.menu.availability.unset_done", record.SlackUserID, period)
}

// handleMenuUnsetAway removes the single leave period whose row button was
// clicked, then re-renders the list in place.
func handleMenuUnsetAway(c *gin.Context, db *gorm.DB, payload SlackActionPayload) {
	meta, ok := menuContext(payload)
	if !ok || len(payload.Actions) == 0 {
		c.Status(http.StatusOK)
		return
	}
	lang := pickModalLanguage(loadChannelConfigs(db, meta.ChannelID), "")

	// A row that is already gone (removed from another modal or by command) is
	// not an error: the refreshed list below shows the current state either way.
	notice := ""
	var record models.ReviewerAvailability
	if err := db.Where("id = ?", payload.Actions[0].Value).First(&record).Error; err == nil {
		// Unscoped like every other leave removal, so no soft-deleted row lingers.
		if err := db.Unscoped().Delete(&record).Error; err != nil {
			log.Printf("menu unset-away failed: id=%s err=%v", record.ID, err)
		} else {
			notice = menuUnsetAwayNotice(db, meta.ChannelID, record, lang)
		}
	}

	view := menuAvailabilityView(db, meta, lang)
	viewID := payload.View.ID
	runAsync(func() {
		// Posted from the background like the view update: Slack shows the
		// clicker an error unless the action is acknowledged within seconds.
		if notice != "" {
			if err := services.PostChannelMessage(meta.ChannelID, notice); err != nil {
				log.Printf("menu unset-away confirmation post failed: %v", err)
			}
		}
		if err := services.UpdateView(viewID, view); err != nil {
			log.Printf("menu availability views.update failed: %v", err)
		}
	})

	c.Status(http.StatusOK)
}
