package handlers

import (
	"log"
	"net/http"
	"slack-review-notify/i18n"
	"slack-review-notify/models"
	"slack-review-notify/services"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// handleOpenAwayManagement opens the away-management modal via views.open
// when the "🌴 休暇管理を開く" button is clicked. trigger_id expires within
// a few seconds so the API call is fired in a goroutine; the HTTP response
// to Slack must return immediately.
func handleOpenAwayManagement(c *gin.Context, db *gorm.DB, payload SlackActionPayload) {
	channelID := payload.Container.ChannelID
	userID := payload.User.ID

	configs := loadChannelConfigs(db, channelID)
	lang := pickModalLanguage(configs, "")

	view := services.BuildAwayManagementModalView(services.AwayManagementModalInputs{
		ChannelID: channelID,
		UserID:    userID,
		Lang:      lang,
	})
	triggerID := payload.TriggerID

	go func() {
		if err := services.OpenView(triggerID, view); err != nil {
			log.Printf("away views.open failed: %v", err)
		}
	}()

	c.Status(http.StatusOK)
}

// handleAwayModalSubmission applies the parsed form to ReviewerAvailability:
// either delete every row for the target user (delete-all branch) or upsert
// a single leave period. Validation errors are surfaced as
// response_action=errors so Slack highlights the offending field in-place.
func handleAwayModalSubmission(c *gin.Context, db *gorm.DB, payload SlackActionPayload) {
	meta, err := services.DecodeAwayModalMetadata(payload.View.PrivateMetadata)
	if err != nil {
		log.Printf("away view_submission has invalid private_metadata: %q (err=%v)", payload.View.PrivateMetadata, err)
		c.JSON(http.StatusOK, gin.H{
			"response_action": "errors",
			"errors":          gin.H{"away_user": "internal error: modal context lost. Please reopen."},
		})
		return
	}

	form, err := services.ParseAwayModalSubmission(payload.View.State.Values)
	if err != nil {
		if ve, ok := err.(*services.ModalValidationError); ok {
			c.JSON(http.StatusOK, gin.H{
				"response_action": "errors",
				"errors":          ve.Errors,
			})
			return
		}
		log.Printf("away modal parse error: %v", err)
		c.JSON(http.StatusOK, gin.H{
			"response_action": "errors",
			"errors":          gin.H{"away_user": err.Error()},
		})
		return
	}

	// Pick the modal language for the post-save confirmation. There's no
	// per-user language preference; fall back to whatever the modal-opening
	// channel had configured, defaulting to ja.
	lang := pickModalLanguage(loadChannelConfigs(db, meta.ChannelID), "")

	if form.DeleteAll {
		// `unset-away @user` (no date) semantics: wipe every row, hard delete
		// so the (slack_user_id) index doesn't collide on a future re-add.
		res := db.Unscoped().Where("slack_user_id = ?", form.SlackUserID).Delete(&models.ReviewerAvailability{})
		if res.Error != nil {
			log.Printf("away delete-all failed: user=%s err=%v", form.SlackUserID, res.Error)
			c.JSON(http.StatusOK, gin.H{
				"response_action": "errors",
				"errors":          gin.H{"away_delete_all": "delete failed"},
			})
			return
		}
		if !services.IsTestMode && meta.UserID != "" {
			var msg string
			if res.RowsAffected == 0 {
				msg = i18n.TWithLang(lang, "modal.away.nothing_deleted", form.SlackUserID)
			} else {
				msg = i18n.TWithLang(lang, "modal.away.deleted", form.SlackUserID)
			}
			if err := services.PostEphemeral(meta.ChannelID, meta.UserID, msg); err != nil {
				log.Printf("away delete confirmation post failed: %v", err)
			}
		}
		c.Status(http.StatusOK)
		return
	}

	// Set/upsert path: same shape as the set-away slash command. An identical
	// period (matching slack_user_id + away_from + away_until, NULL-aware)
	// updates Reason only; a new period inserts a fresh row.
	now := time.Now()
	var existing models.ReviewerAvailability
	query := db.Where("slack_user_id = ?", form.SlackUserID)
	query = applyAwayPeriodMatch(query, form.AwayFrom, form.AwayUntil)
	switch err := query.First(&existing).Error; err {
	case nil:
		existing.Reason = form.Reason
		existing.UpdatedAt = now
		if err := db.Save(&existing).Error; err != nil {
			log.Printf("away upsert update failed: user=%s err=%v", form.SlackUserID, err)
			c.JSON(http.StatusOK, gin.H{
				"response_action": "errors",
				"errors":          gin.H{"away_user": "save failed"},
			})
			return
		}
	default:
		// First() returns ErrRecordNotFound when no match exists. Treat any
		// non-nil error (including transient DB errors) as "not found" only
		// when it IS the not-found sentinel; otherwise log and bail.
		if err != gorm.ErrRecordNotFound {
			log.Printf("away lookup failed: user=%s err=%v", form.SlackUserID, err)
			c.JSON(http.StatusOK, gin.H{
				"response_action": "errors",
				"errors":          gin.H{"away_user": "save failed"},
			})
			return
		}
		record := models.ReviewerAvailability{
			ID:          uuid.NewString(),
			SlackUserID: form.SlackUserID,
			AwayFrom:    form.AwayFrom,
			AwayUntil:   form.AwayUntil,
			Reason:      form.Reason,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := db.Create(&record).Error; err != nil {
			log.Printf("away create failed: user=%s err=%v", form.SlackUserID, err)
			c.JSON(http.StatusOK, gin.H{
				"response_action": "errors",
				"errors":          gin.H{"away_user": "save failed"},
			})
			return
		}
	}

	if !services.IsTestMode && meta.UserID != "" {
		msg := i18n.TWithLang(lang, "modal.away.saved", form.SlackUserID)
		if err := services.PostEphemeral(meta.ChannelID, meta.UserID, msg); err != nil {
			log.Printf("away saved confirmation post failed: %v", err)
		}
	}

	c.Status(http.StatusOK)
}

// applyAwayPeriodMatch narrows a query so it matches a row with the same
// (away_from, away_until) tuple as the form. Required because GORM's
// `Where("away_from = ?", nil)` doesn't translate to `IS NULL`.
func applyAwayPeriodMatch(q *gorm.DB, from, until *time.Time) *gorm.DB {
	if from == nil {
		q = q.Where("away_from IS NULL")
	} else {
		q = q.Where("away_from = ?", from)
	}
	if until == nil {
		q = q.Where("away_until IS NULL")
	} else {
		q = q.Where("away_until = ?", until)
	}
	return q
}
