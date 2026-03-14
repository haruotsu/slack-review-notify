package services

import (
	"fmt"
	"os"
	"slack-review-notify/models"
	"testing"
	"time"

	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
)

func TestSendSlackMessage(t *testing.T) {
	// Save environment variables before test and restore after
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()

	// Set test environment variables
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	// Set up mocks
	defer gock.Off() // Clear mocks when test ends

	// Mock for success case
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		MatchHeader("Content-Type", "application/json").
		Reply(200).
		JSON(map[string]interface{}{
			"ok":      true,
			"channel": "C12345",
			"ts":      "1234.5678",
		})

	// Execute function
	ts, channel, err := SendSlackMessage(
		"https://github.com/owner/repo/pull/1",
		"Test PR Title",
		"C12345",
		"U12345",
		"",   // PR creator's Slack ID (empty for test)
		"ja", // language
	)

	// Assertions
	assert.NoError(t, err)
	assert.Equal(t, "1234.5678", ts)
	assert.Equal(t, "C12345", channel)
	assert.True(t, gock.IsDone(), "Not all mocks were used")

	// Test error case
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{
			"ok":    false,
			"error": "channel_not_found",
		})

	// Execute function
	_, _, err = SendSlackMessage(
		"https://github.com/owner/repo/pull/1",
		"Test PR Title",
		"INVALID",
		"U12345",
		"",   // PR creator's Slack ID (empty for test)
		"ja", // language
	)

	// Assertions
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "channel_not_found")
	assert.True(t, gock.IsDone(), "Not all mocks were used")
}

func TestPostToThread(t *testing.T) {
	// Save environment variables before test and restore after
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()

	// Set test environment variables
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	// Set up mocks
	defer gock.Off() // Clear mocks when test ends

	// Mock for success case
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		MatchHeader("Content-Type", "application/json").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
		})

	// Execute function
	err := PostToThread("C12345", "1234.5678", "テストメッセージ")

	// Assertions
	assert.NoError(t, err)
	assert.True(t, gock.IsDone(), "Not all mocks were used")

	// Test error case
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{
			"ok":    false,
			"error": "invalid_thread_ts",
		})

	// Execute function
	err = PostToThread("C12345", "invalid", "テストメッセージ")

	// Assertions
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid_thread_ts")
	assert.True(t, gock.IsDone(), "Not all mocks were used")
}

func TestIsChannelArchived(t *testing.T) {
	// Save environment variables before test and restore after
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()

	// Set test environment variables
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	// Set up mocks
	defer gock.Off() // Clear mocks when test ends

	// Mock for an archived channel
	gock.New("https://slack.com").
		Get("/api/conversations.info").
		MatchParam("channel", "C12345").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
			"channel": map[string]interface{}{
				"id":          "C12345",
				"is_archived": true,
			},
		})

	// Execute function
	isArchived, err := IsChannelArchived("C12345")

	// Assertions
	assert.NoError(t, err)
	assert.True(t, isArchived)
	assert.True(t, gock.IsDone(), "Not all mocks were used")

	// Mock for a non-archived channel
	gock.New("https://slack.com").
		Get("/api/conversations.info").
		MatchParam("channel", "C67890").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
			"channel": map[string]interface{}{
				"id":          "C67890",
				"is_archived": false,
			},
		})

	// Execute function
	isArchived, err = IsChannelArchived("C67890")

	// Assertions
	assert.NoError(t, err)
	assert.False(t, isArchived)
	assert.True(t, gock.IsDone(), "Not all mocks were used")

	// Mock for a non-existent channel
	gock.New("https://slack.com").
		Get("/api/conversations.info").
		MatchParam("channel", "INVALID").
		Reply(200).
		JSON(map[string]interface{}{
			"ok":    false,
			"error": "channel_not_found",
		})

	// Execute function
	isArchived, err = IsChannelArchived("INVALID")

	// Assertions
	assert.True(t, isArchived) // Non-existent channels are also treated as archived
	assert.NoError(t, err)     // Not an error, simply returns true
	assert.True(t, gock.IsDone(), "Not all mocks were used")
}

