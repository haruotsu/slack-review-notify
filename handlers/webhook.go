package handlers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/go-github/v71/github"
	"github.com/google/uuid"
	"slack-review-notify/models"
	"slack-review-notify/services"
	"gorm.io/gorm"
)

func HandleGitHubWebhook(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		payload, err := github.ValidatePayload(c.Request, []byte(os.Getenv("GITHUB_WEBHOOK_SECRET")))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}

		event, err := github.ParseWebHook(github.WebHookType(c.Request), payload)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "cannot parse webhook"})
			return
		}

		switch e := event.(type) {
		case *github.PullRequestEvent:
			if e.Action != nil && e.Label != nil {
				switch *e.Action {
				case "labeled":
					handleLabeledEvent(c, db, e)
				case "unlabeled":
					handleUnlabeledEvent(c, db, e)
				}
			}
		case *github.PullRequestReviewEvent:
			if e.Action != nil && *e.Action == "submitted" {
				handleReviewSubmittedEvent(c, db, e)
			}
		case *github.PullRequestReviewCommentEvent:
			if e.Action != nil && *e.Action == "created" {
				handleReviewCommentCreatedEvent(c, db, e)
			}
		}

		c.Status(http.StatusOK)
	}
}

func handleLabeledEvent(c *gin.Context, db *gorm.DB, e *github.PullRequestEvent) {
	pr := e.PullRequest
	repo := e.Repo
	repoFullName := fmt.Sprintf("%s/%s", repo.GetOwner().GetLogin(), repo.GetName())
	labelName := e.Label.GetName()

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
		isArchived, err := services.IsChannelArchived(config.SlackChannelID)
		if err != nil {
			log.Printf("channel status check error (channel: %s): %v", config.SlackChannelID, err)
		}

		if isArchived {
			log.Printf("channel %s is archived. skip", config.SlackChannelID)

			// アーカイブされたチャンネルの設定を非アクティブに更新
			config.IsActive = false
			config.UpdatedAt = time.Now()
			if err := db.Save(&config).Error; err != nil {
				log.Printf("channel config update error: %v", err)
			} else {
				log.Printf("✅ archived channel %s config is deactivated", config.SlackChannelID)
			}
			continue
		}

		// リポジトリフィルタがある場合はチェック
		if !services.IsRepositoryWatched(&config, repoFullName) {
			log.Printf("repository %s is not watched (channel: %s, config: %s)",
				repoFullName, config.SlackChannelID, config.RepositoryList)
			continue
		}

		// ラベルをチェック
		if config.LabelName != "" && config.LabelName != labelName {
			log.Printf("label %s is not watched (channel: %s, config: %s)",
				labelName, config.SlackChannelID, config.LabelName)
			continue
		}

		// ランダムにレビュワーを選択
		randomReviewerID := services.SelectRandomReviewer(db, config.SlackChannelID, config.LabelName)

		// メッセージ送信後のタスク作成部分
		slackTs, slackChannelID, err := services.SendSlackMessage(
			pr.GetHTMLURL(),
			pr.GetTitle(),
			config.SlackChannelID,
			config.DefaultMentionID,
		)

		if err != nil {
			log.Printf("slack message failed (channel: %s): %v", config.SlackChannelID, err)
			continue
		}

		log.Printf("slack message sent: ts=%s, channel=%s", slackTs, slackChannelID)

		// ランダムにレビュワーを選択してタスクに設定
		task := models.ReviewTask{
			ID:           uuid.NewString(),
			PRURL:        pr.GetHTMLURL(),
			Repo:         repoFullName,
			PRNumber:     pr.GetNumber(),
			Title:        pr.GetTitle(),
			SlackTS:      slackTs,
			SlackChannel: slackChannelID,
			Reviewer:     randomReviewerID,
			Status:       "in_review",
			LabelName:    config.LabelName,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}

		if err := db.Create(&task).Error; err != nil {
			log.Printf("db insert failed (channel: %s): %v", config.SlackChannelID, err)
			continue
		}

		log.Printf("pr registered (channel: %s): %s", config.SlackChannelID, task.PRURL)
		notified = true

		// レビュワーが割り当てられたことをスレッドに通知
		if randomReviewerID != "" {
			if err := services.PostReviewerAssignedMessageWithChangeButton(task); err != nil {
				log.Printf("reviewer assigned notification error: %v", err)
			}
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
	labelName := e.Label.GetName()

	log.Printf("handling unlabeled event: repo=%s, pr=%d, label=%s", repoFullName, pr.GetNumber(), labelName)

	// 該当するレビュータスクを検索
	var tasks []models.ReviewTask
	db.Where("repo = ? AND pr_number = ? AND label_name = ? AND status IN (?)", 
		repoFullName, pr.GetNumber(), labelName, []string{"pending", "in_review", "snoozed"}).Find(&tasks)

	if len(tasks) == 0 {
		log.Printf("no active tasks found for unlabeled event: repo=%s, pr=%d, label=%s", repoFullName, pr.GetNumber(), labelName)
		return
	}

	for _, task := range tasks {
		// Slackメッセージを更新してタスク完了を通知
		if err := services.UpdateSlackMessageForCompletedTask(task); err != nil {
			log.Printf("failed to update slack message for completed task: %v", err)
			continue
		}

		// タスクのステータスを完了に更新
		task.Status = "completed"
		task.UpdatedAt = time.Now()
		if err := db.Save(&task).Error; err != nil {
			log.Printf("failed to update task status to completed: %v", err)
			continue
		}

		log.Printf("task completed due to unlabeled event: id=%s, repo=%s, pr=%d", task.ID, repoFullName, pr.GetNumber())
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

	// 該当するタスクを検索
	var tasks []models.ReviewTask
	result := db.Where("repo = ? AND pr_number = ? AND status IN ?",
		repoFullName, pr.GetNumber(), []string{"in_review", "pending"}).Find(&tasks)

	if result.Error != nil {
		log.Printf("review submitted task search error: %v", result.Error)
		return
	}

	if len(tasks) == 0 {
		log.Printf("no active tasks found for review submitted event: repo=%s, pr=%d", repoFullName, pr.GetNumber())
		return
	}

	// 各タスクについて完了通知を送信
	for _, task := range tasks {
		// レビュー完了通知をスレッドに投稿
		if err := services.SendReviewCompletedAutoNotification(task, review.GetUser().GetLogin(), reviewState); err != nil {
			log.Printf("failed to send review completed notification: %v", err)
			// チャンネル関連のエラー（アーカイブ済み、権限なしなど）の場合はタスクを完了にする
			if !services.IsChannelRelatedError(err) {
				continue
			}
		}

		// タスクのステータスを完了に更新
		task.Status = "completed"
		task.UpdatedAt = time.Now()
		if err := db.Save(&task).Error; err != nil {
			log.Printf("failed to update task status to completed: %v", err)
			continue
		}

		log.Printf("task auto-completed due to review: id=%s, repo=%s, pr=%d, reviewer=%s",
			task.ID, repoFullName, pr.GetNumber(), review.GetUser().GetLogin())
	}
}

// レビューコメントが作成された際の処理
func handleReviewCommentCreatedEvent(c *gin.Context, db *gorm.DB, e *github.PullRequestReviewCommentEvent) {
	pr := e.PullRequest
	repo := e.Repo
	comment := e.Comment
	repoFullName := fmt.Sprintf("%s/%s", repo.GetOwner().GetLogin(), repo.GetName())

	log.Printf("handling review comment created event: repo=%s, pr=%d, commenter=%s",
		repoFullName, pr.GetNumber(), comment.GetUser().GetLogin())

	// 該当するタスクを検索
	var tasks []models.ReviewTask
	result := db.Where("repo = ? AND pr_number = ? AND status IN ?",
		repoFullName, pr.GetNumber(), []string{"in_review", "pending"}).Find(&tasks)

	if result.Error != nil {
		log.Printf("review comment task search error: %v", result.Error)
		return
	}

	if len(tasks) == 0 {
		log.Printf("no active tasks found for review comment event: repo=%s, pr=%d", repoFullName, pr.GetNumber())
		return
	}

	// 各タスクについて完了通知を送信
	for _, task := range tasks {
		// レビューコメント完了通知をスレッドに投稿
		if err := services.SendReviewCompletedAutoNotification(task, comment.GetUser().GetLogin(), "commented"); err != nil {
			log.Printf("failed to send review comment notification: %v", err)
			// チャンネル関連のエラー（アーカイブ済み、権限なしなど）の場合はタスクを完了にする
			if !services.IsChannelRelatedError(err) {
				continue
			}
		}

		// タスクのステータスを完了に更新
		task.Status = "completed"
		task.UpdatedAt = time.Now()
		if err := db.Save(&task).Error; err != nil {
			log.Printf("failed to update task status to completed: %v", err)
			continue
		}

		log.Printf("task auto-completed due to review comment: id=%s, repo=%s, pr=%d, commenter=%s",
			task.ID, repoFullName, pr.GetNumber(), comment.GetUser().GetLogin())
	}
}