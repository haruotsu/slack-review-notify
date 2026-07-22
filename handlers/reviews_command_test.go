package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"slack-review-notify/models"
	"slack-review-notify/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type reviewsCommandResponse struct {
	ResponseType string `json:"response_type"`
	Text         string `json:"text"`
}

func runReviewsCommand(t *testing.T, db *gorm.DB, channelID string) reviewsCommandResponse {
	t.Helper()
	services.IsTestMode = true
	t.Cleanup(func() { services.IsTestMode = false })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", HandleSlackCommand(db))

	req := setupHTTPRequest(t, "show-my-reviews", channelID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	var response reviewsCommandResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	return response
}

func TestReviewsCommandShowsAssignedActiveTasksAcrossChannels(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)
	base := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	tasks := []models.ReviewTask{
		{ID: "02", PRURL: "https://github.com/example/web/pull/102", Repo: "example/web", PRNumber: 102, Title: "Web fix", SlackChannel: "C_FRONTEND", Reviewer: " U12345 ", Status: "waiting_business_hours", CreatedAt: base},
		{ID: "01", PRURL: "https://github.com/example/api/pull/101", Repo: "example/api", PRNumber: 101, Title: "  Fix  <auth> &\n login ", SlackChannel: "C_BACKEND", Reviewers: "U999, U12345", Status: "in_review", CreatedAt: base},
		{ID: "03", PRURL: "https://github.com/example/app/pull/103", Repo: "example/app", PRNumber: 103, Title: "App fix", SlackChannel: "C_APP", Reviewer: "U12345", Reviewers: "U12345", Status: "paused", CreatedAt: base.Add(2 * time.Minute)},
		{ID: "04", PRURL: "https://github.com/example/job/pull/104", Repo: "example/job", PRNumber: 104, Title: "Job fix", SlackChannel: "C_JOB", Reviewers: "U12345", Status: "pending", CreatedAt: base.Add(3 * time.Minute)},
		{ID: "05", PRURL: "https://github.com/example/legacy/pull/105", Repo: "example/legacy", PRNumber: 105, Title: "Legacy fix", SlackChannel: "C_LEGACY", Reviewers: "U12345", Status: "snoozed", CreatedAt: base.Add(4 * time.Minute)},
		{ID: "06", PRURL: "https://github.com/example/other/pull/106", Repo: "example/other", PRNumber: 106, Title: "Other user", SlackChannel: "C_OTHER", Reviewers: "U123450", Status: "in_review", CreatedAt: base.Add(5 * time.Minute)},
		{ID: "07", PRURL: "https://github.com/example/done/pull/107", Repo: "example/done", PRNumber: 107, Title: "Completed", SlackChannel: "C_DONE", Reviewers: "U12345", Status: "completed", CreatedAt: base.Add(6 * time.Minute)},
		{ID: "08", PRURL: "https://github.com/example/manual/pull/108", Repo: "example/manual", PRNumber: 108, Title: "Done", SlackChannel: "C_DONE", Reviewers: "U12345", Status: "done", CreatedAt: base.Add(7 * time.Minute)},
		{ID: "09", PRURL: "https://github.com/example/archive/pull/109", Repo: "example/archive", PRNumber: 109, Title: "Archived", SlackChannel: "C_ARCHIVE", Reviewers: "U12345", Status: "archived", CreatedAt: base.Add(8 * time.Minute)},
	}
	for i := range tasks {
		require.NoError(t, db.Create(&tasks[i]).Error)
	}
	deleted := models.ReviewTask{ID: "10", PRURL: "https://github.com/example/deleted/pull/110", Repo: "example/deleted", PRNumber: 110, Title: "Deleted", SlackChannel: "C_DELETED", Reviewers: "U12345", Status: "in_review", CreatedAt: base.Add(9 * time.Minute)}
	require.NoError(t, db.Create(&deleted).Error)
	require.NoError(t, db.Delete(&deleted).Error)

	response := runReviewsCommand(t, db, "C_COMMAND")
	assert.Equal(t, "ephemeral", response.ResponseType)
	assert.Contains(t, response.Text, ":clipboard: *あなたへの未完了レビュー依頼 (4件)*")
	assert.Contains(t, response.Text, ":large_blue_circle: <https://github.com/example/api/pull/101|example/api #101>")
	assert.Contains(t, response.Text, "Fix &lt;auth&gt; &amp; login")
	assert.Contains(t, response.Text, "<#C_BACKEND> | レビュー中")
	assert.Contains(t, response.Text, ":crescent_moon:")
	assert.Contains(t, response.Text, ":hourglass_flowing_sand:")
	assert.Contains(t, response.Text, ":zzz:")

	expectedOrder := []string{"pull/101", "pull/102", "pull/104", "pull/105"}
	lastIndex := -1
	for _, fragment := range expectedOrder {
		index := strings.Index(response.Text, fragment)
		assert.Greater(t, index, lastIndex, "expected %s after the previous PR", fragment)
		lastIndex = index
	}
	assert.NotContains(t, response.Text, "pull/103")
	assert.NotContains(t, response.Text, "pull/106")
	assert.NotContains(t, response.Text, "pull/107")
	assert.NotContains(t, response.Text, "pull/108")
	assert.NotContains(t, response.Text, "pull/109")
	assert.NotContains(t, response.Text, "pull/110")
}

