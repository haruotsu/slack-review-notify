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
