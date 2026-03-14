package services

import (
	"log"
	"strings"
	"time"

	"gorm.io/gorm"

	"slack-review-notify/models"
)

// CheckBusinessHoursTasks processes tasks waiting for business hours when business hours begin
func CheckBusinessHoursTasks(db *gorm.DB) {
	// Get current time
	now := time.Now()

	var tasks []models.ReviewTask
	result := db.Where("status = ?", "waiting_business_hours").Find(&tasks)

	if result.Error != nil {
		log.Printf("waiting_business_hours task check error: %v", result.Error)
		return
	}

	for _, task := range tasks {
		// Get channel config to retrieve mention ID
		var config models.ChannelConfig
		labelName := task.LabelName
		if labelName == "" {
			labelName = "needs-review"
		}

		if err := db.Where("slack_channel_id = ? AND label_name = ?", task.SlackChannel, labelName).First(&config).Error; err != nil {
			log.Printf("channel config not found for waiting task: %s, error: %v", task.ID, err)
			continue
		}

		// Check business hours settings for this channel
		if !IsWithinBusinessHours(&config, now) {
			continue // Outside business hours, skip processing
		}

		// Randomly select reviewers (excluding PR author)
		excludeIDs := []string{}
		if task.PRAuthorSlackID != "" {
			excludeIDs = append(excludeIDs, task.PRAuthorSlackID)
		}
		requiredApprovals := config.RequiredApprovals
		if requiredApprovals <= 0 {
			requiredApprovals = 1
		}
		reviewerIDs := SelectRandomReviewers(db, task.SlackChannel, labelName, requiredApprovals, excludeIDs)
		reviewerID := ""
		if len(reviewerIDs) > 0 {
			reviewerID = reviewerIDs[0]
		}
		reviewersStr := strings.Join(reviewerIDs, ",")

		// Send business hours notification to thread
		if err := PostBusinessHoursNotificationToThread(task, config.DefaultMentionID); err != nil {
			log.Printf("business hours notification error (task: %s): %v", task.ID, err)
			continue
		}

		// Update task status
		task.Status = "in_review"
		task.Reviewer = reviewerID
		task.Reviewers = reviewersStr
		task.UpdatedAt = time.Now()

		if err := db.Save(&task).Error; err != nil {
			log.Printf("task status update error (task: %s): %v", task.ID, err)
			continue
		}

		log.Printf("waiting_business_hours task activated: %s", task.ID)

		// If a reviewer was assigned, also send notification with change button
		if reviewerID != "" {
			if err := PostReviewerAssignedMessageWithChangeButton(task); err != nil {
				log.Printf("reviewer assigned notification error: %v", err)
			}
		}
	}
}

// CheckInReviewTasks checks tasks that are in review and sends reminders as needed
func CheckInReviewTasks(db *gorm.DB) {
	var tasks []models.ReviewTask

	// Search for tasks in "in_review" status that are not in "archived" status
	result := db.Where("status = ? AND reviewer != ?", "in_review", "").
		Where("status != ?", "archived").Find(&tasks)

	if result.Error != nil {
		log.Printf("review in review task check error: %v", result.Error)
		return
	}

	now := time.Now()

	for _, task := range tasks {
		// Check if reminder is temporarily paused
		if task.ReminderPausedUntil != nil && now.Before(*task.ReminderPausedUntil) {
			continue // Paused, so skip
		}

		// Skip if status is paused
		if task.Status == "paused" {
			continue
		}

		// Get reminder frequency from channel config
		var config models.ChannelConfig
		reminderInterval := 30 // Default value (30 minutes)

		// Get config considering LabelName
		labelName := task.LabelName
		if labelName == "" {
			labelName = "needs-review" // Default label name
		}

		if err := db.Where("slack_channel_id = ? AND label_name = ?", task.SlackChannel, labelName).First(&config).Error; err == nil {
			if config.ReviewerReminderInterval > 0 {
				reminderInterval = config.ReviewerReminderInterval
			}
		}

		// Check if outside business hours
		isOutsideBusinessHours := !IsWithinBusinessHours(&config, now)
		if isOutsideBusinessHours {
			// Outside business hours and haven't sent off-hours reminder yet
			if !task.OutOfHoursReminded {
				// Send reminder at configured frequency (first time only)
				reminderTime := now.Add(-time.Duration(reminderInterval) * time.Minute)
				if task.UpdatedAt.Before(reminderTime) {
					// Send off-hours reminder message
					err := SendOutOfHoursReminderMessage(db, task)
					if err != nil {
						log.Printf("out of hours reminder send error (task id: %s): %v", task.ID, err)

						// Skip without continuing the loop for channel-related errors
						if strings.Contains(err.Error(), "channel is archived") ||
							strings.Contains(err.Error(), "not accessible") {
							continue
						}
					} else {
						// Pause until the next business day's opening time
						nextBusinessDay := GetNextBusinessDayMorningWithConfig(now, &config)
						task.ReminderPausedUntil = &nextBusinessDay
						task.OutOfHoursReminded = true
						task.UpdatedAt = now

						if err := db.Save(&task).Error; err != nil {
							log.Printf("task update error: %v", err)
						}

						log.Printf("out of hours reminder sent and paused until next business day (task id: %s)", task.ID)
					}
				}
			}
			// Skip if off-hours reminder has already been sent (controlled by ReminderPausedUntil)
		} else {
			// During business hours

			// Reset off-hours reminder flag
			if task.OutOfHoursReminded {
				task.OutOfHoursReminded = false
				if err := db.Model(&task).Update("out_of_hours_reminded", false).Error; err != nil {
					log.Printf("task out_of_hours_reminded reset error: %v", err)
				}
			}

			// Normal reminder processing
			reminderTime := now.Add(-time.Duration(reminderInterval) * time.Minute)
			if task.UpdatedAt.Before(reminderTime) {
				err := SendReviewerReminderMessage(db, task)
				if err != nil {
					log.Printf("reviewer reminder send error (task id: %s): %v", task.ID, err)

					// Skip without continuing the loop for channel-related errors
					if strings.Contains(err.Error(), "channel is archived") ||
						strings.Contains(err.Error(), "not accessible") {
						continue
					}
				} else {
					task.UpdatedAt = now
					if err := db.Model(&task).Update("updated_at", now).Error; err != nil {
						log.Printf("task update error: %v", err)
					}

					log.Printf("reviewer reminder sent (task id: %s)", task.ID)
				}
			}
		}
	}
}

