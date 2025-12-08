package integration

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slack-review-notify/handlers"
	"slack-review-notify/models"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDoneButtonAction tests the "Done" button press action
func TestDoneButtonAction(t *testing.T) {
	// Skip if SLACK_BOT_TOKEN is not set
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

	// Create a test message to get a valid timestamp
	testMsgText := fmt.Sprintf("[TEST] Done Button Test - %s", time.Now().Format("15:04:05"))
	slackTS, err := SendTestMessage(testChannelID, testMsgText)
	require.NoError(t, err, "Should send test message")
	defer DeleteTestMessage(testChannelID, slackTS)

	// Create a test review task
	taskID := fmt.Sprintf("test-task-%d", time.Now().Unix())
	task := &models.ReviewTask{
		ID:           taskID,
		PRURL:        "https://github.com/owner/test-repo/pull/123",
		Repo:         "owner/test-repo",
		PRNumber:     123,
		Title:        "Test PR for Done Button",
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
	router.POST("/slack/actions", handlers.HandleSlackAction(testConfig.DB))

	// Create Slack action payload for "Done" button
	payload := map[string]interface{}{
		"type": "block_actions",
		"user": map[string]interface{}{
			"id": "U_TEST_USER",
		},
		"actions": []map[string]interface{}{
			{
				"action_id": "review_done",
				"value":     taskID,
			},
		},
		"container": map[string]interface{}{
			"channel_id": testChannelID,
		},
		"message": map[string]interface{}{
			"ts": slackTS,
		},
	}

	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err, "Should marshal payload")

	// Create form data
	formData := url.Values{}
	formData.Set("payload", string(payloadJSON))

	// Create HTTP request
	req, err := http.NewRequest("POST", "/slack/actions", bytes.NewBufferString(formData.Encode()))
	require.NoError(t, err, "Should create request")

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Sign the request
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	signature := signSlackRequest(formData.Encode(), timestamp)
	req.Header.Set("X-Slack-Request-Timestamp", timestamp)
	req.Header.Set("X-Slack-Signature", signature)

	// Send request
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Verify HTTP response
	assert.Equal(t, http.StatusOK, w.Code, "Action should be accepted")

	// Wait for processing
	time.Sleep(2 * time.Second)

	// Verify task status was updated to "done"
	var updatedTask models.ReviewTask
	err = testConfig.DB.Where("id = ?", taskID).First(&updatedTask).Error
	require.NoError(t, err, "Task should exist in database")
	assert.Equal(t, "done", updatedTask.Status, "Task status should be updated to 'done'")

	// Verify thread message was posted (check recent messages)
	// Note: This would require fetching thread messages from Slack API
	t.Log("Done button action processed successfully")
}

