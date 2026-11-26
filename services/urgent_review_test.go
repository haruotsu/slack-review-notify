package services

import (
	"testing"
	"time"

	"slack-review-notify/models"

	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func createWaitingTaskFixture(t *testing.T, db *gorm.DB, taskID string) (models.ReviewTask, models.ChannelConfig) {
	t.Helper()
	config := models.ChannelConfig{
		ID:             "cfg-urgent",
		SlackChannelID: "C12345",
		LabelName:      "needs-review",
		ReviewerList:   "UREV1",
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, db.Create(&config).Error)
	task := models.ReviewTask{
		ID:           taskID,
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Status:       "waiting_business_hours",
		LabelName:    "needs-review",
		Language:     "ja",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	require.NoError(t, db.Create(&task).Error)
	return task, config
}

// A failed urgent notification must leave the task waiting for business hours, so the
// morning activation still mentions the reviewers.
func TestActivateWaitingTaskUrgently_FailedNotificationKeepsTaskWaiting(t *testing.T) {
	t.Setenv("SLACK_BOT_TOKEN", "test-token")
	db := setupTestDB(t)
	task, _ := createWaitingTaskFixture(t, db, "task-urgent-fail")

	defer gock.OffAll()
	gock.CleanUnmatchedRequest()
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": false, "error": "channel_not_found"})

	err := ActivateWaitingTaskUrgently(db, task, "UCLICKER")
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrTaskNotWaiting)
	assert.False(t, gock.HasUnmatchedRequest(), "the off-hours message must not be rewritten when nobody was mentioned")

	var updated models.ReviewTask
	require.NoError(t, db.First(&updated, "id = ?", "task-urgent-fail").Error)
	assert.Equal(t, "waiting_business_hours", updated.Status)
	assert.Empty(t, updated.Reviewer)
	assert.Empty(t, updated.Reviewers)
}

// The morning tick reads the waiting tasks before activating them, so an urgent review
// request can activate a task in between. The tick must then not mention it again.
func TestActivateWaitingTask_TaskActivatedAfterItWasReadIsNotNotifiedAgain(t *testing.T) {
	db := setupTestDB(t)
	staleTask, config := createWaitingTaskFixture(t, db, "task-race")
	require.NoError(t, db.Model(&models.ReviewTask{}).Where("id = ?", "task-race").
		Updates(map[string]interface{}{"status": "in_review", "reviewer": "UREV1", "reviewers": "UREV1"}).Error)

	notified := false
	err := activateWaitingTask(db, staleTask, config, "needs-review", func(models.ReviewTask) error {
		notified = true
		return nil
	})

	assert.ErrorIs(t, err, ErrTaskNotWaiting)
	assert.False(t, notified)
}

// A "Review Done" press or a PR close can land while the notification is being sent.
// Activation must not overwrite that status afterwards.
func TestActivateWaitingTask_KeepsStatusChangedDuringNotification(t *testing.T) {
	db := setupTestDB(t)
	task, config := createWaitingTaskFixture(t, db, "task-done-during-notify")

	err := activateWaitingTask(db, task, config, "needs-review", func(models.ReviewTask) error {
		return db.Model(&models.ReviewTask{}).Where("id = ?", "task-done-during-notify").Update("status", "done").Error
	})
	require.NoError(t, err)

	var updated models.ReviewTask
	require.NoError(t, db.First(&updated, "id = ?", "task-done-during-notify").Error)
	assert.Equal(t, "done", updated.Status)
	assert.Equal(t, "UREV1", updated.Reviewer)
}