func TestSendReminderMessage(t *testing.T) {
	// Set up test DB
	db := setupTestDB(t)

	// Save environment variables before test and restore after
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()

	// Set test environment variables
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	// Set up mocks
	defer gock.Off() // Clear mocks when test ends

	// Mock for channel info retrieval
	gock.New("https://slack.com").
		Get("/api/conversations.info").
		MatchParam("channel", "C12345").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
			"channel": map[string]interface{}{
				"id":          "C12345",
				"is_archived": false,
			},
		})

	// Mock for message sending
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
		})

	// Create a test task
	task := models.ReviewTask{
		ID:           "test-id",
		PRURL:        "https://github.com/owner/repo/pull/1",
		Repo:         "owner/repo",
		PRNumber:     1,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Status:       "pending",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Execute function
	err := SendReviewerReminderMessage(db, task)

	// Assertions
	assert.NoError(t, err)
	assert.True(t, gock.IsDone(), "Not all mocks were used")

	// Test when channel is archived
	gock.New("https://slack.com").
		Get("/api/conversations.info").
		MatchParam("channel", "C67890").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
			"channel": map[string]interface{}{
				"id":          "C67890",
				"is_archived": true,
			},
		})

	// Create test task and channel config
	task2 := models.ReviewTask{
		ID:           "test-id-2",
		PRURL:        "https://github.com/owner/repo/pull/2",
		Repo:         "owner/repo",
		PRNumber:     2,
		Title:        "Test PR 2",
		SlackTS:      "1234.5679",
		SlackChannel: "C67890",
		Status:       "pending",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	config := models.ChannelConfig{
		ID:               "config-id",
		SlackChannelID:   "C67890",
		DefaultMentionID: "U12345",
		RepositoryList:   "owner/repo",
		LabelName:        "needs-review",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	db.Create(&config)

	// Execute function
	err = SendReviewerReminderMessage(db, task2)

	// Assertions
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "channel is archived")

	// Verify DB was updated
	var updatedTask models.ReviewTask
	db.Where("id = ?", "test-id-2").First(&updatedTask)
	assert.Equal(t, "archived", updatedTask.Status)

	var updatedConfig models.ChannelConfig
	db.Where("slack_channel_id = ?", "C67890").First(&updatedConfig)
	assert.False(t, updatedConfig.IsActive)

	assert.True(t, gock.IsDone(), "Not all mocks were used")
}

func TestSendReviewerReminderMessage(t *testing.T) {
	// Set up test DB
	db := setupTestDB(t)

	// Save environment variables before test and restore after
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()

	// Set test environment variables
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	// Set up mocks
	defer gock.Off() // Clear mocks when test ends

	// Mock for channel info retrieval
	gock.New("https://slack.com").
		Get("/api/conversations.info").
		MatchParam("channel", "C12345").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
			"channel": map[string]interface{}{
				"id":          "C12345",
				"is_archived": false,
			},
		})

	// Mock for message sending
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
		})

	// Create a test task
	task := models.ReviewTask{
		ID:           "test-id",
		PRURL:        "https://github.com/owner/repo/pull/1",
		Repo:         "owner/repo",
		PRNumber:     1,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Reviewer:     "U12345",
		Status:       "in_review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Execute function
	err := SendReviewerReminderMessage(db, task)

	// Assertions
	assert.NoError(t, err)
	assert.True(t, gock.IsDone(), "Not all mocks were used")

	// Test when channel is archived
	gock.New("https://slack.com").
		Get("/api/conversations.info").
		MatchParam("channel", "C67890").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
			"channel": map[string]interface{}{
				"id":          "C67890",
				"is_archived": true,
			},
		})

	// Create test task and channel config
	task2 := models.ReviewTask{
		ID:           "test-id-2",
		PRURL:        "https://github.com/owner/repo/pull/2",
		Repo:         "owner/repo",
		PRNumber:     2,
		Title:        "Test PR 2",
		SlackTS:      "1234.5679",
		SlackChannel: "C67890",
		Reviewer:     "U67890",
		Status:       "in_review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	config := models.ChannelConfig{
		ID:               "config-id",
		SlackChannelID:   "C67890",
		DefaultMentionID: "U12345",
		RepositoryList:   "owner/repo",
		LabelName:        "needs-review",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	db.Create(&config)

	// Execute function
	err = SendReviewerReminderMessage(db, task2)

	// Assertions
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "channel is archived")

	// Verify DB was updated
	var updatedTask models.ReviewTask
	db.Where("id = ?", "test-id-2").First(&updatedTask)
	assert.Equal(t, "archived", updatedTask.Status)

	var updatedConfig models.ChannelConfig
	db.Where("slack_channel_id = ?", "C67890").First(&updatedConfig)
	assert.False(t, updatedConfig.IsActive)

	assert.True(t, gock.IsDone(), "Not all mocks were used")
}

