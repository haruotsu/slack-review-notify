package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
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
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			log.Printf("failed to read request body: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
			return
		}

		// ボディを復元
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// 署名を検証
		if !services.ValidateSlackRequest(c.Request, bodyBytes) {
			log.Println("invalid slack signature")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid slack signature"})
			return
		}

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
		if actionID == "pause_reminder" || actionID == "pause_reminder_initial" {
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
			case "1h":
				pauseUntil = time.Now().Add(1 * time.Hour)
				taskToUpdate.ReminderPausedUntil = &pauseUntil
			case "2h":
				pauseUntil = time.Now().Add(2 * time.Hour)
				taskToUpdate.ReminderPausedUntil = &pauseUntil
			case "4h":
				pauseUntil = time.Now().Add(4 * time.Hour)
				taskToUpdate.ReminderPausedUntil = &pauseUntil
			case "today":
				// 翌営業日の朝まで停止
				pauseUntil = services.GetNextBusinessDayMorning()
				taskToUpdate.ReminderPausedUntil = &pauseUntil
			case "stop":
				// レビュー担当者が決まるまで通知しない
				taskToUpdate.Status = "paused"
			default:
				pauseUntil = time.Now().Add(1 * time.Hour) // デフォルト
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

		// 各アクションに対する処理
		switch actionID {
		case "review_done":
			// tsとchannelを使ってタスクを検索
			var task models.ReviewTask
			if err := db.Where("slack_ts = ? AND slack_channel = ?", ts, channel).First(&task).Error; err != nil {
				log.Printf("task not found: ts=%s, channel=%s", ts, channel)
				c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
				return
			}

			// レビュー完了通知をスレッドに投稿
			message := fmt.Sprintf("✅ <@%s> さんがレビューを完了しました！感謝！👏", slackUserID)
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

		case "change_reviewer":
			// タスクIDを取得
			taskID := payload.Actions[0].Value

			// タスクIDを使ってデータベースからタスクを検索
			var taskToUpdate models.ReviewTask
			if err := db.Where("id = ?", taskID).First(&taskToUpdate).Error; err != nil {
				log.Printf("task id %s not found: %v", taskID, err)
				c.JSON(http.StatusNotFound, gin.H{"error": "task not found by ID"})
				return
			}

			// 古いレビュワーIDを保存
			oldReviewerID := taskToUpdate.Reviewer

			// もしLabelNameが設定されていない既存のタスクの場合はデフォルト値を使用
			if taskToUpdate.LabelName == "" {
				// 既存のタスクのためデフォルト値を設定
				taskToUpdate.LabelName = "needs-review"
				// DBに保存（次回のために）
				db.Save(&taskToUpdate)
			}

			// 新しいレビュワーをランダムに選択
			newReviewerID := services.SelectRandomReviewer(db, taskToUpdate.SlackChannel, taskToUpdate.LabelName)

			// 新しいレビュワーが前と同じであれば、再度選択
			// (レビュワーリストが1人しかない場合は同じになる)
			var config models.ChannelConfig
			if newReviewerID == oldReviewerID && db.Where("slack_channel_id = ? AND label_name = ?", taskToUpdate.SlackChannel, taskToUpdate.LabelName).First(&config).Error == nil {
				reviewers := strings.Split(config.ReviewerList, ",")
				if len(reviewers) > 1 {
					// リストから古いレビュワー以外を選ぶ
					validReviewers := []string{}
					for _, r := range reviewers {
						if trimmed := strings.TrimSpace(r); trimmed != "" && trimmed != oldReviewerID {
							validReviewers = append(validReviewers, trimmed)
						}
					}

					if len(validReviewers) > 0 {
						r := rand.New(rand.NewSource(time.Now().UnixNano()))
						randomIndex := r.Intn(len(validReviewers))
						newReviewerID = validReviewers[randomIndex]
					}
				} else {
					// レビュワーが1人しかいない場合は通知メッセージを送信
					message := "レビュワーが1人しか登録されていないため、変更できません。他のレビュワーを登録してください。"
					if err := services.PostToThread(taskToUpdate.SlackChannel, taskToUpdate.SlackTS, message); err != nil {
						log.Printf("notification error: %v", err)
					}
				}
			}

			// レビュワーを更新
			taskToUpdate.Reviewer = newReviewerID
			taskToUpdate.UpdatedAt = time.Now()
			db.Save(&taskToUpdate)

			// レビュワーが変更されたことを通知
			err := services.SendReviewerChangedMessage(taskToUpdate, oldReviewerID)
			if err != nil {
				log.Printf("reviewer change notification error: %v", err)
			}

			c.Status(http.StatusOK)
			return
		}
	}
}
