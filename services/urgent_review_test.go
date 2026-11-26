package services

import (
	"os"
	"testing"
	"time"

	"slack-review-notify/models"

	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
)

func TestSendSlackMessageOffHours_CarriesUrgentReviewButton(t *testing.T) {
	t.Setenv("SLACK_BOT_TOKEN", "test-token")
	defer gock.OffAll()

	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		BodyString(`review_done[\s\S]*urgent_review`).
		Reply(200).
		JSON(map[string]interface{}{"ok": true, "ts": "1.2", "channel": "C12345"})

	_, _, err := SendSlackMessageOffHours("https://github.com/owner/repo/pull/1", "Test PR", "C12345", "", "ja")
	assert.NoError(t, err)
	assert.True(t, gock.IsDone())
}

// A failed urgent notification must leave the task waiting for business hours, so the
// morning activation still mentions the reviewers.
func TestActivateWaitingTaskUrgently_FailedNotificationKeepsTaskWaiting(t *testing.T) {
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() { _ = os.Setenv("SLACK_BOT_TOKEN", originalToken) }()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	db := setupTestDB(t)
	db.Create(&models.ChannelConfig{
		ID:             "cfg-urgent",
		SlackChannelID: "C12345",
		LabelName:      "needs-review",
		ReviewerList:   "UREV1",
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	})
	task := models.ReviewTask{
		ID:           "task-urgent-fail",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Status:       "waiting_business_hours",
		LabelName:    "needs-review",
		Language:     "ja",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	defer gock.OffAll()
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": false, "error": "channel_not_found"})

	err := ActivateWaitingTaskUrgently(db, task, "UCLICKER")
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrTaskNotWaiting)

	var updated models.ReviewTask
	db.First(&updated, "id = ?", "task-urgent-fail")
	assert.Equal(t, "waiting_business_hours", updated.Status)
	assert.Empty(t, updated.Reviewer)
}
