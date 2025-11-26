package main

import (
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"slack-review-notify/handlers"
	"slack-review-notify/models"
	"slack-review-notify/services"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("fail to load .env file")
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "review_tasks.db"
	}
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})

	if err != nil {
		log.Fatal("fail to connect db:", err)
	}

	if err := db.AutoMigrate(&models.ReviewTask{}, &models.ChannelConfig{}, &models.UserMapping{}); err != nil {
		log.Fatal("fail to migrate db:", err)
	}

	// バックグラウンドでウォッチングタスクをチェックする定期実行タスク
	go runTaskChecker(db)

	// バックグラウンドでチャンネル状態をチェック
	go runChannelChecker(db)

	r := gin.Default()

	// Slackボタン押下イベント
	r.POST("/slack/actions", handlers.HandleSlackAction(db))

	// GitHub Webhook受信
	r.POST("/webhook", handlers.HandleGitHubWebhook(db))

	// Slackコマンド受信
	r.POST("/slack/command", handlers.HandleSlackCommand(db))

	// Slackイベント受信エンドポイント
	r.POST("/slack/events", handlers.HandleSlackEvents(db))

	if err := r.Run(":8080"); err != nil {
		log.Fatal("failed to start server:", err)
	}
}

// 定期的にタスクをチェックするバックグラウンド処理
func runTaskChecker(db *gorm.DB) {
	taskTicker := time.NewTicker(60 * time.Second) // 1分ごとにチェック
	cleanupTicker := time.NewTicker(1 * time.Hour) // 1時間ごとにクリーンアップ
	defer taskTicker.Stop()
	defer cleanupTicker.Stop()

	for {
		select {
		case <-taskTicker.C:
			log.Println("start task check")

			// 営業時間外待機タスクのチェック
			services.CheckBusinessHoursTasks(db)

			// レビュー中タスク（レビュアー割り当て済み）のチェック
			services.CheckInReviewTasks(db)

		case <-cleanupTicker.C:
			log.Println("start old task cleanup")

			// 古いタスクの削除処理
			services.CleanupOldTasks(db)
		}
	}
}

// 定期的にチャンネル状態を確認するバックグラウンド処理
func runChannelChecker(db *gorm.DB) {
	ticker := time.NewTicker(1 * time.Hour) // 1時間ごとにチェック
	defer ticker.Stop()

	for range ticker.C {
		log.Println("start channel status check")
		services.CleanupArchivedChannels(db) // アーカイブされたチャンネルの設定を非アクティブに更新
	}
}
