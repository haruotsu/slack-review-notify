package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"slack-review-notify/models"
	"slack-review-notify/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type SlackActionPayload struct {
    Type string `json:"type"`
    User struct {
        ID string `json:"id"`
    } `json:"user"`
    Actions []struct {
        ActionID string `json:"action_id"`
    } `json:"actions"`
    Container struct {
        ChannelID string `json:"channel_id"`
    } `json:"container"`
    Message struct {
        Ts string `json:"ts"`
    } `json:"message"`
}

func HandleSlackAction(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
		
        payloadStr := c.PostForm("payload")
        payloadStr = strings.TrimSpace(payloadStr)

        var payload SlackActionPayload
        if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
            return
        }

        slackUserID := payload.User.ID
        ts := payload.Message.Ts
        channel := payload.Container.ChannelID

		log.Printf("Slack action受信: ts=%s, channel=%s, userID=%s", ts, channel, slackUserID)


        var task models.ReviewTask
        if err := db.Where("slack_ts = ? AND slack_channel = ?", ts, channel).First(&task).Error; err != nil {
            c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
            return
        }

        switch payload.Actions[0].ActionID {
        case "review_take":
            task.Reviewer = slackUserID
            task.Status = "pending"
        case "review_watch":
            task.Status = "watching"
            t := time.Now().Add(2 * time.Hour)
            task.WatchingUntil = &t
        }

        task.UpdatedAt = time.Now()
        db.Save(&task)

        // メッセージ更新
        err := services.UpdateSlackMessage(channel, ts, task)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update message"})
            return
        }

        c.Status(http.StatusOK)
    }
}
