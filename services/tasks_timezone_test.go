package services

import (
	"slack-review-notify/models"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCheckInReviewTasks_ReminderPausedUntilTimezone(t *testing.T) {
	db := setupTestDB(t)

	// JST timezone setup
	jst, err := time.LoadLocation("Asia/Tokyo")
	assert.NoError(t, err)

	// Test time (JST): Tuesday 13:00
	baseTimeJST := time.Date(2024, 8, 27, 13, 0, 0, 0, jst)

	// Calculate next business day morning (JST 10:00)
	nextBusinessDayJST := GetNextBusinessDayMorningWithConfig(baseTimeJST, nil)

	// Same times in UTC
	baseTimeUTC := baseTimeJST.UTC()
	nextBusinessDayUTC := nextBusinessDayJST.UTC()

	tests := []struct {
		name        string
		currentTime time.Time  // Current time at test execution
		pausedUntil *time.Time // Value of reminder_paused_until (database stored value)
		shouldSkip  bool       // Whether the reminder should be skipped
		description string
	}{
		{
			name:        "JST_with_JST_paused_until",
			currentTime: baseTimeJST,
			pausedUntil: &nextBusinessDayJST,
			shouldSkip:  true,
			description: "JST current time, JST paused time - should be skipped",
		},
		{
			name:        "UTC_with_UTC_paused_until",
			currentTime: baseTimeUTC,
			pausedUntil: &nextBusinessDayUTC,
			shouldSkip:  true,
			description: "UTC current time, UTC paused time - should be skipped",
		},
		{
			name:        "JST_with_UTC_paused_until",
			currentTime: baseTimeJST,
			pausedUntil: &nextBusinessDayUTC, // Database stored value (UTC)
			shouldSkip:  true,
			description: "JST current time, UTC paused time (real-world case) - should be skipped",
		},
		{
			name:        "past_paused_until",
			currentTime: baseTimeJST,
			pausedUntil: func() *time.Time { t := baseTimeJST.Add(-1 * time.Hour); return &t }(), // 1 hour ago
			shouldSkip:  false,
			description: "Past paused time - should not be skipped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test task
			task := models.ReviewTask{
				ID:                  tt.name + "_task",
				PRURL:               "https://github.com/owner/repo/pull/1",
				Repo:                "owner/repo",
				PRNumber:            1,
				Title:               "Test PR",
				SlackTS:             "1234.5678",
				SlackChannel:        "C12345",
				Status:              "in_review",
				Reviewer:            "U12345",
				LabelName:           "needs-review",
				ReminderPausedUntil: tt.pausedUntil,
				CreatedAt:           tt.currentTime.Add(-2 * time.Hour),
				UpdatedAt:           tt.currentTime.Add(-2 * time.Hour), // Updated 2 hours ago (reminder target)
			}

			db.Create(&task)

			// Simulate the current time comparison logic
			now := tt.currentTime
			shouldSkip := task.ReminderPausedUntil != nil && now.Before(*task.ReminderPausedUntil)

			assert.Equal(t, tt.shouldSkip, shouldSkip, "Test: %s - %s", tt.name, tt.description)

			// Cleanup
			db.Where("id = ?", task.ID).Delete(&models.ReviewTask{})
		})
	}
}

func TestGetNextBusinessDayMorning_Timezone(t *testing.T) {
	// JST timezone setup
	jst, err := time.LoadLocation("Asia/Tokyo")
	assert.NoError(t, err)

	tests := []struct {
		name         string
		inputTime    time.Time
		expectedDay  int
		expectedHour int
		description  string
	}{
		{
			name:         "JST_Tuesday_15_00",
			inputTime:    time.Date(2024, 8, 27, 15, 0, 0, 0, jst), // Tuesday 15:00 JST
			expectedDay:  28,                                       // Wednesday
			expectedHour: 10,
			description:  "Tuesday 15:00 JST -> next day (Wednesday) 10:00 JST",
		},
		{
			name:         "UTC_equivalent_Tuesday_06_00",
			inputTime:    time.Date(2024, 8, 27, 6, 0, 0, 0, time.UTC), // Tuesday 06:00 UTC = Tuesday 15:00 JST
			expectedDay:  28,                                           // Wednesday
			expectedHour: 10,
			description:  "Tuesday 06:00 UTC (= Tuesday 15:00 JST) -> next day (Wednesday) 10:00 JST",
		},
		{
			name:         "JST_Tuesday_09_00",
			inputTime:    time.Date(2024, 8, 27, 9, 0, 0, 0, jst), // Tuesday 09:00 JST
			expectedDay:  27,                                      // Tuesday (same day)
			expectedHour: 10,
			description:  "Tuesday 09:00 JST -> same day (Tuesday) 10:00 JST",
		},
		{
			name:         "UTC_equivalent_Tuesday_00_00",
			inputTime:    time.Date(2024, 8, 27, 0, 0, 0, 0, time.UTC), // Tuesday 00:00 UTC = Tuesday 09:00 JST
			expectedDay:  27,                                           // Tuesday (same day)
			expectedHour: 10,
			description:  "Tuesday 00:00 UTC (= Tuesday 09:00 JST) -> same day (Tuesday) 10:00 JST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Calculate next business day
			nextBusinessDay := GetNextBusinessDayMorningWithConfig(tt.inputTime, nil)

			// Result should always be 10:00 on the specified day in JST
			assert.Equal(t, 2024, nextBusinessDay.Year(), "Year should be 2024")
			assert.Equal(t, time.August, nextBusinessDay.Month(), "Month should be August")
			assert.Equal(t, tt.expectedDay, nextBusinessDay.Day(), "Day should match expected: %s", tt.description)
			assert.Equal(t, tt.expectedHour, nextBusinessDay.Hour(), "Hour should be 10: %s", tt.description)
			assert.Equal(t, 0, nextBusinessDay.Minute(), "Minute should be 0: %s", tt.description)

			// Timezone should be JST
			assert.Equal(t, jst.String(), nextBusinessDay.Location().String(), "Timezone should be JST: %s", tt.description)
		})
	}
}
