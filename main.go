package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/go-github/v71/github"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"slack-review-notify/models"
	"slack-review-notify/services"
)

func main() {
    // .env 読み込み
    err := godotenv.Load()
    if err != nil {
        log.Println("⚠️ .env ファイルが読み込めませんでした（環境変数が使われている場合はOK）")
    }

    // DB接続
    db, err := gorm.Open(sqlite.Open("review_tasks.db"), &gorm.Config{})
    if err != nil {
        log.Fatal("DB接続失敗:", err)
    }

    db.AutoMigrate(&models.ReviewTask{})

    // Ginでサーバー起動
    r := gin.Default()

    r.POST("/webhook", func(c *gin.Context) {
        payload, err := github.ValidatePayload(c.Request, []byte(os.Getenv("GITHUB_WEBHOOK_SECRET")))
        if err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
            return
        }

        event, err := github.ParseWebHook(github.WebHookType(c.Request), payload)
        if err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "cannot parse webhook"})
            return
        }

        switch e := event.(type) {
        case *github.PullRequestEvent:
            if e.Action != nil && *e.Action == "labeled" && e.Label != nil && e.PullRequest != nil {
                if e.Label.GetName() == "needs-review" {
                    pr := e.PullRequest
                    repo := e.Repo

                    // Slack通知を送信
                    slackChannel := os.Getenv("SLACK_CHANNEL_ID")
                    slackTs, slackChannelID, err := services.SendSlackMessage(pr.GetHTMLURL(), pr.GetTitle(), slackChannel)
                    if err != nil {
                        log.Println("Slack送信失敗:", err)
                        c.JSON(http.StatusInternalServerError, gin.H{"error": "slack message failed"})
                        return
                    }

                    // DB保存
                    task := models.ReviewTask{
                        ID:           uuid.NewString(),
                        PRURL:        pr.GetHTMLURL(),
                        Repo:         fmt.Sprintf("%s/%s", repo.GetOwner().GetLogin(), repo.GetName()),
                        PRNumber:     pr.GetNumber(),
                        Title:        pr.GetTitle(),
                        SlackTS:      slackTs,
                        SlackChannel: slackChannelID,
                        Status:       "pending",
                        CreatedAt:    time.Now(),
                        UpdatedAt:    time.Now(),
                    }

                    if err := db.Create(&task).Error; err != nil {
                        log.Println("DB保存失敗:", err)
                        c.JSON(http.StatusInternalServerError, gin.H{"error": "db insert failed"})
                        return
                    }

                    log.Println("✅ PRを登録しました:", task.PRURL)
                }
            }
        }

        c.Status(http.StatusOK)
    })

    r.Run(":8080")
}
