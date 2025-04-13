package handlers

import (
	"encoding/json"
	"fmt"
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
        Value    string `json:"value,omitempty"`
        // 選択メニュー用の項目
        SelectedOption struct {
            Value string `json:"value"`
            Text  struct {
                Text string `json:"text"`
            } `json:"text"`
        } `json:"selected_option,omitempty"`
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
            log.Printf("payload json parse error: %v", err)
            c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
            return
        }
        
        slackUserID := payload.User.ID
        ts := payload.Message.Ts
        channel := payload.Container.ChannelID
        
        log.Printf("slack action received: ts=%s, channel=%s, userID=%s", ts, channel, slackUserID)
        
        // アクションがない場合はエラー
        if len(payload.Actions) == 0 {
            c.JSON(http.StatusBadRequest, gin.H{"error": "no action provided"})
            return
        }
        
        // アクションIDを取得
        actionID := payload.Actions[0].ActionID
        
        // 「リマインダー停止」の選択メニュー処理
        if actionID == "pause_reminder" {
            // 選択メニューからの値を取得
            var selectedValue string
            if payload.Actions[0].SelectedOption.Value != "" {
                selectedValue = payload.Actions[0].SelectedOption.Value
            } else {
                selectedValue = payload.Actions[0].Value
            }
            
            if selectedValue == "" {
                log.Printf("selected value is empty")
                c.JSON(http.StatusBadRequest, gin.H{"error": "selected value is empty"})
                return
            }
            
            // 値からタスクIDと期間を抽出 (形式: "taskID:duration")
            parts := strings.Split(selectedValue, ":")
            if len(parts) != 2 {
                log.Printf("invalid value format: %s", selectedValue)
                c.JSON(http.StatusBadRequest, gin.H{"error": "invalid value format"})
                return
            }
            
            taskID := parts[0]
            duration := parts[1]
            
            // タスクIDを使ってデータベースから直接タスクを検索
            var taskToUpdate models.ReviewTask
            if err := db.Where("id = ?", taskID).First(&taskToUpdate).Error; err != nil {
                log.Printf("task id %s not found: %v", taskID, err)
                c.JSON(http.StatusNotFound, gin.H{"error": "task not found by ID"})
                return
            }
            
            // 選択された期間に基づいてリマインダーを一時停止
            var pauseUntil time.Time
            
            switch duration {
            case "20s":
                pauseUntil = time.Now().Add(20 * time.Second)
                taskToUpdate.ReminderPausedUntil = &pauseUntil
            case "30s":
                pauseUntil = time.Now().Add(30 * time.Second)
                taskToUpdate.ReminderPausedUntil = &pauseUntil
            case "1m":
                pauseUntil = time.Now().Add(1 * time.Minute)
                taskToUpdate.ReminderPausedUntil = &pauseUntil
            case "today":
                // 今日の23:59:59まで停止
                now := time.Now()
                pauseUntil = time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())
                taskToUpdate.ReminderPausedUntil = &pauseUntil
            case "stop":
                // レビュー担当者が決まるまで通知しない
                taskToUpdate.Status = "paused"
            default:
                pauseUntil = time.Now().Add(30 * time.Second) // デフォルト
                taskToUpdate.ReminderPausedUntil = &pauseUntil
            }
            
            db.Save(&taskToUpdate)
            
            // 一時停止を通知
            err := services.SendReminderPausedMessage(taskToUpdate, duration)
            if err != nil {
                log.Printf("pause reminder send error: %v", err)
            }
            
            c.Status(http.StatusOK)
            return
        }
        
        // 「ちょっと待って」以外のアクション（レビューします！など）の場合
        var task models.ReviewTask
        if err := db.Where("slack_ts = ? AND slack_channel = ?", ts, channel).First(&task).Error; err != nil {
            log.Printf("task not found: ts=%s, channel=%s", ts, channel)
            c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
            return
        }
        
        // 残りは既存のスイッチケース
        switch actionID {
        case "review_take":
            // レビュアーを設定
            task.Reviewer = slackUserID
            // ステータスを確実に in_review に設定
            task.Status = "in_review"
            
            // タスクを保存
            if err := db.Save(&task).Error; err != nil {
                log.Printf("task save error: %v", err)
                c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save task"})
                return
            }
            
            // レビュアーが割り当てられたことをスレッドに通知
            if err := services.SendReviewerAssignedMessage(task); err != nil {
                log.Printf("reviewer assigned notification error: %v", err)
            }
            
            // メッセージ更新は行わない
            
            c.Status(http.StatusOK)
            return
		
		case "review_done":
			// レビュー完了通知をスレッドに投稿
			message := fmt.Sprintf("✅ <@%s> さんがレビューを完了しました！", slackUserID)
			if err := services.PostToThread(task.SlackChannel, task.SlackTS, message); err != nil {
				log.Printf("review done notification error: %v", err)
			}
			
			// ステータスを完了に変更
			task.Status = "done"
			task.UpdatedAt = time.Now()

			if err := db.Save(&task).Error; err != nil {
				log.Printf("task save error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save task"})
				return
			}
			
			c.Status(http.StatusOK)
			return
		}
    }
}
