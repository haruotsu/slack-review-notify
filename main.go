package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/go-github/v71/github"
	"github.com/google/uuid"
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
		log.Println("WARNING: fail to load .env file, using default env vars if available", err)
	} else {
		log.Println("INFO: .env file loaded successfully")
	}

	db, err := gorm.Open(sqlite.Open("review_tasks.db"), &gorm.Config{})
	if err != nil {
		log.Fatal("fail to connect db:", err)
	}

	if err := db.AutoMigrate(&models.ReviewTask{}, &models.ChannelConfig{}); err != nil {
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
			if e.Action != nil && (*e.Action == "labeled" || *e.Action == "opened" || *e.Action == "reopened") && e.PullRequest != nil {
				pr := e.PullRequest
				repo := e.Repo
				repoFullName := fmt.Sprintf("%s/%s", repo.GetOwner().GetLogin(), repo.GetName())
				
				var labelName string
				if *e.Action == "labeled" && e.Label != nil {
					labelName = e.Label.GetName()
				}

				// チャンネル設定を全て取得
				var configs []models.ChannelConfig
				db.Where("is_active = ?", true).Find(&configs)

				if len(configs) == 0 {
					log.Println("no active channel config found.")
					c.JSON(http.StatusOK, gin.H{"message": "no active channel config"})
					return
				}

				notified := false

				for _, config := range configs {
					// チャンネルがアーカイブされているか確認
					isArchived, err := services.IsChannelArchived(config.SlackChannelID)
					if err != nil {
						log.Printf("channel status check error (channel: %s): %v", config.SlackChannelID, err)
					}
					
					if isArchived {
						log.Printf("channel %s is archived. skip", config.SlackChannelID)
						
						// アーカイブされたチャンネルの設定を非アクティブに更新
						config.IsActive = false
						config.UpdatedAt = time.Now()
						if err := db.Save(&config).Error; err != nil {
							log.Printf("channel config update error: %v", err)
						} else {
							log.Printf("✅ archived channel %s config is deactivated", config.SlackChannelID)
						}
						continue
					}
					
					// リポジトリフィルタがある場合はチェック
					if !services.IsRepositoryWatched(&config, repoFullName) {
						log.Printf("repository %s is not watched (channel: %s, config: %s)",
							repoFullName, config.SlackChannelID, config.RepositoryList)
						continue
					}

					// ラベルをチェック (labeledイベントの場合のみ)
					if *e.Action == "labeled" && config.LabelName != "" && config.LabelName != labelName {
						log.Printf("label %s is not watched (channel: %s, config: %s)",
							labelName, config.SlackChannelID, config.LabelName)
						continue
					}
					// labeled以外のアクションの場合はラベルチェックをスキップ
					if *e.Action != "labeled" && config.LabelName != "" {
						log.Printf("action %s does not require label check or label %s not present (channel: %s)", 
							*e.Action, config.LabelName, config.SlackChannelID)
					}

					// ★★★ ログ追加①: 取得したconfigのデフォルトメンションIDを確認 ★★★
					log.Printf("DEBUG: ChannelConfig found for %s. DefaultMentionID: '%s'", config.SlackChannelID, config.DefaultMentionID)

					// レビュアーをランダムに選択
					reviewerID := ""
					if config.ReviewerList != "" {
						reviewers := strings.Split(config.ReviewerList, ",")
						if len(reviewers) > 0 {
							// 有効なレビュアー（空文字列でない）のみを抽出
							validReviewers := []string{}
							for _, r := range reviewers {
								trimmed := strings.TrimSpace(r)
								if trimmed != "" {
									validReviewers = append(validReviewers, trimmed)
								}
							}
							if len(validReviewers) > 0 {
								rand.Seed(time.Now().UnixNano()) // 乱数シードを設定
								randomIndex := rand.Intn(len(validReviewers))
								reviewerID = validReviewers[randomIndex]
								log.Printf("randomly selected reviewer: %s from %v", reviewerID, validReviewers)
							}
						}
					}

					// デフォルトメンションを初期値とする
					mentionID := config.DefaultMentionID
					initialStatus := "pending" // デフォルトステータス

					// レビュアーがランダム選択された場合の処理
					if reviewerID != "" {
						// mentionID = fmt.Sprintf("<@%s>", reviewerID) // ←コメントアウト済み
						initialStatus = "in_review"
					}

					// デフォルトメンションも空の場合のフォールバック
					if mentionID == "" {
						mentionID = "<!channel>"
					}

					// ★★★ ログ追加②: SendSlackMessageに渡す最終的なmentionIDを確認 ★★★
					log.Printf("DEBUG: Sending initial message to %s with MentionID: '%s' (ReviewerID: '%s')", config.SlackChannelID, mentionID, reviewerID)

					slackTs, slackChannelID, err := services.SendSlackMessage(
						pr.GetHTMLURL(),
						pr.GetTitle(),
						config.SlackChannelID,
						mentionID,
					)

					if err != nil {
						log.Printf("slack message failed (channel: %s): %v", config.SlackChannelID, err)
						continue // 次の設定へ
					}

					log.Printf("slack message sent: ts=%s, channel=%s", slackTs, slackChannelID)

					task := models.ReviewTask{
						ID:           uuid.NewString(),
						PRURL:        pr.GetHTMLURL(),
						Repo:         repoFullName,
						PRNumber:     pr.GetNumber(),
						Title:        pr.GetTitle(),
						SlackTS:      slackTs,
						SlackChannel: slackChannelID,
						Reviewer:     reviewerID, // 選択されたレビュアーIDを保存
						Status:       initialStatus, // pending または in_review
						CreatedAt:    time.Now(),
						UpdatedAt:    time.Now(),
					}

					if err := db.Create(&task).Error; err != nil {
						log.Printf("db insert failed (channel: %s): %v", config.SlackChannelID, err)
						// エラーが発生しても他のチャンネルへの通知は試みる
						continue
					}

					log.Printf("pr registered (channel: %s): %s", config.SlackChannelID, task.PRURL)
					notified = true

					// レビュアーが自動アサインされた場合、スレッドに通知
					if reviewerID != "" {
						if err := services.SendReviewerAssignedMessage(task); err != nil {
							log.Printf("reviewer assigned notification error (channel: %s, task: %s): %v", config.SlackChannelID, task.ID, err)
						}
					}
				}

				if !notified {
					log.Println("no matching channel and reviewer assignment")
					c.JSON(http.StatusOK, gin.H{"message": "no matching channel or assignment rule"})
					return
				}
			}
		}

		c.Status(http.StatusOK)
	})

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
	taskTicker := time.NewTicker(5 * time.Second) // 5秒ごとにチェック
	cleanupTicker := time.NewTicker(1 * time.Hour) // 1時間ごとにクリーンアップ
	defer taskTicker.Stop()
	defer cleanupTicker.Stop()

	for range time.NewTicker(100 * time.Millisecond).C {
		select {
		case <-taskTicker.C:
			log.Println("start task check")
			
			// レビュー待ちタスク（レビュアー未割り当て）のチェック
			services.CheckPendingTasks(db)
			
			// レビュー中タスク（レビュアー割り当て済み）のチェック
			services.CheckInReviewTasks(db)
			
		case <-cleanupTicker.C:
			log.Println("start old task cleanup")
			
			// 古いタスクの削除処理
			services.CleanupOldTasks(db)
		default:
			// ティッカーイベントがない場合はスキップ
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
