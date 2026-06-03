package services

import (
	"os"
	"slack-review-notify/models"
	"testing"

	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
)

// A reviewer stored as a resolved Slack user ID (e.g. "U05JSSXA6RL") must be
// posted as the mention markup "<@U05JSSXA6RL>", not the bare "@U05JSSXA6RL"
// literal — Slack only turns the former into a clickable mention. These tests
// pin the markup the bot actually sends for reminder/morning notifications.

func TestSendOutOfHoursReminderMessage_RendersReviewerAsMention(t *testing.T) {
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() { _ = os.Setenv("SLACK_BOT_TOKEN", originalToken) }()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	// The mock only matches when the posted body contains the proper mention
	// markup. If the bot posts the bare "@U05JSSXA6RL" literal, the request
	// fails to match and SendOutOfHoursReminderMessage returns an error.
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		// json.Marshal HTML-escapes "<" / ">" to < / >, so the
		// proper mention "<@U05JSSXA6RL>" appears in the body as
		// "<@U05JSSXA6RL>". The leading < is what distinguishes
		// a real mention from the broken bare "@U05JSSXA6RL" literal.
		BodyString(`\\u003c@U05JSSXA6RL\\u003e`).
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	task := models.ReviewTask{
		ID:           "task-mention",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Reviewer:     "U05JSSXA6RL",
		Status:       "in_review",
	}

	err := SendOutOfHoursReminderMessage(nil, task)
	assert.NoError(t, err)
	assert.True(t, gock.IsDone(), "expected a chat.postMessage containing <@U05JSSXA6RL>")
}

func TestPostBusinessHoursNotificationToThread_RendersReviewerAsMention(t *testing.T) {
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() { _ = os.Setenv("SLACK_BOT_TOKEN", originalToken) }()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		// json.Marshal HTML-escapes "<" / ">" to < / >, so the
		// proper mention "<@U05JSSXA6RL>" appears in the body as
		// "<@U05JSSXA6RL>". The leading < is what distinguishes
		// a real mention from the broken bare "@U05JSSXA6RL" literal.
		BodyString(`\\u003c@U05JSSXA6RL\\u003e`).
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	task := models.ReviewTask{
		ID:           "task-morning",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Reviewer:     "U05JSSXA6RL",
		Status:       "in_review",
	}

	err := PostBusinessHoursNotificationToThread(task, "UDEFAULT")
	assert.NoError(t, err)
	assert.True(t, gock.IsDone(), "expected a chat.postMessage containing <@U05JSSXA6RL>")
}
