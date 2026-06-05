package services

import (
	"os"
	"slack-review-notify/models"
	"testing"
	"time"

	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
)

// When a task waiting for business hours is activated, the reviewer is selected at
// activation time and announced in a single morning message. Previously the reviewer
// was announced twice: once in the morning greeting and again in a follow-up
// "auto-assigned" message. The two are now merged, so activation must send exactly one
// chat.postMessage that mentions the reviewer and carries the change/pause controls.
func TestActivateBusinessHoursTask_SendsSingleMergedReviewerMessage(t *testing.T) {
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() { _ = os.Setenv("SLACK_BOT_TOKEN", originalToken) }()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	db := setupTestDB(t)

	config := models.ChannelConfig{
		ID:               "cfg-activation",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U_DEFAULT",
		ReviewerList:     "UREV1",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&config)

	task := models.ReviewTask{
		ID:           "task-activation",
		PRURL:        "https://github.com/owner/repo/pull/1",
		Repo:         "owner/repo",
		PRNumber:     1,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Reviewer:     "", // not yet assigned while waiting for business hours
		Status:       "waiting_business_hours",
		LabelName:    "needs-review",
		Language:     "ja",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	defer gock.OffAll()
	gock.CleanUnmatchedRequest()

	// The single morning message must mention the reviewer and carry the change
	// reviewer button and the reminder pause select.
	// json.Marshal escapes "<@UREV1>" to "<@UREV1>" in the body.
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		BodyString(`おはよう[\s\S]*\\u003c@UREV1\\u003e[\s\S]*change_reviewer[\s\S]*pause_reminder_initial`).
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	err := activateBusinessHoursTask(db, task, config, "needs-review")
	assert.NoError(t, err)
	assert.True(t, gock.IsDone(), "expected one merged morning message with reviewer mention and controls")
	assert.False(t, gock.HasUnmatchedRequest(), "expected no second reviewer-assigned message")

	var updated models.ReviewTask
	db.First(&updated, "id = ?", "task-activation")
	assert.Equal(t, "in_review", updated.Status)
	assert.Equal(t, "UREV1", updated.Reviewer)
}

// The merged morning message mentions every assigned reviewer (not just the first) and
// includes the change reviewer button and the reminder pause select.
func TestPostBusinessHoursNotificationToThread_MentionsAllReviewersWithControls(t *testing.T) {
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() { _ = os.Setenv("SLACK_BOT_TOKEN", originalToken) }()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.OffAll()
	gock.CleanUnmatchedRequest()

	// Reviewers are serialized in list order, so the mentions and controls appear in a
	// deterministic sequence in the request body.
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		BodyString(`\\u003c@UREV1\\u003e \\u003c@UREV2\\u003e[\s\S]*change_reviewer[\s\S]*pause_reminder_initial`).
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	task := models.ReviewTask{
		ID:           "task-morning-multi",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Reviewer:     "UREV1",
		Reviewers:    "UREV1,UREV2",
		Status:       "in_review",
		Language:     "ja",
	}

	err := PostBusinessHoursNotificationToThread(task, "UDEFAULT")
	assert.NoError(t, err)
	assert.True(t, gock.IsDone(), "expected a single message mentioning all reviewers with controls")
	assert.False(t, gock.HasUnmatchedRequest())
}