func TestReviewsCommandFiltersUnrelatedTasksBeforeLoading(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)
	assigned := models.ReviewTask{
		ID: "assigned", PRURL: "https://github.com/example/assigned/pull/1", Repo: "example/assigned",
		PRNumber: 1, Title: "Assigned", SlackChannel: "C_ASSIGNED", Reviewers: "U12345", Status: "in_review",
	}
	unrelated := models.ReviewTask{
		ID: "unrelated", PRURL: "https://github.com/example/unrelated/pull/2", Repo: "example/unrelated",
		PRNumber: 2, Title: "Unrelated", SlackChannel: "C_UNRELATED", Reviewers: "U99999", Status: "in_review",
	}
	require.NoError(t, db.Create(&assigned).Error)
	require.NoError(t, db.Create(&unrelated).Error)

	const callbackName = "test:reject_unrelated_review_load"
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(callbackName, func(tx *gorm.DB) {
		tasks, ok := tx.Statement.Dest.(*[]models.ReviewTask)
		if !ok {
			return
		}
		for _, task := range *tasks {
			if task.ID == unrelated.ID {
				_ = tx.AddError(errors.New("unrelated review task was loaded"))
				return
			}
		}
	}))

	response := runReviewsCommand(t, db, "C_COMMAND")
	assert.Contains(t, response.Text, "pull/1|example/assigned #1")
	assert.NotContains(t, response.Text, "レビュー依頼の取得に失敗しました")
}

func TestReviewsCommandExcludesTaskAlreadyApprovedByUser(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)
	task := models.ReviewTask{
		ID: "already-approved", PRURL: "https://github.com/example/approved/pull/1", Repo: "example/approved",
		PRNumber: 1, Title: "Already approved", SlackChannel: "C_APPROVED",
		Reviewers: "U12345,U67890", ApprovedBy: "U12345", Status: "in_review",
	}
	require.NoError(t, db.Create(&task).Error)

	response := runReviewsCommand(t, db, "C_COMMAND")
	assert.Equal(t, ":white_check_mark: あなたへの未完了レビュー依頼はありません。", response.Text)
}

func TestReviewsLabelStillShowsItsConfiguration(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)
	config := models.ChannelConfig{
		ID:             "reviews-label-config",
		SlackChannelID: "C_COMMAND",
		LabelName:      "reviews",
		RepositoryList: "example/reviews",
		IsActive:       true,
	}
	require.NoError(t, db.Create(&config).Error)

	services.IsTestMode = true
	t.Cleanup(func() { services.IsTestMode = false })
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", HandleSlackCommand(db))

	req := setupHTTPRequest(t, "reviews", "C_COMMAND")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "reviews")
	assert.Contains(t, w.Body.String(), "example/reviews")
	assert.NotContains(t, w.Body.String(), `"response_type":"ephemeral"`)
}

func TestReviewsCommandReturnsLocalizedEmptyState(t *testing.T) {
	tests := []struct {
		name      string
		language  string
		expected  string
		channelID string
	}{
		{name: "Japanese default", expected: ":white_check_mark: あなたへの未完了レビュー依頼はありません。", channelID: "C_JA"},
		{name: "English config", language: "en", expected: ":white_check_mark: You have no unfinished review requests.", channelID: "C_EN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupCommandIntegrationTestDB(t)
			if tt.language != "" {
				config := models.ChannelConfig{ID: "config-" + tt.language, SlackChannelID: tt.channelID, LabelName: "needs-review", Language: tt.language}
				require.NoError(t, db.Create(&config).Error)
			}
			response := runReviewsCommand(t, db, tt.channelID)
			assert.Equal(t, "ephemeral", response.ResponseType)
			assert.Equal(t, tt.expected, response.Text)
		})
	}
}

func TestReviewsCommandUsesEnglishPresentation(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)
	config := models.ChannelConfig{ID: "config-en", SlackChannelID: "C_EN", LabelName: "needs-review", Language: "en"}
	task := models.ReviewTask{ID: "english-task", PRURL: "https://github.com/example/en/pull/201", Repo: "example/en", PRNumber: 201, Title: "English title", SlackChannel: "C_EN_REVIEW", Reviewers: "U12345", Status: "waiting_business_hours", CreatedAt: time.Now()}
	require.NoError(t, db.Create(&config).Error)
	require.NoError(t, db.Create(&task).Error)

	response := runReviewsCommand(t, db, "C_EN")
	assert.Contains(t, response.Text, ":clipboard: *Your unfinished review requests (1)*")
	assert.Contains(t, response.Text, ":crescent_moon: <https://github.com/example/en/pull/201|example/en #201>")
	assert.Contains(t, response.Text, "<#C_EN_REVIEW> | Waiting for business hours")
}

func TestReviewsCommandLimitsResultsToOldest50(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)
	base := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 51; i++ {
		task := models.ReviewTask{
			ID: fmt.Sprintf("task-%03d", i), PRURL: fmt.Sprintf("https://github.com/example/limit/pull/%03d", i),
			Repo: "example/limit", PRNumber: i, Title: fmt.Sprintf("Task %03d", i), SlackChannel: "C_LIMIT",
			Reviewers: "U12345", Status: "in_review", CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}
		require.NoError(t, db.Create(&task).Error)
	}

	response := runReviewsCommand(t, db, "C_COMMAND")
	assert.Contains(t, response.Text, "未完了レビュー依頼 (51件)")
	assert.Equal(t, 50, strings.Count(response.Text, ":large_blue_circle:"))
	assert.Contains(t, response.Text, "pull/000|example/limit #0")
	assert.Contains(t, response.Text, "pull/049|example/limit #49")
	assert.NotContains(t, response.Text, "pull/050|example/limit #50")
	assert.Contains(t, response.Text, ":information_source: ほか1件あります。")
}

func TestReviewsCommandReturnsEphemeralDatabaseError(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	response := runReviewsCommand(t, db, "C_COMMAND")
	assert.Equal(t, "ephemeral", response.ResponseType)
	assert.Equal(t, ":warning: レビュー依頼の取得に失敗しました。", response.Text)
}
