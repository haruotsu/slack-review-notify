package handlers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"slack-review-notify/models"
	"slack-review-notify/services"

	"github.com/gin-gonic/gin"
	"github.com/google/go-github/v71/github"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func HandleGitHubWebhook(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		eventType := c.GetHeader("X-GitHub-Event")
		log.Printf("GitHub Webhook received: event_type=%s", eventType)

		payload, err := github.ValidatePayload(c.Request, []byte(os.Getenv("GITHUB_WEBHOOK_SECRET")))
		if err != nil {
			log.Printf("Webhook validation error: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}

		event, err := github.ParseWebHook(github.WebHookType(c.Request), payload)
		if err != nil {
			log.Printf("Webhook parse error: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot parse webhook"})
			return
		}

		switch e := event.(type) {
		case *github.PullRequestEvent:
			log.Printf("PullRequestEvent received: action=%s", e.GetAction())
			if e.Action != nil && e.Label != nil {
				switch *e.Action {
				case "labeled":
					handleLabeledEvent(c, db, e)
				case "unlabeled":
					handleUnlabeledEvent(c, db, e)
				}
			}
		case *github.PullRequestReviewEvent:
			log.Printf("PullRequestReviewEvent received: action=%s", e.GetAction())
			if e.Action != nil && *e.Action == "submitted" {
				handleReviewSubmittedEvent(c, db, e)
			}
		default:
			log.Printf("Unknown event type received: %T", e)
		}

		c.Status(http.StatusOK)
	}
}

