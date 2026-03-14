package services

import (
	"slack-review-notify/models"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCheckBusinessHoursTasks(t *testing.T) {
	db := setupTestDB(t)

	// JST timezone setup
	jst, err := time.LoadLocation("Asia/Tokyo")
	assert.NoError(t, err)

	// Create channel configuration
	config := models.ChannelConfig{
		ID:               "test-config",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U12345",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&config)

	// Create a task in waiting-for-business-hours state
	task := models.ReviewTask{
		ID:           "waiting-task",
		PRURL:        "https://github.com/owner/repo/pull/1",
		Repo:         "owner/repo",
		PRNumber:     1,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Reviewer:     "", // Empty outside business hours
		Status:       "waiting_business_hours",
		LabelName:    "needs-review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// Test with a time outside business hours (nothing should happen)
	outsideHours := time.Date(2024, 8, 27, 20, 0, 0, 0, jst) // Tuesday 20:00 JST

	// Instead of mocking the actual time, test the logic directly
	// Test business hours check with default settings (10:00-19:00 JST)
	defaultConfig := &models.ChannelConfig{
		BusinessHoursStart: "10:00",
		BusinessHoursEnd:   "19:00",
		Timezone:           "Asia/Tokyo",
	}

	// Test outside business hours check (20:00 is outside business hours)
	assert.False(t, IsWithinBusinessHours(defaultConfig, outsideHours), "20:00 is outside business hours")

	// Test with a time within business hours
	businessHours := time.Date(2024, 8, 28, 10, 0, 0, 0, jst) // Wednesday 10:00 JST
	assert.True(t, IsWithinBusinessHours(defaultConfig, businessHours), "10:00 is within business hours")

	// At this point the task should still be in waiting state
	var beforeTask models.ReviewTask
	db.First(&beforeTask, "id = ?", "waiting-task")
	assert.Equal(t, "waiting_business_hours", beforeTask.Status)
	assert.Equal(t, "", beforeTask.Reviewer)
}

func TestBusinessHoursTaskFlow(t *testing.T) {
	db := setupTestDB(t)

	// Create channel configuration
	config := models.ChannelConfig{
		ID:                       "test-config",
		SlackChannelID:           "C12345",
		LabelName:                "needs-review",
		DefaultMentionID:         "U12345",
		ReviewerReminderInterval: 30,
		IsActive:                 true,
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
	}
	db.Create(&config)

	// Set the reviewer list
	config.ReviewerList = "U67890,U11111,U22222"
	db.Save(&config)

	// Create a task in waiting-for-business-hours state
	task := models.ReviewTask{
		ID:           "waiting-task",
		PRURL:        "https://github.com/owner/repo/pull/1",
		Repo:         "owner/repo",
		PRNumber:     1,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Reviewer:     "",
		Status:       "waiting_business_hours",
		LabelName:    "needs-review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// Test reviewer selection
	selectedReviewers := SelectRandomReviewers(db, "C12345", "needs-review", 1, nil)
	assert.Len(t, selectedReviewers, 1, "One reviewer should be selected")
	assert.Contains(t, []string{"U67890", "U11111", "U22222"}, selectedReviewers[0], "Reviewer should be correctly selected from the list")

	// Verify task status
	var retrievedTask models.ReviewTask
	db.First(&retrievedTask, "id = ?", "waiting-task")
	assert.Equal(t, "waiting_business_hours", retrievedTask.Status)
}

func TestIsWithinBusinessHoursEdgeCases(t *testing.T) {
	jst, err := time.LoadLocation("Asia/Tokyo")
	assert.NoError(t, err)

	// Default settings (10:00-19:00 JST)
	defaultConfig := &models.ChannelConfig{
		BusinessHoursStart: "10:00",
		BusinessHoursEnd:   "19:00",
		Timezone:           "Asia/Tokyo",
	}

	tests := []struct {
		name                   string
		testTime               time.Time
		expectedWithinBusiness bool // Whether it is within business hours
	}{
		{
			name:                   "金曜日18時59分59秒",
			testTime:               time.Date(2024, 8, 30, 18, 59, 59, 0, jst),
			expectedWithinBusiness: true, // Within business hours
		},
		{
			name:                   "金曜日19時00分00秒",
			testTime:               time.Date(2024, 8, 30, 19, 0, 0, 0, jst),
			expectedWithinBusiness: false, // Outside business hours
		},
		{
			name:                   "月曜日00時00分00秒",
			testTime:               time.Date(2024, 8, 26, 0, 0, 0, 0, jst),
			expectedWithinBusiness: false, // Weekday late night is outside business hours (before 10:00)
		},
		{
			name:                   "土曜日00時00分00秒",
			testTime:               time.Date(2024, 8, 24, 0, 0, 0, 0, jst),
			expectedWithinBusiness: false, // Saturday is outside business hours
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsWithinBusinessHours(defaultConfig, tt.testTime)
			assert.Equal(t, tt.expectedWithinBusiness, result, tt.name)
		})
	}
}
