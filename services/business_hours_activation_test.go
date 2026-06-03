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
// activation time. The morning greeting must mention that freshly-selected reviewer,
// which only works if the reviewer is assigned to the task before the notification is
// sent. This guards against the notification being posted before the assignment.
func TestActivateBusinessHoursTask_MorningGreetingMentionsAssignedReviewer(t *testing.T) {
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

	defer gock.Off()

	// The morning greeting ("おはよう") must contain the assigned reviewer mention.
	// json.Marshal escapes "<@UREV1>" to "<@UREV1>" in the body.
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		BodyString(`おはよう[\s\S]*\\u003c@UREV1\\u003e`).
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// The follow-up "reviewer assigned + change button" message.
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	err := activateBusinessHoursTask(db, task, config, "needs-review")
	assert.NoError(t, err)
	assert.True(t, gock.IsDone(), "expected the morning greeting to mention <@UREV1>")

	var updated models.ReviewTask
	db.First(&updated, "id = ?", "task-activation")
	assert.Equal(t, "in_review", updated.Status)
	assert.Equal(t, "UREV1", updated.Reviewer)
}
