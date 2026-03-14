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

	// Get all channel configs
	var configs []models.ChannelConfig
	db.Where("is_active = ?", true).Find(&configs)

	if len(configs) == 0 {
		log.Println("no active channel config found")
		c.JSON(http.StatusOK, gin.H{"message": "no active channel config"})
		return
	}

	notified := false

	for _, config := range configs {
		// Check if channel is archived
		isArchived, checkErr := services.IsChannelArchived(config.SlackChannelID)
		if checkErr != nil {
			log.Printf("channel status check error (channel: %s): %v", config.SlackChannelID, checkErr)
		}

		if isArchived {
			log.Printf("channel %s is archived. skip", config.SlackChannelID)

			// Deactivate the archived channel's config
			config.IsActive = false
			config.UpdatedAt = time.Now()
			if err := db.Save(&config).Error; err != nil {
				log.Printf("channel config update error: %v", err)
			} else {
				log.Printf("archived channel %s config is deactivated", config.SlackChannelID)
			}
			continue
		}

		// Check repository filter if configured
		if !services.IsRepositoryWatched(&config, repoFullName) {
			log.Printf("repository %s is not watched (channel: %s, config: %s)",
				repoFullName, config.SlackChannelID, config.RepositoryList)
			continue
		}

		// Check if the added label is relevant to the config
		if !services.IsAddedLabelRelevant(&config, addedLabelName) {
			log.Printf("added label '%s' is not relevant to config (channel: %s, config: %s)",
				addedLabelName, config.SlackChannelID, config.LabelName)
			continue
		}

		// Check labels (supports multiple labels)
		if !services.IsLabelMatched(&config, pr.Labels) {
			log.Printf("label requirements not met (channel: %s, config: %s)",
				config.SlackChannelID, config.LabelName)
			continue
		}

		// Transaction with retry handling
		const maxRetries = 3
		const baseDelay = 100 * time.Millisecond

		var taskCreated bool
		var txErr error
		var processTask func()

		for retry := 0; retry < maxRetries; retry++ {
			txErr = db.Transaction(func(tx *gorm.DB) error {
				// Check for existing active tasks (only create one per channel and PR)
				var existingTask models.ReviewTask
				existingErr := tx.Where("repo = ? AND pr_number = ? AND slack_channel = ? AND status IN (?)",
					repoFullName, pr.GetNumber(), config.SlackChannelID,
					[]string{"pending", "in_review", "snoozed", "waiting_business_hours"}).
					First(&existingTask).Error

				if existingErr == nil {
					// Skip if an active task already exists (regardless of label name)
					log.Printf("active task already exists for PR %d in channel %s (existing label: %s, current config: %s), skipping",
						pr.GetNumber(), config.SlackChannelID, existingTask.LabelName, config.LabelName)
					taskCreated = false
					return nil // End transaction normally and skip
				}

				// First create a temporary task record (before sending Slack message)
				tempTask := models.ReviewTask{
					ID:           uuid.NewString(),
					PRURL:        pr.GetHTMLURL(),
					Repo:         repoFullName,
					PRNumber:     pr.GetNumber(),
					Title:        pr.GetTitle(),
					SlackTS:      "", // Updated later
					SlackChannel: config.SlackChannelID,
					Reviewer:     "",        // Updated later
					Status:       "pending", // Temporary state
					LabelName:    config.LabelName,
					CreatedAt:    time.Now(),
					UpdatedAt:    time.Now(),
				}

				if err := tx.Create(&tempTask).Error; err != nil {
					log.Printf("temp task insert failed (channel: %s): %v", config.SlackChannelID, err)
					return err
				}

				taskCreated = true

				// Send Slack message and update task outside the transaction
				processTask = func() {
					var slackTs, slackChannelID string
					var taskStatus string
					var reviewerID string
					var reviewersStr string

					// Check if outside business hours
					creatorGithubUsername := pr.GetUser().GetLogin()
					creatorSlackID := services.GetSlackUserIDFromGitHub(db, creatorGithubUsername)
					if creatorSlackID != "" {
						log.Printf("PR creator slack ID found: github=%s, slack=%s", creatorGithubUsername, creatorSlackID)
					}

					if !services.IsWithinBusinessHours(&config, time.Now()) {
						// Outside business hours: send message without mention
						var err error
						slackTs, slackChannelID, err = services.SendSlackMessageOffHours(
							pr.GetHTMLURL(),
							pr.GetTitle(),
							config.SlackChannelID,
							creatorSlackID,
						)
						taskStatus = "waiting_business_hours"
						// Reviewer will be set on the next business day morning
						reviewerID = ""
						if err != nil {
							log.Printf("off-hours slack message failed (channel: %s): %v", config.SlackChannelID, err)
							// Delete task on error
							db.Delete(&tempTask)
							return
						}
						log.Printf("off-hours message sent: ts=%s, channel=%s", slackTs, slackChannelID)
					} else {
						// During business hours: send message with mention
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

						// Add PR author to exclusion ID list
						excludeIDs := []string{}
						if creatorSlackID != "" {
							excludeIDs = append(excludeIDs, creatorSlackID)
						}

						// Get required number of approvals
						requiredApprovals := config.RequiredApprovals
						if requiredApprovals <= 0 {
							requiredApprovals = 1
						}

						// Select reviewers
						reviewerIDs := services.SelectRandomReviewers(db, config.SlackChannelID, config.LabelName, requiredApprovals, excludeIDs)
						if len(reviewerIDs) > 0 {
							reviewerID = reviewerIDs[0]
						}
						reviewersStr = strings.Join(reviewerIDs, ",")
					}

					// Update task to its final state
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

					// Also update local object
					tempTask.SlackTS = slackTs
					tempTask.Reviewer = reviewerID
					tempTask.Reviewers = reviewersStr
					tempTask.PRAuthorSlackID = creatorSlackID
					tempTask.Status = taskStatus
					tempTask.UpdatedAt = time.Now()

					log.Printf("pr registered (channel: %s): %s", config.SlackChannelID, tempTask.PRURL)

					// Only notify in thread during business hours when a reviewer is assigned
					if taskStatus == "in_review" && reviewerID != "" {
						if err := services.PostReviewerAssignedMessageWithChangeButton(tempTask); err != nil {
							log.Printf("reviewer assigned notification error: %v", err)
						}
					}
				}

				return nil
			})

			// Break out of loop on success
			if txErr == nil {
				// Execute task update after successful transaction
				if taskCreated {
					// Run synchronously in test mode, asynchronously via goroutine in production
					if services.IsTestMode {
						processTask()
					} else {
						go processTask()
					}
				}
				break
			}

			// Retry on database lock error
			if strings.Contains(txErr.Error(), "database is locked") && retry < maxRetries-1 {
				delay := baseDelay * time.Duration(1<<retry) // Exponential backoff
				log.Printf("database lock detected for channel %s, retrying in %v (attempt %d/%d)",
					config.SlackChannelID, delay, retry+1, maxRetries)
				time.Sleep(delay)
				continue
			}

			// Other errors or max retries reached
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

	// Search for all active tasks for the PR
	var tasks []models.ReviewTask
	db.Where("repo = ? AND pr_number = ? AND status IN (?)",
		repoFullName, pr.GetNumber(), []string{"pending", "in_review", "snoozed", "waiting_business_hours"}).Find(&tasks)

	if len(tasks) == 0 {
		log.Printf("no active tasks found for unlabeled event: repo=%s, pr=%d", repoFullName, pr.GetNumber())
		return
	}

	// Get channel configs and check label conditions
	var configs []models.ChannelConfig
	db.Where("is_active = ?", true).Find(&configs)

	for _, task := range tasks {
		// Find the config corresponding to this task
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

		// Check if the task's conditions are still met with the PR's current label state
		if !services.IsLabelMatched(matchingConfig, pr.Labels) {
			log.Printf("label conditions no longer met for task: %s", task.ID)

			// Identify the removed labels
			missingLabels := services.GetMissingLabels(matchingConfig, pr.Labels)

			// Update Slack message to notify task completion
			if err := services.UpdateSlackMessageForCompletedTask(task); err != nil {
				log.Printf("failed to update slack message for completed task: %v", err)
				continue
			}

			// Notify in thread about completion due to label removal
			if err := services.PostLabelRemovedNotification(task, missingLabels); err != nil {
				log.Printf("failed to post label removed notification: %v", err)
				// Still complete the task even if notification fails
			}

			// Update task status to completed
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

// handleClosedEvent handles the event when a PR is closed
func handleClosedEvent(c *gin.Context, db *gorm.DB, e *github.PullRequestEvent) {
	pr := e.PullRequest
	repo := e.Repo
	repoFullName := fmt.Sprintf("%s/%s", repo.GetOwner().GetLogin(), repo.GetName())

	log.Printf("handling closed event: repo=%s, pr=%d, merged=%v", repoFullName, pr.GetNumber(), pr.GetMerged())

	// Search for all active tasks for the PR
	var tasks []models.ReviewTask
	db.Where("repo = ? AND pr_number = ? AND status IN (?)",
		repoFullName, pr.GetNumber(), []string{"pending", "in_review", "snoozed", "waiting_business_hours"}).Find(&tasks)

	if len(tasks) == 0 {
		log.Printf("no active tasks found for closed event: repo=%s, pr=%d", repoFullName, pr.GetNumber())
		return
	}

	// Execute completion processing for each task
	for _, task := range tasks {
		// Send close notification to Slack
		if err := services.PostPRClosedNotification(task, pr.GetMerged()); err != nil {
			log.Printf("failed to post PR closed notification: %v", err)
			// Still complete the task even if notification fails
		}

		// Update task status to completed
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

// handleReviewRequestedEvent handles the event when GitHub's re-request review button is pressed
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

	// Search for completed or in_review tasks
	var tasks []models.ReviewTask
	if err := db.Where("repo = ? AND pr_number = ? AND status IN ?",
		repoFullName, pr.GetNumber(), []string{"completed", "in_review", "pending", "snoozed", "waiting_business_hours"}).
		Order("created_at DESC").
		Find(&tasks).Error; err != nil {
		log.Printf("review_requested task search error: %v", err)
		return
	}

	if len(tasks) == 0 {
		log.Printf("no active tasks found for review_requested event: repo=%s, pr=%d", repoFullName, pr.GetNumber())
		return
	}

	// Extract only the latest task per channel
	channelLatestTasks := make(map[string]models.ReviewTask)
	for _, task := range tasks {
		if _, exists := channelLatestTasks[task.SlackChannel]; !exists {
			channelLatestTasks[task.SlackChannel] = task
		}
	}

	senderSlackID := services.GetSlackUserIDFromGitHub(db, senderLogin)
	reviewerSlackID := services.GetSlackUserIDFromGitHub(db, reviewerLogin)

	// Build mention strings
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

	for _, latestTask := range channelLatestTasks {
		// Revert completed tasks to in_review
		if latestTask.Status == "completed" {
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
		}

		// Post re-review request notification to thread
		message := fmt.Sprintf("🔄 %s さんが %s に再レビューを依頼しました。対応をお願いします！",
			senderMention, reviewerMention)
		if err := services.PostToThread(latestTask.SlackChannel, latestTask.SlackTS, message); err != nil {
			log.Printf("re-review notification error: %v", err)
		}
	}
}

// handleReviewSubmittedEvent handles the event when a review is submitted
func handleReviewSubmittedEvent(c *gin.Context, db *gorm.DB, e *github.PullRequestReviewEvent) {
	pr := e.PullRequest
	repo := e.Repo
	review := e.Review
	repoFullName := fmt.Sprintf("%s/%s", repo.GetOwner().GetLogin(), repo.GetName())

	log.Printf("handling review submitted event: repo=%s, pr=%d, reviewer=%s, state=%s",
		repoFullName, pr.GetNumber(), review.GetUser().GetLogin(), review.GetState())

	// Skip notification if PR is closed
	if pr.GetState() == "closed" {
		log.Printf("PR is closed, skipping notification: repo=%s, pr=%d", repoFullName, pr.GetNumber())
		return
	}

	// Only process if the review is "approved", "changes_requested", or "commented"
	reviewState := review.GetState()
	if reviewState != "approved" && reviewState != "changes_requested" && reviewState != "commented" && reviewState != "dismissed" {
		log.Printf("review state %s is not handled", reviewState)
		return
	}

	// Search for matching tasks (including completed status)
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

	// Extract only the latest task per channel
	channelLatestTasks := make(map[string]models.ReviewTask)
	for _, task := range tasks {
		// Check if a task for this channel already exists
		if _, exists := channelLatestTasks[task.SlackChannel]; !exists {
			// Since sorted by created_at DESC, the first found is the latest
			channelLatestTasks[task.SlackChannel] = task
		}
	}

	// Get Slack ID from reviewer's GitHub username
	reviewerSlackID := services.GetSlackUserIDFromGitHub(db, review.GetUser().GetLogin())

	// Send notifications for the latest task in each channel
	for channel, latestTask := range channelLatestTasks {
		// Get RequiredApprovals
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
			// Only remove the dismissed reviewer from approved_by
			// No status change or notification (handled via re-request)
			oldApprovedBy := latestTask.ApprovedBy
			if services.RemoveApproval(&latestTask, approvalID) {
				result := db.Model(&models.ReviewTask{}).
					Where("id = ? AND approved_by = ?", latestTask.ID, oldApprovedBy).
					Updates(map[string]interface{}{
						"approved_by": latestTask.ApprovedBy,
						"updated_at":  time.Now(),
					})
				if result.Error != nil {
					log.Printf("failed to update approved_by on dismiss: %v", result.Error)
				} else if result.RowsAffected == 0 {
					log.Printf("CAS conflict on dismiss update, skipping: id=%s", latestTask.ID)
					continue
				}
				log.Printf("approval dismissed: id=%s, repo=%s, pr=%d, reviewer=%s",
					latestTask.ID, repoFullName, pr.GetNumber(), review.GetUser().GetLogin())
			}

		case "approved":
			// Post review completion notification to thread
			if err := services.SendReviewCompletedAutoNotification(latestTask, review.GetUser().GetLogin(), reviewState); err != nil {
				log.Printf("failed to send review completed notification: %v", err)
				if !services.IsChannelRelatedError(err) {
					continue
				}
			}

			// Approval tracking: use CAS-like WHERE clause to prevent concurrent approval conflicts
			oldApprovedBy := latestTask.ApprovedBy
			services.AddApproval(&latestTask, approvalID)

			// Determine if all required approvals are met
			fullyApproved := services.IsReviewFullyApproved(latestTask, requiredApprovals)

			if fullyApproved {
				// Post review complete message to thread
				approvedCount := services.CountApprovals(latestTask)
				completeMsg := fmt.Sprintf("🎉 %d/%d approved - レビュー完了！", approvedCount, requiredApprovals)
				if err := services.PostToThread(latestTask.SlackChannel, latestTask.SlackTS, completeMsg); err != nil {
					log.Printf("failed to post review complete message: %v", err)
				}

				// All approvals met -> mark all tasks as completed
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
				// Partial approval: update approved_by with CAS-like WHERE clause (concurrent approval protection)
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

				// Post progress message to thread
				approvedCount := services.CountApprovals(latestTask)
				progressMsg := fmt.Sprintf("✅ %d/%d approved", approvedCount, requiredApprovals)
				if err := services.PostToThread(latestTask.SlackChannel, latestTask.SlackTS, progressMsg); err != nil {
					log.Printf("failed to post approval progress: %v", err)
				}

				log.Printf("partial approval for task: id=%s, approved=%d/%d",
					latestTask.ID, approvedCount, requiredApprovals)
			}

		default:
			// changes_requested, commented, etc.
			if err := services.SendReviewCompletedAutoNotification(latestTask, review.GetUser().GetLogin(), reviewState); err != nil {
				log.Printf("failed to send review completed notification: %v", err)
				if !services.IsChannelRelatedError(err) {
					continue
				}
			}

			// Update all tasks for the same channel and PR to completed (to prevent reminders)
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