func TestSendReminderPausedMessage(t *testing.T) {
	// Save environment variables before test and restore after
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()

	// Set test environment variables
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	// Set up mocks
	defer gock.Off() // Clear mocks when test ends

	testCases := []struct {
		name     string
		duration string
		message  string
	}{
		{"1 hour", "1h", "はい！1時間リマインドをストップします！"},
		{"2 hours", "2h", "はい！2時間リマインドをストップします！"},
		{"4 hours", "4h", "はい！4時間リマインドをストップします！"},
		{"today", "today", "今日はもうリマインドしません。翌営業日の朝に再開します！"},
		{"full stop", "stop", "リマインダーを完全に停止しました。レビュー担当者が決まるまで通知しません。"},
		{"default", "unknown", "リマインドをストップします！"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Mock for thread message sending
			gock.New("https://slack.com").
				Post("/api/chat.postMessage").
				MatchHeader("Authorization", "Bearer test-token").
				Reply(200).
				JSON(map[string]interface{}{
					"ok": true,
				})

			// Create a test task
			task := models.ReviewTask{
				ID:           "test-id",
				SlackTS:      "1234.5678",
				SlackChannel: "C12345",
				Status:       "pending",
			}

			// Execute function
			err := SendReminderPausedMessage(task, tc.duration)

			// Assertions
			assert.NoError(t, err)
			assert.True(t, gock.IsDone(), "Not all mocks were used")
		})
	}
}

// Test for IsChannelRelatedError
func TestIsChannelRelatedError(t *testing.T) {
	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"not_in_channel", fmt.Errorf("slack error: not_in_channel"), true},
		{"channel_not_found", fmt.Errorf("slack error: channel_not_found"), true},
		{"is_archived", fmt.Errorf("slack error: is_archived"), true},
		{"missing_scope", fmt.Errorf("slack error: missing_scope"), true},
		{"other error", fmt.Errorf("other error"), false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := IsChannelRelatedError(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// Test for GetNextBusinessDayMorning function
func TestGetNextBusinessDayMorning(t *testing.T) {
	// Define JST timezone
	jst, _ := time.LoadLocation("Asia/Tokyo")

	testCases := []struct {
		name     string
		baseTime time.Time
		expected time.Time
	}{
		{
			name:     "Monday_9am_expect_same_day_10am",
			baseTime: time.Date(2024, 1, 8, 9, 0, 0, 0, jst),  // Monday 9:00 JST
			expected: time.Date(2024, 1, 8, 10, 0, 0, 0, jst), // Monday 10:00 JST
		},
		{
			name:     "Monday_2pm_expect_Tuesday_10am",
			baseTime: time.Date(2024, 1, 8, 14, 0, 0, 0, jst), // Monday 14:00 JST
			expected: time.Date(2024, 1, 9, 10, 0, 0, 0, jst), // Tuesday 10:00 JST
		},
		{
			name:     "Friday_2pm_expect_Monday_10am",
			baseTime: time.Date(2024, 1, 12, 14, 0, 0, 0, jst), // Friday 14:00 JST
			expected: time.Date(2024, 1, 15, 10, 0, 0, 0, jst), // Monday 10:00 JST
		},
		{
			name:     "Saturday_2pm_expect_Monday_10am",
			baseTime: time.Date(2024, 1, 13, 14, 0, 0, 0, jst), // Saturday 14:00 JST
			expected: time.Date(2024, 1, 15, 10, 0, 0, 0, jst), // Monday 10:00 JST
		},
		{
			name:     "Sunday_2pm_expect_Monday_10am",
			baseTime: time.Date(2024, 1, 14, 14, 0, 0, 0, jst), // Sunday 14:00 JST
			expected: time.Date(2024, 1, 15, 10, 0, 0, 0, jst), // Monday 10:00 JST
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GetNextBusinessDayMorningWithConfig(tc.baseTime, nil)
			assert.Equal(t, tc.expected, result)
		})
	}

	result := GetNextBusinessDayMorningWithConfig(time.Now(), nil)

	// Result should be set to 10:00
	assert.Equal(t, 10, result.Hour(), "Hour should be set to 10")
	assert.Equal(t, 0, result.Minute(), "Minute should be set to 0")
	assert.Equal(t, 0, result.Second(), "Second should be set to 0")

	// Check that the result is at or after the current time
	assert.True(t, result.After(time.Now().Add(-time.Second)), "Result should be at or after the current time")
}

func TestSendReviewCompletedAutoNotification(t *testing.T) {
	// Save environment variables before test and restore after
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()

	// Set test environment variables
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	// Set up mocks
	defer gock.Off() // Clear mocks when test ends

	testCases := []struct {
		name          string
		reviewerLogin string
		reviewState   string
		expectedMsg   string
	}{
		{"approved", "reviewer1", "approved", "✅ reviewer1さんがレビューを承認しました！感謝！👏"},
		{"changes_requested", "reviewer2", "changes_requested", "🔄 reviewer2さんが変更を要求しました 感謝！👏"},
		{"commented", "reviewer3", "commented", "💬 reviewer3さんがレビューコメントを残しました 感謝！👏"},
		{"other", "reviewer4", "other", "👀 reviewer4さんがレビューしました 感謝！👏"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Mock for thread message sending
			gock.New("https://slack.com").
				Post("/api/chat.postMessage").
				MatchHeader("Authorization", "Bearer test-token").
				Reply(200).
				JSON(map[string]interface{}{
					"ok": true,
				})

			// Create a test task
			task := models.ReviewTask{
				ID:           "test-id",
				SlackTS:      "1234.5678",
				SlackChannel: "C12345",
				Status:       "in_review",
			}

			// Execute function
			err := SendReviewCompletedAutoNotification(task, tc.reviewerLogin, tc.reviewState)

			// Assertions
			assert.NoError(t, err)
			assert.True(t, gock.IsDone(), "Not all mocks were used")
		})
	}
}