func handleLabeledEvent(c *gin.Context, db *gorm.DB, e *github.PullRequestEvent) {
	pr := e.PullRequest
	repo := e.Repo
	addedLabel := e.Label
	repoFullName := fmt.Sprintf("%s/%s", repo.GetOwner().GetLogin(), repo.GetName())

	if addedLabel == nil || addedLabel.Name == nil {
		log.Printf("added label is nil or has no name")
		return
	}

	addedLabelName := *addedLabel.Name
	log.Printf("handling labeled event: repo=%s, pr=%d, added_label=%s", repoFullName, pr.GetNumber(), addedLabelName)

	// チャンネル設定を全て取得
	var configs []models.ChannelConfig
	db.Where("is_active = ?", true).Find(&configs)

	if len(configs) == 0 {
		log.Println("no active channel config found")
		c.JSON(http.StatusOK, gin.H{"message": "no active channel config"})
		return
	}

	notified := false

	for _, config := range configs {
		// チャンネルがアーカイブされているか確認
		isArchived, checkErr := services.IsChannelArchived(config.SlackChannelID)
		if checkErr != nil {
			log.Printf("channel status check error (channel: %s): %v", config.SlackChannelID, checkErr)
		}

		if isArchived {
			log.Printf("channel %s is archived. skip", config.SlackChannelID)

			// アーカイブされたチャンネルの設定を非アクティブに更新
			config.IsActive = false
			config.UpdatedAt = time.Now()
			if err := db.Save(&config).Error; err != nil {
				log.Printf("channel config update error: %v", err)
			} else {
				log.Printf("archived channel %s config is deactivated", config.SlackChannelID)
			}
			continue
		}

		// リポジトリフィルタがある場合はチェック
		if !services.IsRepositoryWatched(&config, repoFullName) {
			log.Printf("repository %s is not watched (channel: %s, config: %s)",
				repoFullName, config.SlackChannelID, config.RepositoryList)
			continue
		}

		// 追加されたラベルが設定に関連するかチェック
		if !services.IsAddedLabelRelevant(&config, addedLabelName) {
			log.Printf("added label '%s' is not relevant to config (channel: %s, config: %s)",
				addedLabelName, config.SlackChannelID, config.LabelName)
			continue
		}

		// ラベルをチェック（複数ラベル対応）
		if !services.IsLabelMatched(&config, pr.Labels) {
			log.Printf("label requirements not met (channel: %s, config: %s)",
				config.SlackChannelID, config.LabelName)
			continue
		}

		// リトライトランザクション処理
		const maxRetries = 3
		const baseDelay = 100 * time.Millisecond

		var taskCreated bool
		var txErr error

		for retry := 0; retry < maxRetries; retry++ {
			txErr = db.Transaction(func(tx *gorm.DB) error {
				// 既存のアクティブなタスクをチェック（同一チャンネル・同一PRでは1つのみ作成）
				var existingTask models.ReviewTask
				existingErr := tx.Where("repo = ? AND pr_number = ? AND slack_channel = ? AND status IN (?)",
					repoFullName, pr.GetNumber(), config.SlackChannelID,
					[]string{"pending", "in_review", "snoozed", "waiting_business_hours"}).
					First(&existingTask).Error

				if existingErr == nil {
					// 既存のタスクが存在する場合はスキップ（ラベル名に関係なく）
					log.Printf("active task already exists for PR %d in channel %s (existing label: %s, current config: %s), skipping",
						pr.GetNumber(), config.SlackChannelID, existingTask.LabelName, config.LabelName)
					taskCreated = false
					return nil // トランザクションを正常終了させてスキップ
				}

				// まず仮のタスクレコードを作成（Slackメッセージ送信前）
				tempTask := models.ReviewTask{
					ID:           uuid.NewString(),
					PRURL:        pr.GetHTMLURL(),
					Repo:         repoFullName,
					PRNumber:     pr.GetNumber(),
					Title:        pr.GetTitle(),
					SlackTS:      "", // 後で更新
					SlackChannel: config.SlackChannelID,
					Reviewer:     "", // 後で更新
					Status:       "pending", // 仮の状態
					LabelName:    config.LabelName,
					CreatedAt:    time.Now(),
					UpdatedAt:    time.Now(),
				}

				if err := tx.Create(&tempTask).Error; err != nil {
					log.Printf("temp task insert failed (channel: %s): %v", config.SlackChannelID, err)
					return err
				}

				taskCreated = true

				// トランザクション外でSlackメッセージ送信とタスク更新を行う
				go func() {
					var slackTs, slackChannelID string
					var taskStatus string
					var reviewerID string

					// 営業時間外判定
					if !services.IsWithinBusinessHours(&config, time.Now()) {
						// 営業時間外の場合：メンション抜きメッセージを送信
						var err error
						slackTs, slackChannelID, err = services.SendSlackMessageOffHours(
							pr.GetHTMLURL(),
							pr.GetTitle(),
							config.SlackChannelID,
						)
						taskStatus = "waiting_business_hours"
						// レビュワーは翌営業日朝に設定する
						reviewerID = ""
						if err != nil {
							log.Printf("off-hours slack message failed (channel: %s): %v", config.SlackChannelID, err)
							// エラー時はタスクを削除
							db.Delete(&tempTask)
							return
						}
						log.Printf("off-hours message sent: ts=%s, channel=%s", slackTs, slackChannelID)
					} else {
						// 営業時間内の場合：通常のメンション付きメッセージを送信
						var err error
						slackTs, slackChannelID, err = services.SendSlackMessage(
							pr.GetHTMLURL(),
							pr.GetTitle(),
							config.SlackChannelID,
							config.DefaultMentionID,
						)
						taskStatus = "in_review"
						// ランダムにレビュワーを選択
						reviewerID = services.SelectRandomReviewer(db, config.SlackChannelID, config.LabelName)
						if err != nil {
							log.Printf("business hours slack message failed (channel: %s): %v", config.SlackChannelID, err)
							// エラー時はタスクを削除
							db.Delete(&tempTask)
							return
						}
						log.Printf("business hours message sent: ts=%s, channel=%s", slackTs, slackChannelID)
					}

					// タスクを正式な状態に更新
					tempTask.SlackTS = slackTs
					tempTask.Reviewer = reviewerID
					tempTask.Status = taskStatus
					tempTask.UpdatedAt = time.Now()

					if err := db.Save(&tempTask).Error; err != nil {
						log.Printf("task update failed (channel: %s): %v", config.SlackChannelID, err)
						return
					}

					log.Printf("pr registered (channel: %s): %s", config.SlackChannelID, tempTask.PRURL)

					// 営業時間内で、レビュワーが割り当てられた場合のみスレッドに通知
					if taskStatus == "in_review" && reviewerID != "" {
						if err := services.PostReviewerAssignedMessageWithChangeButton(tempTask); err != nil {
							log.Printf("reviewer assigned notification error: %v", err)
						}
					}
				}()

				return nil
			})

			// 成功した場合はループから抜ける
			if txErr == nil {
				break
			}

			// データベースロックエラーの場合はリトライ
			if strings.Contains(txErr.Error(), "database is locked") && retry < maxRetries-1 {
				delay := baseDelay * time.Duration(1<<retry) // 指数バックオフ
				log.Printf("database lock detected for channel %s, retrying in %v (attempt %d/%d)",
					config.SlackChannelID, delay, retry+1, maxRetries)
				time.Sleep(delay)
				continue
			}

			// その他のエラーまたは最大リトライ回数に達した場合
			break
		}

		if txErr != nil {
			log.Printf("transaction failed for channel %s after %d retries: %v", config.SlackChannelID, maxRetries, txErr)
			continue
		}

		if taskCreated {
			notified = true
		}
	}

	if !notified {
		log.Println("no matching channel")
		c.JSON(http.StatusOK, gin.H{"message": "no matching channel"})
		return
	}
}

