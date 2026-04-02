package services

import (
	"os"
	"slack-review-notify/models"
	"testing"
	"time"

	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
)

func TestCheckPendingReReviewNotifications_SendsDuringBusinessHours(t *testing.T) {
	now := time.Now()
	loc, _ := time.LoadLocation("Asia/Tokyo")
	nowJST := now.In(loc)
	if nowJST.Weekday() == time.Saturday || nowJST.Weekday() == time.Sunday {
		t.Skip("This test requires running on a weekday")
	}

	db := setupTestDB(t)
	IsTestMode = true

	// Mock Slack API
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() { _ = os.Setenv("SLACK_BOT_TOKEN", originalToken) }()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")
	defer gock.Off()
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// Create channel config with 24h business hours (always within business hours on weekdays)
	config := models.ChannelConfig{
		ID:                 "config-pending-rr",
		SlackChannelID:     "C_PENDING_RR",
		LabelName:          "needs-review",
		IsActive:           true,
		BusinessHoursStart: "00:00",
		BusinessHoursEnd:   "23:59",
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
	db.Create(&config)

	task := models.ReviewTask{
		ID:                      "pending-rr-task",
		PRURL:                   "https://github.com/owner/repo/pull/600",
		Repo:                    "owner/repo",
		PRNumber:                600,
		Title:                   "Test PR",
		SlackTS:                 "1234.6666",
		SlackChannel:            "C_PENDING_RR",
		Status:                  "in_review",
		LabelName:               "needs-review",
		PendingReReviewNotify:   true,
		PendingReReviewSender:   "<@USENDER>",
		PendingReReviewReviewer: "<@UREVIEWER>",
		CreatedAt:               time.Now(),
		UpdatedAt:               time.Now(),
	}
	db.Create(&task)

	CheckPendingReReviewNotifications(db)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "pending-rr-task").First(&updatedTask)
	assert.False(t, updatedTask.PendingReReviewNotify, "Pending flag should be cleared after notification is sent")
	assert.Empty(t, updatedTask.PendingReReviewSender, "Sender should be cleared")
	assert.Empty(t, updatedTask.PendingReReviewReviewer, "Reviewer should be cleared")
}

func TestCheckPendingReReviewNotifications_SkipsOutsideBusinessHours(t *testing.T) {
	db := setupTestDB(t)
	IsTestMode = true

	loc, _ := time.LoadLocation("Asia/Tokyo")
	now := time.Now().In(loc)
	// Use a narrow window (03:00-03:01) to ensure we're almost always outside
	if now.Hour() == 3 && now.Minute() == 0 {
		t.Skip("Skipping: current time falls within the narrow test business hours window")
	}
	if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
		t.Skip("Skipping: weekend")
	}

	config := models.ChannelConfig{
		ID:                 "config-pending-rr-bh",
		SlackChannelID:     "C_PENDING_RR_BH",
		LabelName:          "needs-review",
		IsActive:           true,
		BusinessHoursStart: "03:00",
		BusinessHoursEnd:   "03:01",
		Timezone:           "Asia/Tokyo",
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
	db.Create(&config)

	task := models.ReviewTask{
		ID:                      "pending-rr-bh-task",
		PRURL:                   "https://github.com/owner/repo/pull/601",
		Repo:                    "owner/repo",
		PRNumber:                601,
		Title:                   "Test PR",
		SlackTS:                 "1234.5555",
		SlackChannel:            "C_PENDING_RR_BH",
		Status:                  "in_review",
		LabelName:               "needs-review",
		PendingReReviewNotify:   true,
		PendingReReviewSender:   "<@USENDER>",
		PendingReReviewReviewer: "<@UREVIEWER>",
		CreatedAt:               time.Now(),
		UpdatedAt:               time.Now(),
	}
	db.Create(&task)

	CheckPendingReReviewNotifications(db)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "pending-rr-bh-task").First(&updatedTask)
	assert.True(t, updatedTask.PendingReReviewNotify, "Pending flag should remain outside business hours")
	assert.Equal(t, "<@USENDER>", updatedTask.PendingReReviewSender)
	assert.Equal(t, "<@UREVIEWER>", updatedTask.PendingReReviewReviewer)
}

