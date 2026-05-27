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

// handleOpenSettings is invoked when the user clicks the "Open Settings" button on
// the help response. It looks up the existing ChannelConfig (if any) for the channel/label
// and opens a pre-filled modal via views.open.
func handleOpenSettings(c *gin.Context, db *gorm.DB, payload SlackActionPayload) {
	channelID := payload.Container.ChannelID
	labelName := payload.Actions[0].Value
	if labelName == "" {
		labelName = "needs-review"
	}

	var cfg models.ChannelConfig
	hasCfg := db.Where("slack_channel_id = ? AND label_name = ?", channelID, labelName).First(&cfg).Error == nil

	// If the user clicked the generic "open settings" button (no specific label),
	// fall back to the first config in the channel so the modal isn't empty.
	if !hasCfg {
		if err := db.Where("slack_channel_id = ?", channelID).First(&cfg).Error; err == nil {
			hasCfg = true
		}
	}

	lang := "ja"
	if hasCfg {
		lang = cfg.Language
		if lang == "" {
			lang = "ja"
		}
	}

	var cfgPtr *models.ChannelConfig
	if hasCfg {
		cfgPtr = &cfg
	}
	view := services.BuildSettingsModalView(channelID, cfgPtr, lang)

	if err := services.OpenView(payload.TriggerID, view); err != nil {
		log.Printf("views.open failed: %v", err)
		// Still return 200 — Slack treats non-200 as a retry trigger.
	}

	c.Status(http.StatusOK)
}

// handleSettingsModalSubmission parses the view_submission payload and upserts the
// corresponding ChannelConfig. On validation errors, returns the Slack-mandated
// response_action:errors body so Slack renders inline messages.
func handleSettingsModalSubmission(c *gin.Context, db *gorm.DB, payload SlackActionPayload) {
	channelID := payload.View.PrivateMetadata
	if channelID == "" {
		log.Printf("view_submission missing private_metadata (channel ID)")
		c.JSON(http.StatusOK, gin.H{
			"response_action": "errors",
			"errors":          gin.H{"label_name": "internal error: missing channel context"},
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
			"errors":          gin.H{"label_name": err.Error()},
		})
		return
	}

	var cfg models.ChannelConfig
	now := time.Now()
	result := db.Where("slack_channel_id = ? AND label_name = ?", channelID, form.LabelName).First(&cfg)
	if result.Error != nil {
		cfg = models.ChannelConfig{
			ID:             uuid.NewString(),
			SlackChannelID: channelID,
			LabelName:      form.LabelName,
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
				"errors":          gin.H{"label_name": "save failed"},
			})
			return
		}
	} else {
		if err := db.Save(&cfg).Error; err != nil {
			log.Printf("failed to update config from modal: %v", err)
			c.JSON(http.StatusOK, gin.H{
				"response_action": "errors",
				"errors":          gin.H{"label_name": "save failed"},
			})
			return
		}
	}

	// Post a confirmation message to the channel so the user gets feedback.
	if !services.IsTestMode {
		msg := i18n.TWithLang(form.Language, "modal.saved", form.LabelName)
		if err := services.PostToThread(channelID, "", msg); err != nil {
			// Not fatal — log only.
			log.Printf("settings saved confirmation post failed: %v", err)
		}
	}

	// Empty 200 OK closes the modal.
	c.Status(http.StatusOK)
}
