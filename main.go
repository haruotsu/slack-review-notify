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

	"slack-review-notify/handlers"
	"slack-review-notify/models"
	"slack-review-notify/services"
)

func main() {
    err := godotenv.Load()
    if err != nil {
        log.Println("⚠️ .env ファイルが読み込めませんでした（環境変数が使われている場合はOK）")
    }

    db, err := gorm.Open(sqlite.Open("review_tasks.db"), &gorm.Config{})
    if err != nil {
        log.Fatal("DB接続失敗:", err)
    }

    db.AutoMigrate(&models.ReviewTask{}, &models.ChannelConfig{})

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
            if e.Action != nil && *e.Action == "labeled" && e.Label != nil {
                pr := e.PullRequest
                repo := e.Repo
                repoFullName := fmt.Sprintf("%s/%s", repo.GetOwner().GetLogin(), repo.GetName())
                labelName := e.Label.GetName()
                
                // チャンネル設定を全て取得
                var configs []models.ChannelConfig
                db.Where("is_active = ?", true).Find(&configs)
                
                if len(configs) == 0 {
                    log.Println("⚠️ アクティブなチャンネル設定が見つかりません。デフォルトチャンネルを使用します。")
                    
                    // デフォルトチャンネルを使用
                    slackChannel := os.Getenv("SLACK_CHANNEL_ID")
                    mentionID := os.Getenv("DEFAULT_MENTION_ID")
                    
                    if slackChannel == "" {
                        c.JSON(http.StatusInternalServerError, gin.H{"error": "no channel configured"})
                        return
                    }
                    
                    slackTs, slackChannelID, err := services.SendSlackMessage(
                        pr.GetHTMLURL(), 
                        pr.GetTitle(), 
                        slackChannel,
                        mentionID, // 環境変数のメンションIDを使用
                    )
                    
                    if err != nil {
                        log.Println("Slack送信失敗:", err)
                        c.JSON(http.StatusInternalServerError, gin.H{"error": "slack message failed"})
                        return
                    }
                    
                    log.Printf("Slack送信成功: ts=%s, channel=%s", slackTs, slackChannelID)
                    
                    task := models.ReviewTask{
                        ID:           uuid.NewString(),
                        PRURL:        pr.GetHTMLURL(),
                        Repo:         repoFullName,
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
                    
                    log.Printf("保存直前のtask: %+v\n", task)
                    log.Println("✅ PRを登録しました:", task.PRURL)
                    
                } else {
                    // 設定されたチャンネルごとに通知
                    notified := false
                    
                    for _, config := range configs {
                        // チャンネルがアーカイブされているか確認
                        isArchived, err := services.IsChannelArchived(config.SlackChannelID)
                        if err != nil {
                            log.Printf("チャンネル状態確認エラー（チャンネル: %s）: %v", config.SlackChannelID, err)
                            // エラーが出ても一応続行
                        }
                        
                        if isArchived {
                            log.Printf("⚠️ チャンネル %s はアーカイブされているためスキップします", config.SlackChannelID)
                            
                            // アーカイブされたチャンネルの設定を非アクティブに更新
                            config.IsActive = false
                            config.UpdatedAt = time.Now()
                            if err := db.Save(&config).Error; err != nil {
                                log.Printf("チャンネル設定更新エラー: %v", err)
                            } else {
                                log.Printf("✅ アーカイブされたチャンネル %s の設定を非アクティブにしました", config.SlackChannelID)
                            }
                            continue
                        }
                        
                        // リポジトリフィルタがある場合はチェック
                        if !services.IsRepositoryWatched(&config, repoFullName) {
                            // このチャンネルの監視対象外のリポジトリ
                            log.Printf("リポジトリ %s はチャンネル %s の監視対象外です（設定: %s）", 
                                repoFullName, config.SlackChannelID, config.RepositoryList)
                            continue
                        }
                        
                        // ラベルをチェック
                        if config.LabelName != "" && config.LabelName != labelName {
                            log.Printf("ラベル %s はチャンネル %s の監視対象外です（設定: %s）", 
                                labelName, config.SlackChannelID, config.LabelName)
                            continue
                        }
                        
                        // メンションIDを取得
                        mentionID := config.DefaultMentionID
                        if mentionID == "" {
                            mentionID = os.Getenv("DEFAULT_MENTION_ID")
                        }
                        
                        slackTs, slackChannelID, err := services.SendSlackMessage(
                            pr.GetHTMLURL(), 
                            pr.GetTitle(), 
                            config.SlackChannelID,
                            mentionID,
                        )
                        
                        if err != nil {
                            log.Printf("Slack送信失敗（チャンネル: %s）: %v", config.SlackChannelID, err)
                            continue
                        }
                        
                        log.Printf("Slack送信成功: ts=%s, channel=%s", slackTs, slackChannelID)
                        
                        task := models.ReviewTask{
                            ID:           uuid.NewString(),
                            PRURL:        pr.GetHTMLURL(),
                            Repo:         repoFullName,
                            PRNumber:     pr.GetNumber(),
                            Title:        pr.GetTitle(),
                            SlackTS:      slackTs,
                            SlackChannel: slackChannelID,
                            Status:       "pending",
                            CreatedAt:    time.Now(),
                            UpdatedAt:    time.Now(),
                        }
                        
                        if err := db.Create(&task).Error; err != nil {
                            log.Printf("DB保存失敗（チャンネル: %s）: %v", config.SlackChannelID, err)
                            continue
                        }
                        
                        log.Printf("保存直前のtask: %+v\n", task)
                        log.Printf("✅ PRを登録しました（チャンネル: %s）: %s", config.SlackChannelID, task.PRURL)
                        notified = true
                    }
                    
                    if !notified {
                        log.Println("⚠️ どのチャンネルにも通知されませんでした")
                        c.JSON(http.StatusOK, gin.H{"message": "no matching channel"})
                        return
                    }
                }
            }
        }

        c.Status(http.StatusOK)
    })

    // Slackコマンド受信
    r.POST("/slack/command", handlers.HandleSlackCommand(db))

    // Slackイベント受信エンドポイント
    r.POST("/slack/events", handlers.HandleSlackEvents(db))

    // チャンネル設定を出力
    printChannelConfigs(db)

    r.Run(":8080")
}

// 定期的にタスクをチェックするバックグラウンド処理
func runTaskChecker(db *gorm.DB) {
    ticker := time.NewTicker(10 * time.Second) // 10秒ごとにチェック
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            log.Println("タスクのチェックを開始します...")
            
            // レビュー待ちタスク（レビュアー未割り当て）のチェック
            services.CheckPendingTasks(db)
            
            // レビュー中タスク（レビュアー割り当て済み）のチェック
            services.CheckInReviewTasks(db)
        }
    }
}

// 定期的にチャンネル状態を確認するバックグラウンド処理
func runChannelChecker(db *gorm.DB) {
    ticker := time.NewTicker(1 * time.Hour) // 1時間ごとにチェック
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            log.Println("チャンネル状態チェックを開始します...")
            services.CleanupArchivedChannels(db)
        }
    }
}

// サーバー起動時にチャンネル設定を出力
func printChannelConfigs(db *gorm.DB) {
    var configs []models.ChannelConfig
    db.Find(&configs)
    
    log.Printf("チャンネル設定一覧 (%d件):", len(configs))
    for i, config := range configs {
        log.Printf("[%d] ID=%s, Channel=%s, Mention=%s, Active=%v", 
            i, config.ID, config.SlackChannelID, config.DefaultMentionID, config.IsActive)
    }
}
