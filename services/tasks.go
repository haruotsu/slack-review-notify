package services

import (
	"log"
	"strings"
	"time"

	"gorm.io/gorm"

	"slack-review-notify/i18n"
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

		// 既に事前アサイン済みのレビュワー (プールメンバーが翌朝前にコメントしたケース) を尊重する。
		existingReviewerIDs := []string{}
		for _, id := range strings.Split(task.Reviewers, ",") {
			if trimmed := strings.TrimSpace(id); trimmed != "" {
				existingReviewerIDs = append(existingReviewerIDs, trimmed)
			}
		}

		// Randomly select reviewers (excluding PR author and already pre-assigned reviewers)
		excludeIDs := []string{}
		if task.PRAuthorSlackID != "" {
			excludeIDs = append(excludeIDs, task.PRAuthorSlackID)
		}
		excludeIDs = append(excludeIDs, existingReviewerIDs...)

		requiredApprovals := config.RequiredApprovals
		if requiredApprovals <= 0 {
			requiredApprovals = 1
		}

		needed := requiredApprovals - len(existingReviewerIDs)
		var reviewerIDs []string
		if needed > 0 {
			selected := SelectRandomReviewers(db, task.SlackChannel, labelName, needed, excludeIDs)
			reviewerIDs = append(reviewerIDs, existingReviewerIDs...)
			reviewerIDs = append(reviewerIDs, selected...)
		} else {
			// 事前アサインだけで requiredApprovals を満たしているのでランダム選出はスキップ。
			reviewerIDs = existingReviewerIDs
		}

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

// CheckPendingReReviewNotifications sends deferred re-review notifications when business hours begin
func CheckPendingReReviewNotifications(db *gorm.DB) {
	now := time.Now()

	var tasks []models.ReviewTask
	// Only process active tasks (exclude done/archived/completed that no longer need notification)
	result := db.Where("pending_re_review_notify = ? AND status IN ?", true,
		[]string{"in_review", "pending", "snoozed", "waiting_business_hours"}).Find(&tasks)
	if result.Error != nil {
		log.Printf("pending re-review check error: %v", result.Error)
		return
	}

	for _, task := range tasks {
		var config models.ChannelConfig
		labelName := task.LabelName
		if labelName == "" {
			labelName = "needs-review"
		}

		if err := db.Where("slack_channel_id = ? AND label_name = ?", task.SlackChannel, labelName).First(&config).Error; err != nil {
			// Config deleted or not found: clear pending flag to avoid infinite retry
			log.Printf("channel config not found for pending re-review task: %s, clearing pending flag: %v", task.ID, err)
			if err := clearPendingReReviewFlags(db, task.ID, task.UpdatedAt, now); err != nil {
				log.Printf("failed to clear pending re-review flags for orphan task: %s: %v", task.ID, err)
			}
			continue
		}

		if !IsWithinBusinessHours(&config, now) {
			continue
		}

		// Send deferred re-review notifications (may contain multiple sender/reviewer pairs)
		senders := strings.Split(task.PendingReReviewSender, ",")
		reviewers := strings.Split(task.PendingReReviewReviewer, ",")
		t := i18n.L(task.Language)
		for idx := 0; idx < len(senders) && idx < len(reviewers); idx++ {
			message := t("notify.re_review_requested", senders[idx], reviewers[idx])
			if err := PostToThread(task.SlackChannel, task.SlackTS, message); err != nil {
				log.Printf("deferred re-review notification error (task: %s, idx: %d): %v", task.ID, idx, err)
				// Continue to try remaining notifications
			}
		}

		// Clear the pending flag
		if err := clearPendingReReviewFlags(db, task.ID, task.UpdatedAt, now); err != nil {
			log.Printf("failed to clear pending re-review flags (task: %s): %v", task.ID, err)
		} else {
			log.Printf("deferred re-review notification sent: task=%s", task.ID)
		}
	}
}

// clearPendingReReviewFlags resets the pending re-review fields on a task using CAS pattern
func clearPendingReReviewFlags(db *gorm.DB, taskID string, expectedUpdatedAt time.Time, now time.Time) error {
	result := db.Model(&models.ReviewTask{}).
		Where("id = ? AND updated_at = ?", taskID, expectedUpdatedAt).
		Updates(map[string]interface{}{
			"pending_re_review_notify":   false,
			"pending_re_review_sender":   "",
			"pending_re_review_reviewer": "",
			"updated_at":                 now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		log.Printf("clearPendingReReviewFlags CAS miss (concurrent update): task=%s", taskID)
	}
	return nil
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
