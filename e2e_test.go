//go:build e2e
// +build e2e

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"slack-review-notify/handlers"
	"slack-review-notify/models"
	"slack-review-notify/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const slackhogBaseURL = "http://localhost:14112"

type slackhogMessage struct {
	ID         string `json:"id"`
	Channel    string `json:"channel"`
	Text       string `json:"text"`
	ThreadTS   string `json:"thread_ts"`
	ReplyCount int    `json:"reply_count"`
	RawPayload any    `json:"raw_payload"`
}

type slackhogMessagesResponse struct {
	Messages []slackhogMessage `json:"messages"`
	Channels []string          `json:"channels"`
}

type slackhogRepliesResponse struct {
	ParentID string            `json:"parent_id"`
	Replies  []slackhogMessage `json:"replies"`
}

type slackhogPostResponse struct {
	OK      bool   `json:"ok"`
	Channel string `json:"channel"`
	TS      string `json:"ts"`
}

func setupE2EApp(t *testing.T) (*gorm.DB, *httptest.Server) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.ReviewTask{}, &models.ChannelConfig{}, &models.UserMapping{}, &models.ReviewerAvailability{}))

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/webhook", handlers.HandleGitHubWebhook(db))
	r.POST("/slack/actions", handlers.HandleSlackAction(db))

	ts := httptest.NewServer(r)
	return db, ts
}

func clearSlackhogMessages(t *testing.T) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, slackhogBaseURL+"/_api/messages", nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
}

func getSlackhogMessages(t *testing.T, channel string) []slackhogMessage {
	t.Helper()
	url := slackhogBaseURL + "/_api/messages"
	if channel != "" {
		url += "?channel=" + channel
	}
	resp, err := http.DefaultClient.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result slackhogMessagesResponse
	require.NoError(t, json.Unmarshal(body, &result))
	return result.Messages
}

func getSlackhogReplies(t *testing.T, parentID string) []slackhogMessage {
	t.Helper()
	url := fmt.Sprintf("%s/_api/messages/%s/replies", slackhogBaseURL, parentID)
	resp, err := http.DefaultClient.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result slackhogRepliesResponse
	require.NoError(t, json.Unmarshal(body, &result))
	return result.Replies
}

// postRootMessage creates a root message in slackhog and returns the ts and parent ID.
func postRootMessage(t *testing.T, channel string, text string) (ts string, parentID string) {
	t.Helper()
	payload := fmt.Sprintf(`{"channel":"%s","text":"%s"}`, channel, text)
	resp, err := http.Post(slackhogBaseURL+"/api/chat.postMessage", "application/json", strings.NewReader(payload))
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result slackhogPostResponse
	require.NoError(t, json.Unmarshal(body, &result))
	require.True(t, result.OK)

	// Get the parent ID from slackhog messages
	time.Sleep(100 * time.Millisecond)
	msgs := getSlackhogMessages(t, channel)
	for _, m := range msgs {
		if strings.Contains(m.Text, text) {
			return result.TS, m.ID
		}
	}
	t.Fatal("Root message not found in slackhog")
	return "", ""
}

