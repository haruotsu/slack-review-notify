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

// CheckBusinessHoursTasks loads the whole batch up front, so a task reviewed while an
// earlier task in the same batch is being processed is still held as a stale struct.
// It must not receive the morning greeting nor be rolled back to in_review.
func TestCheckBusinessHoursTasks_SkipsTaskReviewedDuringBatch(t *testing.T) {
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

	// Only the first task legitimately gets a morning message. Sending a second one
	// leaves an unmatched request, which is what the assertion below catches.
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	config := models.ChannelConfig{
		ID:                 "cfg-stale-morning",
		SlackChannelID:     "C_STALE_MORNING",
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

	first := models.ReviewTask{
		ID:           "batch-first-task",
		PRURL:        "https://github.com/owner/repo/pull/899",
		Repo:         "owner/repo",
		PRNumber:     899,
		Title:        "Still waiting",
		SlackTS:      "1234.8990",
		SlackChannel: "C_STALE_MORNING",
		Status:       "waiting_business_hours",
		LabelName:    "needs-review",
		Language:     "ja",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&first)

	second := models.ReviewTask{
		ID:           "stale-morning-task",
		PRURL:        "https://github.com/owner/repo/pull/900",
		Repo:         "owner/repo",
		PRNumber:     900,
		Title:        "Reviewed overnight",
		SlackTS:      "1234.9000",
		SlackChannel: "C_STALE_MORNING",
		Status:       "waiting_business_hours",
		LabelName:    "needs-review",
		Language:     "ja",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&second)

	// A review lands mid-batch: completed right after the batch was loaded.
	err := db.Callback().Update().After("gorm:update").Register("test:complete_second", func(tx *gorm.DB) {
		if tx.Statement.Table != "review_tasks" {
			return
		}
		_ = db.Callback().Update().Remove("test:complete_second")
		db.Model(&models.ReviewTask{}).Where("id = ?", "stale-morning-task").Update("status", "completed")
	})
	assert.NoError(t, err)

	CheckBusinessHoursTasks(db)

	var updated models.ReviewTask
	db.First(&updated, "id = ?", "stale-morning-task")
	assert.Equal(t, "completed", updated.Status, "reviewed task must not be rolled back to in_review")
	assert.False(t, gock.HasUnmatchedRequest(), "no morning notification should be sent for a reviewed task")
}

// activateBusinessHoursTask receives a stale in-memory task (as CheckBusinessHoursTasks
// does): it must re-read the row and skip both the notification and the status update
// when the task is no longer waiting_business_hours.
func TestActivateBusinessHoursTask_SkipsWhenStatusChangedSinceLoad(t *testing.T) {
	db := setupTestDB(t)

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() { _ = os.Setenv("SLACK_BOT_TOKEN", originalToken) }()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.OffAll()
	gock.CleanUnmatchedRequest()

	config := models.ChannelConfig{
		ID:               "cfg-stale-activate",
		SlackChannelID:   "C_STALE_ACTIVATE",
		LabelName:        "needs-review",
		DefaultMentionID: "U_DEFAULT",
		ReviewerList:     "UREV1",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&config)

	task := models.ReviewTask{
		ID:           "stale-activate-task",
		PRURL:        "https://github.com/owner/repo/pull/901",
		Repo:         "owner/repo",
		PRNumber:     901,
		Title:        "Reviewed overnight",
		SlackTS:      "1234.9010",
		SlackChannel: "C_STALE_ACTIVATE",
		Status:       "waiting_business_hours",
		LabelName:    "needs-review",
		Language:     "ja",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// Row is reviewed after the caller loaded the stale struct.
	db.Model(&models.ReviewTask{}).Where("id = ?", task.ID).Update("status", "completed")

	err := activateBusinessHoursTask(db, task, config, "needs-review")
	assert.NoError(t, err, "a task reviewed since load is not an error, just a skip")

	var updated models.ReviewTask
	db.First(&updated, "id = ?", "stale-activate-task")
	assert.Equal(t, "completed", updated.Status, "status must not be rolled back")
	assert.Empty(t, updated.Reviewer, "no reviewer should be assigned to a skipped task")
	assert.False(t, gock.HasUnmatchedRequest(), "no morning notification should be sent")
}

// Regression guard for the happy path: a task that really is still waiting gets the
// morning notification and moves to in_review.
func TestCheckBusinessHoursTasks_ActivatesStillWaitingTask(t *testing.T) {
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

	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	config := models.ChannelConfig{
		ID:                 "cfg-waiting-morning",
		SlackChannelID:     "C_WAITING_MORNING",
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
		ID:           "waiting-morning-task",
		PRURL:        "https://github.com/owner/repo/pull/902",
		Repo:         "owner/repo",
		PRNumber:     902,
		Title:        "Still waiting",
		SlackTS:      "1234.9020",
		SlackChannel: "C_WAITING_MORNING",
		Status:       "waiting_business_hours",
		LabelName:    "needs-review",
		Language:     "ja",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	CheckBusinessHoursTasks(db)

	var updated models.ReviewTask
	db.First(&updated, "id = ?", "waiting-morning-task")
	assert.Equal(t, "in_review", updated.Status, "a still-waiting task must be activated")
	assert.Equal(t, "UREV1", updated.Reviewer)
	assert.True(t, gock.IsDone(), "the morning notification must be sent")
}

// main.go runs CheckBusinessHoursTasks and CheckPendingReReviewNotifications back to
// back in the same tick. The morning greeting mentions randomly picked reviewers, so a
// re-review queued for an explicitly nominated reviewer is a separate request and must
// still be delivered.
func TestBusinessHoursTick_ActivatedTaskAlsoGetsQueuedReReviewRequest(t *testing.T) {
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

	// Two posts are expected: the morning greeting and the queued re-review request.
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Times(2).
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
	assert.Equal(t, "UREV1", updated.Reviewer, "the morning greeting mentions a randomly picked reviewer")
	assert.True(t, gock.IsDone(), "both the morning greeting and the queued re-review request must be posted")
	assert.False(t, updated.PendingReReviewNotify, "the queued request is cleared once delivered")
	assert.Empty(t, updated.PendingReReviewSender)
	assert.Empty(t, updated.PendingReReviewReviewer)
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
