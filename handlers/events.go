package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"slack-review-notify/models"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Slackイベントを処理するハンドラ
func HandleSlackEvents(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
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
			// ボットのユーザーIDを環境変数から取得または固定値を使用
			botUserID := os.Getenv("SLACK_BOT_USER_ID")
			if botUserID == "" {
				botUserID = "TESTTEST"
			}
			
			log.Printf("bot id compare: event=%s, config=%s", payload.Event.User, botUserID)
			
			// ボット自身がチャンネルに追加された場合
			if payload.Event.User == botUserID {
				log.Printf("bot joined channel: %s", payload.Event.Channel)
				
				// チャンネル設定を作成または更新
				var config models.ChannelConfig
				result := db.Where("slack_channel_id = ?", payload.Event.Channel).First(&config)
				
				if result.Error != nil {
					// 新規作成
					config = models.ChannelConfig{
						ID:               uuid.NewString(),
						SlackChannelID:   payload.Event.Channel,
						DefaultMentionID: botUserID, // デフォルトはボット自身
						LabelName:        "needs-review", // デフォルト値
						IsActive:         true,
						CreatedAt:        time.Now(),
						UpdatedAt:        time.Now(),
					}
					
					createResult := db.Create(&config)
					if createResult.Error != nil {
						log.Printf("create channel config error: %v", createResult.Error)
					} else {
						log.Printf("create channel config: %s, ID=%s", 
							payload.Event.Channel, config.ID)
							
						// 設定が保存されたか再確認
						var checkConfig models.ChannelConfig
						checkResult := db.Where("slack_channel_id = ?", payload.Event.Channel).First(&checkConfig)
						if checkResult.Error != nil {
							log.Printf("save check error: %v", checkResult.Error)
						} else {
							log.Printf("save check ok: ID=%s, Channel=%s", 
								checkConfig.ID, checkConfig.SlackChannelID)
						}
					}
				} else {
					log.Printf("this channel config already exists: %s", payload.Event.Channel)
				}
			}
		}
		
		c.Status(http.StatusOK)
	}
} 
