package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
				// 翌営業日の営業開始時刻まで停止
				// チャンネル設定を取得
				var config models.ChannelConfig
				if err := db.Where("slack_channel_id = ? AND label_name = ?", taskToUpdate.SlackChannel, taskToUpdate.LabelName).First(&config).Error; err != nil {
					// 設定が見つからない場合はデフォルト（10:00）を使用
					pauseUntil = services.GetNextBusinessDayMorningWithConfig(time.Now(), nil)
				} else {
					// 設定に基づいて営業開始時刻を使用
					pauseUntil = services.GetNextBusinessDayMorningWithConfig(time.Now(), &config)
				}
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
			// tsとchannelを使ってタスクを検索（pending状態の場合は少し待ってからリトライ）
			var task models.ReviewTask
			const maxRetries = 5
			const retryDelay = 200 * time.Millisecond

			var err error
			for retry := 0; retry < maxRetries; retry++ {
				err = db.Where("slack_ts = ? AND slack_channel = ?", ts, channel).First(&task).Error
				if err == nil {
					break
				}

				// レコードが見つからない場合、少し待ってからリトライ
				if retry < maxRetries-1 {
					log.Printf("task not found (attempt %d/%d): ts=%s, channel=%s, retrying in %v",
						retry+1, maxRetries, ts, channel, retryDelay)
					time.Sleep(retryDelay)
				}
			}

			if err != nil {
				log.Printf("task not found after %d retries: ts=%s, channel=%s", maxRetries, ts, channel)
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
			// 値を解析: "taskID" または "taskID:replacingReviewerID" 形式
			actionValue := payload.Actions[0].Value
			var taskID string
			var replacingReviewerID string
			if idx := strings.Index(actionValue, ":"); idx >= 0 {
				taskID = actionValue[:idx]
				replacingReviewerID = actionValue[idx+1:]
			} else {
				taskID = actionValue
			}

			// タスクIDを使ってデータベースからタスクを検索
			var taskToUpdate models.ReviewTask
			if err := db.Where("id = ?", taskID).First(&taskToUpdate).Error; err != nil {
				log.Printf("task id %s not found: %v", taskID, err)
				c.JSON(http.StatusNotFound, gin.H{"error": "task not found by ID"})
				return
			}

			// もしLabelNameが設定されていない既存のタスクの場合はデフォルト値を使用
			if taskToUpdate.LabelName == "" {
				taskToUpdate.LabelName = "needs-review"
				db.Save(&taskToUpdate)
			}

			// 除外対象: PR作成者 + 他の現行レビュワー
			excludeIDs := []string{}
			if taskToUpdate.PRAuthorSlackID != "" {
				excludeIDs = append(excludeIDs, taskToUpdate.PRAuthorSlackID)
			}
			if taskToUpdate.Reviewers != "" {
				for _, id := range strings.Split(taskToUpdate.Reviewers, ",") {
					if trimmed := strings.TrimSpace(id); trimmed != "" {
						excludeIDs = append(excludeIDs, trimmed)
					}
				}
			} else if taskToUpdate.Reviewer != "" {
				excludeIDs = append(excludeIDs, taskToUpdate.Reviewer)
			}

			// 新しいレビュワーを1人選択
			newReviewerIDs := services.SelectRandomReviewers(db, taskToUpdate.SlackChannel, taskToUpdate.LabelName, 1, excludeIDs)

			// SelectRandomReviewersがDefaultMentionIDのみを返した場合は候補なし
			noRealCandidate := false
			if len(newReviewerIDs) == 0 {
				noRealCandidate = true
			} else {
				var cfg models.ChannelConfig
				if err := db.Where("slack_channel_id = ? AND label_name = ?", taskToUpdate.SlackChannel, taskToUpdate.LabelName).First(&cfg).Error; err == nil {
					if cfg.ReviewerList != "" && len(newReviewerIDs) == 1 && newReviewerIDs[0] == cfg.DefaultMentionID {
						noRealCandidate = true
					}
				}
			}

			if noRealCandidate {
				message := "レビュワーが1人しか登録されていないため、変更できません。他のレビュワーを登録してください。"
				if err := services.PostToThread(taskToUpdate.SlackChannel, taskToUpdate.SlackTS, message); err != nil {
					log.Printf("notification error: %v", err)
				}
				c.Status(http.StatusOK)
				return
			}
			newReviewerID := newReviewerIDs[0]

			// 古いレビュワーIDを保存
			oldReviewerID := taskToUpdate.Reviewer

			// Reviewers フィールドの更新
			if replacingReviewerID != "" && taskToUpdate.Reviewers != "" {
				// 特定のレビュワーを置換
				var updatedReviewers []string
				for _, id := range strings.Split(taskToUpdate.Reviewers, ",") {
					trimmed := strings.TrimSpace(id)
					if trimmed == replacingReviewerID {
						updatedReviewers = append(updatedReviewers, newReviewerID)
					} else {
						updatedReviewers = append(updatedReviewers, trimmed)
					}
				}
				taskToUpdate.Reviewers = strings.Join(updatedReviewers, ",")
				oldReviewerID = replacingReviewerID
			} else {
				// 後方互換: 単一レビュワーの変更
				if taskToUpdate.Reviewers != "" {
					taskToUpdate.Reviewers = strings.Replace(taskToUpdate.Reviewers, taskToUpdate.Reviewer, newReviewerID, 1)
				}
			}

			// Reviewer フィールドの更新（後方互換）
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
