package services

import (
	"os"
	"slack-review-notify/models"
	"testing"

	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
)

// A reviewer's Slack user ID must be posted as "<@U…>" so Slack renders a
// clickable mention; "@U…" alone stays plain text.

func TestSendOutOfHoursReminderMessage_RendersReviewerAsMention(t *testing.T) {
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() { _ = os.Setenv("SLACK_BOT_TOKEN", originalToken) }()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	// json.Marshal escapes "<@U…>" to "<@U…>" in the body.
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		BodyString(`\\u003c@UREVIEWER\\u003e`).
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	task := models.ReviewTask{
		ID:           "task-mention",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Reviewer:     "UREVIEWER",
		Status:       "in_review",
	}

	err := SendOutOfHoursReminderMessage(nil, task)
	assert.NoError(t, err)
	assert.True(t, gock.IsDone(), "expected a chat.postMessage containing <@UREVIEWER>")
}

func TestPostBusinessHoursNotificationToThread_RendersReviewerAsMention(t *testing.T) {
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() { _ = os.Setenv("SLACK_BOT_TOKEN", originalToken) }()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	// json.Marshal escapes "<@U…>" to "<@U…>" in the body.
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		BodyString(`\\u003c@UREVIEWER\\u003e`).
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	task := models.ReviewTask{
		ID:           "task-morning",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Reviewer:     "UREVIEWER",
		Status:       "in_review",
	}

	err := PostBusinessHoursNotificationToThread(task, "UDEFAULT")
	assert.NoError(t, err)
	assert.True(t, gock.IsDone(), "expected a chat.postMessage containing <@UREVIEWER>")
}
