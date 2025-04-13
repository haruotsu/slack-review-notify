package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
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
        
        // デバッグ用にペイロード全体をログ出力
        log.Printf("Slackペイロード全体: %s", payloadStr)
        
        var payload SlackActionPayload
        if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
            log.Printf("ペイロードのJSONパースに失敗: %v", err)
            c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
            return
        }
        
        slackUserID := payload.User.ID
        ts := payload.Message.Ts
        channel := payload.Container.ChannelID
        
        log.Printf("Slack action受信: ts=%s, channel=%s, userID=%s", ts, channel, slackUserID)
        
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
            
            log.Printf("pause_reminderアクション: 選択値=%s", selectedValue)
            
            if selectedValue == "" {
                log.Printf("選択値が空です")
                c.JSON(http.StatusBadRequest, gin.H{"error": "selected value is empty"})
                return
            }
            
            // 値からタスクIDと期間を抽出 (形式: "taskID:duration")
            parts := strings.Split(selectedValue, ":")
            if len(parts) != 2 {
                log.Printf("選択値のフォーマットが不正: %s", selectedValue)
                c.JSON(http.StatusBadRequest, gin.H{"error": "invalid value format"})
                return
            }
            
            taskID := parts[0]
            duration := parts[1]
            
            // タスクIDを使ってデータベースから直接タスクを検索
            var taskToUpdate models.ReviewTask
            if err := db.Where("id = ?", taskID).First(&taskToUpdate).Error; err != nil {
                log.Printf("タスクID %s が見つかりません: %v", taskID, err)
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
                log.Printf("一時停止通知の送信に失敗: %v", err)
            }
            
            c.Status(http.StatusOK)
            return
        }
        
        // 「ちょっと待って」以外のアクション（レビューします！など）の場合
        var task models.ReviewTask
        if err := db.Where("slack_ts = ? AND slack_channel = ?", ts, channel).First(&task).Error; err != nil {
            log.Printf("タスクが見つかりません: ts=%s, channel=%s", ts, channel)
            c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
            return
        }
        
        // 残りは既存のスイッチケース
        switch actionID {
        case "review_take":
            task.Reviewer = slackUserID
            task.Status = "in_review"
            
            // レビュアーが割り当てられたことをスレッドに通知
            services.SendReviewerAssignedMessage(task)
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

// リマインダーメッセージを送信する関数
func SendReminderMessage(task models.ReviewTask) error {
    // リマインダーメッセージ本文
    message := fmt.Sprintf("<@U08MRE10GS2> PRのレビューが必要です。対応できる方はメインメッセージのボタンから対応してください！\n*タイトル*: %s\n*リンク*: <%s>", 
        task.Title, task.PRURL)
    
    // デバッグログを追加
    log.Printf("リマインダー送信時のタスクID: %s", task.ID)
    
    // ボタン付きのメッセージブロックを作成
    blocks := []map[string]interface{}{
        {
            "type": "section",
            "text": map[string]string{
                "type": "mrkdwn",
                "text": message,
            },
        },
        {
            "type": "actions",
            "elements": []map[string]interface{}{
                {
                    "type": "button",
                    "text": map[string]string{
                        "type": "plain_text",
                        "text": "ちょっと待って！",
                    },
                    "action_id": "pause_reminder",
                    "value": task.ID, // ここにタスクIDを明示的に設定
                },
            },
        },
    }
    
    // スレッドにボタン付きメッセージを投稿
    body := map[string]interface{}{
        "channel": task.SlackChannel,
        "thread_ts": task.SlackTS,
        "blocks": blocks,
    }
    
    jsonData, _ := json.Marshal(body)
    req, err := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", bytes.NewBuffer(jsonData))
    if err != nil {
        return err
    }
    
    req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))
    req.Header.Set("Content-Type", "application/json")
    
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    // レスポンスをログに記録
    bodyBytes, _ := io.ReadAll(resp.Body)
    fmt.Println("🧵 リマインダー投稿レスポンス:", string(bodyBytes))
    
    return nil
}
