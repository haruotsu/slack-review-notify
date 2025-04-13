package main

import (
	"log"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"slack-review-notify/handlers"
	"slack-review-notify/models"
)

func main() {
    db, _ := gorm.Open(sqlite.Open("review_tasks.db"), &gorm.Config{})
    db.AutoMigrate(&models.ReviewTask{})

    r := gin.Default()
	handler := handlers.NewGitHubHandler(db)
    r.POST("/webhook", handler.HandleWebhook)

    err := r.Run(":8080")
    if err != nil {
        log.Fatal(err)
    }
}