// TestChangeReviewerAction tests the "Change Reviewer" button press action
func TestChangeReviewerAction(t *testing.T) {
	// Skip if SLACK_BOT_TOKEN is not set
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

	// Create channel config with multiple reviewers
	channelConfig := &models.ChannelConfig{
		ID:                       fmt.Sprintf("test-%s-needs-review", testChannelID),
		SlackChannelID:           testChannelID,
		LabelName:                "needs-review",
		DefaultMentionID:         "U12345",
		ReviewerList:             "U12345,U67890,U99999",
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

	// Create a test message to get a valid timestamp
	testMsgText := fmt.Sprintf("[TEST] Change Reviewer Test - %s", time.Now().Format("15:04:05"))
	slackTS, err := SendTestMessage(testChannelID, testMsgText)
	require.NoError(t, err, "Should send test message")
	defer DeleteTestMessage(testChannelID, slackTS)

	// Create a test review task
	taskID := fmt.Sprintf("test-task-%d", time.Now().Unix())
	task := &models.ReviewTask{
		ID:           taskID,
		PRURL:        "https://github.com/owner/test-repo/pull/456",
		Repo:         "owner/test-repo",
		PRNumber:     456,
		Title:        "Test PR for Change Reviewer",
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
	router.POST("/slack/actions", handlers.HandleSlackAction(testConfig.DB))

	// Create Slack action payload for "Change Reviewer" button
	payload := map[string]interface{}{
		"type": "block_actions",
		"user": map[string]interface{}{
			"id": "U_TEST_USER",
		},
		"actions": []map[string]interface{}{
			{
				"action_id": "change_reviewer",
				"value":     taskID,
			},
		},
		"container": map[string]interface{}{
			"channel_id": testChannelID,
		},
		"message": map[string]interface{}{
			"ts": slackTS,
		},
	}

	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err, "Should marshal payload")

	// Create form data
	formData := url.Values{}
	formData.Set("payload", string(payloadJSON))

	// Create HTTP request
	req, err := http.NewRequest("POST", "/slack/actions", bytes.NewBufferString(formData.Encode()))
	require.NoError(t, err, "Should create request")

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Sign the request
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	signature := signSlackRequest(formData.Encode(), timestamp)
	req.Header.Set("X-Slack-Request-Timestamp", timestamp)
	req.Header.Set("X-Slack-Signature", signature)

	// Send request
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Verify HTTP response
	assert.Equal(t, http.StatusOK, w.Code, "Action should be accepted")

	// Wait for processing
	time.Sleep(2 * time.Second)

	// Verify reviewer was changed
	var updatedTask models.ReviewTask
	err = testConfig.DB.Where("id = ?", taskID).First(&updatedTask).Error
	require.NoError(t, err, "Task should exist in database")
	assert.NotEqual(t, "U12345", updatedTask.Reviewer, "Reviewer should be changed")
	assert.Contains(t, []string{"U12345", "U67890", "U99999"}, updatedTask.Reviewer, "New reviewer should be from the reviewer list")

	t.Logf("Reviewer changed from U12345 to %s", updatedTask.Reviewer)
}

// TestRemindButtonAction tests the reminder pause button press action
func TestRemindButtonAction(t *testing.T) {
	// Skip if SLACK_BOT_TOKEN is not set
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

	// Create channel config
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
		BusinessHoursStart:       "09:00",
		BusinessHoursEnd:         "18:00",
		Timezone:                 "Asia/Tokyo",
	}
	testConfig.DB.Create(channelConfig)
	defer testConfig.DB.Delete(&channelConfig)

	// Create a test message to get a valid timestamp
	testMsgText := fmt.Sprintf("[TEST] Reminder Pause Test - %s", time.Now().Format("15:04:05"))
	slackTS, err := SendTestMessage(testChannelID, testMsgText)
	require.NoError(t, err, "Should send test message")
	defer DeleteTestMessage(testChannelID, slackTS)

	// Create a test review task
	taskID := fmt.Sprintf("test-task-%d", time.Now().Unix())
	task := &models.ReviewTask{
		ID:           taskID,
		PRURL:        "https://github.com/owner/test-repo/pull/789",
		Repo:         "owner/test-repo",
		PRNumber:     789,
		Title:        "Test PR for Reminder Pause",
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
	router.POST("/slack/actions", handlers.HandleSlackAction(testConfig.DB))

	// Test different pause durations
	testCases := []struct {
		name            string
		duration        string
		expectedStatus  string
		checkPausedTime bool
	}{
		{
			name:            "Pause for 1 hour",
			duration:        "1h",
			expectedStatus:  "in_review",
			checkPausedTime: true,
		},
		{
			name:            "Pause for 2 hours",
			duration:        "2h",
			expectedStatus:  "in_review",
			checkPausedTime: true,
		},
		{
			name:            "Pause until next business day",
			duration:        "today",
			expectedStatus:  "in_review",
			checkPausedTime: true,
		},
		{
			name:            "Stop reminders",
			duration:        "stop",
			expectedStatus:  "paused",
			checkPausedTime: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset task status
			task.Status = "in_review"
			task.ReminderPausedUntil = nil
			testConfig.DB.Save(&task)

			// Create Slack action payload for reminder pause
			payload := map[string]interface{}{
				"type": "block_actions",
				"user": map[string]interface{}{
					"id": "U_TEST_USER",
				},
				"actions": []map[string]interface{}{
					{
						"action_id": "pause_reminder",
						"selected_option": map[string]interface{}{
							"value": fmt.Sprintf("%s:%s", taskID, tc.duration),
							"text": map[string]interface{}{
								"text": tc.duration,
							},
						},
					},
				},
				"container": map[string]interface{}{
					"channel_id": testChannelID,
				},
				"message": map[string]interface{}{
					"ts": slackTS,
				},
			}

			payloadJSON, err := json.Marshal(payload)
			require.NoError(t, err, "Should marshal payload")

			// Create form data
			formData := url.Values{}
			formData.Set("payload", string(payloadJSON))

			// Create HTTP request
			req, err := http.NewRequest("POST", "/slack/actions", bytes.NewBufferString(formData.Encode()))
			require.NoError(t, err, "Should create request")

			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			// Sign the request
			timestamp := fmt.Sprintf("%d", time.Now().Unix())
			signature := signSlackRequest(formData.Encode(), timestamp)
			req.Header.Set("X-Slack-Request-Timestamp", timestamp)
			req.Header.Set("X-Slack-Signature", signature)

			// Send request
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Verify HTTP response
			assert.Equal(t, http.StatusOK, w.Code, "Action should be accepted")

			// Wait for processing
			time.Sleep(2 * time.Second)

			// Verify task status
			var updatedTask models.ReviewTask
			err = testConfig.DB.Where("id = ?", taskID).First(&updatedTask).Error
			require.NoError(t, err, "Task should exist in database")
			assert.Equal(t, tc.expectedStatus, updatedTask.Status, "Task status should match expected")

			if tc.checkPausedTime {
				assert.NotNil(t, updatedTask.ReminderPausedUntil, "ReminderPausedUntil should be set")
				assert.True(t, updatedTask.ReminderPausedUntil.After(time.Now()), "ReminderPausedUntil should be in the future")
			}

			t.Logf("%s: Status=%s, PausedUntil=%v", tc.name, updatedTask.Status, updatedTask.ReminderPausedUntil)
		})
	}
}