func TestCheckPendingReReviewNotifications_NoPendingTasks(t *testing.T) {
	db := setupTestDB(t)
	IsTestMode = true

	task := models.ReviewTask{
		ID:                    "no-pending-task",
		PRURL:                 "https://github.com/owner/repo/pull/602",
		Repo:                  "owner/repo",
		PRNumber:              602,
		Title:                 "Test PR",
		SlackTS:               "1234.4444",
		SlackChannel:          "C_NO_PENDING",
		Status:                "in_review",
		LabelName:             "needs-review",
		PendingReReviewNotify: false,
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}
	db.Create(&task)

	CheckPendingReReviewNotifications(db)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "no-pending-task").First(&updatedTask)
	assert.False(t, updatedTask.PendingReReviewNotify)
}

func TestCheckPendingReReviewNotifications_SkipsCompletedTasks(t *testing.T) {
	db := setupTestDB(t)
	IsTestMode = true

	// Task with pending flag but already completed — should NOT be processed
	task := models.ReviewTask{
		ID:                      "completed-pending-task",
		PRURL:                   "https://github.com/owner/repo/pull/603",
		Repo:                    "owner/repo",
		PRNumber:                603,
		Title:                   "Test PR",
		SlackTS:                 "1234.3333",
		SlackChannel:            "C_COMPLETED",
		Status:                  "done",
		LabelName:               "needs-review",
		PendingReReviewNotify:   true,
		PendingReReviewSender:   "<@USENDER>",
		PendingReReviewReviewer: "<@UREVIEWER>",
		CreatedAt:               time.Now(),
		UpdatedAt:               time.Now(),
	}
	db.Create(&task)

	CheckPendingReReviewNotifications(db)

	// Pending flag should remain (task was filtered out by status)
	var updatedTask models.ReviewTask
	db.Where("id = ?", "completed-pending-task").First(&updatedTask)
	assert.True(t, updatedTask.PendingReReviewNotify, "Completed task should not be processed")
}

func TestCheckPendingReReviewNotifications_ClearsOnMissingConfig(t *testing.T) {
	db := setupTestDB(t)
	IsTestMode = true

	// No config created — config lookup will fail
	task := models.ReviewTask{
		ID:                      "orphan-pending-task",
		PRURL:                   "https://github.com/owner/repo/pull/604",
		Repo:                    "owner/repo",
		PRNumber:                604,
		Title:                   "Test PR",
		SlackTS:                 "1234.2222",
		SlackChannel:            "C_NO_CONFIG",
		Status:                  "in_review",
		LabelName:               "needs-review",
		PendingReReviewNotify:   true,
		PendingReReviewSender:   "<@USENDER>",
		PendingReReviewReviewer: "<@UREVIEWER>",
		CreatedAt:               time.Now(),
		UpdatedAt:               time.Now(),
	}
	db.Create(&task)

	CheckPendingReReviewNotifications(db)

	// Pending flag should be cleared to avoid infinite retry
	var updatedTask models.ReviewTask
	db.Where("id = ?", "orphan-pending-task").First(&updatedTask)
	assert.False(t, updatedTask.PendingReReviewNotify, "Pending flag should be cleared when config is missing")
}