// TestFormatReviewerMentions tests the function that converts multiple reviewer IDs into Slack mention format
func TestFormatReviewerMentions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single reviewer ID",
			input:    "haruotsu",
			expected: "<@haruotsu>",
		},
		{
			name:     "multiple reviewer IDs (space-separated)",
			input:    "fuga @hoge",
			expected: "<@fuga> <@hoge>",
		},
		{
			name:     "multiple reviewer IDs (with @ prefix)",
			input:    "@fuga @hoge",
			expected: "<@fuga> <@hoge>",
		},
		{
			name:     "mixed pattern",
			input:    "fuga @hoge",
			expected: "<@fuga> <@hoge>",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "three reviewers",
			input:    "fuga hoge piyo",
			expected: "<@fuga> <@hoge> <@piyo>",
		},
		{
			name:     "extra spaces",
			input:    "  user1   @user2  ",
			expected: "<@user1> <@user2>",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := formatReviewerMentions(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// --- Tests for multiple reviewer support ---

func TestSelectRandomReviewers_Basic(t *testing.T) {
	db := setupTestDB(t)

	testConfig := models.ChannelConfig{
		ID:               "multi-rev-id",
		SlackChannelID:   "C_MULTI",
		LabelName:        "needs-review",
		DefaultMentionID: "UDEFAULT",
		ReviewerList:     "U1,U2,U3,U4,U5",
		IsActive:         true,
	}
	db.Create(&testConfig)

	// Select 2
	result := SelectRandomReviewers(db, "C_MULTI", "needs-review", 2, nil)
	assert.Equal(t, 2, len(result))
	// No duplicates
	assert.NotEqual(t, result[0], result[1])
}

func TestSelectRandomReviewers_ExcludeIDs(t *testing.T) {
	db := setupTestDB(t)

	testConfig := models.ChannelConfig{
		ID:               "exclude-id",
		SlackChannelID:   "C_EXCL",
		LabelName:        "needs-review",
		DefaultMentionID: "UDEFAULT",
		ReviewerList:     "U1,U2,U3",
		IsActive:         true,
	}
	db.Create(&testConfig)

	// Select 2 excluding U1
	result := SelectRandomReviewers(db, "C_EXCL", "needs-review", 2, []string{"U1"})
	assert.Equal(t, 2, len(result))
	for _, id := range result {
		assert.NotEqual(t, "U1", id, "Excluded U1 was included")
	}
}

func TestSelectRandomReviewers_InsufficientAfterExclusion(t *testing.T) {
	db := setupTestDB(t)

	testConfig := models.ChannelConfig{
		ID:               "insuff-id",
		SlackChannelID:   "C_INSUF",
		LabelName:        "needs-review",
		DefaultMentionID: "UDEFAULT",
		ReviewerList:     "U1,U2",
		IsActive:         true,
	}
	db.Create(&testConfig)

	// Request 2 excluding U1 -> only U2 is a candidate -> returns only 1
	result := SelectRandomReviewers(db, "C_INSUF", "needs-review", 2, []string{"U1"})
	assert.Equal(t, 1, len(result))
	assert.Equal(t, "U2", result[0])
}

func TestSelectRandomReviewers_AllExcluded(t *testing.T) {
	db := setupTestDB(t)

	testConfig := models.ChannelConfig{
		ID:               "allexcl-id",
		SlackChannelID:   "C_ALLX",
		LabelName:        "needs-review",
		DefaultMentionID: "UDEFAULT",
		ReviewerList:     "U1,U2",
		IsActive:         true,
	}
	db.Create(&testConfig)

	// All excluded -> returns DefaultMentionID
	result := SelectRandomReviewers(db, "C_ALLX", "needs-review", 1, []string{"U1", "U2"})
	assert.Equal(t, 1, len(result))
	assert.Equal(t, "UDEFAULT", result[0])
}

func TestGetPendingReviewers(t *testing.T) {
	// Normal case: Reviewers set, some already approved
	task := models.ReviewTask{
		Reviewers:  "U1,U2,U3",
		ApprovedBy: "U1",
	}
	pending := GetPendingReviewers(task)
	assert.Equal(t, []string{"U2", "U3"}, pending)

	// All approved
	task2 := models.ReviewTask{
		Reviewers:  "U1,U2",
		ApprovedBy: "U1,U2",
	}
	pending2 := GetPendingReviewers(task2)
	assert.Equal(t, 0, len(pending2))

	// Reviewers empty (legacy data) -> fallback to Reviewer
	task3 := models.ReviewTask{
		Reviewer: "UOLD",
	}
	pending3 := GetPendingReviewers(task3)
	assert.Equal(t, []string{"UOLD"}, pending3)

	// All empty
	task4 := models.ReviewTask{}
	pending4 := GetPendingReviewers(task4)
	assert.Nil(t, pending4)
}

func TestAddApproval(t *testing.T) {
	// New addition
	task := models.ReviewTask{}
	added := AddApproval(&task, "U1")
	assert.True(t, added)
	assert.Equal(t, "U1", task.ApprovedBy)

	// Add second person
	added2 := AddApproval(&task, "U2")
	assert.True(t, added2)
	assert.Equal(t, "U1,U2", task.ApprovedBy)

	// Duplicate addition
	added3 := AddApproval(&task, "U1")
	assert.False(t, added3)
	assert.Equal(t, "U1,U2", task.ApprovedBy)

	// Empty string
	added4 := AddApproval(&task, "")
	assert.False(t, added4)
}

func TestIsReviewFullyApproved(t *testing.T) {
	// 1 required, 1 approved -> complete
	task := models.ReviewTask{ApprovedBy: "U1"}
	assert.True(t, IsReviewFullyApproved(task, 1))

	// 2 required, 1 approved -> incomplete
	assert.False(t, IsReviewFullyApproved(task, 2))

	// 2 required, 2 approved -> complete
	task2 := models.ReviewTask{ApprovedBy: "U1,U2"}
	assert.True(t, IsReviewFullyApproved(task2, 2))

	// 2 required, 3 approved -> complete
	task3 := models.ReviewTask{ApprovedBy: "U1,U2,U3"}
	assert.True(t, IsReviewFullyApproved(task3, 2))

	// ApprovedBy empty -> incomplete
	task4 := models.ReviewTask{}
	assert.False(t, IsReviewFullyApproved(task4, 1))

	// requiredApprovals 0 or less -> treated as 1
	assert.False(t, IsReviewFullyApproved(task4, 0))

	// When assigned count < requiredApprovals, use assigned count for judgment
	task5 := models.ReviewTask{Reviewers: "U1", ApprovedBy: "U1"}
	assert.True(t, IsReviewFullyApproved(task5, 3), "Complete when 1 assigned and approved")

	task6 := models.ReviewTask{Reviewers: "U1,U2", ApprovedBy: "U1"}
	assert.False(t, IsReviewFullyApproved(task6, 3), "Incomplete when 2 assigned and only 1 approved")

	task7 := models.ReviewTask{Reviewers: "U1,U2", ApprovedBy: "U1,U2"}
	assert.True(t, IsReviewFullyApproved(task7, 3), "Complete when 2 assigned and 2 approved")
}

func TestCountApprovals(t *testing.T) {
	assert.Equal(t, 0, CountApprovals(models.ReviewTask{}))
	assert.Equal(t, 0, CountApprovals(models.ReviewTask{ApprovedBy: ""}))
	assert.Equal(t, 1, CountApprovals(models.ReviewTask{ApprovedBy: "U1"}))
	assert.Equal(t, 2, CountApprovals(models.ReviewTask{ApprovedBy: "U1,U2"}))
}

func TestRemoveApproval(t *testing.T) {
	t.Run("removes existing approval", func(t *testing.T) {
		task := models.ReviewTask{ApprovedBy: "U1,U2,U3"}
		assert.True(t, RemoveApproval(&task, "U2"))
		assert.Equal(t, "U1,U3", task.ApprovedBy)
	})

	t.Run("removes last approval", func(t *testing.T) {
		task := models.ReviewTask{ApprovedBy: "U1"}
		assert.True(t, RemoveApproval(&task, "U1"))
		assert.Equal(t, "", task.ApprovedBy)
	})

	t.Run("returns false for non-existent user", func(t *testing.T) {
		task := models.ReviewTask{ApprovedBy: "U1,U2"}
		assert.False(t, RemoveApproval(&task, "U999"))
		assert.Equal(t, "U1,U2", task.ApprovedBy)
	})

	t.Run("returns false for empty ApprovedBy", func(t *testing.T) {
		task := models.ReviewTask{ApprovedBy: ""}
		assert.False(t, RemoveApproval(&task, "U1"))
	})

	t.Run("returns false for empty slackUserID", func(t *testing.T) {
		task := models.ReviewTask{ApprovedBy: "U1"}
		assert.False(t, RemoveApproval(&task, ""))
	})
}

func TestGetPendingReviewersWithApprovedReviewer(t *testing.T) {
	// Also check ApprovedBy for single Reviewer case
	task := models.ReviewTask{
		Reviewer:   "UOLD",
		ApprovedBy: "UOLD",
	}
	pending := GetPendingReviewers(task)
	assert.Nil(t, pending)

	// When single Reviewer is not in ApprovedBy
	task2 := models.ReviewTask{
		Reviewer:   "UOLD",
		ApprovedBy: "UOTHER",
	}
	pending2 := GetPendingReviewers(task2)
	assert.Equal(t, []string{"UOLD"}, pending2)
}

func TestGetAwayUserIDs(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	future := now.Add(24 * time.Hour)
	past := now.Add(-24 * time.Hour)

	// Indefinite leave
	db.Create(&models.ReviewerAvailability{
		ID:          "away-1",
		SlackUserID: "U_AWAY1",
		AwayUntil:   nil,
		Reason:      "育児休業",
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	// Leave until future date
	db.Create(&models.ReviewerAvailability{
		ID:          "away-2",
		SlackUserID: "U_AWAY2",
		AwayUntil:   &future,
		Reason:      "休暇",
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	// Expired (should not be returned)
	db.Create(&models.ReviewerAvailability{
		ID:          "away-3",
		SlackUserID: "U_EXPIRED",
		AwayUntil:   &past,
		Reason:      "過去の休暇",
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	ids := GetAwayUserIDs(db)
	assert.Contains(t, ids, "U_AWAY1")
	assert.Contains(t, ids, "U_AWAY2")
	assert.NotContains(t, ids, "U_EXPIRED")
}

func TestSelectRandomReviewers_ExcludesAwayUsers(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	future := now.Add(24 * time.Hour)

	// Channel config
	testConfig := models.ChannelConfig{
		ID:               "away-test-id",
		SlackChannelID:   "C_AWAY",
		LabelName:        "needs-review",
		DefaultMentionID: "UDEFAULT",
		ReviewerList:     "U1,U2,U3",
		IsActive:         true,
	}
	db.Create(&testConfig)

	// Set U2 as on leave
	db.Create(&models.ReviewerAvailability{
		ID:          "away-u2",
		SlackUserID: "U2",
		AwayUntil:   &future,
		Reason:      "休暇",
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	// Repeat 100 times to verify U2 is never selected
	for i := 0; i < 100; i++ {
		result := SelectRandomReviewers(db, "C_AWAY", "needs-review", 2, nil)
		for _, id := range result {
			assert.NotEqual(t, "U2", id, "U2 on leave was selected")
		}
	}
}
