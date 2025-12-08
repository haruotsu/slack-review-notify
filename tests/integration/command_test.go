package integration

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slack-review-notify/handlers"
	"slack-review-notify/models"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSetChannelCommand tests the set-mention command
func TestSetChannelCommand(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	testChannelID := "C_TEST_SET_CHANNEL"
	testUserID := "U_TEST_USER_123"
	labelName := "needs-review"

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", handlers.HandleSlackCommand(testConfig.DB))

	// Create Slack command payload for set-mention
	formData := url.Values{}
	formData.Set("command", "/slack-review-notify")
	formData.Set("text", fmt.Sprintf("%s set-mention <@%s>", labelName, testUserID))
	formData.Set("channel_id", testChannelID)
	formData.Set("user_id", "U_INVOKER")

	// Create HTTP request
	req, err := http.NewRequest("POST", "/slack/command", bytes.NewBufferString(formData.Encode()))
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
	assert.Equal(t, http.StatusOK, w.Code, "Command should be accepted")
	assert.Contains(t, w.Body.String(), "メンション先を", "Response should confirm mention setting")
	assert.Contains(t, w.Body.String(), testUserID, "Response should contain user ID")

	// Verify config was created in database
	var config models.ChannelConfig
	err = testConfig.DB.Where("slack_channel_id = ? AND label_name = ?", testChannelID, labelName).First(&config).Error
	require.NoError(t, err, "Config should be created in database")

	assert.Equal(t, testChannelID, config.SlackChannelID)
	assert.Equal(t, labelName, config.LabelName)
	assert.Equal(t, testUserID, config.DefaultMentionID)
	assert.True(t, config.IsActive, "Config should be active by default")

	// Cleanup
	testConfig.DB.Delete(&config)
}

// TestSetReviewerCommand tests the add-reviewer command
func TestSetReviewerCommand(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	testChannelID := "C_TEST_SET_REVIEWER"
	labelName := "needs-review"
	reviewer1 := "U_REVIEWER_1"
	reviewer2 := "U_REVIEWER_2"

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", handlers.HandleSlackCommand(testConfig.DB))

	// Create Slack command payload for add-reviewer
	formData := url.Values{}
	formData.Set("command", "/slack-review-notify")
	formData.Set("text", fmt.Sprintf("%s add-reviewer <@%s>,<@%s>", labelName, reviewer1, reviewer2))
	formData.Set("channel_id", testChannelID)
	formData.Set("user_id", "U_INVOKER")

	// Create HTTP request
	req, err := http.NewRequest("POST", "/slack/command", bytes.NewBufferString(formData.Encode()))
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
	assert.Equal(t, http.StatusOK, w.Code, "Command should be accepted")
	assert.Contains(t, w.Body.String(), "レビュワーリスト", "Response should confirm reviewer setting")

	// Verify config was created in database
	var config models.ChannelConfig
	err = testConfig.DB.Where("slack_channel_id = ? AND label_name = ?", testChannelID, labelName).First(&config).Error
	require.NoError(t, err, "Config should be created in database")

	assert.Equal(t, testChannelID, config.SlackChannelID)
	assert.Equal(t, labelName, config.LabelName)
	assert.Contains(t, config.ReviewerList, reviewer1, "Reviewer list should contain first reviewer")
	assert.Contains(t, config.ReviewerList, reviewer2, "Reviewer list should contain second reviewer")

	// Test adding another reviewer
	reviewer3 := "U_REVIEWER_3"
	formData.Set("text", fmt.Sprintf("%s add-reviewer <@%s>", labelName, reviewer3))

	req2, err := http.NewRequest("POST", "/slack/command", bytes.NewBufferString(formData.Encode()))
	require.NoError(t, err, "Should create second request")

	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	timestamp2 := fmt.Sprintf("%d", time.Now().Unix())
	signature2 := signSlackRequest(formData.Encode(), timestamp2)
	req2.Header.Set("X-Slack-Request-Timestamp", timestamp2)
	req2.Header.Set("X-Slack-Signature", signature2)

	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code, "Second command should be accepted")

	// Verify config was updated
	err = testConfig.DB.Where("slack_channel_id = ? AND label_name = ?", testChannelID, labelName).First(&config).Error
	require.NoError(t, err, "Config should still exist")

	assert.Contains(t, config.ReviewerList, reviewer1, "Reviewer list should still contain first reviewer")
	assert.Contains(t, config.ReviewerList, reviewer2, "Reviewer list should still contain second reviewer")
	assert.Contains(t, config.ReviewerList, reviewer3, "Reviewer list should contain third reviewer")

	// Cleanup
	testConfig.DB.Delete(&config)
}

