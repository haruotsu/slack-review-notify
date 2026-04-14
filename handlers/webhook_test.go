package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"slack-review-notify/models"
	"slack-review-notify/services"

	"github.com/gin-gonic/gin"
	"github.com/google/go-github/v71/github"
	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
)

func TestUnlabeledEventWithExistingTask(t *testing.T) {
	// Setup
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	// Create channel config
	config := models.ChannelConfig{
		SlackChannelID:   "C1234567890",
		LabelName:        "needs-review",
		DefaultMentionID: "@here",
		RepositoryList:   "",
		IsActive:         true,
	}
	db.Create(&config)

	// Create an existing review task
	task := models.ReviewTask{
		ID:           "test-task-id",
		PRURL:        "https://github.com/test/repo/pull/123",
		Repo:         "test/repo",
		PRNumber:     123,
		Title:        "Test PR",
		SlackTS:      "1234567890.123456",
		SlackChannel: "C1234567890",
		Status:       "in_review",
		LabelName:    "needs-review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// Create unlabeled event payload
	prNumber := 123
	repoName := "repo"
	ownerLogin := "test"
	prTitle := "Test PR"
	prHTMLURL := "https://github.com/test/repo/pull/123"
	labelName := "needs-review"
	action := "unlabeled"

	payload := github.PullRequestEvent{
		Action: &action,
		Number: &prNumber,
		Label: &github.Label{
			Name: &labelName,
		},
		PullRequest: &github.PullRequest{
			Number:  &prNumber,
			Title:   &prTitle,
			HTMLURL: &prHTMLURL,
		},
		Repo: &github.Repository{
			Name: &repoName,
			Owner: &github.User{
				Login: &ownerLogin,
			},
		},
	}

	// Test the handler
	router := gin.New()
	router.POST("/webhook", HandleGitHubWebhook(db))

	jsonPayload, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(jsonPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify task was updated to completed
	var updatedTask models.ReviewTask
	db.First(&updatedTask, "id = ?", "test-task-id")
	assert.Equal(t, "completed", updatedTask.Status)
}

func TestUnlabeledEventWithoutExistingTask(t *testing.T) {
	// Setup
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	// Create unlabeled event payload (no corresponding task)
	prNumber := 456
	repoName := "repo"
	ownerLogin := "test"
	prTitle := "Another Test PR"
	prHTMLURL := "https://github.com/test/repo/pull/456"
	labelName := "needs-review"
	action := "unlabeled"

	payload := github.PullRequestEvent{
		Action: &action,
		Number: &prNumber,
		Label: &github.Label{
			Name: &labelName,
		},
		PullRequest: &github.PullRequest{
			Number:  &prNumber,
			Title:   &prTitle,
			HTMLURL: &prHTMLURL,
		},
		Repo: &github.Repository{
			Name: &repoName,
			Owner: &github.User{
				Login: &ownerLogin,
			},
		},
	}

	// Test the handler
	router := gin.New()
	router.POST("/webhook", HandleGitHubWebhook(db))

	jsonPayload, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(jsonPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assertions - verify no errors occur
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify no task was created
	var taskCount int64
	db.Model(&models.ReviewTask{}).Where("pr_number = ?", 456).Count(&taskCount)
	assert.Equal(t, int64(0), taskCount)
}

func TestUnlabeledEventWithWaitingBusinessHoursTask(t *testing.T) {
	// Setup
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	// Create channel config
	config := models.ChannelConfig{
		SlackChannelID:   "C1234567890",
		LabelName:        "needs-review",
		DefaultMentionID: "@here",
		RepositoryList:   "",
		IsActive:         true,
	}
	db.Create(&config)

	// Create a review task waiting for business hours
	task := models.ReviewTask{
		ID:           "waiting-task-id",
		PRURL:        "https://github.com/test/repo/pull/789",
		Repo:         "test/repo",
		PRNumber:     789,
		Title:        "Waiting Business Hours PR",
		SlackTS:      "1234567890.789012",
		SlackChannel: "C1234567890",
		Status:       "waiting_business_hours", // Waiting for business hours
		LabelName:    "needs-review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// Create unlabeled event payload
	prNumber := 789
	repoName := "repo"
	ownerLogin := "test"
	prTitle := "Waiting Business Hours PR"
	prHTMLURL := "https://github.com/test/repo/pull/789"
	labelName := "needs-review"
	action := "unlabeled"

	payload := github.PullRequestEvent{
		Action: &action,
		Number: &prNumber,
		Label: &github.Label{
			Name: &labelName,
		},
		PullRequest: &github.PullRequest{
			Number:  &prNumber,
			Title:   &prTitle,
			HTMLURL: &prHTMLURL,
		},
		Repo: &github.Repository{
			Name: &repoName,
			Owner: &github.User{
				Login: &ownerLogin,
			},
		},
	}

	// Create HTTP request
	jsonData, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request")

	// Create response recorder
	w := httptest.NewRecorder()

	// Create Gin router and execute request
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	// Verify HTTP status is 200
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify task was updated to completed status
	var updatedTask models.ReviewTask
	result := db.Where("id = ?", "waiting-task-id").First(&updatedTask)
	assert.NoError(t, result.Error)
	assert.Equal(t, "completed", updatedTask.Status)
	assert.True(t, updatedTask.UpdatedAt.After(task.UpdatedAt))
}

func TestHandleReviewSubmittedEvent(t *testing.T) {
	// Test DB
	db := setupTestDB(t)

	// Save environment variables and restore after test
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()

	// Set test environment variables
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	// Mock setup
	defer gock.Off() // Clean up mocks at test end

	// Mock Slack API success response
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
		})

	// Create test task
	task := models.ReviewTask{
		ID:           "test-task-review",
		PRURL:        "https://github.com/owner/repo/pull/123",
		Repo:         "owner/repo",
		PRNumber:     123,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Status:       "in_review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// PullRequestReviewEvent JSON for testing
	payload := `{
		"action": "submitted",
		"pull_request": {
			"number": 123,
			"html_url": "https://github.com/owner/repo/pull/123"
		},
		"repository": {
			"full_name": "owner/repo",
			"owner": {
				"login": "owner"
			},
			"name": "repo"
		},
		"review": {
			"state": "approved",
			"user": {
				"login": "reviewer123"
			}
		}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()

	r := gin.Default()
	r.POST("/webhook", HandleGitHubWebhook(db))
	r.ServeHTTP(w, req)

	// Verify status code
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify DB was updated
	var updatedTask models.ReviewTask
	db.Where("id = ?", "test-task-review").First(&updatedTask)
	assert.Equal(t, "completed", updatedTask.Status)

	// Verify mocks were used
	assert.True(t, gock.IsDone(), "Not all mocks were consumed")
}

func TestHandleReviewSubmittedEventWithWaitingBusinessHoursTask(t *testing.T) {
	// Test DB
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	// Save environment variables and restore after test
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()

	// Set test environment variables
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	// Mock setup
	defer gock.Off() // Clean up mocks at test end

	// Mock Slack API success response
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		Reply(200).
		JSON(map[string]interface{}{
			"ok":      true,
			"ts":      "1234567890.123456",
			"channel": "C1234567890",
		})

	// Create a review task waiting for business hours
	task := models.ReviewTask{
		ID:           "waiting-review-task-id",
		PRURL:        "https://github.com/test/repo/pull/999",
		Repo:         "test/repo",
		PRNumber:     999,
		Title:        "Waiting Business Hours Review PR",
		SlackTS:      "1234567890.999888",
		SlackChannel: "C1234567890",
		Status:       "waiting_business_hours", // Waiting for business hours
		LabelName:    "needs-review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// Create review submitted event payload
	prNumber := 999
	repoName := "repo"
	ownerLogin := "test"
	reviewerLogin := "reviewer"
	reviewState := "approved"
	reviewBody := "LGTM!"

	payload := github.PullRequestReviewEvent{
		Action: github.Ptr("submitted"),
		PullRequest: &github.PullRequest{
			Number: &prNumber,
		},
		Repo: &github.Repository{
			Name: &repoName,
			Owner: &github.User{
				Login: &ownerLogin,
			},
		},
		Review: &github.PullRequestReview{
			User: &github.User{
				Login: &reviewerLogin,
			},
			State: &reviewState,
			Body:  &reviewBody,
		},
	}

	// Create HTTP request
	jsonData, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	// Create response recorder
	w := httptest.NewRecorder()

	// Create Gin router and execute request
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	// Verify HTTP status is 200
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify task was updated to completed status
	var updatedTask models.ReviewTask
	result := db.Where("id = ?", "waiting-review-task-id").First(&updatedTask)
	assert.NoError(t, result.Error)
	assert.Equal(t, "completed", updatedTask.Status)
	assert.True(t, updatedTask.UpdatedAt.After(task.UpdatedAt))

	// Verify mocks were used (Slack API was called)
	assert.True(t, gock.IsDone(), "Not all mocks were consumed")
}

func TestMultipleLabelMatching(t *testing.T) {
	tests := []struct {
		name         string
		configLabel  string
		prLabels     []*github.Label
		shouldNotify bool
	}{
		{
			name:        "Single label config matches PR with multiple labels",
			configLabel: "needs-review",
			prLabels: []*github.Label{
				{Name: github.Ptr("needs-review")},
				{Name: github.Ptr("bug")},
			},
			shouldNotify: true,
		},
		{
			name:        "Multiple label config matches when all labels present",
			configLabel: "hoge-project,needs-review",
			prLabels: []*github.Label{
				{Name: github.Ptr("hoge-project")},
				{Name: github.Ptr("needs-review")},
				{Name: github.Ptr("bug")},
			},
			shouldNotify: true,
		},
		{
			name:        "Multiple label config does not match when some labels missing",
			configLabel: "hoge-project,needs-review",
			prLabels: []*github.Label{
				{Name: github.Ptr("hoge-project")},
				{Name: github.Ptr("bug")},
			},
			shouldNotify: false,
		},
		{
			name:        "Comma-separated labels with spaces match",
			configLabel: "project-a, needs-review, urgent",
			prLabels: []*github.Label{
				{Name: github.Ptr("project-a")},
				{Name: github.Ptr("needs-review")},
				{Name: github.Ptr("urgent")},
				{Name: github.Ptr("feature")},
			},
			shouldNotify: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			db := setupTestDB(t)
			gin.SetMode(gin.TestMode)
			services.IsTestMode = true

			// Mock Slack API calls
			// Mock for channel status check
			gock.New("https://slack.com").
				Get("/api/conversations.info").
				Reply(200).
				JSON(map[string]interface{}{
					"ok":      true,
					"channel": map[string]interface{}{"is_archived": false},
				})

			if tt.shouldNotify {
				gock.New("https://slack.com").
					Post("/api/chat.postMessage").
					Reply(200).
					JSON(map[string]interface{}{
						"ok":      true,
						"channel": "C1234567890",
						"ts":      "1234567890.123456",
					})
			}

			// Create channel config
			config := models.ChannelConfig{
				SlackChannelID:   "C1234567890",
				LabelName:        tt.configLabel,
				DefaultMentionID: "@here",
				RepositoryList:   "test/repo", // Set repository
				IsActive:         true,
			}
			db.Create(&config)

			// Create labeled event payload
			action := "labeled"
			prNumber := 123

			payload := github.PullRequestEvent{
				Action: &action,
				Number: &prNumber,
				Label: &github.Label{
					Name: github.Ptr("needs-review"), // Triggering label
				},
				PullRequest: &github.PullRequest{
					Number:  &prNumber,
					HTMLURL: github.Ptr("https://github.com/test/repo/pull/123"),
					Title:   github.Ptr("Test PR"),
					Labels:  tt.prLabels, // All labels on the PR
				},
				Repo: &github.Repository{
					FullName: github.Ptr("test/repo"),
					Owner: &github.User{
						Login: github.Ptr("test"),
					},
					Name: github.Ptr("repo"),
				},
			}

			payloadJSON, _ := json.Marshal(payload)

			// Create request
			req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(payloadJSON))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-GitHub-Event", "pull_request")

			w := httptest.NewRecorder()

			// Create Gin router and execute request
			router := gin.Default()
			router.POST("/webhook", HandleGitHubWebhook(db))
			router.ServeHTTP(w, req)

			// Verify HTTP status is 200
			assert.Equal(t, http.StatusOK, w.Code)

			// Check if task was created
			var taskCount int64
			db.Model(&models.ReviewTask{}).Where("repo = ? AND pr_number = ?", "test/repo", 123).Count(&taskCount)

			if tt.shouldNotify {
				assert.Equal(t, int64(1), taskCount, "Task should be created when notification is expected")

				// Verify mocks were used
				if gock.HasUnmatchedRequest() {
					t.Log("Unmatched requests:", gock.GetUnmatchedRequests())
				}
			} else {
				assert.Equal(t, int64(0), taskCount, "Task should not be created when notification is not expected")
			}

			// Cleanup
			gock.Off()
		})
	}
}

func TestMultipleLabelUnlabeling(t *testing.T) {
	tests := []struct {
		name               string
		configLabel        string
		prLabelsAfterEvent []*github.Label // PR label state after unlabeled event
		removedLabel       string          // The removed label
		shouldComplete     bool            // Whether the task should be completed
	}{
		{
			name:        "Single label config completes task when label removed",
			configLabel: "needs-review",
			prLabelsAfterEvent: []*github.Label{
				{Name: github.Ptr("bug")},
			},
			removedLabel:   "needs-review",
			shouldComplete: true,
		},
		{
			name:        "Multiple label config completes task when required label removed",
			configLabel: "hoge-project,needs-review",
			prLabelsAfterEvent: []*github.Label{
				{Name: github.Ptr("hoge-project")}, // needs-review was removed
				{Name: github.Ptr("bug")},
			},
			removedLabel:   "needs-review",
			shouldComplete: true,
		},
		{
			name:        "Multiple label config continues task when unrelated label removed",
			configLabel: "hoge-project,needs-review",
			prLabelsAfterEvent: []*github.Label{
				{Name: github.Ptr("hoge-project")},
				{Name: github.Ptr("needs-review")}, // Both remain
			},
			removedLabel:   "bug", // Unrelated label removed
			shouldComplete: false,
		},
		{
			name:        "Multiple label config completes task when one of multiple required labels removed",
			configLabel: "project-a,needs-review,urgent",
			prLabelsAfterEvent: []*github.Label{
				{Name: github.Ptr("needs-review")},
				{Name: github.Ptr("urgent")}, // project-a was removed
			},
			removedLabel:   "project-a",
			shouldComplete: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			db := setupTestDB(t)
			gin.SetMode(gin.TestMode)
			services.IsTestMode = true

			// Create channel config
			config := models.ChannelConfig{
				SlackChannelID:   "C1234567890",
				LabelName:        tt.configLabel,
				DefaultMentionID: "@here",
				RepositoryList:   "test/repo",
				IsActive:         true,
			}
			db.Create(&config)

			// Create an existing review task
			task := models.ReviewTask{
				ID:           "test-task-id",
				PRURL:        "https://github.com/test/repo/pull/123",
				Repo:         "test/repo",
				PRNumber:     123,
				Title:        "Test PR",
				SlackTS:      "1234567890.123456",
				SlackChannel: "C1234567890",
				Status:       "in_review",
				LabelName:    tt.configLabel,
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			}
			db.Create(&task)

			// Mock Slack API calls
			if tt.shouldComplete {
				// Update message on task completion
				gock.New("https://slack.com").
					Post("/api/chat.update").
					Reply(200).
					JSON(map[string]interface{}{
						"ok": true,
					})

				// Notify completion due to label removal in thread
				gock.New("https://slack.com").
					Post("/api/chat.postMessage").
					Reply(200).
					JSON(map[string]interface{}{
						"ok": true,
						"ts": "1234567890.123457",
					})
			}

			// Create unlabeled event payload
			action := "unlabeled"
			prNumber := 123

			payload := github.PullRequestEvent{
				Action: &action,
				Number: &prNumber,
				Label: &github.Label{
					Name: github.Ptr(tt.removedLabel),
				},
				PullRequest: &github.PullRequest{
					Number:  &prNumber,
					HTMLURL: github.Ptr("https://github.com/test/repo/pull/123"),
					Title:   github.Ptr("Test PR"),
					Labels:  tt.prLabelsAfterEvent, // Label state after removal
				},
				Repo: &github.Repository{
					FullName: github.Ptr("test/repo"),
					Owner: &github.User{
						Login: github.Ptr("test"),
					},
					Name: github.Ptr("repo"),
				},
			}

			payloadJSON, _ := json.Marshal(payload)

			// Create request
			req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(payloadJSON))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-GitHub-Event", "pull_request")

			w := httptest.NewRecorder()

			// Create Gin router and execute request
			router := gin.Default()
			router.POST("/webhook", HandleGitHubWebhook(db))
			router.ServeHTTP(w, req)

			// Verify HTTP status is 200
			assert.Equal(t, http.StatusOK, w.Code)

			// Verify task status
			var updatedTask models.ReviewTask
			result := db.Where("id = ?", "test-task-id").First(&updatedTask)
			assert.NoError(t, result.Error)

			if tt.shouldComplete {
				assert.Equal(t, "completed", updatedTask.Status, "Task should be in completed state")
			} else {
				assert.Equal(t, "in_review", updatedTask.Status, "Task should remain in in_review state")
			}

			// Cleanup
			gock.Off()
		})
	}
}

// Test that thread notifications are sent even for a second review after the initial one
func TestHandleReviewSubmittedEvent_SecondReviewAfterCompletion(t *testing.T) {
	// Test DB
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	// Save environment variables and restore after test
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()

	// Set test environment variables
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	// Mock setup
	defer gock.Off() // Clean up mocks at test end

	// Mock Slack API success response (for second review notification)
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		Reply(200).
		JSON(map[string]interface{}{
			"ok":      true,
			"ts":      "1234567890.999999",
			"channel": "C1234567890",
		})

	// Create an already reviewed (completed) task
	task := models.ReviewTask{
		ID:           "completed-task-id",
		PRURL:        "https://github.com/test/repo/pull/888",
		Repo:         "test/repo",
		PRNumber:     888,
		Title:        "Already Reviewed PR",
		SlackTS:      "1234567890.888888",
		SlackChannel: "C1234567890",
		Status:       "completed", // Already completed
		Reviewer:     "U1234567890",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-1 * time.Hour),    // Created 1 hour ago
		UpdatedAt:    time.Now().Add(-30 * time.Minute), // Updated 30 minutes ago
	}
	db.Create(&task)

	// Create second review submitted event payload
	prNumber := 888
	repoName := "repo"
	ownerLogin := "test"
	reviewerLogin := "second-reviewer"
	reviewState := "changes_requested"
	reviewBody := "いくつか修正をお願いします"

	payload := github.PullRequestReviewEvent{
		Action: github.Ptr("submitted"),
		PullRequest: &github.PullRequest{
			Number: &prNumber,
		},
		Repo: &github.Repository{
			Name: &repoName,
			Owner: &github.User{
				Login: &ownerLogin,
			},
		},
		Review: &github.PullRequestReview{
			User: &github.User{
				Login: &reviewerLogin,
			},
			State: &reviewState,
			Body:  &reviewBody,
		},
	}

	// Create HTTP request
	jsonData, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	// Create response recorder
	w := httptest.NewRecorder()

	// Create Gin router and execute request
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	// Verify HTTP status is 200
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify task status remains completed
	var updatedTask models.ReviewTask
	result := db.Where("id = ?", "completed-task-id").First(&updatedTask)
	assert.NoError(t, result.Error)
	assert.Equal(t, "completed", updatedTask.Status, "Task status should remain completed")

	// Verify mocks were used (Slack API was called = notification was sent)
	assert.True(t, gock.IsDone(), "Slack notification should be sent even for second review")
}

// Test that when a PR has multiple tasks, notifications are only sent to the latest thread
func TestHandleReviewSubmittedEvent_OnlyLatestTaskReceivesNotification(t *testing.T) {
	// Test DB
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off() // Clean up mocks at test end

	// Mock Slack API success response
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		Reply(200).
		JSON(map[string]interface{}{
			"ok":      true,
			"ts":      "1234567890.999999",
			"channel": "C1234567890",
		})

	// Create an old task for the same PR (created 2 hours ago)
	oldTask := models.ReviewTask{
		ID:           "old-task-id",
		PRURL:        "https://github.com/test/repo/pull/777",
		Repo:         "test/repo",
		PRNumber:     777,
		Title:        "Test PR with Multiple Tasks",
		SlackTS:      "1234567890.111111",
		SlackChannel: "C1234567890",
		Status:       "completed",
		Reviewer:     "U1111111111",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-2 * time.Hour), // Created 2 hours ago
		UpdatedAt:    time.Now().Add(-2 * time.Hour),
	}
	db.Create(&oldTask)

	// Create a new task for the same PR (created 1 hour ago)
	newTask := models.ReviewTask{
		ID:           "new-task-id",
		PRURL:        "https://github.com/test/repo/pull/777",
		Repo:         "test/repo",
		PRNumber:     777,
		Title:        "Test PR with Multiple Tasks",
		SlackTS:      "1234567890.222222",
		SlackChannel: "C1234567890",
		Status:       "completed",
		Reviewer:     "U2222222222",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-1 * time.Hour), // Created 1 hour ago (newer)
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}
	db.Create(&newTask)

	// Create review submitted event payload
	prNumber := 777
	repoName := "repo"
	ownerLogin := "test"
	reviewerLogin := "reviewer"
	reviewState := "approved"
	reviewBody := "LGTM"

	payload := github.PullRequestReviewEvent{
		Action: github.Ptr("submitted"),
		PullRequest: &github.PullRequest{
			Number: &prNumber,
		},
		Repo: &github.Repository{
			Name: &repoName,
			Owner: &github.User{
				Login: &ownerLogin,
			},
		},
		Review: &github.PullRequestReview{
			User: &github.User{
				Login: &reviewerLogin,
			},
			State: &reviewState,
			Body:  &reviewBody,
		},
	}

	// Create HTTP request
	jsonData, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	// Create response recorder
	w := httptest.NewRecorder()

	// Create Gin router and execute request
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	// Verify HTTP status is 200
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify mock was used exactly once (notification sent only to the latest task)
	assert.True(t, gock.IsDone(), "Slack notification should be sent only to the latest task")

	// Verify no unconsumed mocks (confirming it was not called twice)
	pendingMocks := gock.Pending()
	assert.Equal(t, 0, len(pendingMocks), "Slack API should be called only once (no notification for old tasks)")
}

// Test that notifications are sent to tasks in different channels
func TestHandleReviewSubmittedEvent_DifferentChannelsReceiveNotifications(t *testing.T) {
	// Test DB
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off() // Clean up mocks at test end

	// Mock for channel 1 notification
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		Reply(200).
		JSON(map[string]interface{}{
			"ok":      true,
			"ts":      "1234567890.111111",
			"channel": "C1111111111",
		})

	// Mock for channel 2 notification
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		Reply(200).
		JSON(map[string]interface{}{
			"ok":      true,
			"ts":      "1234567890.222222",
			"channel": "C2222222222",
		})

	// Channel 1 task (needs-review label)
	task1 := models.ReviewTask{
		ID:           "task-channel-1",
		PRURL:        "https://github.com/test/repo/pull/999",
		Repo:         "test/repo",
		PRNumber:     999,
		Title:        "Test PR with Multiple Channels",
		SlackTS:      "1234567890.111111",
		SlackChannel: "C1111111111", // Channel 1
		Status:       "completed",
		Reviewer:     "U1111111111",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-1 * time.Hour),
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}
	db.Create(&task1)

	// Channel 2 task (needs-design-review label)
	task2 := models.ReviewTask{
		ID:           "task-channel-2",
		PRURL:        "https://github.com/test/repo/pull/999",
		Repo:         "test/repo",
		PRNumber:     999,
		Title:        "Test PR with Multiple Channels",
		SlackTS:      "1234567890.222222",
		SlackChannel: "C2222222222", // Channel 2 (different channel)
		Status:       "completed",
		Reviewer:     "U2222222222",
		LabelName:    "needs-design-review",
		CreatedAt:    time.Now().Add(-1 * time.Hour),
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}
	db.Create(&task2)

	// Create review submitted event payload
	prNumber := 999
	repoName := "repo"
	ownerLogin := "test"
	reviewerLogin := "reviewer"
	reviewState := "approved"
	reviewBody := "LGTM"

	payload := github.PullRequestReviewEvent{
		Action: github.Ptr("submitted"),
		PullRequest: &github.PullRequest{
			Number: &prNumber,
		},
		Repo: &github.Repository{
			Name: &repoName,
			Owner: &github.User{
				Login: &ownerLogin,
			},
		},
		Review: &github.PullRequestReview{
			User: &github.User{
				Login: &reviewerLogin,
			},
			State: &reviewState,
			Body:  &reviewBody,
		},
	}

	// Create HTTP request
	jsonData, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	// Create response recorder
	w := httptest.NewRecorder()

	// Create Gin router and execute request
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	// Verify HTTP status is 200
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify mocks were used twice (notifications sent to both channels)
	assert.True(t, gock.IsDone(), "Slack notifications should be sent to both channels")

	// Verify no unconsumed mocks
	pendingMocks := gock.Pending()
	assert.Equal(t, 0, len(pendingMocks), "Slack API should be called twice (once per channel)")
}

// Test that old tasks for the same channel and PR also get completed
func TestHandleReviewSubmittedEvent_OldTasksAlsoCompleted(t *testing.T) {
	// Test DB
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	// Save environment variables and restore after test
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()

	// Set test environment variables
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	// Mock setup
	defer gock.Off() // Clean up mocks at test end

	// Mock Slack API success response (called only once - latest task only)
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		Reply(200).
		JSON(map[string]interface{}{
			"ok":      true,
			"ts":      "1234567890.999999",
			"channel": "C1234567890",
		})

	// Create old task (in_review) for the same PR and channel
	oldTask := models.ReviewTask{
		ID:           "old-in-review-task",
		PRURL:        "https://github.com/test/repo/pull/555",
		Repo:         "test/repo",
		PRNumber:     555,
		Title:        "Test PR with Old Task",
		SlackTS:      "1234567890.111111",
		SlackChannel: "C1234567890",
		Status:       "in_review", // Old task is in in_review state
		Reviewer:     "U1111111111",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-2 * time.Hour), // 2 hours ago
		UpdatedAt:    time.Now().Add(-2 * time.Hour),
	}
	db.Create(&oldTask)

	// Create new task (in_review) for the same PR and channel
	newTask := models.ReviewTask{
		ID:           "new-in-review-task",
		PRURL:        "https://github.com/test/repo/pull/555",
		Repo:         "test/repo",
		PRNumber:     555,
		Title:        "Test PR with Old Task",
		SlackTS:      "1234567890.222222",
		SlackChannel: "C1234567890",
		Status:       "in_review", // New task is also in in_review state
		Reviewer:     "U2222222222",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-1 * time.Hour), // 1 hour ago (newer)
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}
	db.Create(&newTask)

	// Create review submitted event payload
	prNumber := 555
	repoName := "repo"
	ownerLogin := "test"
	reviewerLogin := "reviewer"
	reviewState := "approved"
	reviewBody := "LGTM"

	payload := github.PullRequestReviewEvent{
		Action: github.Ptr("submitted"),
		PullRequest: &github.PullRequest{
			Number: &prNumber,
		},
		Repo: &github.Repository{
			Name: &repoName,
			Owner: &github.User{
				Login: &ownerLogin,
			},
		},
		Review: &github.PullRequestReview{
			User: &github.User{
				Login: &reviewerLogin,
			},
			State: &reviewState,
			Body:  &reviewBody,
		},
	}

	// Create HTTP request
	jsonData, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	// Create response recorder
	w := httptest.NewRecorder()

	// Create Gin router and execute request
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	// Verify HTTP status is 200
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify old task is also completed
	var updatedOldTask models.ReviewTask
	db.Where("id = ?", "old-in-review-task").First(&updatedOldTask)
	assert.Equal(t, "completed", updatedOldTask.Status, "Old task should also be completed (to prevent reminders)")

	// Verify new task is also completed
	var updatedNewTask models.ReviewTask
	db.Where("id = ?", "new-in-review-task").First(&updatedNewTask)
	assert.Equal(t, "completed", updatedNewTask.Status, "New task should also be completed")

	// Verify mocks were used
	assert.True(t, gock.IsDone(), "Slack notification should be sent only to the latest task")
}

// --- Tests for completing tasks and stopping reminders on review comment/changes requested ---

func TestHandleReviewSubmittedEvent_CommentedCompletesTask(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	// Mock Slack API notification
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	task := models.ReviewTask{
		ID:           "commented-task",
		PRURL:        "https://github.com/owner/repo/pull/300",
		Repo:         "owner/repo",
		PRNumber:     300,
		Title:        "Commented PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C_COMMENT",
		Reviewer:     "UREVIEWER1",
		Status:       "in_review",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-1 * time.Hour),
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}
	db.Create(&task)

	payload := `{
		"action": "submitted",
		"pull_request": {"number": 300, "html_url": "https://github.com/owner/repo/pull/300"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "commented", "user": {"login": "reviewer1"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "commented-task").First(&updatedTask)
	assert.Equal(t, "completed", updatedTask.Status, "Should be completed after commented review to stop reminders")
}

func TestHandleReviewSubmittedEvent_ChangesRequestedCompletesTask(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	// Mock Slack API notification
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	task := models.ReviewTask{
		ID:           "changes-requested-task",
		PRURL:        "https://github.com/owner/repo/pull/301",
		Repo:         "owner/repo",
		PRNumber:     301,
		Title:        "Changes Requested PR",
		SlackTS:      "1234.5679",
		SlackChannel: "C_CHANGES",
		Reviewer:     "UREVIEWER1",
		Status:       "in_review",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-1 * time.Hour),
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}
	db.Create(&task)

	payload := `{
		"action": "submitted",
		"pull_request": {"number": 301, "html_url": "https://github.com/owner/repo/pull/301"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "changes_requested", "user": {"login": "reviewer1"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "changes-requested-task").First(&updatedTask)
	assert.Equal(t, "completed", updatedTask.Status, "Should be completed after changes_requested review to stop reminders")
}

func TestHandleReviewSubmittedEvent_CommentedCompletesOldTasksToo(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// Old task (in_review) for same PR and channel
	oldTask := models.ReviewTask{
		ID:           "old-comment-task",
		PRURL:        "https://github.com/owner/repo/pull/302",
		Repo:         "owner/repo",
		PRNumber:     302,
		Title:        "Comment PR",
		SlackTS:      "1234.1111",
		SlackChannel: "C_COMMENT2",
		Status:       "in_review",
		Reviewer:     "UOLD",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-2 * time.Hour),
		UpdatedAt:    time.Now().Add(-2 * time.Hour),
	}
	db.Create(&oldTask)

	newTask := models.ReviewTask{
		ID:           "new-comment-task",
		PRURL:        "https://github.com/owner/repo/pull/302",
		Repo:         "owner/repo",
		PRNumber:     302,
		Title:        "Comment PR",
		SlackTS:      "1234.2222",
		SlackChannel: "C_COMMENT2",
		Status:       "in_review",
		Reviewer:     "UNEW",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-1 * time.Hour),
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}
	db.Create(&newTask)

	payload := `{
		"action": "submitted",
		"pull_request": {"number": 302, "html_url": "https://github.com/owner/repo/pull/302"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "commented", "user": {"login": "reviewer1"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Old task should also be completed (to prevent reminders)
	var updatedOld models.ReviewTask
	db.Where("id = ?", "old-comment-task").First(&updatedOld)
	assert.Equal(t, "completed", updatedOld.Status)

	var updatedNew models.ReviewTask
	db.Where("id = ?", "new-comment-task").First(&updatedNew)
	assert.Equal(t, "completed", updatedNew.Status)
}

func TestHandleReviewSubmittedEvent_SnoozedTaskCompletedOnComment(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// Task in snoozed state
	task := models.ReviewTask{
		ID:           "snoozed-comment-task",
		PRURL:        "https://github.com/owner/repo/pull/400",
		Repo:         "owner/repo",
		PRNumber:     400,
		Title:        "Snoozed PR",
		SlackTS:      "1234.9999",
		SlackChannel: "C_SNOOZED",
		Reviewer:     "UREVIEWER1",
		Status:       "snoozed",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-2 * time.Hour),
		UpdatedAt:    time.Now().Add(-2 * time.Hour),
	}
	db.Create(&task)

	payload := `{
		"action": "submitted",
		"pull_request": {"number": 400, "html_url": "https://github.com/owner/repo/pull/400"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "commented", "user": {"login": "reviewer1"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "snoozed-comment-task").First(&updatedTask)
	assert.Equal(t, "completed", updatedTask.Status, "Snoozed task should also be completed after commented review")
}

func TestHandleReviewSubmittedEvent_SnoozedTaskCompletedOnChangesRequested(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// Task in snoozed state
	task := models.ReviewTask{
		ID:           "snoozed-changes-task",
		PRURL:        "https://github.com/owner/repo/pull/401",
		Repo:         "owner/repo",
		PRNumber:     401,
		Title:        "Snoozed Changes PR",
		SlackTS:      "1234.8888",
		SlackChannel: "C_SNOOZED2",
		Reviewer:     "UREVIEWER1",
		Status:       "snoozed",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-2 * time.Hour),
		UpdatedAt:    time.Now().Add(-2 * time.Hour),
	}
	db.Create(&task)

	payload := `{
		"action": "submitted",
		"pull_request": {"number": 401, "html_url": "https://github.com/owner/repo/pull/401"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "changes_requested", "user": {"login": "reviewer1"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "snoozed-changes-task").First(&updatedTask)
	assert.Equal(t, "completed", updatedTask.Status, "Snoozed task should also be completed after changes_requested review")
}

// --- Tests ensuring waiting_business_hours tasks are NOT completed by comment / changes_requested ---
// Regression: previously a single comment from anyone before the next business morning would
// flip waiting_business_hours -> completed and cancel the scheduled reviewer mention.

func TestHandleReviewSubmittedEvent_WaitingBusinessHoursTaskPreservedOnComment(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	task := models.ReviewTask{
		ID:           "waiting-bh-comment-task",
		PRURL:        "https://github.com/owner/repo/pull/500",
		Repo:         "owner/repo",
		PRNumber:     500,
		Title:        "Waiting Business Hours PR",
		SlackTS:      "1234.5500",
		SlackChannel: "C_WAITING_BH",
		Status:       "waiting_business_hours",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-2 * time.Hour),
		UpdatedAt:    time.Now().Add(-2 * time.Hour),
	}
	db.Create(&task)

	payload := `{
		"action": "submitted",
		"pull_request": {"number": 500, "html_url": "https://github.com/owner/repo/pull/500"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "commented", "user": {"login": "reviewer1"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "waiting-bh-comment-task").First(&updatedTask)
	assert.Equal(t, "waiting_business_hours", updatedTask.Status,
		"waiting_business_hours task must NOT be completed by a comment review; the next-morning mention must still fire")
	assert.True(t, gock.IsDone(), "all Slack mocks should be consumed (review-completed notification must fire)")
}

func TestHandleReviewSubmittedEvent_WaitingBusinessHoursTaskPreservedOnChangesRequested(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	task := models.ReviewTask{
		ID:           "waiting-bh-changes-task",
		PRURL:        "https://github.com/owner/repo/pull/501",
		Repo:         "owner/repo",
		PRNumber:     501,
		Title:        "Waiting Business Hours PR",
		SlackTS:      "1234.5501",
		SlackChannel: "C_WAITING_BH2",
		Status:       "waiting_business_hours",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-2 * time.Hour),
		UpdatedAt:    time.Now().Add(-2 * time.Hour),
	}
	db.Create(&task)

	payload := `{
		"action": "submitted",
		"pull_request": {"number": 501, "html_url": "https://github.com/owner/repo/pull/501"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "changes_requested", "user": {"login": "reviewer1"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "waiting-bh-changes-task").First(&updatedTask)
	assert.Equal(t, "waiting_business_hours", updatedTask.Status,
		"waiting_business_hours task must NOT be completed by a changes_requested review; the next-morning mention must still fire")
	assert.True(t, gock.IsDone(), "all Slack mocks should be consumed (review-completed notification must fire)")
}

// Mixed-state regression: when a single channel/PR has both a waiting_business_hours task and an
// in_review task, a comment / changes_requested must complete only the in_review one and leave
// waiting_business_hours intact so the next-morning mention still fires.

func TestHandleReviewSubmittedEvent_WaitingBusinessHoursAndInReview_MixedOnComment(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// Older in_review task
	oldInReview := models.ReviewTask{
		ID:           "mixed-comment-in-review",
		PRURL:        "https://github.com/owner/repo/pull/700",
		Repo:         "owner/repo",
		PRNumber:     700,
		Title:        "Mixed PR",
		SlackTS:      "1234.7000",
		SlackChannel: "C_MIXED_COMMENT",
		Status:       "in_review",
		Reviewer:     "UREVIEWER",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-3 * time.Hour),
		UpdatedAt:    time.Now().Add(-3 * time.Hour),
	}
	db.Create(&oldInReview)

	// Newer waiting_business_hours task (this becomes latestTask in the per-channel selection)
	newWaiting := models.ReviewTask{
		ID:           "mixed-comment-waiting",
		PRURL:        "https://github.com/owner/repo/pull/700",
		Repo:         "owner/repo",
		PRNumber:     700,
		Title:        "Mixed PR",
		SlackTS:      "1234.7001",
		SlackChannel: "C_MIXED_COMMENT",
		Status:       "waiting_business_hours",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-1 * time.Hour),
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}
	db.Create(&newWaiting)

	payload := `{
		"action": "submitted",
		"pull_request": {"number": 700, "html_url": "https://github.com/owner/repo/pull/700"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "commented", "user": {"login": "reviewer1"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var waitingAfter, inReviewAfter models.ReviewTask
	db.Where("id = ?", "mixed-comment-waiting").First(&waitingAfter)
	db.Where("id = ?", "mixed-comment-in-review").First(&inReviewAfter)

	assert.Equal(t, "waiting_business_hours", waitingAfter.Status,
		"waiting_business_hours task must be preserved when coexisting with in_review")
	assert.Equal(t, "completed", inReviewAfter.Status,
		"in_review task must be completed to stop reminders")
	assert.True(t, gock.IsDone(), "review-completed notification must fire exactly once")
}

func TestHandleReviewSubmittedEvent_WaitingBusinessHoursAndInReview_MixedOnChangesRequested(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	oldInReview := models.ReviewTask{
		ID:           "mixed-changes-in-review",
		PRURL:        "https://github.com/owner/repo/pull/701",
		Repo:         "owner/repo",
		PRNumber:     701,
		Title:        "Mixed PR",
		SlackTS:      "1234.7010",
		SlackChannel: "C_MIXED_CHANGES",
		Status:       "in_review",
		Reviewer:     "UREVIEWER",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-3 * time.Hour),
		UpdatedAt:    time.Now().Add(-3 * time.Hour),
	}
	db.Create(&oldInReview)

	newWaiting := models.ReviewTask{
		ID:           "mixed-changes-waiting",
		PRURL:        "https://github.com/owner/repo/pull/701",
		Repo:         "owner/repo",
		PRNumber:     701,
		Title:        "Mixed PR",
		SlackTS:      "1234.7011",
		SlackChannel: "C_MIXED_CHANGES",
		Status:       "waiting_business_hours",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-1 * time.Hour),
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}
	db.Create(&newWaiting)

	payload := `{
		"action": "submitted",
		"pull_request": {"number": 701, "html_url": "https://github.com/owner/repo/pull/701"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "changes_requested", "user": {"login": "reviewer1"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var waitingAfter, inReviewAfter models.ReviewTask
	db.Where("id = ?", "mixed-changes-waiting").First(&waitingAfter)
	db.Where("id = ?", "mixed-changes-in-review").First(&inReviewAfter)

	assert.Equal(t, "waiting_business_hours", waitingAfter.Status,
		"waiting_business_hours task must be preserved when coexisting with in_review")
	assert.Equal(t, "completed", inReviewAfter.Status,
		"in_review task must be completed to stop reminders")
	assert.True(t, gock.IsDone(), "review-completed notification must fire exactly once")
}

// Companion to the preservation tests above: full approval IS expected to complete
// waiting_business_hours, because the PR is fully approved and no further review is needed.
// This test pins the intentional asymmetry between the approved branch and the
// commented/changes_requested branch.

func TestHandleReviewSubmittedEvent_WaitingBusinessHoursTaskCompletedOnFullApproval(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Persist().
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// requiredApprovals = 1 (default), so a single approval = fully approved
	task := models.ReviewTask{
		ID:           "waiting-bh-approved",
		PRURL:        "https://github.com/owner/repo/pull/702",
		Repo:         "owner/repo",
		PRNumber:     702,
		Title:        "Waiting Business Hours PR",
		SlackTS:      "1234.7020",
		SlackChannel: "C_WAITING_APPROVED",
		Status:       "waiting_business_hours",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-2 * time.Hour),
		UpdatedAt:    time.Now().Add(-2 * time.Hour),
	}
	db.Create(&task)

	payload := `{
		"action": "submitted",
		"pull_request": {"number": 702, "html_url": "https://github.com/owner/repo/pull/702"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "approved", "user": {"login": "reviewer1"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "waiting-bh-approved").First(&updatedTask)
	assert.Equal(t, "completed", updatedTask.Status,
		"waiting_business_hours task SHOULD be completed on full approval (no further review needed)")
}

// --- Tests for snoozed tasks becoming completed when fully approved ---

func TestHandleReviewSubmittedEvent_SnoozedTaskCompletedOnFullApproval(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	// Review notification + completion message + approve notification
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// Channel config with RequiredApprovals=1
	config := models.ChannelConfig{
		ID:                "config-snoozed-approve",
		SlackChannelID:    "C_SNOOZED_APPROVE",
		LabelName:         "needs-review",
		DefaultMentionID:  "UDEFAULT",
		RequiredApprovals: 1,
		IsActive:          true,
	}
	db.Create(&config)

	userMapping := models.UserMapping{
		ID:             "mapping-snoozed",
		GithubUsername: "reviewer1",
		SlackUserID:    "UREVIEWER1",
	}
	db.Create(&userMapping)

	// Task in snoozed state
	task := models.ReviewTask{
		ID:           "snoozed-approve-task",
		PRURL:        "https://github.com/owner/repo/pull/600",
		Repo:         "owner/repo",
		PRNumber:     600,
		Title:        "Snoozed Approve PR",
		SlackTS:      "1234.6000",
		SlackChannel: "C_SNOOZED_APPROVE",
		Reviewer:     "UREVIEWER1",
		Reviewers:    "UREVIEWER1",
		Status:       "snoozed",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-2 * time.Hour),
		UpdatedAt:    time.Now().Add(-2 * time.Hour),
	}
	db.Create(&task)

	payload := `{
		"action": "submitted",
		"pull_request": {"number": 600, "html_url": "https://github.com/owner/repo/pull/600"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "approved", "user": {"login": "reviewer1"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "snoozed-approve-task").First(&updatedTask)
	assert.Equal(t, "completed", updatedTask.Status, "Snoozed task should be completed when fully approved")
}

// --- Webhook tests for multiple reviewers ---

func TestHandleReviewSubmittedEvent_PartialApproval(t *testing.T) {
	// Test DB
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	// Mock for review notification
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// Mock for progress message on partial approval
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// Channel config with RequiredApprovals=2
	config := models.ChannelConfig{
		ID:                "config-partial",
		SlackChannelID:    "C_PARTIAL",
		LabelName:         "needs-review",
		DefaultMentionID:  "UDEFAULT",
		RequiredApprovals: 2,
		IsActive:          true,
	}
	db.Create(&config)

	// UserMapping
	userMapping := models.UserMapping{
		ID:             "mapping-1",
		GithubUsername: "reviewer1",
		SlackUserID:    "UREVIEWER1",
	}
	db.Create(&userMapping)

	// Task with 2 reviewers assigned
	task := models.ReviewTask{
		ID:           "partial-task",
		PRURL:        "https://github.com/owner/repo/pull/200",
		Repo:         "owner/repo",
		PRNumber:     200,
		Title:        "Partial Approval PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C_PARTIAL",
		Reviewer:     "UREVIEWER1",
		Reviewers:    "UREVIEWER1,UREVIEWER2",
		Status:       "in_review",
		LabelName:    "needs-review",
	}
	db.Create(&task)

	// First reviewer approves
	payload := `{
		"action": "submitted",
		"pull_request": {"number": 200, "html_url": "https://github.com/owner/repo/pull/200"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "approved", "user": {"login": "reviewer1"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Task should still be in_review (1/2 approve)
	var updatedTask models.ReviewTask
	db.Where("id = ?", "partial-task").First(&updatedTask)
	assert.Equal(t, "in_review", updatedTask.Status, "Should still be in_review with 1/2 approvals")
	assert.Contains(t, updatedTask.ApprovedBy, "UREVIEWER1")
}

func TestHandleReviewSubmittedEvent_FullApproval(t *testing.T) {
	// Test DB
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	// Mock for review notification
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// Channel config with RequiredApprovals=2
	config := models.ChannelConfig{
		ID:                "config-full",
		SlackChannelID:    "C_FULL",
		LabelName:         "needs-review",
		DefaultMentionID:  "UDEFAULT",
		RequiredApprovals: 2,
		IsActive:          true,
	}
	db.Create(&config)

	// UserMapping
	userMapping := models.UserMapping{
		ID:             "mapping-2",
		GithubUsername: "reviewer2",
		SlackUserID:    "UREVIEWER2",
	}
	db.Create(&userMapping)

	// Task already approved by one reviewer
	task := models.ReviewTask{
		ID:           "full-task",
		PRURL:        "https://github.com/owner/repo/pull/201",
		Repo:         "owner/repo",
		PRNumber:     201,
		Title:        "Full Approval PR",
		SlackTS:      "1234.5679",
		SlackChannel: "C_FULL",
		Reviewer:     "UREVIEWER1",
		Reviewers:    "UREVIEWER1,UREVIEWER2",
		ApprovedBy:   "UREVIEWER1",
		Status:       "in_review",
		LabelName:    "needs-review",
	}
	db.Create(&task)

	// Second reviewer approves
	payload := `{
		"action": "submitted",
		"pull_request": {"number": 201, "html_url": "https://github.com/owner/repo/pull/201"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "approved", "user": {"login": "reviewer2"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Task should be completed (2/2 approve)
	var updatedTask models.ReviewTask
	db.Where("id = ?", "full-task").First(&updatedTask)
	assert.Equal(t, "completed", updatedTask.Status, "Should be completed with 2/2 approvals")
	assert.Contains(t, updatedTask.ApprovedBy, "UREVIEWER1")
	assert.Contains(t, updatedTask.ApprovedBy, "UREVIEWER2")
}

func TestHandleReviewSubmittedEvent_BackwardCompat(t *testing.T) {
	// Test DB - existing behavior with RequiredApprovals=1 (default)
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	// Mock for review notification
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// Channel config without RequiredApprovals set (default=1)
	config := models.ChannelConfig{
		ID:               "config-compat",
		SlackChannelID:   "C_COMPAT",
		LabelName:        "needs-review",
		DefaultMentionID: "UDEFAULT",
		IsActive:         true,
	}
	db.Create(&config)

	// Task in old data format (empty Reviewers)
	task := models.ReviewTask{
		ID:           "compat-task",
		PRURL:        "https://github.com/owner/repo/pull/202",
		Repo:         "owner/repo",
		PRNumber:     202,
		Title:        "Backward Compat PR",
		SlackTS:      "1234.5680",
		SlackChannel: "C_COMPAT",
		Reviewer:     "UOLD",
		Status:       "in_review",
		LabelName:    "needs-review",
	}
	db.Create(&task)

	// Approve event
	payload := `{
		"action": "submitted",
		"pull_request": {"number": 202, "html_url": "https://github.com/owner/repo/pull/202"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "approved", "user": {"login": "unknownUser"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Since RequiredApprovals=1, one approval results in completed
	var updatedTask models.ReviewTask
	db.Where("id = ?", "compat-task").First(&updatedTask)
	assert.Equal(t, "completed", updatedTask.Status, "Should be completed with RequiredApprovals=1 and 1 approval")
}

func TestHandleReviewRequestedEvent_CompletedTask(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	// Mock Slack API with gock
	defer gock.Off()
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// Create completed task
	task := models.ReviewTask{
		ID:           "completed-task",
		PRURL:        "https://github.com/owner/repo/pull/300",
		Repo:         "owner/repo",
		PRNumber:     300,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Status:       "completed",
		LabelName:    "needs-review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	payload := `{
		"action": "review_requested",
		"pull_request": {"number": 300, "html_url": "https://github.com/owner/repo/pull/300"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"sender": {"login": "author"},
		"requested_reviewer": {"login": "reviewer1"}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Completed task should be reverted to in_review
	var updatedTask models.ReviewTask
	db.Where("id = ?", "completed-task").First(&updatedTask)
	assert.Equal(t, "in_review", updatedTask.Status)

	// Notification should have been sent (gock mocks consumed)
	assert.True(t, gock.IsDone(), "Re-request notification should be sent to Slack")
}

func TestHandleReviewRequestedEvent_InReviewTask(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	// Mock Slack API with gock
	defer gock.Off()
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// Create in_review task
	task := models.ReviewTask{
		ID:           "inreview-task",
		PRURL:        "https://github.com/owner/repo/pull/301",
		Repo:         "owner/repo",
		PRNumber:     301,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Status:       "in_review",
		LabelName:    "needs-review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	payload := `{
		"action": "review_requested",
		"pull_request": {"number": 301, "html_url": "https://github.com/owner/repo/pull/301"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"sender": {"login": "author"},
		"requested_reviewer": {"login": "reviewer1"}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Should remain in_review
	var updatedTask models.ReviewTask
	db.Where("id = ?", "inreview-task").First(&updatedTask)
	assert.Equal(t, "in_review", updatedTask.Status)

	// Notification should have been sent (gock mocks consumed)
	assert.True(t, gock.IsDone(), "Re-request notification should be sent to Slack")
}

func TestHandleReviewSubmittedEvent_DismissedOnlyUpdatesApprovedBy(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	// Create channel config (requires 2 approvals)
	config := models.ChannelConfig{
		SlackChannelID:    "C12345",
		LabelName:         "needs-review",
		DefaultMentionID:  "@here",
		IsActive:          true,
		RequiredApprovals: 2,
	}
	db.Create(&config)

	// Create UserMapping
	db.Create(&models.UserMapping{
		SlackUserID:    "UREVIEWER",
		GithubUsername: "reviewer1",
	})

	// Create an in_review task that has been approved
	task := models.ReviewTask{
		ID:           "dismiss-task",
		PRURL:        "https://github.com/owner/repo/pull/400",
		Repo:         "owner/repo",
		PRNumber:     400,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Reviewer:     "UREVIEWER",
		ApprovedBy:   "UREVIEWER",
		Status:       "in_review",
		LabelName:    "needs-review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// Dismiss event
	payload := `{
		"action": "submitted",
		"pull_request": {"number": 400, "html_url": "https://github.com/owner/repo/pull/400"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "dismissed", "user": {"login": "reviewer1"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Status should not change, only approved_by is updated
	var updatedTask models.ReviewTask
	db.Where("id = ?", "dismiss-task").First(&updatedTask)
	assert.Equal(t, "in_review", updatedTask.Status, "Status should not change")
	assert.Equal(t, "", updatedTask.ApprovedBy, "Dismissed reviewer should be removed from approved_by")
	assert.Nil(t, updatedTask.ReminderPausedUntil, "Reminder pause should not be set")
}

func TestHandleReviewRequestedEvent_OutsideBusinessHours_DefersNotification(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	defer gock.Off()
	// Feedback message (without mention) should be sent even outside business hours
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// Set business hours to 03:00-03:01 so we're almost always outside
	loc, _ := time.LoadLocation("Asia/Tokyo")
	now := time.Now().In(loc)
	if now.Hour() == 3 && now.Minute() == 0 {
		t.Skip("Skipping: current time falls within the narrow test business hours window")
	}
	if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
		t.Skip("Skipping: weekend (business hours always false regardless of time range)")
	}

	config := models.ChannelConfig{
		ID:                 "config-bh-test",
		SlackChannelID:     "C_BH_TEST",
		LabelName:          "needs-review",
		IsActive:           true,
		BusinessHoursStart: "03:00",
		BusinessHoursEnd:   "03:01",
		Timezone:           "Asia/Tokyo",
	}
	db.Create(&config)

	task := models.ReviewTask{
		ID:           "rereview-offhours-task",
		PRURL:        "https://github.com/owner/repo/pull/500",
		Repo:         "owner/repo",
		PRNumber:     500,
		Title:        "Test PR",
		SlackTS:      "1234.7777",
		SlackChannel: "C_BH_TEST",
		Status:       "in_review",
		LabelName:    "needs-review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	payload := `{
		"action": "review_requested",
		"pull_request": {"number": 500, "html_url": "https://github.com/owner/repo/pull/500"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"sender": {"login": "author"},
		"requested_reviewer": {"login": "reviewer1"}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "rereview-offhours-task").First(&updatedTask)
	assert.True(t, updatedTask.PendingReReviewNotify, "Re-review notification should be deferred outside business hours")
	assert.NotEmpty(t, updatedTask.PendingReReviewSender, "Sender should be stored")
	assert.NotEmpty(t, updatedTask.PendingReReviewReviewer, "Reviewer should be stored")
	assert.True(t, gock.IsDone(), "Deferred feedback message should have been sent to Slack")
}

func TestHandleReviewRequestedEvent_WithinBusinessHours_SendsImmediately(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	defer gock.Off()
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// No channel config created — without config, business hours check is skipped
	// and notification should be sent immediately

	task := models.ReviewTask{
		ID:           "rereview-bh-task",
		PRURL:        "https://github.com/owner/repo/pull/501",
		Repo:         "owner/repo",
		PRNumber:     501,
		Title:        "Test PR",
		SlackTS:      "1234.8888",
		SlackChannel: "C_NO_CONFIG",
		Status:       "in_review",
		LabelName:    "needs-review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	payload := `{
		"action": "review_requested",
		"pull_request": {"number": 501, "html_url": "https://github.com/owner/repo/pull/501"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"sender": {"login": "author"},
		"requested_reviewer": {"login": "reviewer1"}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "rereview-bh-task").First(&updatedTask)
	assert.False(t, updatedTask.PendingReReviewNotify, "Notification should be sent immediately without business hours config")
	assert.True(t, gock.IsDone(), "Slack notification should have been sent")
}
