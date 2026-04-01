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

	// Create task with pending re-review notification
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

	// Run the checker (no business hours config = always within business hours)
	CheckPendingReReviewNotifications(db)

	// Verify that the pending flag was cleared
	var updatedTask models.ReviewTask
	db.Where("id = ?", "pending-rr-task").First(&updatedTask)
	assert.False(t, updatedTask.PendingReReviewNotify, "Pending flag should be cleared after notification is sent")
	assert.Empty(t, updatedTask.PendingReReviewSender, "Sender should be cleared")
	assert.Empty(t, updatedTask.PendingReReviewReviewer, "Reviewer should be cleared")
}

func TestCheckPendingReReviewNotifications_SkipsOutsideBusinessHours(t *testing.T) {
	db := setupTestDB(t)
	IsTestMode = true

	jst, _ := time.LoadLocation("Asia/Tokyo")
	now := time.Now().In(jst)
	hour := now.Hour()
	isCurrentlyBusinessHours := hour >= 9 && hour < 18 && now.Weekday() != time.Saturday && now.Weekday() != time.Sunday

	if isCurrentlyBusinessHours {
		t.Skip("This test requires running outside business hours (JST 9:00-18:00)")
	}

	// Create channel config with business hours
	config := models.ChannelConfig{
		ID:                 "config-pending-rr-bh",
		SlackChannelID:     "C_PENDING_RR_BH",
		LabelName:          "needs-review",
		IsActive:           true,
		BusinessHoursStart: "09:00",
		BusinessHoursEnd:   "18:00",
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

	// Pending flag should remain since we're outside business hours
	var updatedTask models.ReviewTask
	db.Where("id = ?", "pending-rr-bh-task").First(&updatedTask)
	assert.True(t, updatedTask.PendingReReviewNotify, "Pending flag should remain outside business hours")
	assert.Equal(t, "<@USENDER>", updatedTask.PendingReReviewSender)
	assert.Equal(t, "<@UREVIEWER>", updatedTask.PendingReReviewReviewer)
}

func TestCheckPendingReReviewNotifications_NoPendingTasks(t *testing.T) {
	db := setupTestDB(t)
	IsTestMode = true

	// Create a task without pending re-review
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

	// Should not panic or error
	CheckPendingReReviewNotifications(db)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "no-pending-task").First(&updatedTask)
	assert.False(t, updatedTask.PendingReReviewNotify)
}