func sendWebhook(t *testing.T, serverURL string, payload string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", serverURL+"/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func TestMain(m *testing.M) {
	_, err := http.Get(slackhogBaseURL + "/_api/messages")
	if err != nil {
		fmt.Println("slackhog is not running at " + slackhogBaseURL)
		fmt.Println("Start it with: docker run --rm -d -p 14112:4112 --name slackhog-e2e ghcr.io/harakeishi/slackhog")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// Test: Re-review during business hours sends notification immediately to slackhog
func TestE2E_ReReview_DuringBusinessHours_SendsImmediately(t *testing.T) {
	originalURL := os.Getenv("SLACK_API_BASE_URL")
	os.Setenv("SLACK_API_BASE_URL", slackhogBaseURL+"/api")
	defer os.Setenv("SLACK_API_BASE_URL", originalURL)

	services.IsTestMode = true
	db, ts := setupE2EApp(t)
	defer ts.Close()
	clearSlackhogMessages(t)

	loc, _ := time.LoadLocation("Asia/Tokyo")
	now := time.Now().In(loc)
	if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
		t.Skip("Skipping: weekend")
	}

	config := models.ChannelConfig{
		ID:                 "e2e-bh-config",
		SlackChannelID:     "C_E2E_BH",
		LabelName:          "needs-review",
		DefaultMentionID:   "U_E2E_USER",
		IsActive:           true,
		BusinessHoursStart: "00:00",
		BusinessHoursEnd:   "23:59",
		Timezone:           "Asia/Tokyo",
	}
	db.Create(&config)

	// Create a root message in slackhog (simulates the original PR notification)
	rootTS, parentID := postRootMessage(t, "C_E2E_BH", "PR notification for pull/10")

	task := models.ReviewTask{
		ID:           "e2e-rereview-bh",
		PRURL:        "https://github.com/e2e-owner/e2e-repo/pull/10",
		Repo:         "e2e-owner/e2e-repo",
		PRNumber:     10,
		Title:        "E2E Re-review PR",
		SlackTS:      rootTS,
		SlackChannel: "C_E2E_BH",
		Status:       "in_review",
		LabelName:    "needs-review",
		Reviewer:     "U_REVIEWER1",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	payload := `{
		"action": "review_requested",
		"pull_request": {"number": 10, "html_url": "https://github.com/e2e-owner/e2e-repo/pull/10"},
		"repository": {"full_name": "e2e-owner/e2e-repo", "owner": {"login": "e2e-owner"}, "name": "e2e-repo"},
		"sender": {"login": "e2e-author"},
		"requested_reviewer": {"login": "e2e-reviewer"}
	}`

	resp := sendWebhook(t, ts.URL, payload)
	resp.Body.Close()
	time.Sleep(500 * time.Millisecond)

	// Verify: re-review notification appears as a thread reply in slackhog
	replies := getSlackhogReplies(t, parentID)
	found := false
	for _, r := range replies {
		if strings.Contains(r.Text, "再レビュー") || strings.Contains(r.Text, "re-review") {
			found = true
			break
		}
	}
	assert.True(t, found, "Re-review notification should appear as thread reply in slackhog")

	// Verify: no pending flag on task
	var updatedTask models.ReviewTask
	db.Where("id = ?", "e2e-rereview-bh").First(&updatedTask)
	assert.False(t, updatedTask.PendingReReviewNotify, "No pending flag during business hours")
}

// Test: Re-review outside business hours defers, then sends after business hours start
func TestE2E_ReReview_OutsideBusinessHours_DefersAndSendsLater(t *testing.T) {
	originalURL := os.Getenv("SLACK_API_BASE_URL")
	os.Setenv("SLACK_API_BASE_URL", slackhogBaseURL+"/api")
	defer os.Setenv("SLACK_API_BASE_URL", originalURL)

	services.IsTestMode = true
	db, ts := setupE2EApp(t)
	defer ts.Close()
	clearSlackhogMessages(t)

	loc, _ := time.LoadLocation("Asia/Tokyo")
	now := time.Now().In(loc)
	if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
		t.Skip("Skipping: weekend")
	}
	if now.Hour() == 3 && now.Minute() == 0 {
		t.Skip("Skipping: inside the narrow test business hours window")
	}

	config := models.ChannelConfig{
		ID:                 "e2e-offhours-config",
		SlackChannelID:     "C_E2E_BH",
		LabelName:          "needs-review",
		DefaultMentionID:   "U_E2E_USER",
		IsActive:           true,
		BusinessHoursStart: "03:00",
		BusinessHoursEnd:   "03:01",
		Timezone:           "Asia/Tokyo",
	}
	db.Create(&config)

	rootTS, parentID := postRootMessage(t, "C_E2E_BH", "PR notification for pull/20")

	task := models.ReviewTask{
		ID:           "e2e-rereview-offhours",
		PRURL:        "https://github.com/e2e-owner/e2e-repo/pull/20",
		Repo:         "e2e-owner/e2e-repo",
		PRNumber:     20,
		Title:        "E2E Off-hours PR",
		SlackTS:      rootTS,
		SlackChannel: "C_E2E_BH",
		Status:       "in_review",
		LabelName:    "needs-review",
		Reviewer:     "U_REVIEWER1",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// Step 1: Send re-review (should be deferred)
	payload := `{
		"action": "review_requested",
		"pull_request": {"number": 20, "html_url": "https://github.com/e2e-owner/e2e-repo/pull/20"},
		"repository": {"full_name": "e2e-owner/e2e-repo", "owner": {"login": "e2e-owner"}, "name": "e2e-repo"},
		"sender": {"login": "e2e-author"},
		"requested_reviewer": {"login": "e2e-reviewer"}
	}`
	resp := sendWebhook(t, ts.URL, payload)
	resp.Body.Close()
	time.Sleep(500 * time.Millisecond)

	// Step 2: Verify only the deferred feedback message is sent (no mention, no re-review assignment)
	replies := getSlackhogReplies(t, parentID)
	require.Len(t, replies, 1, "Only deferred feedback message should be sent outside business hours")
	assert.Contains(t, replies[0].Text, "営業時間外", "Feedback message should mention outside business hours")
	assert.NotContains(t, replies[0].Text, "<@", "Feedback message should not contain mentions")

	// Step 3: Verify pending flag is set
	var deferredTask models.ReviewTask
	db.Where("id = ?", "e2e-rereview-offhours").First(&deferredTask)
	assert.True(t, deferredTask.PendingReReviewNotify, "Pending flag should be set")
	assert.NotEmpty(t, deferredTask.PendingReReviewSender)
	assert.NotEmpty(t, deferredTask.PendingReReviewReviewer)

	// Step 4: Simulate business hours starting (change config to 24h)
	db.Model(&models.ChannelConfig{}).Where("id = ?", "e2e-offhours-config").Updates(map[string]interface{}{
		"business_hours_start": "00:00",
		"business_hours_end":   "23:59",
	})

	// Step 5: Run the checker
	services.CheckPendingReReviewNotifications(db)
	time.Sleep(500 * time.Millisecond)

	// Step 6: Verify deferred notification NOW appears (total: 1 feedback + 1 deferred = 2 replies)
	replies = getSlackhogReplies(t, parentID)
	reReviewCount := 0
	for _, r := range replies {
		if strings.Contains(r.Text, "再レビュー") || strings.Contains(r.Text, "re-review") {
			reReviewCount++
		}
	}
	assert.Equal(t, 2, reReviewCount, "1 feedback message + 1 deferred re-review notification should appear")

	// Step 7: Verify pending flag is cleared
	db.Where("id = ?", "e2e-rereview-offhours").First(&deferredTask)
	assert.False(t, deferredTask.PendingReReviewNotify, "Pending flag should be cleared")
}

// Test: Multiple re-reviews outside business hours accumulate and all send later
func TestE2E_MultipleReReviews_OutsideBusinessHours_AllSendLater(t *testing.T) {
	originalURL := os.Getenv("SLACK_API_BASE_URL")
	os.Setenv("SLACK_API_BASE_URL", slackhogBaseURL+"/api")
	defer os.Setenv("SLACK_API_BASE_URL", originalURL)

	services.IsTestMode = true
	db, ts := setupE2EApp(t)
	defer ts.Close()
	clearSlackhogMessages(t)

	loc, _ := time.LoadLocation("Asia/Tokyo")
	now := time.Now().In(loc)
	if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
		t.Skip("Skipping: weekend")
	}
	if now.Hour() == 3 && now.Minute() == 0 {
		t.Skip("Skipping: inside the narrow test business hours window")
	}

	config := models.ChannelConfig{
		ID:                 "e2e-multi-config",
		SlackChannelID:     "C_E2E_BH",
		LabelName:          "needs-review",
		DefaultMentionID:   "U_E2E_USER",
		IsActive:           true,
		BusinessHoursStart: "03:00",
		BusinessHoursEnd:   "03:01",
		Timezone:           "Asia/Tokyo",
	}
	db.Create(&config)

	rootTS, parentID := postRootMessage(t, "C_E2E_BH", "PR notification for pull/30")

	task := models.ReviewTask{
		ID:           "e2e-multi-rereview",
		PRURL:        "https://github.com/e2e-owner/e2e-repo/pull/30",
		Repo:         "e2e-owner/e2e-repo",
		PRNumber:     30,
		Title:        "E2E Multi re-review PR",
		SlackTS:      rootTS,
		SlackChannel: "C_E2E_BH",
		Status:       "in_review",
		LabelName:    "needs-review",
		Reviewer:     "U_REVIEWER1",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// First re-review
	payload1 := `{
		"action": "review_requested",
		"pull_request": {"number": 30, "html_url": "https://github.com/e2e-owner/e2e-repo/pull/30"},
		"repository": {"full_name": "e2e-owner/e2e-repo", "owner": {"login": "e2e-owner"}, "name": "e2e-repo"},
		"sender": {"login": "author1"},
		"requested_reviewer": {"login": "reviewer-a"}
	}`
	resp1 := sendWebhook(t, ts.URL, payload1)
	resp1.Body.Close()
	time.Sleep(300 * time.Millisecond)

	// Second re-review
	payload2 := `{
		"action": "review_requested",
		"pull_request": {"number": 30, "html_url": "https://github.com/e2e-owner/e2e-repo/pull/30"},
		"repository": {"full_name": "e2e-owner/e2e-repo", "owner": {"login": "e2e-owner"}, "name": "e2e-repo"},
		"sender": {"login": "author2"},
		"requested_reviewer": {"login": "reviewer-b"}
	}`
	resp2 := sendWebhook(t, ts.URL, payload2)
	resp2.Body.Close()
	time.Sleep(300 * time.Millisecond)

	// Verify both accumulated
	var deferredTask models.ReviewTask
	db.Where("id = ?", "e2e-multi-rereview").First(&deferredTask)
	assert.True(t, deferredTask.PendingReReviewNotify)
	assert.Contains(t, deferredTask.PendingReReviewSender, "author1")
	assert.Contains(t, deferredTask.PendingReReviewSender, "author2")

	// Verify 2 deferred feedback messages were sent (one per re-review request)
	feedbackReplies := getSlackhogReplies(t, parentID)
	feedbackCount := 0
	for _, r := range feedbackReplies {
		if strings.Contains(r.Text, "営業時間外") {
			feedbackCount++
		}
	}
	assert.Equal(t, 2, feedbackCount, "Each re-review should produce a deferred feedback message")

	// Simulate business hours start
	db.Model(&models.ChannelConfig{}).Where("id = ?", "e2e-multi-config").Updates(map[string]interface{}{
		"business_hours_start": "00:00",
		"business_hours_end":   "23:59",
	})

	services.CheckPendingReReviewNotifications(db)
	time.Sleep(500 * time.Millisecond)

	// Verify both deferred re-review notifications also appear (total: 2 feedback + 2 re-review = 4)
	replies := getSlackhogReplies(t, parentID)
	reReviewCount := 0
	for _, r := range replies {
		if strings.Contains(r.Text, "再レビュー") || strings.Contains(r.Text, "re-review") {
			reReviewCount++
		}
	}
	// 2 feedback messages + 2 deferred notifications = 4 messages containing re-review text
	// But feedback messages use "再レビューをリクエストしました" and deferred notifications use "再レビューを依頼しました"
	// Both match "再レビュー", so count should be 4
	assert.Equal(t, 4, reReviewCount, "2 feedback + 2 deferred re-review notifications should appear as thread replies")

	// Pending flag cleared
	db.Where("id = ?", "e2e-multi-rereview").First(&deferredTask)
	assert.False(t, deferredTask.PendingReReviewNotify)
}

// Test: Immediate away user is excluded from reviewer assignment, but scheduled (future) away is still eligible
func TestE2E_ScheduledAway_NotExcludedFromReviewerAssignment(t *testing.T) {
	originalURL := os.Getenv("SLACK_API_BASE_URL")
	os.Setenv("SLACK_API_BASE_URL", slackhogBaseURL+"/api")
	defer os.Setenv("SLACK_API_BASE_URL", originalURL)

	services.IsTestMode = true
	db, ts := setupE2EApp(t)
	defer ts.Close()
	clearSlackhogMessages(t)

	// Setup channel config with 3 reviewers: U_R1, U_R2, U_R3
	config := models.ChannelConfig{
		ID:                 "e2e-away-config",
		SlackChannelID:     "C_E2E_BH",
		LabelName:          "needs-review",
		DefaultMentionID:   "U_E2E_DEFAULT",
		RepositoryList:    "e2e-owner/e2e-repo",
		ReviewerList:      "U_R1,U_R2,U_R3",
		RequiredApprovals: 2,
		IsActive:           true,
		BusinessHoursStart: "00:00",
		BusinessHoursEnd:   "23:59",
		Timezone:           "Asia/Tokyo",
	}
	db.Create(&config)

	now := time.Now()
	future := now.Add(7 * 24 * time.Hour) // 1 week from now

	// U_R1: immediately away (no AwayFrom, indefinite)
	db.Create(&models.ReviewerAvailability{
		ID:          "e2e-away-immediate",
		SlackUserID: "U_R1",
		AwayFrom:    nil,
		AwayUntil:   nil,
		Reason:      "育児休業",
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	// U_R2: scheduled away (future AwayFrom)
	db.Create(&models.ReviewerAvailability{
		ID:          "e2e-away-scheduled",
		SlackUserID: "U_R2",
		AwayFrom:    &future,
		AwayUntil:   nil,
		Reason:      "来週から休暇",
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	// Send labeled webhook
	payload := `{
		"action": "labeled",
		"label": {"name": "needs-review"},
		"pull_request": {
			"number": 100,
			"html_url": "https://github.com/e2e-owner/e2e-repo/pull/100",
			"title": "E2E Scheduled Away Test PR",
			"user": {"login": "e2e-author"},
			"labels": [{"name": "needs-review"}]
		},
		"repository": {"full_name": "e2e-owner/e2e-repo", "owner": {"login": "e2e-owner"}, "name": "e2e-repo"},
		"sender": {"login": "e2e-author"}
	}`
	resp := sendWebhook(t, ts.URL, payload)
	resp.Body.Close()
	time.Sleep(500 * time.Millisecond)

	// Verify: slackhog received a message in C_E2E_BH
	msgs := getSlackhogMessages(t, "C_E2E_BH")
	require.NotEmpty(t, msgs, "Should have at least one message in slackhog")

	// Find the PR notification message
	var prMsg *slackhogMessage
	for i, m := range msgs {
		if strings.Contains(m.Text, "pull/100") || strings.Contains(m.Text, "E2E Scheduled Away Test PR") {
			prMsg = &msgs[i]
			break
		}
	}

	// Get thread replies for reviewer assignment
	require.NotNil(t, prMsg, "PR notification message should be found in slackhog")

	replies := getSlackhogReplies(t, prMsg.ID)
	allText := prMsg.Text
	for _, r := range replies {
		allText += " " + r.Text
	}

	// U_R1 (immediate away) should NOT be assigned
	assert.NotContains(t, allText, "<@U_R1>", "Immediately away user should not be assigned as reviewer")

	// U_R2 (scheduled future away) should still be eligible
	// U_R3 is always eligible
	// With 2 reviewers needed and U_R1 excluded, U_R2 and U_R3 should both be assigned
	assert.Contains(t, allText, "<@U_R2>", "Scheduled (future) away user should still be eligible for review")
	assert.Contains(t, allText, "<@U_R3>", "Available user should be assigned as reviewer")

	// Verify the review task was created
	var task models.ReviewTask
	err := db.Where("pr_number = ? AND slack_channel = ?", 100, "C_E2E_BH").First(&task).Error
	assert.NoError(t, err, "Review task should be created")
}

// Test: Pool-member commented review on a waiting_business_hours task should be pre-assigned
// (PR #95). Verifies that:
//   1. task.Reviewers contains the pool member's Slack ID after the comment webhook fires
//   2. The thread receives the "review commented" notification
//   3. The task status is preserved as waiting_business_hours (PR #94 behavior)
func TestE2E_PoolMemberCommentOnWaitingBusinessHours_PreAssignsReviewer(t *testing.T) {
	originalURL := os.Getenv("SLACK_API_BASE_URL")
	os.Setenv("SLACK_API_BASE_URL", slackhogBaseURL+"/api")
	defer os.Setenv("SLACK_API_BASE_URL", originalURL)

	services.IsTestMode = true
	db, ts := setupE2EApp(t)
	defer ts.Close()
	clearSlackhogMessages(t)

	const (
		channelID    = "C_E2E_BH"
		poolMember   = "U_E2E_POOL"
		nonPool      = "U_E2E_NONPOOL"
		ghPoolUser   = "e2e-pool-reviewer"
		taskID       = "e2e-preassign-pool-task"
		prNumber     = 200
	)

	// Channel pool contains the pool member
	config := models.ChannelConfig{
		ID:                "e2e-preassign-config",
		SlackChannelID:    channelID,
		LabelName:         "needs-review",
		DefaultMentionID:  "U_E2E_DEFAULT",
		ReviewerList:      poolMember + "," + nonPool,
		IsActive:          true,
		RequiredApprovals: 2,
		Timezone:          "Asia/Tokyo",
	}
	require.NoError(t, db.Create(&config).Error)

	// GitHub username -> Slack ID mapping for the pool member
	mapping := models.UserMapping{
		ID:             "e2e-mapping-pool",
		GithubUsername: ghPoolUser,
		SlackUserID:    poolMember,
	}
	require.NoError(t, db.Create(&mapping).Error)

	// Root Slack message + waiting_business_hours review task awaiting next-morning mention
	rootTS, parentID := postRootMessage(t, channelID, "PR notification for pull/200")

	task := models.ReviewTask{
		ID:              taskID,
		PRURL:           "https://github.com/e2e-owner/e2e-repo/pull/200",
		Repo:            "e2e-owner/e2e-repo",
		PRNumber:        prNumber,
		Title:           "E2E Pre-assign PR",
		SlackTS:         rootTS,
		SlackChannel:    channelID,
		Status:          "waiting_business_hours",
		LabelName:       "needs-review",
		PRAuthorSlackID: "U_E2E_AUTHOR",
		CreatedAt:       time.Now().Add(-12 * time.Hour),
		UpdatedAt:       time.Now().Add(-12 * time.Hour),
	}
	require.NoError(t, db.Create(&task).Error)

	// Pool member leaves a comment review on GitHub before the next-morning mention
	payload := fmt.Sprintf(`{
		"action": "submitted",
		"pull_request": {"number": %d, "html_url": "https://github.com/e2e-owner/e2e-repo/pull/%d"},
		"repository": {"full_name": "e2e-owner/e2e-repo", "owner": {"login": "e2e-owner"}, "name": "e2e-repo"},
		"review": {"state": "commented", "user": {"login": "%s"}}
	}`, prNumber, prNumber, ghPoolUser)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	time.Sleep(500 * time.Millisecond)

	// (1) DB: task.Reviewers should now contain the pool member's Slack ID
	var updatedTask models.ReviewTask
	require.NoError(t, db.Where("id = ?", taskID).First(&updatedTask).Error)
	assert.Equal(t, "waiting_business_hours", updatedTask.Status,
		"status must be preserved (PR #94 behavior)")
	assert.Equal(t, poolMember, updatedTask.Reviewer,
		"single reviewer slot should be filled with the pool commenter")
	assert.Contains(t, updatedTask.Reviewers, poolMember,
		"reviewers list should include the pool commenter so the morning mention picks them")

	// (2) Slack thread: review-commented notification should be delivered via slackhog
	replies := getSlackhogReplies(t, parentID)
	require.NotEmpty(t, replies, "review-commented notification should appear in thread")
	commentNotificationFound := false
	for _, r := range replies {
		if strings.Contains(r.Text, "レビューコメントを残しました") || strings.Contains(r.Text, "left a review comment") {
			commentNotificationFound = true
			break
		}
	}
	assert.True(t, commentNotificationFound,
		"thread should contain the review-commented notification, got: %v", replies)
}
