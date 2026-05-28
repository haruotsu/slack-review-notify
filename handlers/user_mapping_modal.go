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

// handleOpenUserMapping opens the user-mapping modal. Triggered by the
// "👥 User mapping" help button. Existing mappings are loaded so the modal
// can render them — important so the operator can spot legacy non-U-id rows
// that still need re-registration.
func handleOpenUserMapping(c *gin.Context, db *gorm.DB, payload SlackActionPayload) {
	channelID := payload.Container.ChannelID
	userID := payload.User.ID

	configs := loadChannelConfigs(db, channelID)
	lang := pickModalLanguage(configs, "")

	var mappings []models.UserMapping
	if err := db.Order("github_username").Find(&mappings).Error; err != nil {
		log.Printf("user-mapping modal: failed to list mappings: %v", err)
	}

	view := services.BuildUserMappingModalView(services.UserMappingModalInputs{
		ChannelID: channelID,
		UserID:    userID,
		Lang:      lang,
		Mappings:  mappings,
	})
	triggerID := payload.TriggerID

	go func() {
		if err := services.OpenView(triggerID, view); err != nil {
			log.Printf("user-mapping views.open failed: %v", err)
		}
	}()

	c.Status(http.StatusOK)
}

// handleUserMappingModalSubmission upserts or deletes a single row.
// Deletes are Unscoped so a subsequent re-add of the same github_username
// can't collide on the unique index.
func handleUserMappingModalSubmission(c *gin.Context, db *gorm.DB, payload SlackActionPayload) {
	meta, err := services.DecodeUserMappingModalMetadata(payload.View.PrivateMetadata)
	if err != nil {
		log.Printf("user-mapping view_submission has invalid private_metadata: %q (err=%v)", payload.View.PrivateMetadata, err)
		c.JSON(http.StatusOK, gin.H{
			"response_action": "errors",
			"errors":          gin.H{services.UserMappingGithubBlockID: "internal error: modal context lost. Please reopen."},
		})
		return
	}

	configs := loadChannelConfigs(db, meta.ChannelID)
	lang := pickModalLanguage(configs, "")

	form, err := services.ParseUserMappingModalSubmission(payload.View.State.Values, lang)
	if err != nil {
		var ve *services.ModalValidationError
		if errors.As(err, &ve) {
			c.JSON(http.StatusOK, gin.H{
				"response_action": "errors",
				"errors":          ve.Errors,
			})
			return
		}
		log.Printf("user-mapping parse error: %v", err)
		c.JSON(http.StatusOK, gin.H{
			"response_action": "errors",
			"errors":          gin.H{services.UserMappingGithubBlockID: err.Error()},
		})
		return
	}

	now := time.Now()

	if form.Delete {
		var existing models.UserMapping
		res := db.Where("github_username = ?", form.GithubUsername).First(&existing)
		if errors.Is(res.Error, gorm.ErrRecordNotFound) {
			if !services.IsTestMode && meta.UserID != "" {
				msg := i18n.TWithLang(lang, "modal.user_mapping.delete_not_found", form.GithubUsername)
				if err := services.PostEphemeral(meta.ChannelID, meta.UserID, msg); err != nil {
					log.Printf("user-mapping delete-notfound notice failed: %v", err)
				}
			}
			c.Status(http.StatusOK)
			return
		}
		if res.Error != nil {
			log.Printf("user-mapping lookup failed: github=%s err=%v", form.GithubUsername, res.Error)
			c.JSON(http.StatusOK, gin.H{
				"response_action": "errors",
				"errors":          gin.H{services.UserMappingGithubBlockID: "delete failed"},
			})
			return
		}
		// Unscoped so the unique index on github_username doesn't reject a future
		// re-mapping of the same handle.
		if err := db.Unscoped().Delete(&existing).Error; err != nil {
			log.Printf("user-mapping delete failed: github=%s err=%v", form.GithubUsername, err)
			c.JSON(http.StatusOK, gin.H{
				"response_action": "errors",
				"errors":          gin.H{services.UserMappingDeleteBlockID: "delete failed"},
			})
			return
		}
		if !services.IsTestMode && meta.UserID != "" {
			msg := i18n.TWithLang(lang, "modal.user_mapping.deleted", form.GithubUsername)
			if err := services.PostEphemeral(meta.ChannelID, meta.UserID, msg); err != nil {
				log.Printf("user-mapping deleted notice failed: %v", err)
			}
		}
		c.Status(http.StatusOK)
		return
	}

	// Upsert
	var existing models.UserMapping
	res := db.Where("github_username = ?", form.GithubUsername).First(&existing)
	switch {
	case res.Error == nil:
		existing.SlackUserID = form.SlackUserID
		existing.UpdatedAt = now
		if err := db.Save(&existing).Error; err != nil {
			log.Printf("user-mapping update failed: github=%s err=%v", form.GithubUsername, err)
			c.JSON(http.StatusOK, gin.H{
				"response_action": "errors",
				"errors":          gin.H{services.UserMappingGithubBlockID: "save failed"},
			})
			return
		}
	case errors.Is(res.Error, gorm.ErrRecordNotFound):
		record := models.UserMapping{
			ID:             uuid.NewString(),
			GithubUsername: form.GithubUsername,
			SlackUserID:    form.SlackUserID,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := db.Create(&record).Error; err != nil {
			log.Printf("user-mapping create failed: github=%s err=%v", form.GithubUsername, err)
			c.JSON(http.StatusOK, gin.H{
				"response_action": "errors",
				"errors":          gin.H{services.UserMappingGithubBlockID: "save failed"},
			})
			return
		}
	default:
		log.Printf("user-mapping lookup failed: github=%s err=%v", form.GithubUsername, res.Error)
		c.JSON(http.StatusOK, gin.H{
			"response_action": "errors",
			"errors":          gin.H{services.UserMappingGithubBlockID: "save failed"},
		})
		return
	}

	if !services.IsTestMode && meta.UserID != "" {
		msg := i18n.TWithLang(lang, "modal.user_mapping.saved", form.GithubUsername)
		if err := services.PostEphemeral(meta.ChannelID, meta.UserID, msg); err != nil {
			log.Printf("user-mapping saved notice failed: %v", err)
		}
	}

	c.Status(http.StatusOK)
}
