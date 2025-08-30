package services

import (
	"log"
	"strings"
	"time"

	"gorm.io/gorm"

	"slack-review-notify/models"
)

// CheckBusinessHoursTasks は営業時間外待機タスクを営業時間内になったときに処理する
func CheckBusinessHoursTasks(db *gorm.DB) {
	// 営業時間外の場合は何もしない
	if IsOutsideBusinessHours(time.Now()) {
		return
	}

	var tasks []models.ReviewTask
	result := db.Where("status = ?", "waiting_business_hours").Find(&tasks)
	
	if result.Error != nil {
		log.Printf("waiting_business_hours task check error: %v", result.Error)
		return
	}

	for _, task := range tasks {
		// チャンネル設定を取得してメンションIDを取得
		var config models.ChannelConfig
		labelName := task.LabelName
		if labelName == "" {
			labelName = "needs-review"
		}
		
		if err := db.Where("slack_channel_id = ? AND label_name = ?", task.SlackChannel, labelName).First(&config).Error; err != nil {
			log.Printf("channel config not found for waiting task: %s, error: %v", task.ID, err)
			continue
		}

		// レビュワーをランダム選択
		reviewerID := SelectRandomReviewer(db, task.SlackChannel, labelName)
		
		// スレッドに営業時間通知を送信
		if err := PostBusinessHoursNotificationToThread(task, config.DefaultMentionID); err != nil {
			log.Printf("business hours notification error (task: %s): %v", task.ID, err)
			continue
		}

		// タスクの状態を更新
		task.Status = "in_review"
		task.Reviewer = reviewerID
		task.UpdatedAt = time.Now()
		
		if err := db.Save(&task).Error; err != nil {
			log.Printf("task status update error (task: %s): %v", task.ID, err)
			continue
		}

		log.Printf("waiting_business_hours task activated: %s", task.ID)

		// レビュワーが設定された場合は変更ボタン付きの通知も送信
		if reviewerID != "" {
			if err := PostReviewerAssignedMessageWithChangeButton(task); err != nil {
				log.Printf("reviewer assigned notification error: %v", err)
			}
		}
	}
}

// CheckInReviewTasks 関数も同様に修正
func CheckInReviewTasks(db *gorm.DB) {
	var tasks []models.ReviewTask

	// "in_review" 状態で、"archived" 状態ではないタスクを検索
	result := db.Where("status = ? AND reviewer != ?", "in_review", "").
		Where("status != ?", "archived").Find(&tasks)

	if result.Error != nil {
		log.Printf("review in review task check error: %v", result.Error)
		return
	}

	now := time.Now()

	for _, task := range tasks {
		// リマインダー一時停止中かチェック
		if task.ReminderPausedUntil != nil && now.Before(*task.ReminderPausedUntil) {
			continue // 一時停止中なのでスキップ
		}

		// 一時停止ステータスならスキップ
		if task.Status == "paused" {
			continue
		}

		// チャンネル設定からリマインド頻度を取得
		var config models.ChannelConfig
		var reminderInterval int = 30 // デフォルト値（30分）

		// LabelNameも考慮して設定を取得
		labelName := task.LabelName
		if labelName == "" {
			labelName = "needs-review" // デフォルトのラベル名
		}

		if err := db.Where("slack_channel_id = ? AND label_name = ?", task.SlackChannel, labelName).First(&config).Error; err == nil {
			if config.ReviewerReminderInterval > 0 {
				reminderInterval = config.ReviewerReminderInterval
			}
		}

		// 営業時間外かチェック
		if IsOutsideBusinessHours(now) {
			// 営業時間外で、まだ営業時間外リマインドを送っていない場合
			if !task.OutOfHoursReminded {
				// 設定された頻度でリマインダーを送信（初回のみ）
				reminderTime := now.Add(-time.Duration(reminderInterval) * time.Minute)
				if task.UpdatedAt.Before(reminderTime) {
					// 営業時間外用のリマインドメッセージを送信
					err := SendOutOfHoursReminderMessage(db, task)
					if err != nil {
						log.Printf("out of hours reminder send error (task id: %s): %v", task.ID, err)

						// チャンネル関連のエラーの場合はループ継続せずスキップ
						if strings.Contains(err.Error(), "channel is archived") ||
							strings.Contains(err.Error(), "not accessible") {
							continue
						}
					} else {
						// 翌営業日10時まで一時停止
						nextBusinessDay := GetNextBusinessDayMorning()
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
			// 既に営業時間外リマインドを送信済みの場合はスキップ（ReminderPausedUntilで制御される）
		} else {
			// 営業時間内の場合
			
			// 営業時間外リマインドフラグをリセット
			if task.OutOfHoursReminded {
				task.OutOfHoursReminded = false
				if err := db.Model(&task).Update("out_of_hours_reminded", false).Error; err != nil {
					log.Printf("task out_of_hours_reminded reset error: %v", err)
				}
			}

			// 通常のリマインド処理
			reminderTime := now.Add(-time.Duration(reminderInterval) * time.Minute)
			if task.UpdatedAt.Before(reminderTime) {
				err := SendReviewerReminderMessage(db, task)
				if err != nil {
					log.Printf("reviewer reminder send error (task id: %s): %v", task.ID, err)

					// チャンネル関連のエラーの場合はループ継続せずスキップ
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

// CleanupOldTasks は完了したタスクや不要になったタスクを削除する関数
func CleanupOldTasks(db *gorm.DB) {
	// 現在の時刻
	now := time.Now()

	// 1. 完了（done）状態のタスクで、1日以上経過しているものを削除
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

	// 2. 一時停止（paused）状態のタスクで、1週間以上経過しているものを削除
	oneWeekAgo := now.AddDate(0, 0, -7)
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

	// 3. アーカイブ（archived）状態のタスクを全て削除
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

	// 合計削除件数
	totalDeleted := doneTasksCount + pausedTasksCount + archivedTasksCount
	if totalDeleted > 0 {
		log.Printf("total task deleted: %d", totalDeleted)
	}
}
