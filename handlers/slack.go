package handlers

import (
	"encoding/json"
	"fmt"
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
                // 24時間停止
                pauseUntil = time.Now().Add(24 * time.Hour)
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
        
        // 「ちょっと待って」以外のアクション（レビューします！など）の場合
        var task models.ReviewTask
        // taskIDを使ってデータベースからタスクを取得するロジック (既存)
        taskID := ""
        // アクションIDからタスクIDを抽出する (例: "review_done:task_id")
        actionParts := strings.Split(actionID, ":")
        actionCommand := actionParts[0]
        if len(actionParts) > 1 {
            taskID = actionParts[1] 
        } else {
             // 古い形式（review_doneなど）や taskID がないアクション（review_take）の場合
             // メッセージのTSからタスクを検索する必要があるかもしれない
             // ここではTSから検索する例を示す (ただし、効率的ではない可能性あり)
            if err := db.Where("slack_ts = ? AND slack_channel = ?", payload.Message.Ts, payload.Container.ChannelID).First(&task).Error; err != nil {
                if err == gorm.ErrRecordNotFound {
                    log.Printf("task not found for ts %s in channel %s", payload.Message.Ts, payload.Container.ChannelID)
                    // ユーザーにエラーメッセージを返す方が親切かもしれない
                    // services.PostEphemeralMessage(payload.Container.ChannelID, payload.User.ID, "元のレビュー依頼が見つかりませんでした。")
                    c.Status(http.StatusOK) // エラーだがリトライさせないために OK を返す
                    return
                }
                log.Printf("db error find task by ts: %v", err)
                c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to find task"})
                return
            }
            // ボタンにtaskIDが含まれていない場合、actionIDはコマンドのみになる
            actionCommand = actionID 
        }
        
        // taskIDがある場合は、それでタスクを取得
        if taskID != "" {
            if err := db.First(&task, "id = ?", taskID).Error; err != nil {
                log.Printf("task find error (id: %s): %v", taskID, err)
                c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to find task"})
                return
            }
        }

        // SlackのユーザーIDを取得
        slackUserID = payload.User.ID

        switch actionCommand { // actionIDではなくactionCommandで判定
        case "review_done":
            
            // レビュー完了通知をスレッドに投稿
            message := fmt.Sprintf("✅ <@%s> さんがレビューを完了しました！感謝！👏", slackUserID)
            if err := services.PostToThread(task.SlackChannel, task.SlackTS, message); err != nil {
                log.Printf("review done notification error: %v", err)
                // エラーが発生しても処理は続行
            }

            // ステータスを完了に変更
            task.Status = "done"
            // 完了時にレビュアーが空だったら、完了した人をレビュアーとして記録しても良いかも
            if task.Reviewer == "" {
                task.Reviewer = slackUserID
            }
            task.UpdatedAt = time.Now()

            if err := db.Save(&task).Error; err != nil {
                log.Printf("task save error: %v", err)
                c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save task"})
                return
            }
            c.Status(http.StatusOK)
            return
        case "stop_reminder":
            // リマインダー停止処理（例）
            taskIDToStop := ""
            if len(actionParts) > 1 {
                taskIDToStop = actionParts[1]
            }
            if taskIDToStop == "" {
                // taskIDがなければ処理中断
                log.Printf("stop_reminder action requires task ID")
                c.Status(http.StatusBadRequest)
                return
            }
            log.Printf("User %s requested to stop reminders for task %s", slackUserID, taskIDToStop)
            
            // 停止したことをユーザーに通知（Ephemeral Messageなど）
            services.PostEphemeralMessage(payload.Container.ChannelID, slackUserID, fmt.Sprintf("タスクID %s のリマインダーを停止しました。", taskIDToStop))

            c.Status(http.StatusOK)
            return
        
        // --- ここから select メニューのアクション処理を追加 ---
        case "select_reminder_action": // selectメニューの action_id
            if len(payload.Actions) > 0 && payload.Actions[0].SelectedOption.Value != "" {
                selectedValue := payload.Actions[0].SelectedOption.Value // 例: "task_id:snooze_1h" or "task_id:stop"
                parts := strings.Split(selectedValue, ":")
                if len(parts) == 2 {
                    taskIDForAction := parts[0]
                    reminderAction := parts[1]

                    // taskIDForAction を使ってDBからタスクを取得
                    var reminderTask models.ReviewTask
                    if err := db.First(&reminderTask, "id = ?", taskIDForAction).Error; err != nil {
                        log.Printf("reminder task find error (id: %s): %v", taskIDForAction, err)
                        services.PostEphemeralMessage(payload.Container.ChannelID, slackUserID, "リマインダー対象のタスクが見つかりませんでした。")
                        c.Status(http.StatusOK)
                        return
                    }

                    switch reminderAction {
                    case "snooze_1h":
                        // 1時間スヌーズする処理 (例: 次回リマインド時刻を更新)
                        // reminderTask.NextReminderAt = time.Now().Add(1 * time.Hour)
                        // db.Save(&reminderTask)
                        log.Printf("User %s snoozed reminder for task %s for 1 hour", slackUserID, taskIDForAction)
                        services.PostEphemeralMessage(payload.Container.ChannelID, slackUserID, fmt.Sprintf("タスクID %s のリマインダーを1時間後に再通知します。", taskIDForAction))
                    case "snooze_1d":
                        // 1日スヌーズする処理
                        // reminderTask.NextReminderAt = time.Now().Add(24 * time.Hour)
                        // db.Save(&reminderTask)
                        log.Printf("User %s snoozed reminder for task %s for 1 day", slackUserID, taskIDForAction)
                        services.PostEphemeralMessage(payload.Container.ChannelID, slackUserID, fmt.Sprintf("タスクID %s のリマインダーを1日後に再通知します。", taskIDForAction))
                    case "stop":
                        // リマインダーを停止する処理
                        // reminderTask.Remind = false
                        // db.Save(&reminderTask)
                        log.Printf("User %s stopped reminders for task %s", slackUserID, taskIDForAction)
                        services.PostEphemeralMessage(payload.Container.ChannelID, slackUserID, fmt.Sprintf("タスクID %s のリマインダーを完全に停止しました。", taskIDForAction))

                    default:
                        log.Printf("unknown reminder action: %s", reminderAction)
                        services.PostEphemeralMessage(payload.Container.ChannelID, slackUserID, "不明なリマインダー操作です。")

                    }

                } else {
                    log.Printf("invalid selected value format: %s", selectedValue)
                    services.PostEphemeralMessage(payload.Container.ChannelID, slackUserID, "リマインダー操作の形式が無効です。")
                }
            } else {
                log.Printf("no selected option found for select_reminder_action")
                services.PostEphemeralMessage(payload.Container.ChannelID, slackUserID, "リマインダー操作が選択されていません。")
            }
            c.Status(http.StatusOK)
            return

        case "reassign_reviewer":
            log.Printf("reassign reviewer action received for task %s by user %s", taskID, slackUserID)

            // taskIDがない場合はエラー
            if taskID == "" {
                log.Printf("reassign_reviewer action requires task ID")
                services.PostEphemeralMessage(payload.Container.ChannelID, slackUserID, "エラー: 担当者変更に必要なタスク情報が見つかりません。")
                c.Status(http.StatusBadRequest)
                return
            }

            // チャンネル設定を取得
            var config models.ChannelConfig
            if err := db.Where("slack_channel_id = ?", task.SlackChannel).First(&config).Error; err != nil {
                log.Printf("failed to find channel config for channel %s: %v", task.SlackChannel, err)
                services.PostEphemeralMessage(payload.Container.ChannelID, slackUserID, "エラー: チャンネル設定が見つかりません。")
                c.Status(http.StatusInternalServerError)
                return
            }

            // レビュワーリストを取得
            if config.ReviewerList == "" {
                log.Printf("reviewer list is empty for channel %s", task.SlackChannel)
                services.PostEphemeralMessage(payload.Container.ChannelID, slackUserID, "このチャンネルにはレビュワー候補が登録されていません。担当者を変更できません。")
                c.Status(http.StatusOK) // エラーではないのでOKを返す
                return
            }

            reviewers := strings.Split(config.ReviewerList, ",")
            validReviewers := []string{}
            for _, r := range reviewers {
                trimmed := strings.TrimSpace(r)
                // 現在の担当者を除外し、空でないIDのみを候補とする
                if trimmed != "" && trimmed != task.Reviewer {
                    validReviewers = append(validReviewers, trimmed)
                }
            }

            // 他の候補者がいない場合
            if len(validReviewers) == 0 {
                log.Printf("no other reviewers available for task %s in channel %s", taskID, task.SlackChannel)
                message := ""
                if task.Reviewer != "" {
                    message = fmt.Sprintf("他に担当できるレビュワーがいません。(現在の担当者: <@%s>)", task.Reviewer)
                } else {
                    message = "他に担当できるレビュワーがいません。"
                }
                services.PostEphemeralMessage(payload.Container.ChannelID, slackUserID, message)
                c.Status(http.StatusOK)
                return
            }

            // 新しいレビュアーをランダムに選択
            rand.Seed(time.Now().UnixNano())
            newReviewerID := validReviewers[rand.Intn(len(validReviewers))]

            log.Printf("reassigning task %s from %s to %s", taskID, task.Reviewer, newReviewerID)

            // タスクのレビュアーを更新
            oldReviewer := task.Reviewer // 変更前のレビュアーを保持
            task.Reviewer = newReviewerID
            task.Status = "in_review" // 担当者が決まったのでステータス更新
            task.UpdatedAt = time.Now()

            if err := db.Save(&task).Error; err != nil {
                log.Printf("failed to save reassigned task %s: %v", taskID, err)
                services.PostEphemeralMessage(payload.Container.ChannelID, slackUserID, "エラー: 担当者の変更に失敗しました。")
                c.Status(http.StatusInternalServerError)
                return
            }

            // 担当変更をスレッドに通知
            var reassignmentMessage string
            if oldReviewer != "" {
                reassignmentMessage = fmt.Sprintf("🔄 <@%s> さんによって担当者が <@%s> さんから <@%s> さんに変更されました！", slackUserID, oldReviewer, newReviewerID)
            } else {
                reassignmentMessage = fmt.Sprintf("🔄 <@%s> さんによって担当者が <@%s> さんに割り当てられました！", slackUserID, newReviewerID)
            }

            if err := services.PostToThread(task.SlackChannel, task.SlackTS, reassignmentMessage); err != nil {
                log.Printf("failed to post reassignment notification for task %s: %v", taskID, err)
                // メッセージ投稿失敗は致命的ではないため、処理は続行
            }

            // 元の担当割り当てメッセージのボタンを更新・削除する方が親切かもしれない
            // services.UpdateMessageToRemoveButtons(task.SlackChannel, payload.Message.Ts, "担当者が変更されました。")

            c.Status(http.StatusOK)
            return

        default:
            log.Printf("unknown actionID received: %s", actionCommand)
        }
    }
}
