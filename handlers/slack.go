package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"slack-review-notify/i18n"
	"slack-review-notify/models"
	"slack-review-notify/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SlackActionPayload struct {
	Type string `json:"type"`
	User struct {
		ID string `json:"id"`
	} `json:"user"`
	Actions []struct {
		ActionID string `json:"action_id"`
		Value    string `json:"value,omitempty"`
		// Fields for selection menu
		SelectedOption struct {
			Value string `json:"value"`
			Text  struct {
				Text string `json:"text"`
			} `json:"text"`
		} `json:"selected_option,omitempty"`
	} `json:"actions"`
	Container struct {
		ChannelID string `json:"channel_id"`
	} `json:"container"`
	Message struct {
		Ts string `json:"ts"`
	} `json:"message"`
}

func HandleSlackAction(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			log.Printf("failed to read request body: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
			return
		}

		// Restore the body
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// Verify the signature
		if !services.ValidateSlackRequest(c.Request, bodyBytes) {
			log.Println("invalid slack signature")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid slack signature"})
			return
		}

		payloadStr := c.PostForm("payload")
		payloadStr = strings.TrimSpace(payloadStr)

		var payload SlackActionPayload
		if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
			log.Printf("payload json parse error: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}

		slackUserID := payload.User.ID
		ts := payload.Message.Ts
		channel := payload.Container.ChannelID

		log.Printf("slack action received: ts=%s, channel=%s, userID=%s", ts, channel, slackUserID)

		// Error if no actions are present
		if len(payload.Actions) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "no action provided"})
			return
		}

		// Get the action ID
		actionID := payload.Actions[0].ActionID

		// Handle "Pause Reminder" selection menu
		if actionID == "pause_reminder" || actionID == "pause_reminder_initial" {
			// Get value from selection menu
			var selectedValue string
			if payload.Actions[0].SelectedOption.Value != "" {
				selectedValue = payload.Actions[0].SelectedOption.Value
			} else {
				selectedValue = payload.Actions[0].Value
			}

			if selectedValue == "" {
				log.Printf("selected value is empty")
				c.JSON(http.StatusBadRequest, gin.H{"error": "selected value is empty"})
				return
			}

			// Extract task ID and duration from value (format: "taskID:duration")
			parts := strings.Split(selectedValue, ":")
			if len(parts) != 2 {
				log.Printf("invalid value format: %s", selectedValue)
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid value format"})
				return
			}

			taskID := parts[0]
			duration := parts[1]

			// Search for task directly in database using task ID
			var taskToUpdate models.ReviewTask
			if err := db.Where("id = ?", taskID).First(&taskToUpdate).Error; err != nil {
				log.Printf("task id %s not found: %v", taskID, err)
				c.JSON(http.StatusNotFound, gin.H{"error": "task not found by ID"})
				return
			}

			// Pause reminder based on the selected duration
			var pauseUntil time.Time

			switch duration {
			case "1h":
				pauseUntil = time.Now().Add(1 * time.Hour)
				taskToUpdate.ReminderPausedUntil = &pauseUntil
			case "2h":
				pauseUntil = time.Now().Add(2 * time.Hour)
				taskToUpdate.ReminderPausedUntil = &pauseUntil
			case "4h":
				pauseUntil = time.Now().Add(4 * time.Hour)
				taskToUpdate.ReminderPausedUntil = &pauseUntil
			case "today":
				// Pause until the next business day's opening time
				// Get channel config
				var config models.ChannelConfig
				if err := db.Where("slack_channel_id = ? AND label_name = ?", taskToUpdate.SlackChannel, taskToUpdate.LabelName).First(&config).Error; err != nil {
					// Use default (10:00) if config is not found
					pauseUntil = services.GetNextBusinessDayMorningWithConfig(time.Now(), nil)
				} else {
					// Use business hours start time from config
					pauseUntil = services.GetNextBusinessDayMorningWithConfig(time.Now(), &config)
				}
				taskToUpdate.ReminderPausedUntil = &pauseUntil
			case "stop":
				// Do not notify until a reviewer is assigned
				taskToUpdate.Status = "paused"
			default:
				pauseUntil = time.Now().Add(1 * time.Hour) // Default
				taskToUpdate.ReminderPausedUntil = &pauseUntil
			}

			db.Save(&taskToUpdate)

			// Notify about the pause
			err := services.SendReminderPausedMessage(taskToUpdate, duration)
			if err != nil {
				log.Printf("pause reminder send error: %v", err)
			}

			c.Status(http.StatusOK)
			return
		}

		// Handle each action
		switch actionID {
		case "review_done":
			// Search for task using ts and channel (retry after a short delay if in pending state)
			var task models.ReviewTask
			const maxRetries = 5
			const retryDelay = 200 * time.Millisecond

			var err error
			for retry := 0; retry < maxRetries; retry++ {
				err = db.Where("slack_ts = ? AND slack_channel = ?", ts, channel).First(&task).Error
				if err == nil {
					break
				}

				// If record not found, wait briefly and retry
				if retry < maxRetries-1 {
					log.Printf("task not found (attempt %d/%d): ts=%s, channel=%s, retrying in %v",
						retry+1, maxRetries, ts, channel, retryDelay)
					time.Sleep(retryDelay)
				}
			}

			if err != nil {
				log.Printf("task not found after %d retries: ts=%s, channel=%s", maxRetries, ts, channel)
				c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
				return
			}

			// Post review completion notification to thread
			t := i18n.L(task.Language)
			message := t("notify.review_done_button", slackUserID)
			if err := services.PostToThread(task.SlackChannel, task.SlackTS, message); err != nil {
				log.Printf("review done notification error: %v", err)
			}

			// Change status to done
			task.Status = "done"
			task.UpdatedAt = time.Now()

			if err := db.Save(&task).Error; err != nil {
				log.Printf("task save error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save task"})
				return
			}

			c.Status(http.StatusOK)
			return

		case "change_reviewer":
			// Parse value: "taskID" or "taskID:replacingReviewerID" format
			actionValue := payload.Actions[0].Value
			var taskID string
			var replacingReviewerID string
			if idx := strings.Index(actionValue, ":"); idx >= 0 {
				taskID = actionValue[:idx]
				replacingReviewerID = actionValue[idx+1:]
			} else {
				taskID = actionValue
			}

			// Search for task in database using task ID
			var taskToUpdate models.ReviewTask
			if err := db.Where("id = ?", taskID).First(&taskToUpdate).Error; err != nil {
				log.Printf("task id %s not found: %v", taskID, err)
				c.JSON(http.StatusNotFound, gin.H{"error": "task not found by ID"})
				return
			}

			// Use default value if LabelName is not set on existing task
			if taskToUpdate.LabelName == "" {
				taskToUpdate.LabelName = "needs-review"
				db.Save(&taskToUpdate)
			}

			// Build the set of reviewers to replace:
			// 1. The explicitly clicked reviewer (replacingReviewerID or taskToUpdate.Reviewer)
			// 2. Any other current reviewers who are now away
			awayIDs := services.GetAwayUserIDs(db)
			awaySet := make(map[string]bool)
			for _, id := range awayIDs {
				if id != "" {
					awaySet[id] = true
				}
			}

			// Determine which reviewer was explicitly requested to replace
			explicitReplace := replacingReviewerID
			if explicitReplace == "" {
				explicitReplace = taskToUpdate.Reviewer
			}

			// Collect all reviewer IDs that need replacement (explicit + away)
			replaceSet := make(map[string]bool)
			replaceSet[explicitReplace] = true

			var currentReviewerList []string
			if taskToUpdate.Reviewers != "" {
				for _, id := range strings.Split(taskToUpdate.Reviewers, ",") {
					if trimmed := strings.TrimSpace(id); trimmed != "" {
						currentReviewerList = append(currentReviewerList, trimmed)
						if awaySet[trimmed] {
							replaceSet[trimmed] = true
						}
					}
				}
			} else if taskToUpdate.Reviewer != "" {
				currentReviewerList = []string{taskToUpdate.Reviewer}
			}

			replacementCount := len(replaceSet)

			// Exclusions: PR author + all current reviewers (including those being replaced)
			excludeIDs := []string{}
			if taskToUpdate.PRAuthorSlackID != "" {
				excludeIDs = append(excludeIDs, taskToUpdate.PRAuthorSlackID)
			}
			for _, id := range currentReviewerList {
				excludeIDs = append(excludeIDs, id)
			}

			// Select enough new reviewers to replace all
			newReviewerIDs := services.SelectRandomReviewers(db, taskToUpdate.SlackChannel, taskToUpdate.LabelName, replacementCount, excludeIDs)

			// No real candidates if SelectRandomReviewers only returned DefaultMentionID
			noRealCandidate := false
			if len(newReviewerIDs) == 0 {
				noRealCandidate = true
			} else {
				var cfg models.ChannelConfig
				if err := db.Where("slack_channel_id = ? AND label_name = ?", taskToUpdate.SlackChannel, taskToUpdate.LabelName).First(&cfg).Error; err == nil {
					if cfg.ReviewerList != "" && len(newReviewerIDs) == 1 && newReviewerIDs[0] == cfg.DefaultMentionID {
						noRealCandidate = true
					}
				}
			}

			if noRealCandidate {
				t := i18n.L(taskToUpdate.Language)
				message := t("notify.cannot_change_reviewer")
				if err := services.PostToThread(taskToUpdate.SlackChannel, taskToUpdate.SlackTS, message); err != nil {
					log.Printf("notification error: %v", err)
				}
				c.Status(http.StatusOK)
				return
			}

			// Build a mapping of old -> new reviewer replacements
			// Assign explicit replacement first to ensure deterministic mapping
			replacementMap := make(map[string]string) // old ID -> new ID
			newIdx := 0
			if newIdx < len(newReviewerIDs) {
				replacementMap[explicitReplace] = newReviewerIDs[newIdx]
				newIdx++
			}
			// Sort remaining IDs for deterministic assignment
			var remainingOldIDs []string
			for oldID := range replaceSet {
				if oldID != explicitReplace {
					remainingOldIDs = append(remainingOldIDs, oldID)
				}
			}
			sort.Strings(remainingOldIDs)
			for _, oldID := range remainingOldIDs {
				if newIdx < len(newReviewerIDs) {
					replacementMap[oldID] = newReviewerIDs[newIdx]
					newIdx++
				}
			}

			// Save the old reviewer ID for notification
			oldReviewerID := taskToUpdate.Reviewer
			if replacingReviewerID != "" {
				oldReviewerID = replacingReviewerID
			}

			// Update the Reviewers field by replacing all matched reviewers
			if len(currentReviewerList) > 0 {
				var updatedReviewers []string
				for _, id := range currentReviewerList {
					if newID, ok := replacementMap[id]; ok {
						updatedReviewers = append(updatedReviewers, newID)
					} else {
						updatedReviewers = append(updatedReviewers, id)
					}
				}
				taskToUpdate.Reviewers = strings.Join(updatedReviewers, ",")
			}

			// Update the Reviewer field (backward compatibility): use the replacement for the explicit one
			if newID, ok := replacementMap[explicitReplace]; ok {
				taskToUpdate.Reviewer = newID
			} else if len(newReviewerIDs) > 0 {
				taskToUpdate.Reviewer = newReviewerIDs[0]
			}

			taskToUpdate.UpdatedAt = time.Now()
			db.Save(&taskToUpdate)

			// Notify that the reviewer has been changed (for the explicit replacement)
			err := services.SendReviewerChangedMessage(taskToUpdate, oldReviewerID)
			if err != nil {
				log.Printf("reviewer change notification error: %v", err)
			}

			// Notify for each additional away reviewer that was replaced
			for oldID, newID := range replacementMap {
				if oldID != explicitReplace {
					awayTask := taskToUpdate
					awayTask.Reviewer = newID
					if notifyErr := services.SendReviewerChangedMessage(awayTask, oldID); notifyErr != nil {
						log.Printf("away reviewer change notification error: %v", notifyErr)
					}
				}
			}

			c.Status(http.StatusOK)
			return
		}
	}
}
