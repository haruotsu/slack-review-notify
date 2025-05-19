package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"

	"slack-review-notify/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Slackイベントを処理するハンドラ
func HandleSlackEvents(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			log.Printf("failed to read request body: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
			return
		}

		c.Request.Body = io.NopCloser(bytes.NewBuffer(body))
		log.Printf("slack event received: %s", string(body))

		// 署名を検証
		if !services.ValidateSlackRequest(c.Request, body) {
			log.Println("invalid slack signature")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid slack signature"})
			return
		}

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

		// URL検証リクエストの処理
		if payload.Type == "url_verification" {
			c.JSON(http.StatusOK, gin.H{"challenge": payload.Challenge})
			return
		}

		if payload.Event.Type != "" {
			log.Printf("event details: type=%s, channel=%s, user=%s",
				payload.Event.Type, payload.Event.Channel, payload.Event.User)
		}
		c.Status(http.StatusOK)
	}
}
