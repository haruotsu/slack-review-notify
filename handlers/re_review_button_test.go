package handlers

import (
	"net/http"
	"testing"

	"slack-review-notify/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestReReviewButton_MentionsReviewersOnceAndStopsAuthorReminder(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)
	seedMultiReviewerTask(t, db)
	require.NoError(t, db.Model(&models.ReviewTask{}).Where("id = ?", "task-multi").
		Update("awaiting_re_review_from", "<@UAUTHOR>").Error)
	calls := startFakeSlack(t)

	press := func() {
		w := postSignedPayload(t, router, `{"type":"block_actions","user":{"id":"UAUTHOR"},
			"actions":[{"action_id":"request_re_review","value":"task-multi"}],
			"container":{"channel_id":"C12345"},"message":{"ts":"9999.0001"}}`)
		require.Equal(t, http.StatusOK, w.Code)
	}

	press()
	var posted []slackCall
	for len(calls) > 0 {
		if c := <-calls; c.Path == "/chat.postMessage" {
			posted = append(posted, c)
		}
	}
	require.Len(t, posted, 1, "the reviewers should be mentioned right away")
	assert.Equal(t, "1234.5678", posted[0].Body["thread_ts"])
	assert.Contains(t, posted[0].Raw, "UAAA")
	assert.Contains(t, posted[0].Raw, "UBBB")

	var task models.ReviewTask
	require.NoError(t, db.Where("id = ?", "task-multi").First(&task).Error)
	assert.Empty(t, task.AwaitingReReviewFrom)

	press()
	second := nextSlackCall(t, calls)
	assert.Equal(t, "/chat.postEphemeral", second.Path, "a stale button should only tell the presser")
	requireNoSlackCall(t, calls)
}
