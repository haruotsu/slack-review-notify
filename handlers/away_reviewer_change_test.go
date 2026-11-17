package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"slack-review-notify/models"
	"slack-review-notify/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The reminder that skips a reviewer on leave carries "taskID:awayReviewerID"
// in its change button, so pressing it must swap that reviewer and leave the
// ones still at work assigned.
func TestChangeReviewerButtonFromAwayReminder_ReplacesOnlyTheAwayReviewer(t *testing.T) {
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/action", HandleSlackAction(db))

	require.NoError(t, db.Create(&models.ChannelConfig{
		ID: "config-away", SlackChannelID: "C12345", LabelName: "needs-review",
		ReviewerList: "UAAA,UBBB,UCCC", IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)
	require.NoError(t, db.Create(&models.ReviewTask{
		ID: "task-away", PRURL: "https://github.com/test/repo/pull/1", Repo: "test/repo", PRNumber: 1,
		Title: "Test PR", SlackTS: "1234.5678", SlackChannel: "C12345", Status: "in_review",
		Reviewer: "UAAA", Reviewers: "UAAA,UBBB", LabelName: "needs-review",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}).Error)

	payload, err := json.Marshal(map[string]any{
		"type":      "block_actions",
		"user":      map[string]any{"id": "U99999"},
		"actions":   []map[string]any{{"action_id": "change_reviewer", "value": "task-away:UBBB"}},
		"container": map[string]any{"channel_id": "C12345"},
		"message":   map[string]any{"ts": "1234.5678"},
	})
	require.NoError(t, err)
	form := url.Values{}
	form.Add("payload", string(payload))
	req := httptest.NewRequest("POST", "/slack/action", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var got models.ReviewTask
	require.NoError(t, db.Where("id = ?", "task-away").First(&got).Error)
	assert.Equal(t, "UAAA,UCCC", got.Reviewers, "休暇中の UBBB だけが交代し、在席中の UAAA は残る")
}

func pressChangeReviewerButton(t *testing.T, router *gin.Engine, value string) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"type":      "block_actions",
		"user":      map[string]any{"id": "U99999"},
		"actions":   []map[string]any{{"action_id": "change_reviewer", "value": value}},
		"container": map[string]any{"channel_id": "C12345"},
		"message":   map[string]any{"ts": "1234.5678"},
	})
	require.NoError(t, err)
	form := url.Values{}
	form.Add("payload", string(payload))
	req := httptest.NewRequest("POST", "/slack/action", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestChangeReviewerButtonFromAwayReminder_StaleButtonChangesNothing(t *testing.T) {
	tests := []struct {
		name       string
		reviewers  string
		approvedBy string
	}{
		{"ボタンの人が既に交代済み", "UAAA,UCCC", ""},
		{"ボタンの人が既に承認済み", "UAAA,UBBB", "UBBB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			services.IsTestMode = true
			defer func() { services.IsTestMode = false }()
			db := setupTestDB(t)
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.POST("/slack/action", HandleSlackAction(db))

			require.NoError(t, db.Create(&models.ChannelConfig{
				ID: "config-away", SlackChannelID: "C12345", LabelName: "needs-review",
				ReviewerList: "UAAA,UBBB,UCCC,UDDD", IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
			}).Error)
			require.NoError(t, db.Create(&models.ReviewTask{
				ID: "task-away", PRURL: "https://github.com/test/repo/pull/1", Repo: "test/repo", PRNumber: 1,
				Title: "Test PR", SlackTS: "1234.5678", SlackChannel: "C12345", Status: "in_review",
				Reviewer: "UAAA", Reviewers: tt.reviewers, ApprovedBy: tt.approvedBy, LabelName: "needs-review",
				CreatedAt: time.Now(), UpdatedAt: time.Now(),
			}).Error)

			pressChangeReviewerButton(t, router, "task-away:UBBB")

			var got models.ReviewTask
			require.NoError(t, db.Where("id = ?", "task-away").First(&got).Error)
			assert.Equal(t, tt.reviewers, got.Reviewers, "古いボタンでは担当は変わらない")
			assert.Equal(t, "UAAA", got.Reviewer, "Reviewer が担当外の人で上書きされない")
		})
	}
}