// TestSetReviewerCountCommand tests the set-reviewer-reminder-interval command
func TestSetReviewerCountCommand(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	testChannelID := "C_TEST_REMINDER_INTERVAL"
	labelName := "needs-review"
	reminderInterval := 45

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", handlers.HandleSlackCommand(testConfig.DB))

	// Create Slack command payload for set-reviewer-reminder-interval
	formData := url.Values{}
	formData.Set("command", "/slack-review-notify")
	formData.Set("text", fmt.Sprintf("%s set-reviewer-reminder-interval %d", labelName, reminderInterval))
	formData.Set("channel_id", testChannelID)
	formData.Set("user_id", "U_INVOKER")

	// Create HTTP request
	req, err := http.NewRequest("POST", "/slack/command", bytes.NewBufferString(formData.Encode()))
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
	assert.Equal(t, http.StatusOK, w.Code, "Command should be accepted")
	assert.Contains(t, w.Body.String(), "リマインド頻度", "Response should confirm reminder interval setting")
	assert.Contains(t, w.Body.String(), fmt.Sprintf("%d分", reminderInterval), "Response should contain interval")

	// Verify config was created in database
	var config models.ChannelConfig
	err = testConfig.DB.Where("slack_channel_id = ? AND label_name = ?", testChannelID, labelName).First(&config).Error
	require.NoError(t, err, "Config should be created in database")

	assert.Equal(t, testChannelID, config.SlackChannelID)
	assert.Equal(t, labelName, config.LabelName)
	assert.Equal(t, reminderInterval, config.ReviewerReminderInterval)

	// Cleanup
	testConfig.DB.Delete(&config)
}

// TestShowConfigCommand tests the show command
func TestShowConfigCommand(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	testChannelID := "C_TEST_SHOW_CONFIG"
	labelName := "needs-review"

	// Create a test channel configuration
	channelConfig := &models.ChannelConfig{
		ID:                       uuid.NewString(),
		SlackChannelID:           testChannelID,
		LabelName:                labelName,
		DefaultMentionID:         "U_TEST_USER",
		ReviewerList:             "U_REVIEWER_1,U_REVIEWER_2",
		RepositoryList:           "owner/repo1,owner/repo2",
		IsActive:                 true,
		ReminderInterval:         30,
		ReviewerReminderInterval: 45,
		BusinessHoursStart:       "09:00",
		BusinessHoursEnd:         "18:00",
		Timezone:                 "Asia/Tokyo",
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
	}
	testConfig.DB.Create(channelConfig)
	defer testConfig.DB.Delete(&channelConfig)

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", handlers.HandleSlackCommand(testConfig.DB))

	// Create Slack command payload for show
	formData := url.Values{}
	formData.Set("command", "/slack-review-notify")
	formData.Set("text", fmt.Sprintf("%s show", labelName))
	formData.Set("channel_id", testChannelID)
	formData.Set("user_id", "U_INVOKER")

	// Create HTTP request
	req, err := http.NewRequest("POST", "/slack/command", bytes.NewBufferString(formData.Encode()))
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
	assert.Equal(t, http.StatusOK, w.Code, "Command should be accepted")

	responseBody := w.Body.String()
	assert.Contains(t, responseBody, labelName, "Response should contain label name")
	assert.Contains(t, responseBody, "有効", "Response should show active status")
	assert.Contains(t, responseBody, "U_TEST_USER", "Response should contain mention ID")
	assert.Contains(t, responseBody, "U_REVIEWER_1", "Response should contain first reviewer")
	assert.Contains(t, responseBody, "U_REVIEWER_2", "Response should contain second reviewer")
	assert.Contains(t, responseBody, "owner/repo1", "Response should contain first repository")
	assert.Contains(t, responseBody, "owner/repo2", "Response should contain second repository")
	assert.Contains(t, responseBody, "45分", "Response should contain reviewer reminder interval")
	assert.Contains(t, responseBody, "09:00", "Response should contain business hours start")
	assert.Contains(t, responseBody, "18:00", "Response should contain business hours end")
	assert.Contains(t, responseBody, "Asia/Tokyo", "Response should contain timezone")
}

