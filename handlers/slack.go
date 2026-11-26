package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"slack-review-notify/i18n"
	"slack-review-notify/models"
	"slack-review-notify/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// actionOpenSettings is the action_id route prefix for the "Open Settings"
// button family rendered by the help command. Each button instance gets a
// unique suffix (":<index>" or ":new") so Slack's per-block action_id
// uniqueness rule isn't violated.
const actionOpenSettings = "open_settings"

type SlackActionPayload struct {
	Type      string `json:"type"`
	TriggerID string `json:"trigger_id,omitempty"`
	User      struct {
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
	View *SlackViewPayload `json:"view,omitempty"`
}

// SlackViewPayload is the subset of the view object Slack sends with view_submission.
type SlackViewPayload struct {
	ID              string `json:"id"`
	CallbackID      string `json:"callback_id"`
	PrivateMetadata string `json:"private_metadata"`
	State           struct {
		Values map[string]map[string]services.ViewStateValue `json:"values"`
	} `json:"state"`
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

		// Handle view_submission (settings modal save) before the block_actions logic.
		if payload.Type == "view_submission" && payload.View != nil && payload.View.CallbackID == services.SettingsModalCallbackID {
			handleSettingsModalSubmission(c, db, payload)
			return
		}
		if payload.Type == "view_submission" && payload.View != nil && payload.View.CallbackID == services.AwayManagementModalCallbackID {
			handleAwayModalSubmission(c, db, payload)
			return
		}
		if payload.Type == "view_submission" && payload.View != nil && payload.View.CallbackID == services.UserMappingModalCallbackID {
			handleUserMappingModalSubmission(c, db, payload)
			return
		}
		if payload.Type == "view_submission" && payload.View != nil && payload.View.CallbackID == services.ChangeReviewerModalCallbackID {
			handleChangeReviewerModalSubmission(c, db, payload)
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

		// Handle "Open Settings" button: opens the settings modal via views.open.
		// The help command renders one button per label, each with a unique
		// action_id like "open_settings:<index>" (Slack requires action_id
		// uniqueness within an actions block). Route on prefix; the target
		// label is carried in the button's value field, not the action_id.
		if actionID == actionOpenSettings || strings.HasPrefix(actionID, actionOpenSettings+":") {
			handleOpenSettings(c, db, payload)
			return
		}

		if actionID == services.OpenMenuActionID {
			handleOpenMenu(c, db, payload)
			return
		}

		if strings.HasPrefix(actionID, services.MenuActionPrefix) {
			handleMenuAction(c, db, payload)
			return
		}

		if actionID == services.MenuUnsetAwayActionID {
			handleMenuUnsetAway(c, db, payload)
			return
		}

		// "🌴 Manage availability" button → open the away-management modal.
		if actionID == services.OpenAwayManagementActionID {
			handleOpenAwayManagement(c, db, payload)
			return
		}

		// "👥 User mapping" button → open the user-mapping modal.
		if actionID == services.OpenUserMappingActionID {
			handleOpenUserMapping(c, db, payload)
			return
		}

		// Label dropdown changed inside the settings modal → re-render via views.update
		// so prefilled values reflect the newly chosen label (or the create-new mode).
		if actionID == services.LabelSelectActionID && payload.View != nil {
			handleLabelSelectChanged(c, db, payload)
			return
		}

		// All-day box toggled inside the away modal → re-render via views.update
		// with the time pickers removed (ticked) or added (unticked).
		if actionID == services.AwayAllDayActionID && payload.View != nil {
			handleAwayAllDayToggled(c, db, payload)
			return
		}

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
		case services.UrgentReviewActionID:
			task, err := findTaskByMessage(db, ts, channel)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
				return
			}

			t := i18n.L(task.Language)
			err = services.ActivateWaitingTaskUrgently(db, task, slackUserID)
			switch {
			case errors.Is(err, services.ErrTaskNotWaiting):
				if perr := services.PostEphemeral(task.SlackChannel, slackUserID, t("notify.urgent_review_already_sent")); perr != nil {
					log.Printf("urgent review ephemeral error: %v", perr)
				}
			case err != nil:
				log.Printf("urgent review activation failed (task: %s): %v", task.ID, err)
				if perr := services.PostEphemeral(task.SlackChannel, slackUserID, t("notify.urgent_review_failed")); perr != nil {
					log.Printf("urgent review ephemeral error: %v", perr)
				}
			default:
				log.Printf("urgent review requested: task=%s, user=%s", task.ID, slackUserID)
			}

			c.Status(http.StatusOK)
			return

		case "review_done":
			task, err := findTaskByMessage(db, ts, channel)
			if err != nil {
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

			// Every reminder leaves a button naming the absent reviewer, so a
			// stale one can arrive after that reviewer was already swapped out or
			// finished; acting on it would announce a swap that changes nothing.
			if replacingReviewerID != "" && !slices.Contains(services.GetPendingReviewers(taskToUpdate), replacingReviewerID) {
				t := i18n.L(taskToUpdate.Language)
				if err := services.PostEphemeral(taskToUpdate.SlackChannel, payload.User.ID, t("notify.reviewer_already_replaced")); err != nil {
					log.Printf("already-replaced notice error: %v", err)
				}
				c.Status(http.StatusOK)
				return
			}

			// Use default value if LabelName is not set on existing task
			if taskToUpdate.LabelName == "" {
				taskToUpdate.LabelName = "needs-review"
				db.Save(&taskToUpdate)
			}

			if replacingReviewerID == "" && services.UsesChangeReviewerModal(taskToUpdate) {
				handleOpenChangeReviewerModal(c, payload, taskToUpdate)
				return
			}

			newReviewerID, ok := services.DrawReplacementReviewer(db, taskToUpdate, false)
			if !ok {
				t := i18n.L(taskToUpdate.Language)
				message := t("notify.cannot_change_reviewer")
				if err := services.PostToThread(taskToUpdate.SlackChannel, taskToUpdate.SlackTS, message); err != nil {
					log.Printf("notification error: %v", err)
				}
				c.Status(http.StatusOK)
				return
			}

			oldReviewerID := taskToUpdate.Reviewer
			if replacingReviewerID != "" {
				oldReviewerID = replacingReviewerID
			}

			services.ReplaceTaskReviewer(&taskToUpdate, oldReviewerID, newReviewerID)
			taskToUpdate.UpdatedAt = time.Now()
			db.Save(&taskToUpdate)

			err := services.SendReviewerChangedMessage(taskToUpdate, oldReviewerID, newReviewerID)
			if err != nil {
				log.Printf("reviewer change notification error: %v", err)
			}

			c.Status(http.StatusOK)
			return
		}
	}
}

// findTaskByMessage finds the task posted as the Slack message ts in channel. The
// message is posted before its ts is saved to the task, so a button pressed right
// after posting may arrive first; the lookup is retried for a short while.
func findTaskByMessage(db *gorm.DB, ts, channel string) (models.ReviewTask, error) {
	const maxRetries = 5
	const retryDelay = 200 * time.Millisecond

	var task models.ReviewTask
	var err error
	for retry := 0; retry < maxRetries; retry++ {
		err = db.Where("slack_ts = ? AND slack_channel = ?", ts, channel).First(&task).Error
		if err == nil {
			return task, nil
		}
		if retry < maxRetries-1 {
			log.Printf("task not found (attempt %d/%d): ts=%s, channel=%s, retrying in %v",
				retry+1, maxRetries, ts, channel, retryDelay)
			time.Sleep(retryDelay)
		}
	}
	log.Printf("task not found after %d retries: ts=%s, channel=%s", maxRetries, ts, channel)
	return task, err
}