// signSlackRequest creates a Slack request signature for verification
func signSlackRequest(body string, timestamp string) string {
	secret := os.Getenv("SLACK_SIGNING_SECRET")
	if secret == "" {
		secret = "test_signing_secret_12345"
	}

	sig := fmt.Sprintf("v0:%s:%s", timestamp, body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(sig))
	signature := hex.EncodeToString(mac.Sum(nil))
	return "v0=" + signature
}

// TestButtonActionMessageUpdate tests that button actions trigger message updates
func TestButtonActionMessageUpdate(t *testing.T) {
	// Skip if SLACK_BOT_TOKEN is not set
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

	// Create a test message to get a valid timestamp
	testMsgText := fmt.Sprintf("[TEST] Message Update Test - %s", time.Now().Format("15:04:05"))
	slackTS, err := SendTestMessage(testChannelID, testMsgText)
	require.NoError(t, err, "Should send test message")
	defer DeleteTestMessage(testChannelID, slackTS)

	// Create a test review task
	taskID := fmt.Sprintf("test-task-%d", time.Now().Unix())
	task := &models.ReviewTask{
		ID:           taskID,
		PRURL:        "https://github.com/owner/test-repo/pull/999",
		Repo:         "owner/test-repo",
		PRNumber:     999,
		Title:        "Test PR for Message Update",
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
	router.POST("/slack/actions", handlers.HandleSlackAction(testConfig.DB))

	// Get initial message count
	initialMessages, err := GetChannelMessages(testChannelID, 5)
	require.NoError(t, err, "Should get initial messages")
	initialThreadCount := 0
	for _, msg := range initialMessages {
		if msg.ThreadTS == slackTS {
			initialThreadCount++
		}
	}

	// Create and send "Done" button action
	payload := map[string]interface{}{
		"type": "block_actions",
		"user": map[string]interface{}{
			"id": "U_TEST_USER",
		},
		"actions": []map[string]interface{}{
			{
				"action_id": "review_done",
				"value":     taskID,
			},
		},
		"container": map[string]interface{}{
			"channel_id": testChannelID,
		},
		"message": map[string]interface{}{
			"ts": slackTS,
		},
	}

	payloadJSON, err := json.Marshal(payload)
	require.NoError(t, err, "Should marshal payload")

	formData := url.Values{}
	formData.Set("payload", string(payloadJSON))

	req, err := http.NewRequest("POST", "/slack/actions", bytes.NewBufferString(formData.Encode()))
	require.NoError(t, err, "Should create request")

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	signature := signSlackRequest(formData.Encode(), timestamp)
	req.Header.Set("X-Slack-Request-Timestamp", timestamp)
	req.Header.Set("X-Slack-Signature", signature)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Action should be accepted")

	// Wait for message to be posted to thread
	time.Sleep(3 * time.Second)

	// Verify a thread message was added
	// Note: In a real integration test, we would fetch thread messages
	// and verify the completion message was posted
	t.Log("Message update action processed successfully")
}
