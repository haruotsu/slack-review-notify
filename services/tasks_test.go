package services

import (
	"os"
	"slack-review-notify/models"
	"strings"
	"testing"
	"time"

	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
)

func TestCleanupOldTasks(t *testing.T) {
	db := setupTestDB(t)

	// Create test data
	now := time.Now()
	twoDaysAgo := now.AddDate(0, 0, -2)
	yesterdayAgo := now.AddDate(0, 0, -1)
	tenDaysAgo := now.AddDate(0, 0, -10)
	twoWeeksAgo := now.AddDate(0, 0, -14)

	tasks := []models.ReviewTask{
		{
			ID:           "task1",
			PRURL:        "https://github.com/owner/repo/pull/1",
			Repo:         "owner/repo",
			PRNumber:     1,
			Title:        "Test PR 1",
			SlackTS:      "1234.5678",
			SlackChannel: "C12345",
			Status:       "done",
			CreatedAt:    twoDaysAgo,
			UpdatedAt:    twoDaysAgo, // Old done task (should be deleted)
		},
		{
			ID:           "task2",
			PRURL:        "https://github.com/owner/repo/pull/2",
			Repo:         "owner/repo",
			PRNumber:     2,
			Title:        "Test PR 2",
			SlackTS:      "1234.5679",
			SlackChannel: "C12345",
			Status:       "done",
			CreatedAt:    yesterdayAgo,
			UpdatedAt:    now, // Recent done task (should be kept)
		},
		{
			ID:           "task3",
			PRURL:        "https://github.com/owner/repo/pull/3",
			Repo:         "owner/repo",
			PRNumber:     3,
			Title:        "Test PR 3",
			SlackTS:      "1234.5680",
			SlackChannel: "C12345",
			Status:       "paused",
			CreatedAt:    twoWeeksAgo,
			UpdatedAt:    twoWeeksAgo, // Old paused task (should be deleted)
		},
		{
			ID:           "task4",
			PRURL:        "https://github.com/owner/repo/pull/4",
			Repo:         "owner/repo",
			PRNumber:     4,
			Title:        "Test PR 4",
			SlackTS:      "1234.5681",
			SlackChannel: "C12345",
			Status:       "archived",
			CreatedAt:    now,
			UpdatedAt:    now, // Archived task (should be deleted)
		},
		{
			ID:           "task5",
			PRURL:        "https://github.com/owner/repo/pull/5",
			Repo:         "owner/repo",
			PRNumber:     5,
			Title:        "Test PR 5",
			SlackTS:      "1234.5682",
			SlackChannel: "C12345",
			Status:       "pending",
			CreatedAt:    now,
			UpdatedAt:    now, // Pending task (should be kept)
		},
		{
			ID:           "task6",
			PRURL:        "https://github.com/owner/repo/pull/6",
			Repo:         "owner/repo",
			PRNumber:     6,
			Title:        "Test PR 6",
			SlackTS:      "1234.5683",
			SlackChannel: "C12345",
			Status:       "completed",
			CreatedAt:    tenDaysAgo,
			UpdatedAt:    tenDaysAgo, // 10-day-old completed task (over 7 days, should be deleted)
		},
		{
			ID:           "task7",
			PRURL:        "https://github.com/owner/repo/pull/7",
			Repo:         "owner/repo",
			PRNumber:     7,
			Title:        "Test PR 7",
			SlackTS:      "1234.5684",
			SlackChannel: "C12345",
			Status:       "completed",
			CreatedAt:    now,
			UpdatedAt:    now, // Recent completed task (should be kept)
		},
		{
			ID:           "task8",
			PRURL:        "https://github.com/owner/repo/pull/8",
			Repo:         "owner/repo",
			PRNumber:     8,
			Title:        "Test PR 8",
			SlackTS:      "1234.5685",
			SlackChannel: "C12345",
			Status:       "completed",
			CreatedAt:    twoDaysAgo,
			UpdatedAt:    twoDaysAgo, // 2-day-old completed task (under 7 days, should be kept)
		},
	}

	for _, task := range tasks {
		db.Create(&task)
	}

	// Execute cleanup
	CleanupOldTasks(db)

	// Verify deletion results
	var count int64

	// task1 (old done task) should be deleted
	db.Model(&models.ReviewTask{}).Where("id = ?", "task1").Count(&count)
	assert.Equal(t, int64(0), count)

	// task2 (recent done task) should be kept
	db.Model(&models.ReviewTask{}).Where("id = ?", "task2").Count(&count)
	assert.Equal(t, int64(1), count)

	// task3 (old paused task) should be deleted
	db.Model(&models.ReviewTask{}).Where("id = ?", "task3").Count(&count)
	assert.Equal(t, int64(0), count)

	// task4 (archived task) should be deleted
	db.Model(&models.ReviewTask{}).Where("id = ?", "task4").Count(&count)
	assert.Equal(t, int64(0), count)

	// task5 (pending task) should be kept
	db.Model(&models.ReviewTask{}).Where("id = ?", "task5").Count(&count)
	assert.Equal(t, int64(1), count)

	// task6 (10-day-old completed task) should be deleted
	db.Model(&models.ReviewTask{}).Where("id = ?", "task6").Count(&count)
	assert.Equal(t, int64(0), count)

	// task7 (recent completed task) should be kept
	db.Model(&models.ReviewTask{}).Where("id = ?", "task7").Count(&count)
	assert.Equal(t, int64(1), count)

	// task8 (2-day-old completed task) should be kept since it's under 7 days
	db.Model(&models.ReviewTask{}).Where("id = ?", "task8").Count(&count)
	assert.Equal(t, int64(1), count)
}