func handleUnlabeledEvent(c *gin.Context, db *gorm.DB, e *github.PullRequestEvent) {
	pr := e.PullRequest
	repo := e.Repo
	repoFullName := fmt.Sprintf("%s/%s", repo.GetOwner().GetLogin(), repo.GetName())

	log.Printf("handling unlabeled event: repo=%s, pr=%d", repoFullName, pr.GetNumber())

	// 該当するPRの全てのアクティブタスクを検索
	var tasks []models.ReviewTask
	db.Where("repo = ? AND pr_number = ? AND status IN (?)",
		repoFullName, pr.GetNumber(), []string{"pending", "in_review", "snoozed", "waiting_business_hours"}).Find(&tasks)

	if len(tasks) == 0 {
		log.Printf("no active tasks found for unlabeled event: repo=%s, pr=%d", repoFullName, pr.GetNumber())
		return
	}

	// チャンネル設定を取得して、ラベル条件をチェック
	var configs []models.ChannelConfig
	db.Where("is_active = ?", true).Find(&configs)

	for _, task := range tasks {
		// このタスクに対応する設定を探す
		var matchingConfig *models.ChannelConfig
		for _, config := range configs {
			if config.SlackChannelID == task.SlackChannel && config.LabelName == task.LabelName {
				matchingConfig = &config
				break
			}
		}

		if matchingConfig == nil {
			log.Printf("no matching config found for task: %s", task.ID)
			continue
		}

		// PRの現在のラベル状態で、このタスクの条件を満たすかチェック
		if !services.IsLabelMatched(matchingConfig, pr.Labels) {
			log.Printf("label conditions no longer met for task: %s", task.ID)

			// 削除されたラベルを特定
			missingLabels := services.GetMissingLabels(matchingConfig, pr.Labels)

			// Slackメッセージを更新してタスク完了を通知
			if err := services.UpdateSlackMessageForCompletedTask(task); err != nil {
				log.Printf("failed to update slack message for completed task: %v", err)
				continue
			}

			// ラベル削除による完了をスレッドに通知
			if err := services.PostLabelRemovedNotification(task, missingLabels); err != nil {
				log.Printf("failed to post label removed notification: %v", err)
				// 通知失敗してもタスクは完了状態にする
			}

			// タスクのステータスを完了に更新
			task.Status = "completed"
			task.UpdatedAt = time.Now()
			if err := db.Save(&task).Error; err != nil {
				log.Printf("failed to update task status to completed: %v", err)
				continue
			}

			log.Printf("task completed due to unlabeled event: id=%s, repo=%s, pr=%d", task.ID, repoFullName, pr.GetNumber())
		} else {
			log.Printf("label conditions still met for task: %s, continuing", task.ID)
		}
	}
}