// TestListCommand tests the show command without label name (showing all labels)
func TestListCommand(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	testChannelID := "C_TEST_LIST_ALL"

	// Create multiple test channel configurations
	config1 := &models.ChannelConfig{
		ID:               uuid.NewString(),
		SlackChannelID:   testChannelID,
		LabelName:        "needs-review",
		DefaultMentionID: "U_TEAM_REVIEW",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	testConfig.DB.Create(config1)
	defer testConfig.DB.Delete(&config1)

	config2 := &models.ChannelConfig{
		ID:               uuid.NewString(),
		SlackChannelID:   testChannelID,
		LabelName:        "security-review",
		DefaultMentionID: "U_TEAM_SECURITY",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	testConfig.DB.Create(config2)
	defer testConfig.DB.Delete(&config2)

	config3 := &models.ChannelConfig{
		ID:               uuid.NewString(),
		SlackChannelID:   testChannelID,
		LabelName:        "bug-review",
		DefaultMentionID: "U_TEAM_BUG",
		IsActive:         false,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	testConfig.DB.Create(config3)
	defer testConfig.DB.Delete(&config3)

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", handlers.HandleSlackCommand(testConfig.DB))

	// Create Slack command payload for show (without label name)
	formData := url.Values{}
	formData.Set("command", "/slack-review-notify")
	formData.Set("text", "show")
	formData.Set("channel_id", testChannelID)
	formData.Set("user_id", "U_INVOKER")

	// Create HTTP request
	req, err := http.NewRequest("POST", "/slack/command", bytes.NewBufferString(formData.Encode()))
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
	assert.Equal(t, http.StatusOK, w.Code, "Command should be accepted")

	responseBody := w.Body.String()
	assert.Contains(t, responseBody, "設定済みのラベル", "Response should contain header")
	assert.Contains(t, responseBody, "needs-review", "Response should contain first label")
	assert.Contains(t, responseBody, "security-review", "Response should contain second label")
	assert.Contains(t, responseBody, "bug-review", "Response should contain third label")
	assert.Contains(t, responseBody, "有効", "Response should contain active status")
	assert.Contains(t, responseBody, "無効", "Response should contain inactive status")
	assert.Contains(t, responseBody, "U_TEAM_REVIEW", "Response should contain first mention ID")
	assert.Contains(t, responseBody, "U_TEAM_SECURITY", "Response should contain second mention ID")
	assert.Contains(t, responseBody, "U_TEAM_BUG", "Response should contain third mention ID")
}

// TestAddRepositoryCommand tests the add-repo command
func TestAddRepositoryCommand(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	testChannelID := "C_TEST_ADD_REPO"
	labelName := "needs-review"

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", handlers.HandleSlackCommand(testConfig.DB))

	// Create Slack command payload for add-repo
	formData := url.Values{}
	formData.Set("command", "/slack-review-notify")
	formData.Set("text", fmt.Sprintf("%s add-repo owner/repo1,owner/repo2", labelName))
	formData.Set("channel_id", testChannelID)
	formData.Set("user_id", "U_INVOKER")

	// Create HTTP request
	req, err := http.NewRequest("POST", "/slack/command", bytes.NewBufferString(formData.Encode()))
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
	assert.Equal(t, http.StatusOK, w.Code, "Command should be accepted")
	assert.Contains(t, w.Body.String(), "通知対象リポジトリ", "Response should confirm repository addition")

	// Verify config was created in database
	var config models.ChannelConfig
	err = testConfig.DB.Where("slack_channel_id = ? AND label_name = ?", testChannelID, labelName).First(&config).Error
	require.NoError(t, err, "Config should be created in database")

	assert.Contains(t, config.RepositoryList, "owner/repo1", "Repository list should contain first repo")
	assert.Contains(t, config.RepositoryList, "owner/repo2", "Repository list should contain second repo")

	// Cleanup
	testConfig.DB.Delete(&config)
}

// TestClearReviewersCommand tests the clear-reviewers command
func TestClearReviewersCommand(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	testChannelID := "C_TEST_CLEAR_REVIEWERS"
	labelName := "needs-review"

	// Create a test channel configuration with reviewers
	channelConfig := &models.ChannelConfig{
		ID:             uuid.NewString(),
		SlackChannelID: testChannelID,
		LabelName:      labelName,
		ReviewerList:   "U_REVIEWER_1,U_REVIEWER_2,U_REVIEWER_3",
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	testConfig.DB.Create(channelConfig)
	defer testConfig.DB.Delete(&channelConfig)

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", handlers.HandleSlackCommand(testConfig.DB))

	// Create Slack command payload for clear-reviewers
	formData := url.Values{}
	formData.Set("command", "/slack-review-notify")
	formData.Set("text", fmt.Sprintf("%s clear-reviewers", labelName))
	formData.Set("channel_id", testChannelID)
	formData.Set("user_id", "U_INVOKER")

	// Create HTTP request
	req, err := http.NewRequest("POST", "/slack/command", bytes.NewBufferString(formData.Encode()))
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
	assert.Equal(t, http.StatusOK, w.Code, "Command should be accepted")
	assert.Contains(t, w.Body.String(), "クリア", "Response should confirm reviewer clearing")

	// Verify reviewer list was cleared in database
	var config models.ChannelConfig
	err = testConfig.DB.Where("slack_channel_id = ? AND label_name = ?", testChannelID, labelName).First(&config).Error
	require.NoError(t, err, "Config should still exist")

	assert.Empty(t, config.ReviewerList, "Reviewer list should be empty")
}

// TestActivateDeactivateCommand tests the activate and deactivate commands
func TestActivateDeactivateCommand(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	testChannelID := "C_TEST_ACTIVATE"
	labelName := "needs-review"

	// Create a test channel configuration
	channelConfig := &models.ChannelConfig{
		ID:             uuid.NewString(),
		SlackChannelID: testChannelID,
		LabelName:      labelName,
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	testConfig.DB.Create(channelConfig)
	defer testConfig.DB.Delete(&channelConfig)

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", handlers.HandleSlackCommand(testConfig.DB))

	// Test deactivate command
	formData := url.Values{}
	formData.Set("command", "/slack-review-notify")
	formData.Set("text", fmt.Sprintf("%s deactivate", labelName))
	formData.Set("channel_id", testChannelID)
	formData.Set("user_id", "U_INVOKER")

	req, err := http.NewRequest("POST", "/slack/command", bytes.NewBufferString(formData.Encode()))
	require.NoError(t, err, "Should create request")

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	signature := signSlackRequest(formData.Encode(), timestamp)
	req.Header.Set("X-Slack-Request-Timestamp", timestamp)
	req.Header.Set("X-Slack-Signature", signature)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code, "Command should be accepted")
	assert.Contains(t, w.Body.String(), "無効化", "Response should confirm deactivation")

	// Verify config was deactivated
	var config models.ChannelConfig
	err = testConfig.DB.Where("slack_channel_id = ? AND label_name = ?", testChannelID, labelName).First(&config).Error
	require.NoError(t, err, "Config should exist")
	assert.False(t, config.IsActive, "Config should be inactive")

	// Test activate command
	formData.Set("text", fmt.Sprintf("%s activate", labelName))

	req2, err := http.NewRequest("POST", "/slack/command", bytes.NewBufferString(formData.Encode()))
	require.NoError(t, err, "Should create second request")

	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	timestamp2 := fmt.Sprintf("%d", time.Now().Unix())
	signature2 := signSlackRequest(formData.Encode(), timestamp2)
	req2.Header.Set("X-Slack-Request-Timestamp", timestamp2)
	req2.Header.Set("X-Slack-Signature", signature2)

	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code, "Second command should be accepted")
	assert.Contains(t, w2.Body.String(), "有効化", "Response should confirm activation")

	// Verify config was reactivated
	err = testConfig.DB.Where("slack_channel_id = ? AND label_name = ?", testChannelID, labelName).First(&config).Error
	require.NoError(t, err, "Config should still exist")
	assert.True(t, config.IsActive, "Config should be active again")
}

// TestShowReviewersCommand tests the show-reviewers command
func TestShowReviewersCommand(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	testChannelID := "C_TEST_SHOW_REVIEWERS"
	labelName := "needs-review"

	// Create a test channel configuration with reviewers
	channelConfig := &models.ChannelConfig{
		ID:             uuid.NewString(),
		SlackChannelID: testChannelID,
		LabelName:      labelName,
		ReviewerList:   "U_REVIEWER_1,U_REVIEWER_2",
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	testConfig.DB.Create(channelConfig)
	defer testConfig.DB.Delete(&channelConfig)

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", handlers.HandleSlackCommand(testConfig.DB))

	// Create Slack command payload for show-reviewers
	formData := url.Values{}
	formData.Set("command", "/slack-review-notify")
	formData.Set("text", fmt.Sprintf("%s show-reviewers", labelName))
	formData.Set("channel_id", testChannelID)
	formData.Set("user_id", "U_INVOKER")

	// Create HTTP request
	req, err := http.NewRequest("POST", "/slack/command", bytes.NewBufferString(formData.Encode()))
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
	assert.Equal(t, http.StatusOK, w.Code, "Command should be accepted")

	responseBody := w.Body.String()
	assert.Contains(t, responseBody, "レビュワーリスト", "Response should contain header")
	assert.Contains(t, responseBody, "U_REVIEWER_1", "Response should contain first reviewer")
	assert.Contains(t, responseBody, "U_REVIEWER_2", "Response should contain second reviewer")
}


// TestHelpCommand tests the help command
func TestHelpCommand(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	testChannelID := "C_TEST_HELP"

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", handlers.HandleSlackCommand(testConfig.DB))

	// Create Slack command payload for help
	formData := url.Values{}
	formData.Set("command", "/slack-review-notify")
	formData.Set("text", "help")
	formData.Set("channel_id", testChannelID)
	formData.Set("user_id", "U_INVOKER")

	// Create HTTP request
	req, err := http.NewRequest("POST", "/slack/command", bytes.NewBufferString(formData.Encode()))
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
	assert.Equal(t, http.StatusOK, w.Code, "Command should be accepted")

	responseBody := w.Body.String()
	assert.Contains(t, responseBody, "Review通知Bot設定コマンド", "Response should contain help header")
	assert.Contains(t, responseBody, "set-mention", "Response should contain set-mention command")
	assert.Contains(t, responseBody, "add-reviewer", "Response should contain add-reviewer command")
	assert.Contains(t, responseBody, "show", "Response should contain show command")
	assert.Contains(t, responseBody, "activate", "Response should contain activate command")
}

// TestMultipleLabelConfig tests configuring multiple labels in the same channel
func TestMultipleLabelConfig(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	testChannelID := "C_TEST_MULTI_LABEL"

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", handlers.HandleSlackCommand(testConfig.DB))

	// Configure first label
	formData1 := url.Values{}
	formData1.Set("command", "/slack-review-notify")
	formData1.Set("text", "bug-fix set-mention <@U_BUG_TEAM>")
	formData1.Set("channel_id", testChannelID)
	formData1.Set("user_id", "U_INVOKER")

	req1, err := http.NewRequest("POST", "/slack/command", bytes.NewBufferString(formData1.Encode()))
	require.NoError(t, err, "Should create first request")

	req1.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	timestamp1 := fmt.Sprintf("%d", time.Now().Unix())
	signature1 := signSlackRequest(formData1.Encode(), timestamp1)
	req1.Header.Set("X-Slack-Request-Timestamp", timestamp1)
	req1.Header.Set("X-Slack-Signature", signature1)

	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)

	assert.Equal(t, http.StatusOK, w1.Code, "First command should be accepted")

	// Configure second label
	formData2 := url.Values{}
	formData2.Set("command", "/slack-review-notify")
	formData2.Set("text", "feature set-mention <@U_FEATURE_TEAM>")
	formData2.Set("channel_id", testChannelID)
	formData2.Set("user_id", "U_INVOKER")

	req2, err := http.NewRequest("POST", "/slack/command", bytes.NewBufferString(formData2.Encode()))
	require.NoError(t, err, "Should create second request")

	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	timestamp2 := fmt.Sprintf("%d", time.Now().Unix())
	signature2 := signSlackRequest(formData2.Encode(), timestamp2)
	req2.Header.Set("X-Slack-Request-Timestamp", timestamp2)
	req2.Header.Set("X-Slack-Signature", signature2)

	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w2.Code, "Second command should be accepted")

	// Verify both configs exist independently
	var configs []models.ChannelConfig
	err = testConfig.DB.Where("slack_channel_id = ?", testChannelID).Find(&configs).Error
	require.NoError(t, err, "Should find configs")
	assert.Equal(t, 2, len(configs), "Should have two configs")

	// Find each config
	var bugConfig, featureConfig models.ChannelConfig
	for _, config := range configs {
		if config.LabelName == "bug-fix" {
			bugConfig = config
		} else if config.LabelName == "feature" {
			featureConfig = config
		}
	}

	assert.Equal(t, "bug-fix", bugConfig.LabelName)
	assert.Equal(t, "U_BUG_TEAM", bugConfig.DefaultMentionID)

	assert.Equal(t, "feature", featureConfig.LabelName)
	assert.Equal(t, "U_FEATURE_TEAM", featureConfig.DefaultMentionID)

	// Cleanup
	testConfig.DB.Delete(&bugConfig)
	testConfig.DB.Delete(&featureConfig)
}

// TestCommandWithSpacesInLabelName tests commands with quoted label names containing spaces
func TestCommandWithSpacesInLabelName(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	testChannelID := "C_TEST_SPACES"
	labelName := "needs review" // Label with space

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", handlers.HandleSlackCommand(testConfig.DB))

	// Create Slack command payload with quoted label name
	formData := url.Values{}
	formData.Set("command", "/slack-review-notify")
	formData.Set("text", fmt.Sprintf("\"%s\" set-mention <@U_TEST_USER>", labelName))
	formData.Set("channel_id", testChannelID)
	formData.Set("user_id", "U_INVOKER")

	// Create HTTP request
	req, err := http.NewRequest("POST", "/slack/command", bytes.NewBufferString(formData.Encode()))
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
	assert.Equal(t, http.StatusOK, w.Code, "Command should be accepted")

	// Verify config was created with the correct label name
	var config models.ChannelConfig
	err = testConfig.DB.Where("slack_channel_id = ? AND label_name = ?", testChannelID, labelName).First(&config).Error
	require.NoError(t, err, "Config should be created in database")

	assert.Equal(t, labelName, config.LabelName, "Label name should preserve spaces")

	// Cleanup
	testConfig.DB.Delete(&config)
}

// TestInvalidCommandParameters tests error handling for invalid command parameters
func TestInvalidCommandParameters(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	testChannelID := "C_TEST_INVALID"

	// Setup Gin router
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", handlers.HandleSlackCommand(testConfig.DB))

	tests := []struct {
		name           string
		commandText    string
		expectedInBody string
	}{
		{
			name:           "set-mention without user ID",
			commandText:    "needs-review set-mention",
			expectedInBody: "ユーザーIDを指定",
		},
		{
			name:           "add-reviewer without user ID",
			commandText:    "needs-review add-reviewer",
			expectedInBody: "ユーザーIDをカンマ区切りで指定",
		},
		{
			name:           "set-reviewer-reminder-interval without interval",
			commandText:    "needs-review set-reviewer-reminder-interval",
			expectedInBody: "リマインド頻度を分単位で指定",
		},
		{
			name:           "add-repo without repository",
			commandText:    "needs-review add-repo",
			expectedInBody: "リポジトリ名をカンマ区切りで指定",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formData := url.Values{}
			formData.Set("command", "/slack-review-notify")
			formData.Set("text", tt.commandText)
			formData.Set("channel_id", testChannelID)
			formData.Set("user_id", "U_INVOKER")

			req, err := http.NewRequest("POST", "/slack/command", bytes.NewBufferString(formData.Encode()))
			require.NoError(t, err, "Should create request")

			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			timestamp := fmt.Sprintf("%d", time.Now().Unix())
			signature := signSlackRequest(formData.Encode(), timestamp)
			req.Header.Set("X-Slack-Request-Timestamp", timestamp)
			req.Header.Set("X-Slack-Signature", signature)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code, "Command should return 200 with error message")

			responseBody := w.Body.String()
			// Check if response contains expected error message or instruction
			if !strings.Contains(responseBody, tt.expectedInBody) {
				t.Errorf("Expected response to contain '%s', got: %s", tt.expectedInBody, responseBody)
			}
		})
	}
}
