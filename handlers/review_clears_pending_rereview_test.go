package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"slack-review-notify/models"
	"slack-review-notify/services"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

func postReviewSubmitted(t *testing.T, db *gorm.DB, payload string) *httptest.ResponseRecorder {
	t.Helper()
	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)
	return w
}

// A partial approval leaves the task in_review, so a pending re-review notification
// queued overnight would still fire the next morning unless it is cleared here.
func TestHandleReviewSubmittedEvent_PartialApprovalClearsPendingReReview(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() { _ = os.Setenv("SLACK_BOT_TOKEN", originalToken) }()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Times(2).
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	config := models.ChannelConfig{
		ID:                "config-partial-pending",
		SlackChannelID:    "C_PARTIAL_PENDING",
		LabelName:         "needs-review",
		DefaultMentionID:  "UDEFAULT",
		RequiredApprovals: 2,
		IsActive:          true,
	}
	db.Create(&config)

	db.Create(&models.UserMapping{
		ID:             "mapping-partial-pending",
		GithubUsername: "reviewer1",
		SlackUserID:    "UREVIEWER1",
	})

	task := models.ReviewTask{
		ID:                      "partial-pending-task",
		PRURL:                   "https://github.com/owner/repo/pull/910",
		Repo:                    "owner/repo",
		PRNumber:                910,
		Title:                   "Partial approval with pending re-review",
		SlackTS:                 "1234.9100",
		SlackChannel:            "C_PARTIAL_PENDING",
		Reviewer:                "UREVIEWER1",
		Reviewers:               "UREVIEWER1,UREVIEWER2",
		Status:                  "in_review",
		LabelName:               "needs-review",
		PendingReReviewNotify:   true,
		PendingReReviewSender:   "<@USENDER>",
		PendingReReviewReviewer: "<@UREVIEWER1>",
		CreatedAt:               time.Now(),
		UpdatedAt:               time.Now(),
	}
	db.Create(&task)

	payload := `{
		"action": "submitted",
		"pull_request": {"number": 910, "html_url": "https://github.com/owner/repo/pull/910"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "approved", "user": {"login": "reviewer1"}}
	}`

	assert.Equal(t, http.StatusOK, postReviewSubmitted(t, db, payload).Code)

	var updated models.ReviewTask
	db.Where("id = ?", "partial-pending-task").First(&updated)
	assert.Equal(t, "in_review", updated.Status, "partial approval keeps the task in_review")
	assert.False(t, updated.PendingReReviewNotify, "reviewing must clear the pending re-review flag")
	assert.Empty(t, updated.PendingReReviewSender)
	assert.Empty(t, updated.PendingReReviewReviewer)
}

func TestHandleReviewSubmittedEvent_FullApprovalClearsPendingReReview(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() { _ = os.Setenv("SLACK_BOT_TOKEN", originalToken) }()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Times(2).
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	config := models.ChannelConfig{
		ID:                "config-full-pending",
		SlackChannelID:    "C_FULL_PENDING",
		LabelName:         "needs-review",
		DefaultMentionID:  "UDEFAULT",
		RequiredApprovals: 1,
		IsActive:          true,
	}
	db.Create(&config)

	db.Create(&models.UserMapping{
		ID:             "mapping-full-pending",
		GithubUsername: "reviewer1",
		SlackUserID:    "UREVIEWER1",
	})

	task := models.ReviewTask{
		ID:                      "full-pending-task",
		PRURL:                   "https://github.com/owner/repo/pull/911",
		Repo:                    "owner/repo",
		PRNumber:                911,
		Title:                   "Full approval with pending re-review",
		SlackTS:                 "1234.9110",
		SlackChannel:            "C_FULL_PENDING",
		Reviewer:                "UREVIEWER1",
		Status:                  "in_review",
		LabelName:               "needs-review",
		PendingReReviewNotify:   true,
		PendingReReviewSender:   "<@USENDER>",
		PendingReReviewReviewer: "<@UREVIEWER1>",
		CreatedAt:               time.Now(),
		UpdatedAt:               time.Now(),
	}
	db.Create(&task)

	payload := `{
		"action": "submitted",
		"pull_request": {"number": 911, "html_url": "https://github.com/owner/repo/pull/911"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "approved", "user": {"login": "reviewer1"}}
	}`

	assert.Equal(t, http.StatusOK, postReviewSubmitted(t, db, payload).Code)

	var updated models.ReviewTask
	db.Where("id = ?", "full-pending-task").First(&updated)
	assert.Equal(t, "completed", updated.Status)
	assert.False(t, updated.PendingReReviewNotify, "reviewing must clear the pending re-review flag")
}

func TestHandleReviewSubmittedEvent_ChangesRequestedClearsPendingReReview(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() { _ = os.Setenv("SLACK_BOT_TOKEN", originalToken) }()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Times(2).
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	config := models.ChannelConfig{
		ID:               "config-changes-pending",
		SlackChannelID:   "C_CHANGES_PENDING",
		LabelName:        "needs-review",
		DefaultMentionID: "UDEFAULT",
		IsActive:         true,
	}
	db.Create(&config)

	db.Create(&models.UserMapping{
		ID:             "mapping-changes-pending",
		GithubUsername: "reviewer1",
		SlackUserID:    "UREVIEWER1",
	})

	task := models.ReviewTask{
		ID:                      "changes-pending-task",
		PRURL:                   "https://github.com/owner/repo/pull/912",
		Repo:                    "owner/repo",
		PRNumber:                912,
		Title:                   "Changes requested with pending re-review",
		SlackTS:                 "1234.9120",
		SlackChannel:            "C_CHANGES_PENDING",
		Reviewer:                "UREVIEWER1",
		Status:                  "in_review",
		LabelName:               "needs-review",
		PendingReReviewNotify:   true,
		PendingReReviewSender:   "<@USENDER>",
		PendingReReviewReviewer: "<@UREVIEWER1>",
		CreatedAt:               time.Now(),
		UpdatedAt:               time.Now(),
	}
	db.Create(&task)

	payload := `{
		"action": "submitted",
		"pull_request": {"number": 912, "html_url": "https://github.com/owner/repo/pull/912"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "changes_requested", "user": {"login": "reviewer1"}}
	}`

	assert.Equal(t, http.StatusOK, postReviewSubmitted(t, db, payload).Code)

	var updated models.ReviewTask
	db.Where("id = ?", "changes-pending-task").First(&updated)
	assert.Equal(t, "completed", updated.Status)
	assert.False(t, updated.PendingReReviewNotify, "reviewing must clear the pending re-review flag")
}

