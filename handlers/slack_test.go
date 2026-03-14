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

func TestHandleSlackAction_PauseReminder_Today(t *testing.T) {
	// Set up test DB and router
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/action", HandleSlackAction(db))

	// Create a test task
	task := models.ReviewTask{
		ID:           "test-task-id",
		PRURL:        "https://github.com/owner/repo/pull/1",
		Repo:         "owner/repo",
		PRNumber:     1,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Status:       "in_review",
		Reviewer:     "U12345",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// Record time before the update
	beforeUpdate := time.Now()

	// Create Slack action payload
	payload := SlackActionPayload{
		Type: "block_actions",
		User: struct {
			ID string `json:"id"`
		}{ID: "U12345"},
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
				ActionID: "pause_reminder",
				SelectedOption: struct {
					Value string `json:"value"`
					Text  struct {
						Text string `json:"text"`
					} `json:"text"`
				}{
					Value: "test-task-id:today",
					Text: struct {
						Text string `json:"text"`
					}{
						Text: "翌営業日の朝まで",
					},
				},
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

	// Mock signature verification (needs proper setup in actual tests)
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()

	// Execute request
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	// Get task from DB and verify
	var updatedTask models.ReviewTask
	db.Where("id = ?", "test-task-id").First(&updatedTask)

	// Verify that ReminderPausedUntil is set
	assert.NotNil(t, updatedTask.ReminderPausedUntil)

	// Verify it's set to next business day morning
	pausedUntil := *updatedTask.ReminderPausedUntil

	// Verify it's after the current time
	assert.True(t, pausedUntil.After(beforeUpdate))

	// Verify it's set to 10:00
	assert.Equal(t, 10, pausedUntil.Hour())
	assert.Equal(t, 0, pausedUntil.Minute())
	assert.Equal(t, 0, pausedUntil.Second())

	// Verify it's a business day (Monday to Friday)
	assert.NotEqual(t, time.Saturday, pausedUntil.Weekday())
	assert.NotEqual(t, time.Sunday, pausedUntil.Weekday())
}

func TestHandleSlackAction_PauseReminder_Hours(t *testing.T) {
	testCases := []struct {
		name     string
		value    string
		duration time.Duration
	}{
		{"1 hour", "test-task-id:1h", 1 * time.Hour},
		{"2 hours", "test-task-id:2h", 2 * time.Hour},
		{"4 hours", "test-task-id:4h", 4 * time.Hour},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set up test DB and router
			db := setupTestDB(t)
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.POST("/slack/action", HandleSlackAction(db))

			// Create a test task
			task := models.ReviewTask{
				ID:           "test-task-id",
				PRURL:        "https://github.com/owner/repo/pull/1",
				Repo:         "owner/repo",
				PRNumber:     1,
				Title:        "Test PR",
				SlackTS:      "1234.5678",
				SlackChannel: "C12345",
				Status:       "in_review",
				Reviewer:     "U12345",
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			}
			db.Create(&task)

			// Record time before the update
			beforeUpdate := time.Now()

			// Create Slack action payload
			payload := SlackActionPayload{
				Type: "block_actions",
				User: struct {
					ID string `json:"id"`
				}{ID: "U12345"},
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
						ActionID: "pause_reminder",
						SelectedOption: struct {
							Value string `json:"value"`
							Text  struct {
								Text string `json:"text"`
							} `json:"text"`
						}{
							Value: tc.value,
						},
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

			// Mock signature verification
			services.IsTestMode = true
			defer func() { services.IsTestMode = false }()

			// Execute request
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Verify response
			assert.Equal(t, http.StatusOK, w.Code)

			// Get task from DB and verify
			var updatedTask models.ReviewTask
			db.Where("id = ?", "test-task-id").First(&updatedTask)

			// Verify that ReminderPausedUntil is set
			assert.NotNil(t, updatedTask.ReminderPausedUntil)

			// Verify it's set to the specified duration in the future
			pausedUntil := *updatedTask.ReminderPausedUntil
			expectedTime := beforeUpdate.Add(tc.duration)

			// Verify with tolerance (allow up to 1 minute difference)
			diff := pausedUntil.Sub(expectedTime)
			assert.True(t, diff < time.Minute && diff > -time.Minute,
				"Expected pause until around %v, but got %v (diff: %v)",
				expectedTime, pausedUntil, diff)
		})
	}
}

func TestHandleSlackAction_PauseReminderInitial(t *testing.T) {
	// Enable test mode (skip signature verification)
	services.IsTestMode = true

	// Create test DB
	db := setupTestDB(t)

	// Create a test task
	task := models.ReviewTask{
		ID:           "test-task-initial",
		PRURL:        "https://github.com/test/test/pull/1",
		Repo:         "test/test",
		PRNumber:     1,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Status:       "in_review",
		Reviewer:     "U54321",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// Mock Slack payload
	payload := SlackActionPayload{
		Type: "block_actions",
		User: struct {
			ID string `json:"id"`
		}{ID: "U12345"},
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
				ActionID: "pause_reminder_initial",
				SelectedOption: struct {
					Value string `json:"value"`
					Text  struct {
						Text string `json:"text"`
					} `json:"text"`
				}{
					Value: "test-task-initial:4h",
					Text: struct {
						Text string `json:"text"`
					}{
						Text: "4時間停止",
					},
				},
			},
		},
		Container: struct {
			ChannelID string `json:"channel_id"`
		}{ChannelID: "C12345"},
	}

	// Convert to JSON
	payloadJSON, _ := json.Marshal(payload)

	// Create request
	form := url.Values{}
	form.Add("payload", string(payloadJSON))

	req, _ := http.NewRequest("POST", "/slack/actions", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Record response
	w := httptest.NewRecorder()

	// Execute handler
	handler := HandleSlackAction(db)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	handler(c)

	// Verify status code
	assert.Equal(t, http.StatusOK, w.Code)

	// Verify DB has been updated
	var updatedTask models.ReviewTask
	err := db.Where("id = ?", "test-task-initial").First(&updatedTask).Error
	assert.NoError(t, err)
	assert.NotNil(t, updatedTask.ReminderPausedUntil)

	// Verify it's set to 4 hours later
	expected := time.Now().Add(4 * time.Hour)
	actual := *updatedTask.ReminderPausedUntil
	assert.WithinDuration(t, expected, actual, 10*time.Second)
}
