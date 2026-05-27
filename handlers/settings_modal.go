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

// loadChannelConfigs returns every ChannelConfig row for a channel, used to
// populate the modal's label dropdown.
func loadChannelConfigs(db *gorm.DB, channelID string) []*models.ChannelConfig {
	var configs []*models.ChannelConfig
	if err := db.Where("slack_channel_id = ?", channelID).Order("label_name").Find(&configs).Error; err != nil {
		log.Printf("loadChannelConfigs failed: %v", err)
		return nil
	}
	return configs
}

// pickModalLanguage returns the language for the modal, preferring the existing
// row's setting (if any). Falls back to "ja" when nothing is configured yet.
func pickModalLanguage(configs []*models.ChannelConfig, selectedLabel string) string {
	for _, c := range configs {
		if c.LabelName == selectedLabel && c.Language != "" {
			return c.Language
		}
	}
	for _, c := range configs {
		if c.Language != "" {
			return c.Language
		}
	}
	return "ja"
}

// handleOpenSettings is invoked when the user clicks an "Open Settings" button.
// The button's value carries the initially-selected label (or the create-new
// sentinel). We load all the channel's label configs and open the modal with
// the dropdown preselected accordingly. Slack's trigger_id expires ~3 seconds
// after issue, so views.open runs asynchronously.
func handleOpenSettings(c *gin.Context, db *gorm.DB, payload SlackActionPayload) {
	channelID := payload.Container.ChannelID
	userID := payload.User.ID
	selectedLabel := payload.Actions[0].Value
	if selectedLabel == "" {
		selectedLabel = "needs-review"
	}

	configs := loadChannelConfigs(db, channelID)
	lang := pickModalLanguage(configs, selectedLabel)

	view := services.BuildSettingsModalView(services.SettingsModalInputs{
		ChannelID:     channelID,
		UserID:        userID,
		SelectedLabel: selectedLabel,
		Configs:       configs,
		Lang:          lang,
	})
	triggerID := payload.TriggerID

	go func() {
		if err := services.OpenView(triggerID, view); err != nil {
			log.Printf("views.open failed: %v", err)
		}
	}()

	c.Status(http.StatusOK)
}

// handleLabelSelectChanged is the dispatch_action callback for the label
// dropdown. When the user picks a different label (or the create-new sentinel),
// we rebuild the modal with the chosen label's prefilled values and push the
// new view via views.update — Slack does NOT persist field values across
// re-renders, so the other fields visually "reset" to the selected label.
func handleLabelSelectChanged(c *gin.Context, db *gorm.DB, payload SlackActionPayload) {
	if payload.View == nil {
		c.Status(http.StatusOK)
		return
	}
	meta, err := services.DecodeSettingsModalMetadata(payload.View.PrivateMetadata)
	if err != nil || meta.ChannelID == "" {
		log.Printf("label_select dispatch with bad private_metadata: %v", err)
		c.Status(http.StatusOK)
		return
	}

	newSelected := payload.Actions[0].SelectedOption.Value
	if newSelected == "" {
		newSelected = services.CreateNewLabelSentinel
	}

	configs := loadChannelConfigs(db, meta.ChannelID)
	lang := pickModalLanguage(configs, newSelected)

	view := services.BuildSettingsModalView(services.SettingsModalInputs{
		ChannelID:     meta.ChannelID,
		UserID:        meta.UserID,
		SelectedLabel: newSelected,
		Configs:       configs,
		Lang:          lang,
	})

	viewID := payload.View.ID
	go func() {
		if err := services.UpdateView(viewID, view); err != nil {
			log.Printf("views.update failed: %v", err)
		}
	}()

	c.Status(http.StatusOK)
}

