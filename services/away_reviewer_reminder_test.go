package services

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"slack-review-notify/models"

	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func reminderText(t *testing.T, blocks []map[string]interface{}) string {
	t.Helper()
	require.NotEmpty(t, blocks)
	return blocks[0]["text"].(map[string]interface{})["text"].(string)
}

func reminderActionIDs(blocks []map[string]interface{}) []string {
	var ids []string
	for _, b := range blocks {
		if b["type"] != "actions" {
			continue
		}
		for _, e := range b["elements"].([]map[string]interface{}) {
			ids = append(ids, e["action_id"].(string))
		}
	}
	return ids
}

func TestBuildReviewerReminderBlocks(t *testing.T) {
	task := models.ReviewTask{ID: "task-1", Reviewer: "UAAA", Reviewers: "UAAA,UBBB", Language: "ja"}

	t.Run("休暇中のレビュワーがいなければ従来どおり全員をメンションする", func(t *testing.T) {
		blocks := BuildReviewerReminderBlocks(task, nil)

		text := reminderText(t, blocks)
		assert.Contains(t, text, "<@UAAA> <@UBBB>")
		assert.NotContains(t, text, "休暇中")
		assert.Equal(t, []string{"pause_reminder"}, reminderActionIDs(blocks))
	})

	t.Run("休暇中のレビュワーはメンションせず人数だけ案内し交代ボタンを出す", func(t *testing.T) {
		blocks := BuildReviewerReminderBlocks(task, []string{"UAAA", "UOTHER"})

		text := reminderText(t, blocks)
		assert.NotContains(t, text, "UAAA", "休暇中の人は ID も含めない（<@…> だと通知が飛ぶ）")
		assert.Contains(t, text, "<@UBBB>")
		assert.Contains(t, text, "休暇中のレビュワー 1 人にはメンションしていません")
		assert.Equal(t, []string{"change_reviewer", "pause_reminder"}, reminderActionIDs(blocks))
	})

	t.Run("全員が休暇中なら誰もメンションしない", func(t *testing.T) {
		blocks := BuildReviewerReminderBlocks(task, []string{"UAAA", "UBBB"})

		text := reminderText(t, blocks)
		assert.NotContains(t, text, "<@")
		assert.Contains(t, text, "レビュワー全員（2 人）が休暇中")
		assert.Equal(t, []string{"change_reviewer", "pause_reminder"}, reminderActionIDs(blocks))
	})

	t.Run("承認済みのレビュワーは休暇の有無に関係なく数えない", func(t *testing.T) {
		approved := task
		approved.ApprovedBy = "UAAA"

		blocks := BuildReviewerReminderBlocks(approved, []string{"UAAA"})

		text := reminderText(t, blocks)
		assert.Contains(t, text, "<@UBBB>")
		assert.NotContains(t, text, "休暇中")
	})

	t.Run("Reviewers 未設定の旧タスクでも Reviewer の休暇を考慮する", func(t *testing.T) {
		legacy := models.ReviewTask{ID: "task-2", Reviewer: "UAAA", Language: "ja"}

		text := reminderText(t, BuildReviewerReminderBlocks(legacy, []string{"UAAA"}))

		assert.NotContains(t, text, "<@")
		assert.Contains(t, text, "レビュワー全員（1 人）が休暇中")
	})

	t.Run("英語チャンネルでは英語で案内する", func(t *testing.T) {
		en := task
		en.Language = "en"

		text := reminderText(t, BuildReviewerReminderBlocks(en, []string{"UAAA"}))

		assert.Contains(t, text, "Not mentioning 1 reviewer(s) on leave")
	})
}

func TestSendReviewerReminderMessage_SkipsReviewerWhoWentOnLeaveAfterAssignment(t *testing.T) {
	db := setupTestDB(t)
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() { _ = os.Setenv("SLACK_BOT_TOKEN", originalToken) }()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")
	defer gock.Off()

	// Assigned yesterday; the leave started this morning and runs until tomorrow.
	from := time.Now().Add(-2 * time.Hour)
	until := time.Now().Add(24 * time.Hour)
	require.NoError(t, db.Create(&models.ReviewerAvailability{
		ID: "away-1", SlackUserID: "UAAA", AwayFrom: &from, AwayUntil: &until,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)

	gock.New("https://slack.com").
		Get("/api/conversations.info").
		MatchParam("channel", "C12345").
		Reply(200).
		JSON(map[string]interface{}{"ok": true, "channel": map[string]interface{}{"id": "C12345", "is_archived": false}})

	var posted map[string]interface{}
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		AddMatcher(func(req *http.Request, _ *gock.Request) (bool, error) {
			return true, json.NewDecoder(req.Body).Decode(&posted)
		}).
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	task := models.ReviewTask{
		ID: "task-1", SlackTS: "1234.5678", SlackChannel: "C12345", Status: "in_review",
		Reviewer: "UAAA", Reviewers: "UAAA,UBBB", Language: "ja",
		CreatedAt: time.Now().Add(-24 * time.Hour), UpdatedAt: time.Now().Add(-time.Hour),
	}

	require.NoError(t, SendReviewerReminderMessage(db, task))
	assert.True(t, gock.IsDone())

	blocks := posted["blocks"].([]interface{})
	text := blocks[0].(map[string]interface{})["text"].(map[string]interface{})["text"].(string)
	assert.NotContains(t, text, "<@UAAA>", "休暇に入ったレビュワーにはメンションしない")
	assert.Contains(t, text, "<@UBBB>")
}
