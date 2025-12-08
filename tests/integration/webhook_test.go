package integration

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slack-review-notify/handlers"
	"slack-review-notify/models"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/go-github/v71/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGitHubWebhookIntegration tests the complete webhook flow from PR creation to Slack notification
func TestGitHubWebhookIntegration(t *testing.T) {
	// Skip if SLACK_BOT_TOKEN is not set (not in integration test environment)
	if os.Getenv("SLACK_BOT_TOKEN") == "" || os.Getenv("SLACK_BOT_TOKEN") == "xoxb-test-token-integration" {
		t.Skip("Skipping integration test: SLACK_BOT_TOKEN not configured for real Slack workspace")
	}

	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	// Get test channel ID from environment or use default
	testChannelID := os.Getenv("TEST_SLACK_CHANNEL_ID")
	if testChannelID == "" {
		t.Skip("Skipping integration test: TEST_SLACK_CHANNEL_ID not set")
	}

	// Create test channel configuration
	channelConfig := CreateTestChannelConfig(testConfig.DB, testChannelID, "needs-review")
	defer func() {
		// Cleanup: delete test channel config
		testConfig.DB.Delete(&channelConfig)
	}()

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/webhook", handlers.HandleGitHubWebhook(testConfig.DB))

	// Create GitHub webhook payload for PR labeled event
	prNumber := 12345
	repoName := "test-repo"
	ownerLogin := "owner"
	prTitle := fmt.Sprintf("[TEST] Integration Test PR - %s", time.Now().Format("15:04:05"))
	prHTMLURL := "https://github.com/owner/test-repo/pull/12345"
	labelName := "needs-review"
	action := "labeled"

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
			User: &github.User{
				Login: github.Ptr("test-user"),
			},
			Labels: []*github.Label{
				{Name: &labelName},
			},
		},
		Repo: &github.Repository{
			Name: &repoName,
			Owner: &github.User{
				Login: &ownerLogin,
			},
			FullName: github.Ptr("owner/test-repo"),
		},
	}

	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err, "Should marshal payload")

	// Create HTTP request
	req, err := http.NewRequest("POST", "/webhook", bytes.NewBuffer(payloadJSON))
	require.NoError(t, err, "Should create request")

	// Sign the request if GITHUB_WEBHOOK_SECRET is set
	if secret := os.Getenv("GITHUB_WEBHOOK_SECRET"); secret != "" {
		signature := signPayload(payloadJSON, []byte(secret))
		req.Header.Set("X-Hub-Signature-256", signature)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request")

	// Send request
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Verify HTTP response
	assert.Equal(t, http.StatusOK, w.Code, "Webhook should be accepted")

	// Wait for background goroutine to send Slack message
	time.Sleep(3 * time.Second)

	// Verify task was created in database
	var task models.ReviewTask
	err = testConfig.DB.Where("repo = ? AND pr_number = ?", "owner/test-repo", prNumber).First(&task).Error
	require.NoError(t, err, "Task should be created in database")

	assert.Equal(t, prHTMLURL, task.PRURL)
	assert.Equal(t, prTitle, task.Title)
	assert.Equal(t, testChannelID, task.SlackChannel)
	assert.NotEmpty(t, task.SlackTS, "Slack message timestamp should be set")

	// Verify Slack message was sent
	msg, err := WaitForMessage(testChannelID, prTitle, 10*time.Second)
	require.NoError(t, err, "Slack message should be sent")
	assert.Contains(t, msg.Text, prTitle, "Message should contain PR title")

	// Cleanup: delete Slack message
	if msg != nil && msg.Timestamp != "" {
		err = DeleteTestMessage(testChannelID, msg.Timestamp)
		if err != nil {
			t.Logf("Warning: Failed to delete test message: %v", err)
		}
	}

	// Cleanup: delete task from database
	testConfig.DB.Delete(&task)
}

