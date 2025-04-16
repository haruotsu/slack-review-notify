package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"slack-review-notify/models"
	"slack-review-notify/services"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Slackイベントを処理するハンドラ
func HandleSlackEvents(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// ★★★ リクエスト受信直後にログ出力 ★★★
		log.Println("INFO: Received request on /slack/events endpoint")

		body, _ := io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
		log.Printf("slack event received: %s", string(body))
		
		var payload struct {
			Type      string `json:"type"`
			Challenge string `json:"challenge"`
			Event     struct {
				Type      string `json:"type"`
				Channel   string `json:"channel"`
				User      string `json:"user"`
				Timestamp string `json:"event_ts"`
			} `json:"event"`
		}
		
		if err := json.Unmarshal(body, &payload); err != nil {
			log.Printf("json parse error: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}
		if payload.Event.Type != "" {
			log.Printf("event details: type=%s, channel=%s, user=%s", 
				payload.Event.Type, payload.Event.Channel, payload.Event.User)
		}
		
		// ボットがチャンネルに追加されたイベント
		if payload.Event.Type == "member_joined_channel" {
			log.Printf("bot joined channel: %s", payload.Event.Channel)
			
			var config models.ChannelConfig
			var messagePosted bool = false // メッセージ投稿済みフラグ
			result := db.Where("slack_channel_id = ?", payload.Event.Channel).First(&config)
			
			if result.Error != nil {
				if result.Error == gorm.ErrRecordNotFound {
					// --- 新規作成 --- 
					config = models.ChannelConfig{
						ID:               uuid.NewString(),
						SlackChannelID:   payload.Event.Channel,
						DefaultMentionID: "", // 空に設定
						LabelName:        "needs-review",
						IsActive:         true,
						CreatedAt:        time.Now(),
						UpdatedAt:        time.Now(),
					}
					if err := db.Create(&config).Error; err != nil {
						log.Printf("create channel config error: %v", err)
					} else {
						log.Printf("create channel config successful: %s", payload.Event.Channel)
						// ★★★ 作成成功後にメッセージ投稿 ★★★
						if postErr := services.PostJoinMessage(payload.Event.Channel); postErr != nil {
							log.Printf("Failed to post join message after creation: %v", postErr)
						} else {
							messagePosted = true
						}
					}
				} else {
					// レコードが見つからない以外のDBエラー
					log.Printf("db error finding channel config: %v", result.Error)
				}
			} else {
				// --- 既存レコードあり (再参加など) --- 
				log.Printf("channel config already exists for %s, reactivating...", payload.Event.Channel)
				config.IsActive = true // 再参加時はアクティブにする
				config.UpdatedAt = time.Now()
				if err := db.Save(&config).Error; err != nil {
					log.Printf("failed to reactivate channel config for %s: %v", payload.Event.Channel, err)
				} else {
					log.Printf("reactivate channel config successful: %s", payload.Event.Channel)
					// ★★★ 更新成功後にメッセージ投稿 (まだ投稿されていなければ) ★★★
					if !messagePosted {
						if postErr := services.PostJoinMessage(payload.Event.Channel); postErr != nil {
							log.Printf("Failed to post join message after reactivation: %v", postErr)
						} else {
							messagePosted = true
						}
					}
				}
			}
		}
		
		// URL検証チャレンジへの応答
		if payload.Type == "url_verification" {
			c.String(http.StatusOK, payload.Challenge)
			return
		}

		c.Status(http.StatusOK) // 他のイベントタイプはOKだけ返す
	}
} 
