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
			if e.Action != nil {
				switch *e.Action {
				case "labeled":
					if e.Label != nil {
						handleLabeledEvent(c, db, e)
					}
				case "unlabeled":
					if e.Label != nil {
						handleUnlabeledEvent(c, db, e)
					}
				case "closed":
					handleClosedEvent(c, db, e)
				case "review_requested":
					handleReviewRequestedEvent(c, db, e)
				}
			}
		case *github.PullRequestReviewEvent:
			log.Printf("PullRequestReviewEvent received: action=%s", e.GetAction())
			if e.Action != nil && (*e.Action == "submitted" || *e.Action == "dismissed") {
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
		var processTask func()

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
					Reviewer:     "",        // 後で更新
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
				processTask = func() {
					var slackTs, slackChannelID string
					var taskStatus string
					var reviewerID string
					var reviewersStr string

					// 営業時間外判定
					creatorGithubUsername := pr.GetUser().GetLogin()
					creatorSlackID := services.GetSlackUserIDFromGitHub(db, creatorGithubUsername)
					if creatorSlackID != "" {
						log.Printf("PR creator slack ID found: github=%s, slack=%s", creatorGithubUsername, creatorSlackID)
					}

					if !services.IsWithinBusinessHours(&config, time.Now()) {
						// 営業時間外の場合：メンション抜きメッセージを送信
						var err error
						slackTs, slackChannelID, err = services.SendSlackMessageOffHours(
							pr.GetHTMLURL(),
							pr.GetTitle(),
							config.SlackChannelID,
							creatorSlackID,
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
							creatorSlackID,
						)
						taskStatus = "in_review"
						if err != nil {
							log.Printf("business hours slack message failed (channel: %s): %v", config.SlackChannelID, err)
							db.Delete(&tempTask)
							return
						}
						log.Printf("business hours message sent: ts=%s, channel=%s", slackTs, slackChannelID)

						// PR作成者を除外IDリストに追加
						excludeIDs := []string{}
						if creatorSlackID != "" {
							excludeIDs = append(excludeIDs, creatorSlackID)
						}

						// 必要なapprove数を取得
						requiredApprovals := config.RequiredApprovals
						if requiredApprovals <= 0 {
							requiredApprovals = 1
						}

						// レビュワー選択
						reviewerIDs := services.SelectRandomReviewers(db, config.SlackChannelID, config.LabelName, requiredApprovals, excludeIDs)
						if len(reviewerIDs) > 0 {
							reviewerID = reviewerIDs[0]
						}
						reviewersStr = strings.Join(reviewerIDs, ",")
					}

					// タスクを正式な状態に更新
					updates := map[string]interface{}{
						"slack_ts":          slackTs,
						"reviewer":          reviewerID,
						"reviewers":         reviewersStr,
						"pr_author_slack_id": creatorSlackID,
						"status":            taskStatus,
						"updated_at":        time.Now(),
					}

					if err := db.Model(&models.ReviewTask{}).Where("id = ?", tempTask.ID).Updates(updates).Error; err != nil {
						log.Printf("task update failed (channel: %s): %v", config.SlackChannelID, err)
						return
					}

					// ローカルオブジェクトも更新
					tempTask.SlackTS = slackTs
					tempTask.Reviewer = reviewerID
					tempTask.Reviewers = reviewersStr
					tempTask.PRAuthorSlackID = creatorSlackID
					tempTask.Status = taskStatus
					tempTask.UpdatedAt = time.Now()

					log.Printf("pr registered (channel: %s): %s", config.SlackChannelID, tempTask.PRURL)

					// 営業時間内で、レビュワーが割り当てられた場合のみスレッドに通知
					if taskStatus == "in_review" && reviewerID != "" {
						if err := services.PostReviewerAssignedMessageWithChangeButton(tempTask); err != nil {
							log.Printf("reviewer assigned notification error: %v", err)
						}
					}
				}

				return nil
			})

			// 成功した場合はループから抜ける
			if txErr == nil {
				// トランザクション成功後にタスク更新処理を実行
				if taskCreated {
					// テストモードでは同期実行、本番ではgoroutineで非同期実行
					if services.IsTestMode {
						processTask()
					} else {
						go processTask()
					}
				}
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

// PRがcloseされた際の処理
func handleClosedEvent(c *gin.Context, db *gorm.DB, e *github.PullRequestEvent) {
	pr := e.PullRequest
	repo := e.Repo
	repoFullName := fmt.Sprintf("%s/%s", repo.GetOwner().GetLogin(), repo.GetName())

	log.Printf("handling closed event: repo=%s, pr=%d, merged=%v", repoFullName, pr.GetNumber(), pr.GetMerged())

	// 該当するPRの全てのアクティブタスクを検索
	var tasks []models.ReviewTask
	db.Where("repo = ? AND pr_number = ? AND status IN (?)",
		repoFullName, pr.GetNumber(), []string{"pending", "in_review", "snoozed", "waiting_business_hours"}).Find(&tasks)

	if len(tasks) == 0 {
		log.Printf("no active tasks found for closed event: repo=%s, pr=%d", repoFullName, pr.GetNumber())
		return
	}

	// 各タスクについて完了処理を実行
	for _, task := range tasks {
		// Slackにクローズ通知を送信
		if err := services.PostPRClosedNotification(task, pr.GetMerged()); err != nil {
			log.Printf("failed to post PR closed notification: %v", err)
			// 通知失敗してもタスクは完了状態にする
		}

		// タスクのステータスを完了に更新
		task.Status = "completed"
		task.UpdatedAt = time.Now()
		if err := db.Save(&task).Error; err != nil {
			log.Printf("failed to update task status to completed: %v", err)
			continue
		}

		log.Printf("task completed due to PR closed: id=%s, repo=%s, pr=%d, merged=%v",
			task.ID, repoFullName, pr.GetNumber(), pr.GetMerged())
	}
}

// GitHubのre-request reviewボタンが押された際の処理
func handleReviewRequestedEvent(c *gin.Context, db *gorm.DB, e *github.PullRequestEvent) {
	pr := e.PullRequest
	repo := e.Repo
	repoFullName := fmt.Sprintf("%s/%s", repo.GetOwner().GetLogin(), repo.GetName())

	requestedReviewer := e.GetRequestedReviewer()
	if requestedReviewer == nil {
		log.Printf("review_requested event has no requested reviewer (may be a team request), skipping: repo=%s, pr=%d",
			repoFullName, pr.GetNumber())
		return
	}

	senderLogin := e.GetSender().GetLogin()
	reviewerLogin := requestedReviewer.GetLogin()

	log.Printf("handling review_requested event: repo=%s, pr=%d, sender=%s, requested_reviewer=%s",
		repoFullName, pr.GetNumber(), senderLogin, reviewerLogin)

	// completedタスクを検索
	var tasks []models.ReviewTask
	if err := db.Where("repo = ? AND pr_number = ? AND status = ?",
		repoFullName, pr.GetNumber(), "completed").
		Order("created_at DESC").
		Find(&tasks).Error; err != nil {
		log.Printf("review_requested task search error: %v", err)
		return
	}

	if len(tasks) == 0 {
		log.Printf("no completed tasks found for review_requested event: repo=%s, pr=%d", repoFullName, pr.GetNumber())
		return
	}

	// チャンネルごとに最新のタスクのみを抽出
	channelLatestTasks := make(map[string]models.ReviewTask)
	for _, task := range tasks {
		if _, exists := channelLatestTasks[task.SlackChannel]; !exists {
			channelLatestTasks[task.SlackChannel] = task
		}
	}

	senderSlackID := services.GetSlackUserIDFromGitHub(db, senderLogin)
	reviewerSlackID := services.GetSlackUserIDFromGitHub(db, reviewerLogin)

	for _, latestTask := range channelLatestTasks {
		// タスクをin_reviewに戻す
		result := db.Model(&models.ReviewTask{}).
			Where("id = ? AND status = ?", latestTask.ID, "completed").
			Updates(map[string]interface{}{
				"status":     "in_review",
				"updated_at": time.Now(),
			})
		if result.Error != nil {
			log.Printf("failed to update task to in_review: %v", result.Error)
			continue
		}
		if result.RowsAffected == 0 {
			log.Printf("task already updated (CAS miss): id=%s", latestTask.ID)
			continue
		}

		log.Printf("task reactivated for re-review: id=%s, repo=%s, pr=%d", latestTask.ID, repoFullName, pr.GetNumber())

		// スレッドに再レビュー依頼通知を投稿
		var senderMention string
		if senderSlackID != "" {
			senderMention = fmt.Sprintf("<@%s>", senderSlackID)
		} else {
			senderMention = senderLogin
		}

		var reviewerMention string
		if reviewerSlackID != "" {
			reviewerMention = fmt.Sprintf("<@%s>", reviewerSlackID)
		} else {
			reviewerMention = reviewerLogin
		}

		message := fmt.Sprintf("🔄 %s さんが %s に再レビューを依頼しました。対応をお願いします！",
			senderMention, reviewerMention)
		if err := services.PostToThread(latestTask.SlackChannel, latestTask.SlackTS, message); err != nil {
			log.Printf("re-review notification error: %v", err)
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

	// PRが閉じている場合は通知をスキップ
	if pr.GetState() == "closed" {
		log.Printf("PR is closed, skipping notification: repo=%s, pr=%d", repoFullName, pr.GetNumber())
		return
	}

	// レビューが「承認」「変更要求」「コメント」のいずれかの場合のみ処理
	reviewState := review.GetState()
	if reviewState != "approved" && reviewState != "changes_requested" && reviewState != "commented" && reviewState != "dismissed" {
		log.Printf("review state %s is not handled", reviewState)
		return
	}

	// 該当するタスクを検索（completed状態も含める）
	var tasks []models.ReviewTask
	result := db.Where("repo = ? AND pr_number = ? AND status IN ?",
		repoFullName, pr.GetNumber(), []string{"in_review", "pending", "snoozed", "waiting_business_hours", "completed"}).
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

	// レビュワーのGitHubユーザー名からSlack IDを取得
	reviewerSlackID := services.GetSlackUserIDFromGitHub(db, review.GetUser().GetLogin())

	// 各チャンネルの最新タスクについて通知を送信
	for channel, latestTask := range channelLatestTasks {
		// RequiredApprovalsを取得
		requiredApprovals := 1
		if latestTask.LabelName != "" {
			var config models.ChannelConfig
			if err := db.Where("slack_channel_id = ? AND label_name = ?", latestTask.SlackChannel, latestTask.LabelName).First(&config).Error; err == nil {
				if config.RequiredApprovals > 0 {
					requiredApprovals = config.RequiredApprovals
				}
			}
		}

		approvalID := reviewerSlackID
		if approvalID == "" {
			approvalID = review.GetUser().GetLogin()
		}

		switch reviewState {
		case "dismissed":
			// approveを取り消し、再レビュー依頼通知を送信
			oldApprovedBy := latestTask.ApprovedBy
			if services.RemoveApproval(&latestTask, approvalID) {
				// CAS: approved_byが変更されていない場合のみ更新（同時approve対策）
				result := db.Model(&models.ReviewTask{}).
					Where("id = ? AND approved_by = ?", latestTask.ID, oldApprovedBy).
					Updates(map[string]interface{}{
						"approved_by": latestTask.ApprovedBy,
						"status":      "in_review",
						"updated_at":  time.Now(),
					})
				if result.Error != nil {
					log.Printf("failed to update approved_by on dismiss: %v", result.Error)
				} else if result.RowsAffected == 0 {
					log.Printf("CAS conflict on dismiss update, skipping: id=%s", latestTask.ID)
					continue
				}
				approvedCount := services.CountApprovals(latestTask)
				dismissMsg := fmt.Sprintf("⚠️ %s のapproveが取り消されました（%d/%d approved）。再レビューをお願いします。",
					review.GetUser().GetLogin(), approvedCount, requiredApprovals)
				if err := services.PostToThread(latestTask.SlackChannel, latestTask.SlackTS, dismissMsg); err != nil {
					log.Printf("failed to post dismiss notification: %v", err)
				}
				log.Printf("approval dismissed: id=%s, repo=%s, pr=%d, reviewer=%s",
					latestTask.ID, repoFullName, pr.GetNumber(), review.GetUser().GetLogin())
			}

		case "approved":
			// レビュー完了通知をスレッドに投稿
			if err := services.SendReviewCompletedAutoNotification(latestTask, review.GetUser().GetLogin(), reviewState); err != nil {
				log.Printf("failed to send review completed notification: %v", err)
				if !services.IsChannelRelatedError(err) {
					continue
				}
			}

			// approve追跡: CAS的なwhere句で同時approve時の競合を防止
			oldApprovedBy := latestTask.ApprovedBy
			services.AddApproval(&latestTask, approvalID)

			// 全員approve済みか判定
			fullyApproved := services.IsReviewFullyApproved(latestTask, requiredApprovals)

			if fullyApproved {
				// レビュー完了メッセージをスレッドに投稿
				approvedCount := services.CountApprovals(latestTask)
				completeMsg := fmt.Sprintf("🎉 %d/%d approved - レビュー完了！", approvedCount, requiredApprovals)
				if err := services.PostToThread(latestTask.SlackChannel, latestTask.SlackTS, completeMsg); err != nil {
					log.Printf("failed to post review complete message: %v", err)
				}

				// 全員approve済み → 全タスクをcompletedに
				var channelTasks []models.ReviewTask
				db.Where("repo = ? AND pr_number = ? AND slack_channel = ? AND status IN ?",
					repoFullName, pr.GetNumber(), channel,
					[]string{"in_review", "pending", "snoozed", "waiting_business_hours", "completed"}).Find(&channelTasks)

				for _, task := range channelTasks {
					if task.Status != "completed" {
						task.Status = "completed"
						task.ApprovedBy = latestTask.ApprovedBy
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
			} else {
				// 部分approve: CAS的なwhere句でapproved_byを更新（同時approve対策）
				result := db.Model(&models.ReviewTask{}).
					Where("id = ? AND (approved_by = ? OR approved_by IS NULL OR approved_by = '')", latestTask.ID, oldApprovedBy).
					Updates(map[string]interface{}{
						"approved_by": latestTask.ApprovedBy,
						"updated_at":  time.Now(),
					})
				if result.Error != nil {
					log.Printf("failed to update approved_by: %v", result.Error)
				} else if result.RowsAffected == 0 {
					log.Printf("CAS conflict on approval update, skipping: id=%s", latestTask.ID)
					continue
				}

				// 進捗メッセージをスレッドに投稿
				approvedCount := services.CountApprovals(latestTask)
				progressMsg := fmt.Sprintf("✅ %d/%d approved", approvedCount, requiredApprovals)
				if err := services.PostToThread(latestTask.SlackChannel, latestTask.SlackTS, progressMsg); err != nil {
					log.Printf("failed to post approval progress: %v", err)
				}

				log.Printf("partial approval for task: id=%s, approved=%d/%d",
					latestTask.ID, approvedCount, requiredApprovals)
			}

		default:
			// changes_requested, commented など
			if err := services.SendReviewCompletedAutoNotification(latestTask, review.GetUser().GetLogin(), reviewState); err != nil {
				log.Printf("failed to send review completed notification: %v", err)
				if !services.IsChannelRelatedError(err) {
					continue
				}
			}

			// 同一チャンネル・同一PRの全タスクをcompletedに更新（リマインド防止）
			var channelTasks []models.ReviewTask
			db.Where("repo = ? AND pr_number = ? AND slack_channel = ? AND status IN ?",
				repoFullName, pr.GetNumber(), channel,
				[]string{"in_review", "pending", "snoozed", "waiting_business_hours"}).Find(&channelTasks)

			for _, task := range channelTasks {
				task.Status = "completed"
				task.UpdatedAt = time.Now()
				if err := db.Save(&task).Error; err != nil {
					log.Printf("failed to update task status to completed: %v", err)
					continue
				}
				log.Printf("task completed due to review (%s): id=%s, repo=%s, pr=%d, reviewer=%s",
					reviewState, task.ID, repoFullName, pr.GetNumber(), review.GetUser().GetLogin())
			}
		}
	}
}
