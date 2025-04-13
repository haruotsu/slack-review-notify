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
		// リクエストボディをログに出力
		body, _ := io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
		log.Printf("Slackイベント受信: %s", string(body))
		
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
			log.Printf("JSONパースエラー: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}
		
		log.Printf("イベントタイプ: %s", payload.Type)
		if payload.Event.Type != "" {
			log.Printf("イベント詳細: type=%s, channel=%s, user=%s", 
				payload.Event.Type, payload.Event.Channel, payload.Event.User)
		}
		
		// URL検証チャレンジへの応答
		if payload.Type == "url_verification" {
			c.String(http.StatusOK, payload.Challenge)
			return
		}
		
		// ボットがチャンネルに追加されたイベント
		if payload.Event.Type == "member_joined_channel" {
			// ボットのユーザーIDを環境変数から取得または固定値を使用
			botUserID := os.Getenv("SLACK_BOT_USER_ID")
			if botUserID == "" {
				botUserID = "U08MJBQAS6T" // ログから確認したボットのユーザーID
			}
			
			log.Printf("ボットID比較: event=%s, config=%s", payload.Event.User, botUserID)
			
			// ボット自身がチャンネルに追加された場合
			if payload.Event.User == botUserID {
				log.Printf("ボットがチャンネルに追加されました: %s", payload.Event.Channel)
				
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
					
					log.Printf("作成する設定: %+v", config) // 作成前の設定内容をログ出力
					
					createResult := db.Create(&config)
					if createResult.Error != nil {
						log.Printf("チャンネル設定作成エラー: %v", createResult.Error)
					} else {
						log.Printf("✅ チャンネル設定を作成しました: %s, ID=%s", 
							payload.Event.Channel, config.ID)
							
						// 設定が保存されたか再確認
						var checkConfig models.ChannelConfig
						checkResult := db.Where("slack_channel_id = ?", payload.Event.Channel).First(&checkConfig)
						if checkResult.Error != nil {
							log.Printf("⚠️ 保存確認エラー: %v", checkResult.Error)
						} else {
							log.Printf("✅ 保存確認OK: ID=%s, Channel=%s", 
								checkConfig.ID, checkConfig.SlackChannelID)
						}
					}
				} else {
					log.Printf("このチャンネルの設定はすでに存在します: %s", payload.Event.Channel)
				}
			}
		}
		
		c.Status(http.StatusOK)
	}
} 
