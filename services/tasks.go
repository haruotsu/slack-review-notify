package services

import (
	"log"
	"strings"
	"time"

	"gorm.io/gorm"

	"slack-review-notify/models"
)

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

		if err := db.Where("slack_channel_id = ?", task.SlackChannel).First(&config).Error; err == nil {
			if config.ReviewerReminderInterval > 0 {
				reminderInterval = config.ReviewerReminderInterval
			}
		}

		// 設定された頻度でリマインダーを送信
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