func TestCheckPendingReReviewNotifications_MultipleNotifications(t *testing.T) {
	now := time.Now()
	loc, _ := time.LoadLocation("Asia/Tokyo")
	nowJST := now.In(loc)
	if nowJST.Weekday() == time.Saturday || nowJST.Weekday() == time.Sunday {
		t.Skip("This test requires running on a weekday")
	}

	db := setupTestDB(t)
	IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() { _ = os.Setenv("SLACK_BOT_TOKEN", originalToken) }()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")
	defer gock.Off()
	// Expect 2 Slack API calls (one per sender/reviewer pair)
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	config := models.ChannelConfig{
		ID:                 "config-multi-rr",
		SlackChannelID:     "C_MULTI_RR",
		LabelName:          "needs-review",
		IsActive:           true,
		BusinessHoursStart: "00:00",
		BusinessHoursEnd:   "23:59",
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
	db.Create(&config)

	// Task with multiple accumulated re-review requests
	task := models.ReviewTask{
		ID:                      "multi-rr-task",
		PRURL:                   "https://github.com/owner/repo/pull/605",
		Repo:                    "owner/repo",
		PRNumber:                605,
		Title:                   "Test PR",
		SlackTS:                 "1234.1111",
		SlackChannel:            "C_MULTI_RR",
		Status:                  "in_review",
		LabelName:               "needs-review",
		PendingReReviewNotify:   true,
		PendingReReviewSender:   "<@USENDER1>,<@USENDER2>",
		PendingReReviewReviewer: "<@UREVIEWER1>,<@UREVIEWER2>",
		CreatedAt:               time.Now(),
		UpdatedAt:               time.Now(),
	}
	db.Create(&task)

	CheckPendingReReviewNotifications(db)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "multi-rr-task").First(&updatedTask)
	assert.False(t, updatedTask.PendingReReviewNotify, "Pending flag should be cleared")
	assert.True(t, gock.IsDone(), "Both Slack notifications should have been sent")
}

func TestClearPendingReReviewFlags_CASMiss(t *testing.T) {
	db := setupTestDB(t)
	IsTestMode = true

	now := time.Now()
	task := models.ReviewTask{
		ID:                      "cas-miss-task",
		PRURL:                   "https://github.com/owner/repo/pull/700",
		Repo:                    "owner/repo",
		PRNumber:                700,
		Title:                   "Test PR",
		SlackTS:                 "1234.0000",
		SlackChannel:            "C_CAS_MISS",
		Status:                  "in_review",
		LabelName:               "needs-review",
		PendingReReviewNotify:   true,
		PendingReReviewSender:   "<@USENDER>",
		PendingReReviewReviewer: "<@UREVIEWER>",
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	db.Create(&task)

	// Use a stale updated_at (1 hour ago) to simulate concurrent modification
	staleUpdatedAt := now.Add(-1 * time.Hour)
	err := clearPendingReReviewFlags(db, task.ID, staleUpdatedAt, time.Now())
	assert.NoError(t, err, "CAS miss should not return an error")

	// Pending flag should remain because CAS missed
	var updatedTask models.ReviewTask
	db.Where("id = ?", "cas-miss-task").First(&updatedTask)
	assert.True(t, updatedTask.PendingReReviewNotify, "Pending flag should remain after CAS miss")
	assert.Equal(t, "<@USENDER>", updatedTask.PendingReReviewSender)
}

func TestClearPendingReReviewFlags_CASHit(t *testing.T) {
	db := setupTestDB(t)
	IsTestMode = true

	now := time.Now()
	task := models.ReviewTask{
		ID:                      "cas-hit-task",
		PRURL:                   "https://github.com/owner/repo/pull/701",
		Repo:                    "owner/repo",
		PRNumber:                701,
		Title:                   "Test PR",
		SlackTS:                 "1234.9999",
		SlackChannel:            "C_CAS_HIT",
		Status:                  "in_review",
		LabelName:               "needs-review",
		PendingReReviewNotify:   true,
		PendingReReviewSender:   "<@USENDER>",
		PendingReReviewReviewer: "<@UREVIEWER>",
		CreatedAt:               now,
		UpdatedAt:               now,
	}
	db.Create(&task)

	// Use correct updated_at to simulate successful CAS
	err := clearPendingReReviewFlags(db, task.ID, now, time.Now())
	assert.NoError(t, err)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "cas-hit-task").First(&updatedTask)
	assert.False(t, updatedTask.PendingReReviewNotify, "Pending flag should be cleared on CAS hit")
	assert.Empty(t, updatedTask.PendingReReviewSender)
	assert.Empty(t, updatedTask.PendingReReviewReviewer)
}