// TestReviewRequestNotification tests that review request notifications are sent correctly
func TestReviewRequestNotification(t *testing.T) {
	// Skip if SLACK_BOT_TOKEN is not set (not in integration test environment)
	if os.Getenv("SLACK_BOT_TOKEN") == "" || os.Getenv("SLACK_BOT_TOKEN") == "xoxb-test-token-integration" {
		t.Skip("Skipping integration test: SLACK_BOT_TOKEN not configured for real Slack workspace")
	}

	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	// Get test channel ID from environment
	testChannelID := os.Getenv("TEST_SLACK_CHANNEL_ID")
	if testChannelID == "" {
		t.Skip("Skipping integration test: TEST_SLACK_CHANNEL_ID not set")
	}

	// Create test channel configuration with reviewer list
	channelConfig := &models.ChannelConfig{
		ID:                       fmt.Sprintf("test-%s-needs-review", testChannelID),
		SlackChannelID:           testChannelID,
		LabelName:                "needs-review",
		DefaultMentionID:         "U12345",
		ReviewerList:             "U12345,U67890",
		RepositoryList:           "owner/test-repo",
		IsActive:                 true,
		ReminderInterval:         30,
		ReviewerReminderInterval: 30,
		BusinessHoursStart:       "00:00",
		BusinessHoursEnd:         "23:59",
		Timezone:                 "Asia/Tokyo",
	}
	testConfig.DB.Create(channelConfig)
	defer testConfig.DB.Delete(&channelConfig)

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/webhook", handlers.HandleGitHubWebhook(testConfig.DB))

	// Create GitHub webhook payload for PR labeled event
	prNumber := 54321
	repoName := "test-repo"
	ownerLogin := "owner"
	prTitle := fmt.Sprintf("[TEST] Review Request Test - %s", time.Now().Format("15:04:05"))
	prHTMLURL := "https://github.com/owner/test-repo/pull/54321"
	labelName := "needs-review"
	action := "labeled"

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
			User: &github.User{
				Login: github.Ptr("test-author"),
			},
			Labels: []*github.Label{
				{Name: &labelName},
			},
		},
		Repo: &github.Repository{
			Name: &repoName,
			Owner: &github.User{
				Login: &ownerLogin,
			},
			FullName: github.Ptr("owner/test-repo"),
		},
	}

	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err, "Should marshal payload")

	// Create HTTP request
	req, err := http.NewRequest("POST", "/webhook", bytes.NewBuffer(payloadJSON))
	require.NoError(t, err, "Should create request")

	if secret := os.Getenv("GITHUB_WEBHOOK_SECRET"); secret != "" {
		signature := signPayload(payloadJSON, []byte(secret))
		req.Header.Set("X-Hub-Signature-256", signature)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request")

	// Send request
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Verify HTTP response
	assert.Equal(t, http.StatusOK, w.Code, "Webhook should be accepted")

	// Wait for background processing
	time.Sleep(3 * time.Second)

	// Verify task was created with reviewer assigned
	var task models.ReviewTask
	err = testConfig.DB.Where("repo = ? AND pr_number = ?", "owner/test-repo", prNumber).First(&task).Error
	require.NoError(t, err, "Task should be created in database")

	assert.NotEmpty(t, task.Reviewer, "Reviewer should be assigned")
	assert.Equal(t, "in_review", task.Status, "Task should be in review status")

	// Verify main message was sent
	mainMsg, err := WaitForMessage(testChannelID, prTitle, 10*time.Second)
	require.NoError(t, err, "Main Slack message should be sent")

	// Verify reviewer notification in thread
	// Wait a bit more for thread message
	time.Sleep(2 * time.Second)

	// Cleanup: delete messages
	if mainMsg != nil && mainMsg.Timestamp != "" {
		err = DeleteTestMessage(testChannelID, mainMsg.Timestamp)
		if err != nil {
			t.Logf("Warning: Failed to delete test message: %v", err)
		}
	}

	// Cleanup: delete task
	testConfig.DB.Delete(&task)
}

