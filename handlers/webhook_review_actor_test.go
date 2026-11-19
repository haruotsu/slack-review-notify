package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"slack-review-notify/models"
	"slack-review-notify/services"

	"github.com/gin-gonic/gin"
	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
)

func submitReviewForActorTest(t *testing.T, state, reviewerLogin, reviewerType string) (taskStatus string, notified bool) {
	t.Helper()

	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true
	t.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Persist().
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	task := models.ReviewTask{
		ID:           "actor-task",
		PRURL:        "https://github.com/owner/repo/pull/400",
		Repo:         "owner/repo",
		PRNumber:     400,
		Title:        "Actor PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C_ACTOR",
		Reviewer:     "UREVIEWER1",
		Status:       "in_review",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-1 * time.Hour),
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}
	assert.NoError(t, db.Create(&task).Error)

	notifyCount := 0
	gock.Observe(func(_ *http.Request, _ gock.Mock) { notifyCount++ })
	defer gock.Observe(nil)

	payload := fmt.Sprintf(`{
		"action": "submitted",
		"pull_request": {"number": 400, "user": {"login": "pr-author", "type": "User"}},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": %q, "user": {"login": %q, "type": %q}}
	}`, state, reviewerLogin, reviewerType)

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.New()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var updated models.ReviewTask
	assert.NoError(t, db.Where("id = ?", "actor-task").First(&updated).Error)
	return updated.Status, notifyCount > 0
}

func TestHandleReviewSubmittedEvent_BotFeedbackKeepsRemindersRunning(t *testing.T) {
	for _, state := range []string{"commented", "changes_requested"} {
		t.Run(state, func(t *testing.T) {
			status, notified := submitReviewForActorTest(t, state, "github-actions[bot]", "Bot")

			assert.Equal(t, "in_review", status, "a bot review is not a human review, so the reviewers still need reminding")
			assert.False(t, notified, "a bot should not be thanked in the thread")
		})
	}
}

func TestHandleReviewSubmittedEvent_AuthorReplyKeepsRemindersRunning(t *testing.T) {
	status, notified := submitReviewForActorTest(t, "commented", "pr-author", "User")

	assert.Equal(t, "in_review", status, "the author replying on their own PR is not a review")
	assert.False(t, notified, "the author should not be thanked for reviewing their own PR")
}

func TestHandleReviewSubmittedEvent_HumanReviewerFeedbackStopsReminders(t *testing.T) {
	for _, state := range []string{"commented", "changes_requested"} {
		t.Run(state, func(t *testing.T) {
			status, notified := submitReviewForActorTest(t, state, "reviewer1", "User")

			assert.Equal(t, "completed", status)
			assert.True(t, notified)
		})
	}
}

func TestHandleReviewSubmittedEvent_BotApprovalStillCountsAsApproval(t *testing.T) {
	status, notified := submitReviewForActorTest(t, "approved", "approver[bot]", "Bot")

	assert.Equal(t, "completed", status)
	assert.True(t, notified)
}
