package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slack-review-notify/models"
	"slack-review-notify/services"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestHandleSlackAction_ChangeReviewer_MultipleLabels(t *testing.T) {
	// Enable test mode
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()

	// Create test DB
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/action", HandleSlackAction(db))

	// Create configs for multiple labels
	// For label-1 (2 reviewers)
	config1 := models.ChannelConfig{
		ID:               "config-1",
		SlackChannelID:   "C12345",
		LabelName:        "label-1",
		DefaultMentionID: "U00000",
		ReviewerList:     "U11111,U22222",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&config1)

	// For label-2 (2 reviewers)
	config2 := models.ChannelConfig{
		ID:               "config-2",
		SlackChannelID:   "C12345",
		LabelName:        "label-2",
		DefaultMentionID: "U00000",
		ReviewerList:     "U33333,U44444",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&config2)

	// Create a test task (label-2)
	task := models.ReviewTask{
		ID:           "test-task-1",
		PRURL:        "https://github.com/test/repo/pull/1",
		Repo:         "test/repo",
		PRNumber:     1,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Status:       "in_review",
		Reviewer:     "U33333", // Reviewer from label-2
		LabelName:    "label-2",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// Create Slack action payload
	payload := SlackActionPayload{
		Type: "block_actions",
		User: struct {
			ID string `json:"id"`
		}{ID: "U99999"},
		Actions: []struct {
			ActionID       string `json:"action_id"`
			Value          string `json:"value,omitempty"`
			SelectedOption struct {
				Value string `json:"value"`
				Text  struct {
					Text string `json:"text"`
				} `json:"text"`
			} `json:"selected_option,omitempty"`
		}{
			{
				ActionID: "change_reviewer",
				Value:    "test-task-1",
			},
		},
		Container: struct {
			ChannelID string `json:"channel_id"`
		}{ChannelID: "C12345"},
		Message: struct {
			Ts string `json:"ts"`
		}{Ts: "1234.5678"},
	}

	payloadJSON, _ := json.Marshal(payload)
	form := url.Values{}
	form.Add("payload", string(payloadJSON))

	// Create request
	req := httptest.NewRequest("POST", "/slack/action", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Execute request
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	// Get task from DB and verify
	var updatedTask models.ReviewTask
	db.Where("id = ?", "test-task-1").First(&updatedTask)

	// Verify that the reviewer has been changed
	assert.NotEqual(t, "U33333", updatedTask.Reviewer)
	// Verify selected from label-2's reviewer list
	assert.Contains(t, []string{"U33333", "U44444"}, updatedTask.Reviewer)
	// Verify not selected from label-1's reviewer list
	assert.NotContains(t, []string{"U11111", "U22222"}, updatedTask.Reviewer)
}

func TestHandleSlackAction_ChangeReviewer_SingleReviewer(t *testing.T) {
	// Enable test mode
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()

	// Create test DB
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/action", HandleSlackAction(db))

	// Create config with only one reviewer
	config := models.ChannelConfig{
		ID:               "config-3",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U00000",
		ReviewerList:     "U11111", // Only one
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&config)

	// Create a test task
	task := models.ReviewTask{
		ID:           "test-task-2",
		PRURL:        "https://github.com/test/repo/pull/2",
		Repo:         "test/repo",
		PRNumber:     2,
		Title:        "Test PR",
		SlackTS:      "2345.6789",
		SlackChannel: "C12345",
		Status:       "in_review",
		Reviewer:     "U11111",
		LabelName:    "needs-review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// Create Slack action payload
	payload := SlackActionPayload{
		Type: "block_actions",
		User: struct {
			ID string `json:"id"`
		}{ID: "U99999"},
		Actions: []struct {
			ActionID       string `json:"action_id"`
			Value          string `json:"value,omitempty"`
			SelectedOption struct {
				Value string `json:"value"`
				Text  struct {
					Text string `json:"text"`
				} `json:"text"`
			} `json:"selected_option,omitempty"`
		}{
			{
				ActionID: "change_reviewer",
				Value:    "test-task-2",
			},
		},
		Container: struct {
			ChannelID string `json:"channel_id"`
		}{ChannelID: "C12345"},
		Message: struct {
			Ts string `json:"ts"`
		}{Ts: "2345.6789"},
	}

	payloadJSON, _ := json.Marshal(payload)
	form := url.Values{}
	form.Add("payload", string(payloadJSON))

	// Create request
	req := httptest.NewRequest("POST", "/slack/action", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Execute request
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	// Get task from DB and verify
	var updatedTask models.ReviewTask
	db.Where("id = ?", "test-task-2").First(&updatedTask)

	// Reviewer not changed (only one registered)
	assert.Equal(t, "U11111", updatedTask.Reviewer)
}

func TestHandleSlackAction_ChangeReviewer_NoLabelName(t *testing.T) {
	// Enable test mode
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()

	// Create test DB
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/action", HandleSlackAction(db))

	// Create config for default label
	config := models.ChannelConfig{
		ID:               "config-4",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U00000",
		ReviewerList:     "U11111,U22222",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&config)

	// Create a task with empty LabelName (old task)
	task := models.ReviewTask{
		ID:           "test-task-3",
		PRURL:        "https://github.com/test/repo/pull/3",
		Repo:         "test/repo",
		PRNumber:     3,
		Title:        "Test PR",
		SlackTS:      "3456.7890",
		SlackChannel: "C12345",
		Status:       "in_review",
		Reviewer:     "U11111",
		LabelName:    "", // Empty label name
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// Create Slack action payload
	payload := SlackActionPayload{
		Type: "block_actions",
		User: struct {
			ID string `json:"id"`
		}{ID: "U99999"},
		Actions: []struct {
			ActionID       string `json:"action_id"`
			Value          string `json:"value,omitempty"`
			SelectedOption struct {
				Value string `json:"value"`
				Text  struct {
					Text string `json:"text"`
				} `json:"text"`
			} `json:"selected_option,omitempty"`
		}{
			{
				ActionID: "change_reviewer",
				Value:    "test-task-3",
			},
		},
		Container: struct {
			ChannelID string `json:"channel_id"`
		}{ChannelID: "C12345"},
		Message: struct {
			Ts string `json:"ts"`
		}{Ts: "3456.7890"},
	}

	payloadJSON, _ := json.Marshal(payload)
	form := url.Values{}
	form.Add("payload", string(payloadJSON))

	// Create request
	req := httptest.NewRequest("POST", "/slack/action", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Execute request
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	// Get task from DB and verify
	var updatedTask models.ReviewTask
	db.Where("id = ?", "test-task-3").First(&updatedTask)

	// Verify that the reviewer has been changed
	assert.NotEqual(t, "U11111", updatedTask.Reviewer)
	assert.Contains(t, []string{"U11111", "U22222"}, updatedTask.Reviewer)
	// Verify that LabelName has been updated to default value
	assert.Equal(t, "needs-review", updatedTask.LabelName)
}

// Test that change_reviewer button works correctly with English language setting
func TestHandleSlackAction_ChangeReviewer_EnglishLanguage(t *testing.T) {
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()

	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/action", HandleSlackAction(db))

	// Create config with English language and 2 reviewers
	config := models.ChannelConfig{
		ID:               "config-en-1",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U00000",
		ReviewerList:     "U11111,U22222",
		Language:          "en",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&config)

	// Create task with English language
	task := models.ReviewTask{
		ID:           "test-task-en-1",
		PRURL:        "https://github.com/test/repo/pull/10",
		Repo:         "test/repo",
		PRNumber:     10,
		Title:        "Test PR English",
		SlackTS:      "9999.1111",
		SlackChannel: "C12345",
		Status:       "in_review",
		Reviewer:     "U11111",
		Reviewers:    "U11111",
		LabelName:    "needs-review",
		Language:     "en",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	payload := SlackActionPayload{
		Type: "block_actions",
		User: struct {
			ID string `json:"id"`
		}{ID: "U99999"},
		Actions: []struct {
			ActionID       string `json:"action_id"`
			Value          string `json:"value,omitempty"`
			SelectedOption struct {
				Value string `json:"value"`
				Text  struct {
					Text string `json:"text"`
				} `json:"text"`
			} `json:"selected_option,omitempty"`
		}{
			{
				ActionID: "change_reviewer",
				Value:    "test-task-en-1",
			},
		},
		Container: struct {
			ChannelID string `json:"channel_id"`
		}{ChannelID: "C12345"},
		Message: struct {
			Ts string `json:"ts"`
		}{Ts: "9999.1111"},
	}

	payloadJSON, _ := json.Marshal(payload)
	form := url.Values{}
	form.Add("payload", string(payloadJSON))

	req := httptest.NewRequest("POST", "/slack/action", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Verify reviewer was changed
	var updatedTask models.ReviewTask
	db.Where("id = ?", "test-task-en-1").First(&updatedTask)
	assert.NotEqual(t, "U11111", updatedTask.Reviewer)
	assert.Contains(t, []string{"U11111", "U22222"}, updatedTask.Reviewer)
	// Language should still be English
	assert.Equal(t, "en", updatedTask.Language)
}

// Test that single-reviewer "cannot change" message uses correct language
func TestHandleSlackAction_ChangeReviewer_SingleReviewer_English(t *testing.T) {
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()

	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/action", HandleSlackAction(db))

	// Create config with only one reviewer, English language
	config := models.ChannelConfig{
		ID:               "config-en-2",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U00000",
		ReviewerList:     "U11111",
		Language:          "en",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&config)

	// Create task with English language and single reviewer
	task := models.ReviewTask{
		ID:           "test-task-en-2",
		PRURL:        "https://github.com/test/repo/pull/11",
		Repo:         "test/repo",
		PRNumber:     11,
		Title:        "Test PR English Single",
		SlackTS:      "9999.2222",
		SlackChannel: "C12345",
		Status:       "in_review",
		Reviewer:     "U11111",
		LabelName:    "needs-review",
		Language:     "en",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	payload := SlackActionPayload{
		Type: "block_actions",
		User: struct {
			ID string `json:"id"`
		}{ID: "U99999"},
		Actions: []struct {
			ActionID       string `json:"action_id"`
			Value          string `json:"value,omitempty"`
			SelectedOption struct {
				Value string `json:"value"`
				Text  struct {
					Text string `json:"text"`
				} `json:"text"`
			} `json:"selected_option,omitempty"`
		}{
			{
				ActionID: "change_reviewer",
				Value:    "test-task-en-2",
			},
		},
		Container: struct {
			ChannelID string `json:"channel_id"`
		}{ChannelID: "C12345"},
		Message: struct {
			Ts string `json:"ts"`
		}{Ts: "9999.2222"},
	}

	payloadJSON, _ := json.Marshal(payload)
	form := url.Values{}
	form.Add("payload", string(payloadJSON))

	req := httptest.NewRequest("POST", "/slack/action", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should succeed (message posted to thread in test mode)
	assert.Equal(t, http.StatusOK, w.Code)

	// Reviewer should NOT have changed (only one registered)
	var updatedTask models.ReviewTask
	db.Where("id = ?", "test-task-en-2").First(&updatedTask)
	assert.Equal(t, "U11111", updatedTask.Reviewer)
}

// Test that away reviewers are also replaced when "change reviewer" button is clicked
func TestHandleSlackAction_ChangeReviewer_AlsoReplacesAwayReviewers(t *testing.T) {
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()

	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/action", HandleSlackAction(db))

	// Create config with 5 reviewers
	config := models.ChannelConfig{
		ID:               "config-away-1",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U00000",
		ReviewerList:     "U11111,U22222,U33333,U44444,U55555",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&config)

	// Create task with 2 reviewers assigned: U11111 and U22222
	task := models.ReviewTask{
		ID:           "test-task-away-1",
		PRURL:        "https://github.com/test/repo/pull/20",
		Repo:         "test/repo",
		PRNumber:     20,
		Title:        "Test PR Away",
		SlackTS:      "5555.1111",
		SlackChannel: "C12345",
		Status:       "in_review",
		Reviewer:     "U11111",
		Reviewers:    "U11111,U22222",
		LabelName:    "needs-review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// Set U22222 as away (on vacation)
	away := models.ReviewerAvailability{
		ID:          "away-1",
		SlackUserID: "U22222",
		AwayUntil:   nil, // indefinitely away
		Reason:      "vacation",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	db.Create(&away)

	// Click "change reviewer" for U11111 (replacingReviewerID = U11111)
	payload := SlackActionPayload{
		Type: "block_actions",
		User: struct {
			ID string `json:"id"`
		}{ID: "U99999"},
		Actions: []struct {
			ActionID       string `json:"action_id"`
			Value          string `json:"value,omitempty"`
			SelectedOption struct {
				Value string `json:"value"`
				Text  struct {
					Text string `json:"text"`
				} `json:"text"`
			} `json:"selected_option,omitempty"`
		}{
			{
				ActionID: "change_reviewer",
				Value:    "test-task-away-1:U11111", // replacing U11111
			},
		},
		Container: struct {
			ChannelID string `json:"channel_id"`
		}{ChannelID: "C12345"},
		Message: struct {
			Ts string `json:"ts"`
		}{Ts: "5555.1111"},
	}

	payloadJSON, _ := json.Marshal(payload)
	form := url.Values{}
	form.Add("payload", string(payloadJSON))

	req := httptest.NewRequest("POST", "/slack/action", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "test-task-away-1").First(&updatedTask)

	// Both U11111 (clicked) and U22222 (away) should be replaced
	reviewerIDs := strings.Split(updatedTask.Reviewers, ",")
	assert.Equal(t, 2, len(reviewerIDs), "should still have 2 reviewers")

	for _, id := range reviewerIDs {
		trimmed := strings.TrimSpace(id)
		assert.NotEqual(t, "U11111", trimmed, "U11111 (clicked) should be replaced")
		assert.NotEqual(t, "U22222", trimmed, "U22222 (away) should also be replaced")
		// Should be from the remaining candidates: U33333, U44444, U55555
		assert.Contains(t, []string{"U33333", "U44444", "U55555"}, trimmed,
			"new reviewers should be from available candidates")
	}
}

// Test that non-away reviewers are NOT replaced (only the clicked one is)
func TestHandleSlackAction_ChangeReviewer_NoAwayReviewers(t *testing.T) {
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()

	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/action", HandleSlackAction(db))

	config := models.ChannelConfig{
		ID:               "config-noaway-1",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U00000",
		ReviewerList:     "U11111,U22222,U33333,U44444",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&config)

	// Task with 2 reviewers, neither is away
	task := models.ReviewTask{
		ID:           "test-task-noaway-1",
		PRURL:        "https://github.com/test/repo/pull/30",
		Repo:         "test/repo",
		PRNumber:     30,
		Title:        "Test PR No Away",
		SlackTS:      "6666.1111",
		SlackChannel: "C12345",
		Status:       "in_review",
		Reviewer:     "U11111",
		Reviewers:    "U11111,U22222",
		LabelName:    "needs-review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	payload := SlackActionPayload{
		Type: "block_actions",
		User: struct {
			ID string `json:"id"`
		}{ID: "U99999"},
		Actions: []struct {
			ActionID       string `json:"action_id"`
			Value          string `json:"value,omitempty"`
			SelectedOption struct {
				Value string `json:"value"`
				Text  struct {
					Text string `json:"text"`
				} `json:"text"`
			} `json:"selected_option,omitempty"`
		}{
			{
				ActionID: "change_reviewer",
				Value:    "test-task-noaway-1:U11111",
			},
		},
		Container: struct {
			ChannelID string `json:"channel_id"`
		}{ChannelID: "C12345"},
		Message: struct {
			Ts string `json:"ts"`
		}{Ts: "6666.1111"},
	}

	payloadJSON, _ := json.Marshal(payload)
	form := url.Values{}
	form.Add("payload", string(payloadJSON))

	req := httptest.NewRequest("POST", "/slack/action", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "test-task-noaway-1").First(&updatedTask)

	reviewerIDs := strings.Split(updatedTask.Reviewers, ",")
	assert.Equal(t, 2, len(reviewerIDs), "should still have 2 reviewers")

	// U11111 (clicked) should be replaced, U22222 (not away) should remain
	assert.NotEqual(t, "U11111", strings.TrimSpace(reviewerIDs[0]),
		"U11111 should be replaced")
	assert.Equal(t, "U22222", strings.TrimSpace(reviewerIDs[1]),
		"U22222 should remain (not away)")
}

// Test partial replacement when not enough candidates for all away reviewers
func TestHandleSlackAction_ChangeReviewer_PartialReplacementWhenCandidatesInsufficient(t *testing.T) {
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()

	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/action", HandleSlackAction(db))

	// Only 3 reviewers total, 2 assigned, need to replace 2 but only 1 candidate left
	config := models.ChannelConfig{
		ID:               "config-partial-1",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U00000",
		ReviewerList:     "U11111,U22222,U33333",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&config)

	task := models.ReviewTask{
		ID:           "test-task-partial-1",
		PRURL:        "https://github.com/test/repo/pull/40",
		Repo:         "test/repo",
		PRNumber:     40,
		Title:        "Test PR Partial",
		SlackTS:      "7777.1111",
		SlackChannel: "C12345",
		Status:       "in_review",
		Reviewer:     "U11111",
		Reviewers:    "U11111,U22222",
		LabelName:    "needs-review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// U22222 is away
	away := models.ReviewerAvailability{
		ID:          "away-partial-1",
		SlackUserID: "U22222",
		AwayUntil:   nil,
		Reason:      "vacation",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	db.Create(&away)

	payload := SlackActionPayload{
		Type: "block_actions",
		User: struct {
			ID string `json:"id"`
		}{ID: "U99999"},
		Actions: []struct {
			ActionID       string `json:"action_id"`
			Value          string `json:"value,omitempty"`
			SelectedOption struct {
				Value string `json:"value"`
				Text  struct {
					Text string `json:"text"`
				} `json:"text"`
			} `json:"selected_option,omitempty"`
		}{
			{
				ActionID: "change_reviewer",
				Value:    "test-task-partial-1:U11111",
			},
		},
		Container: struct {
			ChannelID string `json:"channel_id"`
		}{ChannelID: "C12345"},
		Message: struct {
			Ts string `json:"ts"`
		}{Ts: "7777.1111"},
	}

	payloadJSON, _ := json.Marshal(payload)
	form := url.Values{}
	form.Add("payload", string(payloadJSON))

	req := httptest.NewRequest("POST", "/slack/action", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "test-task-partial-1").First(&updatedTask)

	// U11111 (clicked) should be replaced with U33333 (only candidate)
	// U22222 (away) cannot be replaced (no more candidates), stays as is
	reviewerIDs := strings.Split(updatedTask.Reviewers, ",")
	assert.Equal(t, 2, len(reviewerIDs), "should still have 2 reviewers")
	assert.Equal(t, "U33333", strings.TrimSpace(reviewerIDs[0]),
		"U11111 should be replaced with U33333")
	assert.Equal(t, "U22222", strings.TrimSpace(reviewerIDs[1]),
		"U22222 stays because no more candidates available")
}