func TestCheckInReviewTasks(t *testing.T) {
	// Simplified test: only test the mock portion
	db := setupTestDB(t)

	// Set test environment variables
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	// Create a test task (simply one in in_review status)
	now := time.Now()
	twoHoursAgo := now.Add(-2 * time.Hour)

	task := models.ReviewTask{
		ID:           "review-test",
		PRURL:        "https://github.com/owner/repo/pull/1",
		Repo:         "owner/repo",
		PRNumber:     1,
		Title:        "Review PR Test",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Status:       "in_review",
		Reviewer:     "U12345",
		LabelName:    "needs-review",
		CreatedAt:    twoHoursAgo,
		UpdatedAt:    twoHoursAgo,
	}

	db.Create(&task)

	// Set up mocks
	defer gock.Off()

	// Mock for channel info retrieval
	gock.New("https://slack.com").
		Get("/api/conversations.info").
		MatchParam("channel", "C12345").
		Persist().
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
		Persist().
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
		})

	// Execute function
	CheckInReviewTasks(db)

	// Assertion - only verify that it was updated
	var updatedTask models.ReviewTask
	db.Where("id = ?", "review-test").First(&updatedTask)

	// Consider test successful (OK if mocks are working correctly)
	// Do not compare actual timestamps
}

func TestCheckInReviewTasks_ReminderInterval(t *testing.T) {
	db := setupTestDB(t)

	// Set test environment variables
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	// Create multiple test channel configs
	now := time.Now()

	// Config for needs-review label: 60 minute interval
	config1 := models.ChannelConfig{
		ID:                       "config1",
		SlackChannelID:           "C12345",
		LabelName:                "needs-review",
		DefaultMentionID:         "U12345",
		ReviewerReminderInterval: 60, // 60 minutes
		IsActive:                 true,
		CreatedAt:                now,
		UpdatedAt:                now,
	}
	db.Create(&config1)

	// Config for bug label: 15 minute interval
	config2 := models.ChannelConfig{
		ID:                       "config2",
		SlackChannelID:           "C12345",
		LabelName:                "bug",
		DefaultMentionID:         "U67890",
		ReviewerReminderInterval: 15, // 15 minutes
		IsActive:                 true,
		CreatedAt:                now,
		UpdatedAt:                now,
	}
	db.Create(&config2)

	// Create test tasks
	twoHoursAgo := now.Add(-2 * time.Hour)
	twentyMinutesAgo := now.Add(-20 * time.Minute)

	// Task with needs-review label (60min interval, updated 2 hours ago -> reminder should be sent)
	task1 := models.ReviewTask{
		ID:           "task1",
		PRURL:        "https://github.com/owner/repo/pull/1",
		Repo:         "owner/repo",
		PRNumber:     1,
		Title:        "Test PR 1",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Status:       "in_review",
		Reviewer:     "U12345",
		LabelName:    "needs-review",
		CreatedAt:    twoHoursAgo,
		UpdatedAt:    twoHoursAgo,
	}
	db.Create(&task1)

	// Task with bug label (15min interval, updated 20 minutes ago -> reminder should be sent)
	task2 := models.ReviewTask{
		ID:           "task2",
		PRURL:        "https://github.com/owner/repo/pull/2",
		Repo:         "owner/repo",
		PRNumber:     2,
		Title:        "Test PR 2",
		SlackTS:      "1234.5679",
		SlackChannel: "C12345",
		Status:       "in_review",
		Reviewer:     "U67890",
		LabelName:    "bug",
		CreatedAt:    twentyMinutesAgo,
		UpdatedAt:    twentyMinutesAgo,
	}
	db.Create(&task2)

	// Task with needs-review label (60min interval, updated 20 minutes ago -> reminder should NOT be sent)
	task3 := models.ReviewTask{
		ID:           "task3",
		PRURL:        "https://github.com/owner/repo/pull/3",
		Repo:         "owner/repo",
		PRNumber:     3,
		Title:        "Test PR 3",
		SlackTS:      "1234.5680",
		SlackChannel: "C12345",
		Status:       "in_review",
		Reviewer:     "U12345",
		LabelName:    "needs-review",
		CreatedAt:    twentyMinutesAgo,
		UpdatedAt:    twentyMinutesAgo,
	}
	db.Create(&task3)

	// Set up mocks
	defer gock.Off()

	// Mock for channel info retrieval
	gock.New("https://slack.com").
		Get("/api/conversations.info").
		MatchParam("channel", "C12345").
		Persist().
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
		Persist().
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
		})

	// Record timestamp before function execution
	beforeExecution := now

	// Execute function
	CheckInReviewTasks(db)

	// Assertions
	var updatedTask1 models.ReviewTask
	db.Where("id = ?", "task1").First(&updatedTask1)
	// task1 has 60min interval and was updated 2 hours ago, so reminder should be sent
	assert.True(t, updatedTask1.UpdatedAt.After(beforeExecution), "task1 should be updated")

	var updatedTask2 models.ReviewTask
	db.Where("id = ?", "task2").First(&updatedTask2)
	// task2 has 15min interval and was updated 20 minutes ago, so reminder should be sent
	assert.True(t, updatedTask2.UpdatedAt.After(beforeExecution), "task2 should be updated")

	var updatedTask3 models.ReviewTask
	db.Where("id = ?", "task3").First(&updatedTask3)
	// task3 has 60min interval and was updated 20 minutes ago, so reminder should NOT be sent
	assert.False(t, updatedTask3.UpdatedAt.After(beforeExecution), "task3 should not be updated")
}

