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

	if err := db.AutoMigrate(&models.ReviewTask{}, &models.ChannelConfig{}, &models.UserMapping{}, &models.ReviewerAvailability{}); err != nil {
		log.Fatal("fail to migrate db:", err)
	}

	// Background periodic task to check watching tasks
	go runTaskChecker(db)

	// Background check for channel status
	go runChannelChecker(db)

	r := gin.Default()

	// Slack button click events
	r.POST("/slack/actions", handlers.HandleSlackAction(db))

	// Receive GitHub Webhooks
	r.POST("/webhook", handlers.HandleGitHubWebhook(db))

	// Receive Slack commands
	r.POST("/slack/command", handlers.HandleSlackCommand(db))

	// Slack event receiving endpoint
	r.POST("/slack/events", handlers.HandleSlackEvents(db))

	if err := r.Run(":8080"); err != nil {
		log.Fatal("failed to start server:", err)
	}
}

// Background process that periodically checks tasks
func runTaskChecker(db *gorm.DB) {
	taskTicker := time.NewTicker(60 * time.Second) // Check every 1 minute
	cleanupTicker := time.NewTicker(1 * time.Hour) // Cleanup every 1 hour
	defer taskTicker.Stop()
	defer cleanupTicker.Stop()

	for {
		select {
		case <-taskTicker.C:
			log.Println("start task check")

			// Check tasks waiting for business hours
			services.CheckBusinessHoursTasks(db)

			// Check in-review tasks (reviewer already assigned)
			services.CheckInReviewTasks(db)

		case <-cleanupTicker.C:
			log.Println("start old task cleanup")

			// Delete old tasks
			services.CleanupOldTasks(db)

			// Delete expired availability records
			services.CleanupExpiredAvailability(db)
		}
	}
}

// Background process that periodically checks channel status
func runChannelChecker(db *gorm.DB) {
	ticker := time.NewTicker(1 * time.Hour) // Check every 1 hour
	defer ticker.Stop()

	for range ticker.C {
		log.Println("start channel status check")
		services.CleanupArchivedChannels(db) // Deactivate configs for archived channels
	}
}