// レビューが提出された際の処理
func handleReviewSubmittedEvent(c *gin.Context, db *gorm.DB, e *github.PullRequestReviewEvent) {
	pr := e.PullRequest
	repo := e.Repo
	review := e.Review
	repoFullName := fmt.Sprintf("%s/%s", repo.GetOwner().GetLogin(), repo.GetName())

	log.Printf("handling review submitted event: repo=%s, pr=%d, reviewer=%s, state=%s",
		repoFullName, pr.GetNumber(), review.GetUser().GetLogin(), review.GetState())

	// レビューが「承認」「変更要求」「コメント」のいずれかの場合のみ処理
	reviewState := review.GetState()
	if reviewState != "approved" && reviewState != "changes_requested" && reviewState != "commented" {
		log.Printf("review state %s is not handled", reviewState)
		return
	}

	// 該当するタスクを検索（completed状態も含める）
	var tasks []models.ReviewTask
	result := db.Where("repo = ? AND pr_number = ? AND status IN ?",
		repoFullName, pr.GetNumber(), []string{"in_review", "pending", "waiting_business_hours", "completed"}).
		Order("created_at DESC").
		Find(&tasks)

	if result.Error != nil {
		log.Printf("review submitted task search error: %v", result.Error)
		return
	}

	if len(tasks) == 0 {
		log.Printf("no active tasks found for review submitted event: repo=%s, pr=%d", repoFullName, pr.GetNumber())
		return
	}

	// チャンネルごとに最新のタスクのみを抽出
	channelLatestTasks := make(map[string]models.ReviewTask)
	for _, task := range tasks {
		// すでに同じチャンネルのタスクが存在するかチェック
		if _, exists := channelLatestTasks[task.SlackChannel]; !exists {
			// created_at DESCでソート済みなので、最初に見つかったものが最新
			channelLatestTasks[task.SlackChannel] = task
		}
	}

	// 各チャンネルの最新タスクについて完了通知を送信
	for channel, latestTask := range channelLatestTasks {
		// レビュー完了通知をスレッドに投稿（最新タスクのみ）
		if err := services.SendReviewCompletedAutoNotification(latestTask, review.GetUser().GetLogin(), reviewState); err != nil {
			log.Printf("failed to send review completed notification: %v", err)
			// チャンネル関連のエラー（アーカイブ済み、権限なしなど）の場合はタスクを完了にする
			if !services.IsChannelRelatedError(err) {
				continue
			}
		}

		// 同一チャンネル・同一PRの全タスクをcompletedに更新（リマインド防止）
		var channelTasks []models.ReviewTask
		db.Where("repo = ? AND pr_number = ? AND slack_channel = ? AND status IN ?",
			repoFullName, pr.GetNumber(), channel,
			[]string{"in_review", "pending", "waiting_business_hours", "completed"}).Find(&channelTasks)

		for _, task := range channelTasks {
			if task.Status != "completed" {
				task.Status = "completed"
				task.UpdatedAt = time.Now()
				if err := db.Save(&task).Error; err != nil {
					log.Printf("failed to update task status to completed: %v", err)
					continue
				}
				if task.ID == latestTask.ID {
					log.Printf("task auto-completed due to review: id=%s, repo=%s, pr=%d, reviewer=%s",
						task.ID, repoFullName, pr.GetNumber(), review.GetUser().GetLogin())
				} else {
					log.Printf("old task also completed to prevent reminder: id=%s, repo=%s, pr=%d",
						task.ID, repoFullName, pr.GetNumber())
				}
			} else {
				if task.ID == latestTask.ID {
					log.Printf("additional review notification sent for completed task: id=%s, repo=%s, pr=%d, reviewer=%s",
						task.ID, repoFullName, pr.GetNumber(), review.GetUser().GetLogin())
				}
			}
		}
	}
}