// End-to-end guard: after a partial approval the next business-day sweep must not send
// the queued re-review notification.
func TestPartialApprovalThenBusinessHoursSweep_SendsNoReReviewNotification(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Tokyo")
	if wd := time.Now().In(loc).Weekday(); wd == time.Saturday || wd == time.Sunday {
		t.Skip("This test requires running on a weekday")
	}

	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() { _ = os.Setenv("SLACK_BOT_TOKEN", originalToken) }()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.OffAll()
	gock.CleanUnmatchedRequest()

	// Only the two approval-flow messages are expected. A morning re-review notification
	// would be a third, unmatched request.
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Times(2).
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	config := models.ChannelConfig{
		ID:                 "config-sweep-pending",
		SlackChannelID:     "C_SWEEP_PENDING",
		LabelName:          "needs-review",
		DefaultMentionID:   "UDEFAULT",
		RequiredApprovals:  2,
		IsActive:           true,
		BusinessHoursStart: "00:00",
		BusinessHoursEnd:   "23:59",
		Timezone:           "Asia/Tokyo",
	}
	db.Create(&config)

	db.Create(&models.UserMapping{
		ID:             "mapping-sweep-pending",
		GithubUsername: "reviewer1",
		SlackUserID:    "UREVIEWER1",
	})

	task := models.ReviewTask{
		ID:                      "sweep-pending-task",
		PRURL:                   "https://github.com/owner/repo/pull/914",
		Repo:                    "owner/repo",
		PRNumber:                914,
		Title:                   "Reviewed overnight, sweep next morning",
		SlackTS:                 "1234.9140",
		SlackChannel:            "C_SWEEP_PENDING",
		Reviewer:                "UREVIEWER1",
		Reviewers:               "UREVIEWER1,UREVIEWER2",
		Status:                  "in_review",
		LabelName:               "needs-review",
		PendingReReviewNotify:   true,
		PendingReReviewSender:   "<@USENDER>",
		PendingReReviewReviewer: "<@UREVIEWER1>",
		CreatedAt:               time.Now(),
		UpdatedAt:               time.Now(),
	}
	db.Create(&task)

	payload := `{
		"action": "submitted",
		"pull_request": {"number": 914, "html_url": "https://github.com/owner/repo/pull/914"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "approved", "user": {"login": "reviewer1"}}
	}`

	assert.Equal(t, http.StatusOK, postReviewSubmitted(t, db, payload).Code)

	services.CheckPendingReReviewNotifications(db)

	assert.False(t, gock.HasUnmatchedRequest(), "no re-review notification should be sent after the review was done")

	var updated models.ReviewTask
	db.Where("id = ?", "sweep-pending-task").First(&updated)
	assert.False(t, updated.PendingReReviewNotify)
}

// A dismissal revokes an approval, so the queued re-review notification must survive.
func TestHandleReviewSubmittedEvent_DismissedKeepsPendingReReview(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	config := models.ChannelConfig{
		ID:                "config-dismiss-pending",
		SlackChannelID:    "C_DISMISS_PENDING",
		LabelName:         "needs-review",
		DefaultMentionID:  "UDEFAULT",
		RequiredApprovals: 2,
		IsActive:          true,
	}
	db.Create(&config)

	db.Create(&models.UserMapping{
		ID:             "mapping-dismiss-pending",
		GithubUsername: "reviewer1",
		SlackUserID:    "UREVIEWER1",
	})

	task := models.ReviewTask{
		ID:                      "dismiss-pending-task",
		PRURL:                   "https://github.com/owner/repo/pull/913",
		Repo:                    "owner/repo",
		PRNumber:                913,
		Title:                   "Dismissed with pending re-review",
		SlackTS:                 "1234.9130",
		SlackChannel:            "C_DISMISS_PENDING",
		Reviewer:                "UREVIEWER1",
		ApprovedBy:              "UREVIEWER1",
		Status:                  "in_review",
		LabelName:               "needs-review",
		PendingReReviewNotify:   true,
		PendingReReviewSender:   "<@USENDER>",
		PendingReReviewReviewer: "<@UREVIEWER1>",
		CreatedAt:               time.Now(),
		UpdatedAt:               time.Now(),
	}
	db.Create(&task)

	payload := `{
		"action": "submitted",
		"pull_request": {"number": 913, "html_url": "https://github.com/owner/repo/pull/913"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "dismissed", "user": {"login": "reviewer1"}}
	}`

	assert.Equal(t, http.StatusOK, postReviewSubmitted(t, db, payload).Code)

	var updated models.ReviewTask
	db.Where("id = ?", "dismiss-pending-task").First(&updated)
	assert.Equal(t, "in_review", updated.Status)
	assert.True(t, updated.PendingReReviewNotify, "dismissal must keep the pending re-review notification")
	assert.Equal(t, "<@USENDER>", updated.PendingReReviewSender)
	assert.Equal(t, "<@UREVIEWER1>", updated.PendingReReviewReviewer)
}