// handleSettingsModalSubmission parses the view_submission payload and applies
// the change: upsert when editing/creating, soft-delete when the delete
// checkbox is checked. The target (channel, label) is derived from form values
// plus private_metadata, NOT from any free-text field.
func handleSettingsModalSubmission(c *gin.Context, db *gorm.DB, payload SlackActionPayload) {
	meta, err := services.DecodeSettingsModalMetadata(payload.View.PrivateMetadata)
	if err != nil || meta.ChannelID == "" {
		log.Printf("view_submission has invalid private_metadata: %q (err=%v)", payload.View.PrivateMetadata, err)
		c.JSON(http.StatusOK, gin.H{
			"response_action": "errors",
			"errors":          gin.H{"label_select": "internal error: modal context lost. Please reopen the settings."},
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
			"errors":          gin.H{"label_select": err.Error()},
		})
		return
	}

	if form.LabelName == "" {
		c.JSON(http.StatusOK, gin.H{
			"response_action": "errors",
			"errors":          gin.H{"label_select": "select a label or create a new one"},
		})
		return
	}

	now := time.Now()

	// Delete path: hard-delete the row when the user checked "delete this
	// config". We Unscoped() the delete because the (channel, label) unique
	// index doesn't include deleted_at — a soft-deleted row would block the
	// user from recreating a configuration with the same label name.
	if form.DeleteConfig && !form.CreateNew {
		var existing models.ChannelConfig
		if err := db.Where("slack_channel_id = ? AND label_name = ?", meta.ChannelID, form.LabelName).First(&existing).Error; err == nil {
			if err := db.Unscoped().Delete(&existing).Error; err != nil {
				log.Printf("settings modal delete failed: %v", err)
				c.JSON(http.StatusOK, gin.H{
					"response_action": "errors",
					"errors":          gin.H{"delete_config": "delete failed"},
				})
				return
			}
			if !services.IsTestMode && meta.UserID != "" {
				msg := i18n.TWithLang(form.Language, "modal.deleted", form.LabelName)
				if err := services.PostEphemeral(meta.ChannelID, meta.UserID, msg); err != nil {
					log.Printf("settings deleted confirmation post failed: %v", err)
				}
			}
		}
		c.Status(http.StatusOK)
		return
	}

	// Create-new path must not silently overwrite an existing label.
	if form.CreateNew {
		var dup models.ChannelConfig
		if err := db.Where("slack_channel_id = ? AND label_name = ?", meta.ChannelID, form.LabelName).First(&dup).Error; err == nil {
			c.JSON(http.StatusOK, gin.H{
				"response_action": "errors",
				"errors":          gin.H{"new_label_name": "this label already has a configuration; pick it from the dropdown instead"},
			})
			return
		}
	}

	var cfg models.ChannelConfig
	result := db.Where("slack_channel_id = ? AND label_name = ?", meta.ChannelID, form.LabelName).First(&cfg)
	if result.Error != nil {
		cfg = models.ChannelConfig{
			ID:             uuid.NewString(),
			SlackChannelID: meta.ChannelID,
			LabelName:      form.LabelName,
			CreatedAt:      now,
		}
	}
	cfg.DefaultMentionID = form.DefaultMentionID
	cfg.ReviewerList = form.ReviewerList
	cfg.RepositoryList = form.RepositoryList
	cfg.ReminderInterval = form.ReminderInterval
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
				"errors":          gin.H{"label_select": "save failed"},
			})
			return
		}
	} else {
		if err := db.Save(&cfg).Error; err != nil {
			log.Printf("failed to update config from modal: %v", err)
			c.JSON(http.StatusOK, gin.H{
				"response_action": "errors",
				"errors":          gin.H{"label_select": "save failed"},
			})
			return
		}
	}

	if !services.IsTestMode && meta.UserID != "" {
		msg := i18n.TWithLang(form.Language, "modal.saved", form.LabelName)
		if err := services.PostEphemeral(meta.ChannelID, meta.UserID, msg); err != nil {
			log.Printf("settings saved confirmation post failed: %v", err)
		}
	}

	c.Status(http.StatusOK)
}
