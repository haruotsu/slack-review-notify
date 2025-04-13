package services

import (
	"log"
	"strings"
	"time"

	"gorm.io/gorm"

	"slack-review-notify/models"
)

// CheckPendingTasks 関数を修正
func CheckPendingTasks(db *gorm.DB) {
	var tasks []models.ReviewTask
	
	// "pending" 状態で、"archived" 状態ではないタスクを検索
	result := db.Where("status = ? AND reviewer = ?", "pending", "").
		Where("status != ?", "archived").Find(&tasks)
	
	if result.Error != nil {
		log.Printf("レビュー待ちタスクの確認中にエラーが発生しました: %v", result.Error)
		return
	}
 
	now := time.Now()
	tenSecondsAgo := now.Add(-10 * time.Second)
	
	for _, task := range tasks {
		// リマインダー一時停止中かチェック
		if task.ReminderPausedUntil != nil && now.Before(*task.ReminderPausedUntil) {
			continue // 一時停止中なのでスキップ
		}
		
		// 一時停止ステータスならスキップ
		if task.Status == "paused" {
			continue
		}
		
		// 10秒ごとにリマインダーを送信（最終更新から10秒経過しているか確認）
		if task.UpdatedAt.Before(tenSecondsAgo) {
			err := SendReminderMessage(db, task)
			if err != nil {
				log.Printf("リマインダー送信失敗 (Task ID: %s): %v", task.ID, err)
				
				// チャンネル関連のエラーの場合はループ継続せずスキップ
				if strings.Contains(err.Error(), "channel is archived") || 
				   strings.Contains(err.Error(), "not accessible") {
					continue
				}
			} else {
				// 更新時間を記録
				task.UpdatedAt = now
				db.Save(&task)
				
				log.Printf("✅ レビュー待ちリマインダーを送信しました: %s (%s)", task.Title, task.ID)
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
		log.Printf("レビュー中タスクの確認中にエラーが発生しました: %v", result.Error)
		return
	}
	
	now := time.Now()
	tenSecondsAgo := now.Add(-10 * time.Second)
	
	for _, task := range tasks {
		// リマインダー一時停止中かチェック
		if task.ReminderPausedUntil != nil && now.Before(*task.ReminderPausedUntil) {
			continue // 一時停止中なのでスキップ
		}
		
		// 一時停止ステータスならスキップ
		if task.Status == "paused" {
			continue
		}
		
		// 10秒ごとにリマインダーを送信（最終更新から10秒経過しているか確認）
		if task.UpdatedAt.Before(tenSecondsAgo) {
			err := SendReviewerReminderMessage(db, task)
			if err != nil {
				log.Printf("レビュアーリマインダー送信失敗 (Task ID: %s): %v", task.ID, err)
				
				// チャンネル関連のエラーの場合はループ継続せずスキップ
				if strings.Contains(err.Error(), "channel is archived") || 
				   strings.Contains(err.Error(), "not accessible") {
					continue
				}
			} else {
				// 更新時間を記録
				task.UpdatedAt = now
				db.Save(&task)
				
				log.Printf("✅ レビュアーリマインダーを送信しました: %s (%s)", task.Title, task.ID)
			}
		}
	}
}

// CleanupOldTasks は完了したタスクや不要になったタスクを削除する関数
func CleanupOldTasks(db *gorm.DB) {
	// 現在の時刻
	now := time.Now()
	
	// 1. 完了（done）状態のタスクで、3日以上経過しているものを削除
	threeDoysAgo := now.AddDate(0, 0, -3)
	var doneTasksCount int64
	resultDone := db.Where("status = ? AND updated_at < ?", "done", threeDoysAgo).
		Delete(&models.ReviewTask{})
	
	if resultDone.Error != nil {
		log.Printf("完了タスクの削除中にエラーが発生しました: %v", resultDone.Error)
	} else {
		doneTasksCount = resultDone.RowsAffected
		if doneTasksCount > 0 {
			log.Printf("✅ 完了状態の古いタスクを %d 件削除しました", doneTasksCount)
		}
	}
	
	// 2. 一時停止（paused）状態のタスクで、1週間以上経過しているものを削除
	oneWeekAgo := now.AddDate(0, 0, -7)
	var pausedTasksCount int64
	resultPaused := db.Where("status = ? AND updated_at < ?", "paused", oneWeekAgo).
		Delete(&models.ReviewTask{})
	
	if resultPaused.Error != nil {
		log.Printf("一時停止タスクの削除中にエラーが発生しました: %v", resultPaused.Error)
	} else {
		pausedTasksCount = resultPaused.RowsAffected
		if pausedTasksCount > 0 {
			log.Printf("✅ 一時停止状態の古いタスクを %d 件削除しました", pausedTasksCount)
		}
	}
	
	// 3. アーカイブ（archived）状態のタスクを全て削除
	var archivedTasksCount int64
	resultArchived := db.Where("status = ?", "archived").
		Delete(&models.ReviewTask{})
	
	if resultArchived.Error != nil {
		log.Printf("アーカイブタスクの削除中にエラーが発生しました: %v", resultArchived.Error)
	} else {
		archivedTasksCount = resultArchived.RowsAffected
		if archivedTasksCount > 0 {
			log.Printf("✅ アーカイブ状態のタスクを %d 件削除しました", archivedTasksCount)
		}
	}
	
	// 合計削除件数
	totalDeleted := doneTasksCount + pausedTasksCount + archivedTasksCount
	if totalDeleted > 0 {
		log.Printf("🧹 合計 %d 件の古いタスクを削除しました", totalDeleted)
	}
} 
