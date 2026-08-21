package services

import (
	"bytes"
	"log"
	"os"
	"slack-review-notify/models"
	"testing"
	"time"

	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// main.go runs CheckBusinessHoursTasks and CheckPendingReReviewNotifications back to
// back in the same tick. A task activated by the first must not then receive the
// re-review request queued while it was still waiting.
func TestBusinessHoursTick_ActivatedTaskGetsOnlyMorningGreeting(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Tokyo")
	if wd := time.Now().In(loc).Weekday(); wd == time.Saturday || wd == time.Sunday {
		t.Skip("This test requires running on a weekday")
	}

	db := setupTestDB(t)

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() { _ = os.Setenv("SLACK_BOT_TOKEN", originalToken) }()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.OffAll()
	gock.CleanUnmatchedRequest()

	// Only the morning greeting is legitimate. A second post leaves an unmatched
	// request, which is what the assertion below catches.
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	config := models.ChannelConfig{
		ID:                 "cfg-tick-pending",
		SlackChannelID:     "C_TICK_PENDING",
		LabelName:          "needs-review",
		DefaultMentionID:   "U_DEFAULT",
		ReviewerList:       "UREV1",
		IsActive:           true,
		BusinessHoursStart: "00:00",
		BusinessHoursEnd:   "23:59",
		Timezone:           "Asia/Tokyo",
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
	db.Create(&config)

	task := models.ReviewTask{
		ID:                      "tick-pending-task",
		PRURL:                   "https://github.com/owner/repo/pull/903",
		Repo:                    "owner/repo",
		PRNumber:                903,
		Title:                   "Waiting with queued re-review",
		SlackTS:                 "1234.9030",
		SlackChannel:            "C_TICK_PENDING",
		Status:                  "waiting_business_hours",
		LabelName:               "needs-review",
		Language:                "ja",
		PendingReReviewNotify:   true,
		PendingReReviewSender:   "<@USENDER>",
		PendingReReviewReviewer: "<@UREVIEWER>",
		CreatedAt:               time.Now(),
		UpdatedAt:               time.Now(),
	}
	db.Create(&task)

	// Same order as the scheduler tick.
	CheckBusinessHoursTasks(db)
	CheckPendingReReviewNotifications(db)

	var updated models.ReviewTask
	db.First(&updated, "id = ?", "tick-pending-task")
	assert.Equal(t, "in_review", updated.Status)
	assert.False(t, updated.PendingReReviewNotify, "activation must drop the queued re-review request")
	assert.Empty(t, updated.PendingReReviewSender)
	assert.Empty(t, updated.PendingReReviewReviewer)
	assert.False(t, gock.HasUnmatchedRequest(), "only the morning greeting may be posted in one tick")
}

// The reload guard only covers the window before the Slack call. A review landing
// while the notification is in flight is caught by the CAS on status alone, so that
// predicate must keep the task out of in_review.
func TestActivateBusinessHoursTask_CASMissWhenReviewedDuringNotification(t *testing.T) {
	db := setupTestDB(t)

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() { _ = os.Setenv("SLACK_BOT_TOKEN", originalToken) }()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.OffAll()
	gock.CleanUnmatchedRequest()

	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	config := models.ChannelConfig{
		ID:               "cfg-cas-miss",
		SlackChannelID:   "C_CAS_MISS",
		LabelName:        "needs-review",
		DefaultMentionID: "U_DEFAULT",
		ReviewerList:     "UREV1",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&config)

	task := models.ReviewTask{
		ID:           "cas-miss-task",
		PRURL:        "https://github.com/owner/repo/pull/904",
		Repo:         "owner/repo",
		PRNumber:     904,
		Title:        "Reviewed mid-notification",
		SlackTS:      "1234.9040",
		SlackChannel: "C_CAS_MISS",
		Status:       "waiting_business_hours",
		LabelName:    "needs-review",
		Language:     "ja",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// Fire right after the reload guard read the still-waiting row, so only the CAS
	// predicate can stop the write.
	err := db.Callback().Query().After("gorm:query").Register("test:complete_after_reload", func(tx *gorm.DB) {
		if tx.Statement.Table != "review_tasks" {
			return
		}
		_ = db.Callback().Query().Remove("test:complete_after_reload")
		db.Model(&models.ReviewTask{}).Where("id = ?", "cas-miss-task").Update("status", "completed")
	})
	assert.NoError(t, err)

	var logs bytes.Buffer
	originalOutput := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(originalOutput)

	assert.NoError(t, activateBusinessHoursTask(db, task, config, "needs-review"))

	log.SetOutput(originalOutput)
	assert.Contains(t, logs.String(), "activation CAS miss",
		"a CAS miss must be reported, not silently treated as an activation")
	assert.NotContains(t, logs.String(), "task activated",
		"a CAS miss must not be logged as a successful activation")

	var updated models.ReviewTask
	db.First(&updated, "id = ?", "cas-miss-task")
	assert.Equal(t, "completed", updated.Status, "CAS must not roll a reviewed task back to in_review")
	assert.Empty(t, updated.Reviewer, "CAS miss must not persist the reviewer assignment")
}