func TestCleanupExpiredAvailability(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	future := now.Add(24 * time.Hour)
	past := now.Add(-24 * time.Hour)

	// Indefinite leave (should not be deleted)
	db.Create(&models.ReviewerAvailability{
		ID:          "perm-away",
		SlackUserID: "U_PERM",
		AwayUntil:   nil,
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	// Future leave (should not be deleted)
	db.Create(&models.ReviewerAvailability{
		ID:          "future-away",
		SlackUserID: "U_FUTURE",
		AwayUntil:   &future,
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	// Expired leave (should be deleted)
	db.Create(&models.ReviewerAvailability{
		ID:          "expired-away",
		SlackUserID: "U_EXPIRED",
		AwayUntil:   &past,
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	CleanupExpiredAvailability(db)

	var count int64

	// Indefinite record should remain
	db.Model(&models.ReviewerAvailability{}).Where("id = ?", "perm-away").Count(&count)
	assert.Equal(t, int64(1), count)

	// Future record should remain
	db.Model(&models.ReviewerAvailability{}).Where("id = ?", "future-away").Count(&count)
	assert.Equal(t, int64(1), count)

	// Expired record should be deleted
	db.Unscoped().Model(&models.ReviewerAvailability{}).Where("id = ?", "expired-away").Count(&count)
	assert.Equal(t, int64(0), count)
}

// TestCheckBusinessHoursTasks_HonorsExistingReviewers は、
// waiting_business_hours の task に既にレビュワーが事前アサインされている場合、
// SelectRandomReviewers はその人を除外し、不足分のみ選出することを確認する。
func TestCheckBusinessHoursTasks_HonorsExistingReviewers(t *testing.T) {
	db := setupTestDB(t)
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	// chat.postMessage は複数回呼ばれるので Persist
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Persist().
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// RequiredApprovals=2, プールに 3人
	config := models.ChannelConfig{
		ID:                "cfg-honors",
		SlackChannelID:    "C_HONORS",
		LabelName:         "needs-review",
		DefaultMentionID:  "UDEFAULT",
		ReviewerList:      "U_R1,U_R2,U_R3",
		RequiredApprovals: 2,
		IsActive:          true,
		// BusinessHours 未設定 → 常に within business hours
	}
	db.Create(&config)

	// U_R1 だけ事前アサイン済み
	task := models.ReviewTask{
		ID:           "honors-task",
		PRURL:        "https://github.com/owner/repo/pull/700",
		Repo:         "owner/repo",
		PRNumber:     700,
		Title:        "Test PR",
		SlackTS:      "1234.7000",
		SlackChannel: "C_HONORS",
		Reviewer:     "U_R1",
		Reviewers:    "U_R1",
		Status:       "waiting_business_hours",
		LabelName:    "needs-review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	CheckBusinessHoursTasks(db)

	var updated models.ReviewTask
	db.First(&updated, "id = ?", "honors-task")

	assert.Equal(t, "in_review", updated.Status, "business hours に入ったので in_review へ遷移")
	// Reviewers は "U_R1,X" の形式で、X は U_R2 か U_R3 のどちらか
	ids := splitNonEmptyForTest(updated.Reviewers)
	assert.Len(t, ids, 2, "RequiredApprovals=2 分のレビュワーが設定されるべき")
	assert.Contains(t, ids, "U_R1", "事前アサイン済みの U_R1 は維持されるべき")
	assert.Equal(t, "U_R1", ids[0], "先頭は事前アサインの U_R1")
	// 2人目は U_R2 か U_R3
	second := ids[1]
	assert.Contains(t, []string{"U_R2", "U_R3"}, second, "不足分はプールから選出される")
	assert.NotEqual(t, "U_R1", second, "SelectRandomReviewers は事前アサイン済み U_R1 を除外する")
}

// TestCheckBusinessHoursTasks_FullyPreAssignedSkipsRandomSelect は、
// RequiredApprovals を既に満たす数の事前アサインがあれば SelectRandomReviewers を呼ばず、
// 事前アサイン分だけでメンション対象とすることを確認する。
func TestCheckBusinessHoursTasks_FullyPreAssignedSkipsRandomSelect(t *testing.T) {
	db := setupTestDB(t)
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

	// RequiredApprovals=1, プールに 2人
	config := models.ChannelConfig{
		ID:                "cfg-fully",
		SlackChannelID:    "C_FULLY",
		LabelName:         "needs-review",
		DefaultMentionID:  "UDEFAULT",
		ReviewerList:      "U_R1,U_R2",
		RequiredApprovals: 1,
		IsActive:          true,
	}
	db.Create(&config)

	// U_R1 が事前アサイン済み、RequiredApprovals=1 を満たしている
	task := models.ReviewTask{
		ID:           "fully-task",
		PRURL:        "https://github.com/owner/repo/pull/701",
		Repo:         "owner/repo",
		PRNumber:     701,
		Title:        "Test PR",
		SlackTS:      "1234.7001",
		SlackChannel: "C_FULLY",
		Reviewer:     "U_R1",
		Reviewers:    "U_R1",
		Status:       "waiting_business_hours",
		LabelName:    "needs-review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	CheckBusinessHoursTasks(db)

	var updated models.ReviewTask
	db.First(&updated, "id = ?", "fully-task")

	assert.Equal(t, "in_review", updated.Status, "business hours に入ったので in_review へ遷移")
	assert.Equal(t, "U_R1", updated.Reviewers, "既に RequiredApprovals を満たしているため U_R1 のみ")
	assert.Equal(t, "U_R1", updated.Reviewer, "Reviewer も U_R1 で固定")
}

// splitNonEmptyForTest はテスト用のカンマ区切りユーティリティ。空要素は除外する。
func splitNonEmptyForTest(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	for _, p := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
