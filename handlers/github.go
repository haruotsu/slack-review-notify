package handlers

import (
	"fmt"
	"net/http"
	"slack-review-notify/models"
	"time"

	"log"
	"slack-review-notify/services"

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
            // リポジトリ名を取得
            repo := e.Repo
            repoFullName := fmt.Sprintf("%s/%s", repo.GetOwner().GetLogin(), repo.GetName())
            pr := e.PullRequest
            labelName := e.Label.GetName()
            
            // チャンネル設定を全て取得
            var configs []models.ChannelConfig
            h.DB.Where("is_active = ?", true).Find(&configs)
            
            for _, config := range configs {
                // このチャンネルが監視対象のリポジトリとラベルか確認
                if !services.IsRepositoryWatched(&config, repoFullName) {
                    continue // 監視対象外のリポジトリはスキップ
                }
                
                if config.LabelName != "" && config.LabelName != labelName {
                    continue // 監視対象外のラベルはスキップ
                }
                
                // このチャンネルには通知する
                slackTS, slackChannelID, err := services.SendSlackMessage(
                    pr.GetHTMLURL(), 
                    pr.GetTitle(), 
                    config.SlackChannelID,
                    config.DefaultMentionID, // 第4引数にメンションIDを追加
                )
                
                if err != nil {
                    log.Printf("slack send error (channel: %s): %v", config.SlackChannelID, err)
                    continue
                }
                
                // タスク作成処理
                task := models.ReviewTask{
                    ID:           uuid.NewString(),
                    PRURL:        pr.GetHTMLURL(),
                    Repo:         repoFullName,
                    PRNumber:     pr.GetNumber(),
                    Title:        pr.GetTitle(),
                    SlackTS:      slackTS,
                    SlackChannel: slackChannelID,
                    Status:       "pending",
                    CreatedAt:    time.Now(),
                    UpdatedAt:    time.Now(),
                }
                
                if err := h.DB.Create(&task).Error; err != nil {
                    log.Printf("db save error: %v", err)
                    continue
                }
                
                log.Printf("pr registered (channel: %s): %s", config.SlackChannelID, task.PRURL)
            }
        }
    }

    c.Status(http.StatusOK)
	
}
