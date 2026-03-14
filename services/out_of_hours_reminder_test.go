package services

import (
	"slack-review-notify/models"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Create a database for testing
func setupOutOfHoursTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	// Run migrations
	err = db.AutoMigrate(&models.ReviewTask{}, &models.ChannelConfig{})
	assert.NoError(t, err)

	return db
}

// Test for out-of-hours reminders (simple version)
func TestCheckInReviewTasks_OutOfHoursReminder(t *testing.T) {
	db := setupOutOfHoursTestDB(t)

	// Create channel configuration
	config := models.ChannelConfig{
		SlackChannelID:           "C123456",
		LabelName:                "needs-review",
		DefaultMentionID:         "U123456",
		ReviewerReminderInterval: 30,
		IsActive:                 true,
	}
	db.Create(&config)

	// Create a test task with an old UpdatedAt (to ensure a reminder is sent)
	oldTime := time.Now().Add(-2 * time.Hour)
	task := models.ReviewTask{
		ID:           "task-1",
		PRURL:        "https://github.com/test/repo/pull/1",
		SlackTS:      "1234567890.123456",
		SlackChannel: "C123456",
		Reviewer:     "U123456",
		Status:       "in_review",
		LabelName:    "needs-review",
		UpdatedAt:    oldTime,
	}
	db.Create(&task)

	// Execute CheckInReviewTasks
	CheckInReviewTasks(db)

	// Verify task state
	var updatedTask models.ReviewTask
	db.First(&updatedTask, "id = ?", "task-1")

	// Verify that UpdatedAt has been updated (some kind of reminder was sent)
	assert.True(t, updatedTask.UpdatedAt.After(oldTime),
		"UpdatedAt was not updated (no reminder was sent)")

	// Check out-of-hours determination at the current time and log the output
	now := time.Now()
	// Determination using default settings (10:00-19:00 JST)
	defaultConfig := &models.ChannelConfig{
		BusinessHoursStart: "10:00",
		BusinessHoursEnd:   "19:00",
		Timezone:           "Asia/Tokyo",
	}
	isWithinBusiness := IsWithinBusinessHours(defaultConfig, now)
	t.Logf("Current time: %v", now)
	t.Logf("Within business hours: %v", isWithinBusiness)
	t.Logf("OutOfHoursReminded: %v", updatedTask.OutOfHoursReminded)

	if updatedTask.ReminderPausedUntil != nil {
		t.Logf("ReminderPausedUntil: %v", *updatedTask.ReminderPausedUntil)
	}

	// If outside business hours, verify that OutOfHoursReminded is true and ReminderPausedUntil is set
	if !isWithinBusiness {
		assert.True(t, updatedTask.OutOfHoursReminded,
			"OutOfHoursReminded flag should be true outside business hours")
		assert.NotNil(t, updatedTask.ReminderPausedUntil,
			"ReminderPausedUntil should be set outside business hours")
	}
}

// Test calculation of next business day at 10:00
func TestGetNextBusinessDayMorningWithTime_OutOfHours(t *testing.T) {
	// Get JST timezone
	jst, _ := time.LoadLocation("Asia/Tokyo")

	tests := []struct {
		name     string
		input    time.Time
		expected time.Time
	}{
		{
			name:     "月曜9時_同日10時",
			input:    time.Date(2024, 1, 15, 0, 0, 0, 0, jst), // Monday 9:00 (JST)
			expected: time.Date(2024, 1, 15, 10, 0, 0, 0, jst),
		},
		{
			name:     "月曜20時_翌日10時",
			input:    time.Date(2024, 1, 15, 11, 0, 0, 0, jst), // Monday 20:00 (JST)
			expected: time.Date(2024, 1, 16, 10, 0, 0, 0, jst),
		},
		{
			name:     "金曜20時_月曜10時",
			input:    time.Date(2024, 1, 19, 11, 0, 0, 0, jst), // Friday 20:00 (JST)
			expected: time.Date(2024, 1, 22, 10, 0, 0, 0, jst),
		},
		{
			name:     "土曜14時_月曜10時",
			input:    time.Date(2024, 1, 20, 14, 0, 0, 0, jst), // Saturday 14:00 (JST)
			expected: time.Date(2024, 1, 22, 10, 0, 0, 0, jst),
		},
		{
			name:     "日曜14時_月曜10時",
			input:    time.Date(2024, 1, 21, 14, 0, 0, 0, jst), // Sunday 14:00 (JST)
			expected: time.Date(2024, 1, 22, 10, 0, 0, 0, jst),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use default settings (starts at 10:00)
			result := GetNextBusinessDayMorningWithConfig(tt.input, nil)
			assert.Equal(t, tt.expected, result,
				"Next business day 10:00 calculation is incorrect: got %v, want %v", result, tt.expected)
		})
	}
}

// Test for business hours determination
func TestIsWithinBusinessHours_DefaultConfig(t *testing.T) {
	// Get JST timezone
	jst, _ := time.LoadLocation("Asia/Tokyo")

	// Default settings (10:00-19:00 JST)
	defaultConfig := &models.ChannelConfig{
		BusinessHoursStart: "10:00",
		BusinessHoursEnd:   "19:00",
		Timezone:           "Asia/Tokyo",
	}

	tests := []struct {
		name                   string
		input                  time.Time
		expectedWithinBusiness bool
	}{
		{
			name:                   "月曜10時_営業時間内",
			input:                  time.Date(2024, 1, 15, 10, 0, 0, 0, jst),
			expectedWithinBusiness: true,
		},
		{
			name:                   "月曜18時_営業時間内",
			input:                  time.Date(2024, 1, 15, 18, 59, 0, 0, jst),
			expectedWithinBusiness: true,
		},
		{
			name:                   "月曜19時_営業時間外",
			input:                  time.Date(2024, 1, 15, 19, 0, 0, 0, jst),
			expectedWithinBusiness: false,
		},
		{
			name:                   "月曜9時_営業時間外",
			input:                  time.Date(2024, 1, 15, 9, 59, 0, 0, jst),
			expectedWithinBusiness: false,
		},
		{
			name:                   "土曜14時_営業時間外",
			input:                  time.Date(2024, 1, 20, 14, 0, 0, 0, jst),
			expectedWithinBusiness: false,
		},
		{
			name:                   "日曜10時_営業時間外",
			input:                  time.Date(2024, 1, 21, 10, 0, 0, 0, jst),
			expectedWithinBusiness: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsWithinBusinessHours(defaultConfig, tt.input)
			assert.Equal(t, tt.expectedWithinBusiness, result,
				"Business hours determination is incorrect: got %v, want %v", result, tt.expectedWithinBusiness)
		})
	}
}
