package handlers

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"slack-review-notify/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const urgentReviewClick = `{"type":"block_actions","user":{"id":"UCLICKER"},
	"actions":[{"action_id":"urgent_review","value":"urgent"}],
	"message":{"ts":"1700000000.000100"},"container":{"channel_id":"C12345"}}`

func createUrgentReviewFixture(t *testing.T, db *gorm.DB, status string) {
	t.Helper()
	require.NoError(t, db.Create(&models.ChannelConfig{
		ID:               "cfg-urgent",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "UTEAM",
		ReviewerList:     "UREV1",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}).Error)
	require.NoError(t, db.Create(&models.ReviewTask{
		ID:              "task-urgent",
		PRURL:           "https://github.com/owner/repo/pull/1",
		Repo:            "owner/repo",
		PRNumber:        1,
		Title:           "Urgent PR",
		SlackTS:         "1700000000.000100",
		SlackChannel:    "C12345",
		PRAuthorSlackID: "UAUTHOR",
		Status:          status,
		LabelName:       "needs-review",
		Language:        "ja",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}).Error)
}

// Pressing the urgent review button on the off-hours message assigns a reviewer and
// mentions them in the thread right away, then rewrites the off-hours message so it
// no longer promises a mention on the next business day morning.
func TestUrgentReviewButton_MentionsReviewerNowAndRewritesOffHoursMessage(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)
	createUrgentReviewFixture(t, db, "waiting_business_hours")
	calls := startFakeSlack(t)

	w := postSignedPayload(t, router, urgentReviewClick)
	require.Equal(t, http.StatusOK, w.Code)

	thread := nextSlackCall(t, calls)
	assert.Equal(t, "/chat.postMessage", thread.Path)
	assert.Equal(t, "1700000000.000100", thread.Body["thread_ts"])
	threadText := fmt.Sprint(thread.Body["blocks"])
	assert.Contains(t, threadText, `<@UCLICKER> から急ぎのレビュー依頼です`)
	assert.Contains(t, threadText, `<@UREV1>`)
	assert.Contains(t, threadText, "change_reviewer")
	assert.NotContains(t, threadText, "おはよう")

	update := nextSlackCall(t, calls)
	assert.Equal(t, "/chat.update", update.Path)
	assert.Equal(t, "1700000000.000100", update.Body["ts"])
	updateText := fmt.Sprint(update.Body["blocks"])
	assert.Contains(t, updateText, "Urgent PR")
	assert.Contains(t, updateText, "https://github.com/owner/repo/pull/1")
	assert.Contains(t, updateText, "<@UCLICKER> が急ぎのレビューを依頼した")
	assert.NotContains(t, updateText, "翌営業日")
	assert.NotContains(t, updateText, "urgent_review")
	assert.Contains(t, updateText, "review_done")
	requireNoSlackCall(t, calls)

	var task models.ReviewTask
	require.NoError(t, db.First(&task, "id = ?", "task-urgent").Error)
	assert.Equal(t, "in_review", task.Status)
	assert.Equal(t, "UREV1", task.Reviewer)
}

// The button stays on the message after the morning activation, so a later press
// must not mention the reviewers a second time.
func TestUrgentReviewButton_AlreadyActivatedTaskOnlyTellsTheClicker(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)
	createUrgentReviewFixture(t, db, "in_review")
	calls := startFakeSlack(t)

	w := postSignedPayload(t, router, urgentReviewClick)
	require.Equal(t, http.StatusOK, w.Code)

	call := nextSlackCall(t, calls)
	assert.Equal(t, "/chat.postEphemeral", call.Path)
	assert.Equal(t, "UCLICKER", call.Body["user"])
	assert.Equal(t, "このPRのレビュー依頼はすでにメンション済みです。", call.Body["text"])
	requireNoSlackCall(t, calls)
}

// The off-hours message keeps its button after the PR is closed or marked done while
// still waiting, so a press then must not claim that reviewers were mentioned.
func TestUrgentReviewButton_FinishedTaskSaysTheReviewIsFinished(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)
	createUrgentReviewFixture(t, db, "completed")
	calls := startFakeSlack(t)

	require.Equal(t, http.StatusOK, postSignedPayload(t, router, urgentReviewClick).Code)

	call := nextSlackCall(t, calls)
	assert.Equal(t, "/chat.postEphemeral", call.Path)
	assert.Equal(t, "このPRのレビューはすでに完了しています。", call.Body["text"])
	requireNoSlackCall(t, calls)
}

func TestUrgentReviewButton_FailureTellsTheClickerAndKeepsTaskWaiting(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)
	createUrgentReviewFixture(t, db, "waiting_business_hours")
	require.NoError(t, db.Where("id = ?", "cfg-urgent").Delete(&models.ChannelConfig{}).Error)
	calls := startFakeSlack(t)

	require.Equal(t, http.StatusOK, postSignedPayload(t, router, urgentReviewClick).Code)

	call := nextSlackCall(t, calls)
	assert.Equal(t, "/chat.postEphemeral", call.Path)
	assert.Equal(t, "急ぎのレビュー依頼を送れませんでした。時間をおいてもう一度お試しください。", call.Body["text"])
	requireNoSlackCall(t, calls)

	var task models.ReviewTask
	require.NoError(t, db.First(&task, "id = ?", "task-urgent").Error)
	assert.Equal(t, "waiting_business_hours", task.Status)
}

func TestUrgentReviewButton_SecondPressDoesNotMentionAgain(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)
	createUrgentReviewFixture(t, db, "waiting_business_hours")
	calls := startFakeSlack(t)

	require.Equal(t, http.StatusOK, postSignedPayload(t, router, urgentReviewClick).Code)
	assert.Equal(t, "/chat.postMessage", nextSlackCall(t, calls).Path)
	assert.Equal(t, "/chat.update", nextSlackCall(t, calls).Path)

	require.Equal(t, http.StatusOK, postSignedPayload(t, router, urgentReviewClick).Code)
	assert.Equal(t, "/chat.postEphemeral", nextSlackCall(t, calls).Path)
	requireNoSlackCall(t, calls)
}
