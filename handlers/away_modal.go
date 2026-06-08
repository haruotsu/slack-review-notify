package handlers

import (
	"errors"
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
		// PrivateMetadata is JSON we control, so the language hint inside is
		// unreachable on decode failure. Fall back to ja for the error label.
		c.JSON(http.StatusOK, gin.H{
			"response_action": "errors",
			"errors":          gin.H{"away_user": i18n.TWithLang("ja", "modal.away.error.context_lost")},
		})
		return
	}

	configs := loadChannelConfigs(db, meta.ChannelID)
	lang := pickModalLanguage(configs, "")
	loc := pickModalTimezone(configs)

	form, err := services.ParseAwayModalSubmission(payload.View.State.Values, loc, lang)
	if err != nil {
		var ve *services.ModalValidationError
		if errors.As(err, &ve) {
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

	if form.DeleteAll {
		// `unset-away @user` (no date) semantics: wipe every row, hard delete
		// so the (slack_user_id) index doesn't collide on a future re-add.
		res := db.Unscoped().Where("slack_user_id = ?", form.SlackUserID).Delete(&models.ReviewerAvailability{})
		if res.Error != nil {
			log.Printf("away delete-all failed: user=%s err=%v", form.SlackUserID, res.Error)
			c.JSON(http.StatusOK, gin.H{
				"response_action": "errors",
				"errors":          gin.H{"away_delete_all": i18n.TWithLang(lang, "modal.away.error.delete_failed")},
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
	query := models.MatchPeriod(db.Where("slack_user_id = ?", form.SlackUserID), form.AwayFrom, form.AwayUntil)
	err = query.First(&existing).Error
	switch {
	case err == nil:
		existing.Reason = form.Reason
		existing.UpdatedAt = now
		if err := db.Save(&existing).Error; err != nil {
			log.Printf("away upsert update failed: user=%s err=%v", form.SlackUserID, err)
			c.JSON(http.StatusOK, gin.H{
				"response_action": "errors",
				"errors":          gin.H{"away_user": i18n.TWithLang(lang, "modal.away.error.save_failed")},
			})
			return
		}
	case errors.Is(err, gorm.ErrRecordNotFound):
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
				"errors":          gin.H{"away_user": i18n.TWithLang(lang, "modal.away.error.save_failed")},
			})
			return
		}
	default:
		// Any other DB error must not silently fall through to Create.
		log.Printf("away lookup failed: user=%s err=%v", form.SlackUserID, err)
		c.JSON(http.StatusOK, gin.H{
			"response_action": "errors",
			"errors":          gin.H{"away_user": i18n.TWithLang(lang, "modal.away.error.save_failed")},
		})
		return
	}

	if !services.IsTestMode && meta.UserID != "" {
		msg := i18n.TWithLang(lang, "modal.away.saved", form.SlackUserID)
		if err := services.PostEphemeral(meta.ChannelID, meta.UserID, msg); err != nil {
			log.Printf("away saved confirmation post failed: %v", err)
		}
	}

	c.Status(http.StatusOK)
}