// CleanupOldTasks deletes completed tasks and tasks that are no longer needed
func CleanupOldTasks(db *gorm.DB) {
	// Current time
	now := time.Now()

	// 1. Delete tasks in "done" status that are more than 1 day old
	oneDayAgo := now.AddDate(0, 0, -1)
	var doneTasksCount int64
	resultDone := db.Where("status = ? AND updated_at < ?", "done", oneDayAgo).
		Delete(&models.ReviewTask{})

	if resultDone.Error != nil {
		log.Printf("done task delete error: %v", resultDone.Error)
	} else {
		doneTasksCount = resultDone.RowsAffected
		if doneTasksCount > 0 {
			log.Printf("✅ done task deleted: %d", doneTasksCount)
		}
	}

	// 2. Delete tasks in "completed" status that are more than 7 days old
	oneWeekAgo := now.AddDate(0, 0, -7)
	var completedTasksCount int64
	resultCompleted := db.Where("status = ? AND updated_at < ?", "completed", oneWeekAgo).
		Delete(&models.ReviewTask{})

	if resultCompleted.Error != nil {
		log.Printf("completed task delete error: %v", resultCompleted.Error)
	} else {
		completedTasksCount = resultCompleted.RowsAffected
		if completedTasksCount > 0 {
			log.Printf("✅ completed task deleted: %d", completedTasksCount)
		}
	}

	// 3. Delete tasks in "paused" status that are more than 1 week old
	var pausedTasksCount int64
	resultPaused := db.Where("status = ? AND updated_at < ?", "paused", oneWeekAgo).
		Delete(&models.ReviewTask{})

	if resultPaused.Error != nil {
		log.Printf("paused task delete error: %v", resultPaused.Error)
	} else {
		pausedTasksCount = resultPaused.RowsAffected
		if pausedTasksCount > 0 {
			log.Printf("paused task deleted: %d", pausedTasksCount)
		}
	}

	// 4. Delete all tasks in "archived" status
	var archivedTasksCount int64
	resultArchived := db.Where("status = ?", "archived").
		Delete(&models.ReviewTask{})

	if resultArchived.Error != nil {
		log.Printf("archived task delete error: %v", resultArchived.Error)
	} else {
		archivedTasksCount = resultArchived.RowsAffected
		if archivedTasksCount > 0 {
			log.Printf("archived task deleted: %d", archivedTasksCount)
		}
	}

	// Total deleted count
	totalDeleted := doneTasksCount + completedTasksCount + pausedTasksCount + archivedTasksCount
	if totalDeleted > 0 {
		log.Printf("total task deleted: %d", totalDeleted)
	}
}

// CleanupExpiredAvailability permanently deletes expired leave records
func CleanupExpiredAvailability(db *gorm.DB) {
	now := time.Now()
	result := db.Unscoped().Where("away_until IS NOT NULL AND away_until < ?", now).Delete(&models.ReviewerAvailability{})
	if result.Error != nil {
		log.Printf("expired availability cleanup error: %v", result.Error)
	} else if result.RowsAffected > 0 {
		log.Printf("expired availability records deleted: %d", result.RowsAffected)
	}
}
