package handlers

import (
	"fmt"
	"net/http"
	"slack-review-notify/models"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/go-github/v71/github"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type GithubHandler struct {
	DB *gorm.DB
}

func NewGitHubHandler(db *gorm.DB) *GithubHandler {
	return &GithubHandler{
		DB: db,
	}
}

func (h *GithubHandler) HandleWebhook(c *gin.Context) {
	// gitubのwebhookを受け取り中身をvalidateしたのち、中身を取り出す
	payload, err := github.ValidatePayload(c.Request, nil)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid payload"})
		return
	}
	// webhookの中身をどのイベントか判別して、goの構造体に格納する
	// github.WebHookType(c.Request)にリクエストヘッダーの "X-GitHub-Event" があり、イベントの種類（例："push"、"pull_request"など）を取得する
	event, err := github.ParseWebHook(github.WebHookType(c.Request), payload)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid event"})
		return
	}
	
	
	switch e := event.(type) {
		// プルリクエストのイベント
    case *github.PullRequestEvent:
        if e.Action != nil && *e.Action == "labeled" && e.Label != nil && e.PullRequest != nil {
            if e.Label.GetName() == "needs-review" {
                pr := e.PullRequest
                repo := e.Repo

                task := models.ReviewTask{
                    ID:           uuid.NewString(),
                    PRURL:        pr.GetHTMLURL(),
                    Repo:         fmt.Sprintf("%s/%s", repo.GetOwner().GetLogin(), repo.GetName()),
                    PRNumber:     pr.GetNumber(),
                    Title:        pr.GetTitle(),
                    Status:       "pending",
                    CreatedAt:    time.Now(),
                    UpdatedAt:    time.Now(),
                }

                h.DB.Create(&task)
                fmt.Println("✅ New review task saved:", task.PRURL)
            }
        }
    }

    c.Status(http.StatusOK)
	
}