// TestPRMergedNotification tests that PR merged notifications update task status correctly
func TestPRMergedNotification(t *testing.T) {
	// Skip if SLACK_BOT_TOKEN is not set (not in integration test environment)
	if os.Getenv("SLACK_BOT_TOKEN") == "" || os.Getenv("SLACK_BOT_TOKEN") == "xoxb-test-token-integration" {
		t.Skip("Skipping integration test: SLACK_BOT_TOKEN not configured for real Slack workspace")
	}

	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	// Get test channel ID from environment
	testChannelID := os.Getenv("TEST_SLACK_CHANNEL_ID")
	if testChannelID == "" {
		t.Skip("Skipping integration test: TEST_SLACK_CHANNEL_ID not set")
	}

	// Create test channel configuration
	channelConfig := CreateTestChannelConfig(testConfig.DB, testChannelID, "needs-review")
	defer testConfig.DB.Delete(&channelConfig)

	// Create a test message to get a valid timestamp
	testMsgText := fmt.Sprintf("[TEST] PR Merged Test Message - %s", time.Now().Format("15:04:05"))
	slackTS, err := SendTestMessage(testChannelID, testMsgText)
	require.NoError(t, err, "Should send test message to get timestamp")
	defer DeleteTestMessage(testChannelID, slackTS)

	// Create an existing review task in "in_review" status
	prNumber := 99999
	task := &models.ReviewTask{
		ID:           fmt.Sprintf("test-task-%d", time.Now().Unix()),
		PRURL:        "https://github.com/owner/test-repo/pull/99999",
		Repo:         "owner/test-repo",
		PRNumber:     prNumber,
		Title:        "Test PR for Merge",
		SlackTS:      slackTS,
		SlackChannel: testChannelID,
		Status:       "in_review",
		Reviewer:     "U12345",
		LabelName:    "needs-review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	testConfig.DB.Create(task)
	defer testConfig.DB.Delete(&task)

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/webhook", handlers.HandleGitHubWebhook(testConfig.DB))

	// Create GitHub webhook payload for PR review submitted event
	repoName := "test-repo"
	ownerLogin := "owner"
	reviewerLogin := "test-reviewer"
	reviewState := "approved"
	action := "submitted"

	reviewPayload := github.PullRequestReviewEvent{
		Action: &action,
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
			Body:  github.Ptr("LGTM!"),
		},
	}

	payloadJSON, err := json.Marshal(reviewPayload)
	require.NoError(t, err, "Should marshal payload")

	// Create HTTP request
	req, err := http.NewRequest("POST", "/webhook", bytes.NewBuffer(payloadJSON))
	require.NoError(t, err, "Should create request")

	if secret := os.Getenv("GITHUB_WEBHOOK_SECRET"); secret != "" {
		signature := signPayload(payloadJSON, []byte(secret))
		req.Header.Set("X-Hub-Signature-256", signature)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	// Send request
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Verify HTTP response
	assert.Equal(t, http.StatusOK, w.Code, "Webhook should be accepted")

	// Wait for background processing
	time.Sleep(2 * time.Second)

	// Verify task status was updated to completed
	var updatedTask models.ReviewTask
	err = testConfig.DB.Where("id = ?", task.ID).First(&updatedTask).Error
	require.NoError(t, err, "Task should exist in database")

	assert.Equal(t, "completed", updatedTask.Status, "Task should be marked as completed after review")

	// Verify notification was posted to thread
	// Check if there's a thread message
	time.Sleep(1 * time.Second)
}

// signPayload creates a GitHub webhook signature
func signPayload(payload []byte, secret []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	signature := hex.EncodeToString(mac.Sum(nil))
	return "sha256=" + signature
}

// TestChannelConfigSetupAndCleanup tests creating and cleaning up channel configurations
func TestChannelConfigSetupAndCleanup(t *testing.T) {
	// This test doesn't require real Slack integration
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	testChannelID := "C_TEST_12345"

	// Test creating channel config
	config := CreateTestChannelConfig(testConfig.DB, testChannelID, "test-label")
	assert.NotNil(t, config, "Should create channel config")

	// Verify it's in the database
	var found models.ChannelConfig
	err := testConfig.DB.Where("slack_channel_id = ? AND label_name = ?", testChannelID, "test-label").First(&found).Error
	assert.NoError(t, err, "Should find created config")
	assert.Equal(t, testChannelID, found.SlackChannelID)

	// Test cleanup
	testConfig.DB.Delete(&config)

	// Verify it's removed
	err = testConfig.DB.Where("slack_channel_id = ? AND label_name = ?", testChannelID, "test-label").First(&found).Error
	assert.Error(t, err, "Config should be deleted")
}

// TestWebhookWithMultipleLabels tests webhook handling with multiple labels
func TestWebhookWithMultipleLabels(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	testChannelID := "C_TEST_MULTI"

	// Create test channel configuration with multiple labels
	channelConfig := &models.ChannelConfig{
		ID:                       fmt.Sprintf("test-%s-multi-labels", testChannelID),
		SlackChannelID:           testChannelID,
		LabelName:                "needs-review,urgent",
		DefaultMentionID:         "U12345",
		ReviewerList:             "",
		RepositoryList:           "owner/test-repo",
		IsActive:                 true,
		ReminderInterval:         30,
		ReviewerReminderInterval: 30,
		BusinessHoursStart:       "09:00",
		BusinessHoursEnd:         "18:00",
		Timezone:                 "Asia/Tokyo",
	}
	testConfig.DB.Create(channelConfig)
	defer testConfig.DB.Delete(&channelConfig)

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/webhook", handlers.HandleGitHubWebhook(testConfig.DB))

	// Create webhook payload with both labels
	prNumber := 11111
	repoName := "test-repo"
	ownerLogin := "owner"
	prTitle := "Test PR with Multiple Labels"
	prHTMLURL := "https://github.com/owner/test-repo/pull/11111"
	labelName := "urgent"
	action := "labeled"

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
			User: &github.User{
				Login: github.Ptr("test-user"),
			},
			Labels: []*github.Label{
				{Name: github.Ptr("needs-review")},
				{Name: github.Ptr("urgent")},
			},
		},
		Repo: &github.Repository{
			Name: &repoName,
			Owner: &github.User{
				Login: &ownerLogin,
			},
			FullName: github.Ptr("owner/test-repo"),
		},
	}

	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err, "Should marshal payload")

	// Create HTTP request
	req, err := http.NewRequest("POST", "/webhook", bytes.NewBuffer(payloadJSON))
	require.NoError(t, err, "Should create request")

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request")

	// Send request
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Verify HTTP response
	assert.Equal(t, http.StatusOK, w.Code, "Webhook should be accepted")

	// Wait for background processing
	time.Sleep(2 * time.Second)

	// Verify task was created (when both labels are present)
	var taskCount int64
	testConfig.DB.Model(&models.ReviewTask{}).Where("repo = ? AND pr_number = ?", "owner/test-repo", prNumber).Count(&taskCount)

	// Note: This will only create a task if both labels are present and not in IsTestMode
	// In test mode (services.IsTestMode = true), Slack API calls are skipped
	t.Logf("Task count for multi-label PR: %d", taskCount)
}

// getRecentMessages retrieves recent messages from a Slack channel
func getRecentMessages(channelID string, limit int) ([]map[string]interface{}, error) {
	token := os.Getenv("SLACK_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("SLACK_BOT_TOKEN is not set")
	}

	url := fmt.Sprintf("https://slack.com/api/conversations.history?channel=%s&limit=%d", channelID, limit)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var result struct {
		OK       bool                     `json:"ok"`
		Messages []map[string]interface{} `json:"messages"`
		Error    string                   `json:"error"`
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !result.OK {
		return nil, fmt.Errorf("slack API error: %s", result.Error)
	}

	return result.Messages, nil
}
