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

// handleOpenSettings is invoked when the user clicks the "Open Settings" button.
// It resolves the (channel, label) ChannelConfig (if any), then opens a pre-filled
// modal via views.open. The label name is captured here and round-tripped through
// `view.private_metadata` so the submission handler updates exactly that row —
// the modal does NOT let the user retarget another label.
//
// Slack's trigger_id expires ~3 seconds after issue, so we kick off the views.open
// call asynchronously and ack the action immediately.
func handleOpenSettings(c *gin.Context, db *gorm.DB, payload SlackActionPayload) {
	channelID := payload.Container.ChannelID
	userID := payload.User.ID
	labelName := payload.Actions[0].Value
	if labelName == "" {
		labelName = "needs-review"
	}

	var cfg models.ChannelConfig
	var cfgPtr *models.ChannelConfig
	if err := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&cfg).Error; err == nil {
		cfgPtr = &cfg
	}
	// No silent fallback to other labels: an empty modal for a fresh label is
	// clearer than an unexpected pre-fill from an unrelated config.

	lang := "ja"
	if cfgPtr != nil && cfgPtr.Language != "" {
		lang = cfgPtr.Language
	}

	view := services.BuildSettingsModalView(channelID, labelName, userID, cfgPtr, lang)
	triggerID := payload.TriggerID

	// Ack first; open the view in the background to stay within the 3s trigger window
	// even if the Slack API is slow.
	go func() {
		if err := services.OpenView(triggerID, view); err != nil {
			log.Printf("views.open failed: %v", err)
		}
	}()

	c.Status(http.StatusOK)
}

// handleSettingsModalSubmission parses the view_submission payload and upserts the
// corresponding ChannelConfig. The (channel, label) target is read from
// private_metadata, not from form fields, so the modal cannot retarget another
// label. On validation errors, returns response_action:errors for inline display.
func handleSettingsModalSubmission(c *gin.Context, db *gorm.DB, payload SlackActionPayload) {
	meta, err := services.DecodeSettingsModalMetadata(payload.View.PrivateMetadata)
	if err != nil || meta.ChannelID == "" || meta.LabelName == "" {
		log.Printf("view_submission has invalid private_metadata: %q (err=%v)", payload.View.PrivateMetadata, err)
		c.JSON(http.StatusOK, gin.H{
			"response_action": "errors",
			"errors":          gin.H{"default_mention_id": "internal error: modal context lost. Please reopen the settings."},
		})
		return
	}

	form, err := services.ParseSettingsModalSubmission(payload.View.State.Values)
	if err != nil {
		if ve, ok := err.(*services.ModalValidationError); ok {
			c.JSON(http.StatusOK, gin.H{
				"response_action": "errors",
				"errors":          ve.Errors,
			})
			return
		}
		log.Printf("settings modal parse error: %v", err)
		c.JSON(http.StatusOK, gin.H{
			"response_action": "errors",
			"errors":          gin.H{"default_mention_id": err.Error()},
		})
		return
	}

	var cfg models.ChannelConfig
	now := time.Now()
	result := db.Where("slack_channel_id = ? AND label_name = ?", meta.ChannelID, meta.LabelName).First(&cfg)
	if result.Error != nil {
		cfg = models.ChannelConfig{
			ID:             uuid.NewString(),
			SlackChannelID: meta.ChannelID,
			LabelName:      meta.LabelName,
			CreatedAt:      now,
		}
	}
	cfg.DefaultMentionID = form.DefaultMentionID
	cfg.ReviewerList = form.ReviewerList
	cfg.RepositoryList = form.RepositoryList
	cfg.ReviewerReminderInterval = form.ReviewerReminderInterval
	cfg.BusinessHoursStart = form.BusinessHoursStart
	cfg.BusinessHoursEnd = form.BusinessHoursEnd
	cfg.Timezone = form.Timezone
	cfg.RequiredApprovals = form.RequiredApprovals
	cfg.Language = form.Language
	cfg.IsActive = form.IsActive
	cfg.UpdatedAt = now

	if result.Error != nil {
		if err := db.Create(&cfg).Error; err != nil {
			log.Printf("failed to create config from modal: %v", err)
			c.JSON(http.StatusOK, gin.H{
				"response_action": "errors",
				"errors":          gin.H{"default_mention_id": "save failed"},
			})
			return
		}
	} else {
		if err := db.Save(&cfg).Error; err != nil {
			log.Printf("failed to update config from modal: %v", err)
			c.JSON(http.StatusOK, gin.H{
				"response_action": "errors",
				"errors":          gin.H{"default_mention_id": "save failed"},
			})
			return
		}
	}

	// Confirm to the submitting user only — avoids broadcasting setting changes
	// to the whole channel.
	if !services.IsTestMode && meta.UserID != "" {
		msg := i18n.TWithLang(form.Language, "modal.saved", meta.LabelName)
		if err := services.PostEphemeral(meta.ChannelID, meta.UserID, msg); err != nil {
			log.Printf("settings saved confirmation post failed: %v", err)
		}
	}

	c.Status(http.StatusOK)
}
