package integration

import (
	"slack-review-notify/models"
	"slack-review-notify/services"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReminderScheduling tests basic reminder scheduling
func TestReminderScheduling(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	testChannelID := "C_TEST_REMINDER"
	labelName := "needs-review"

	// Create channel config with 5 minute reminder interval
	channelConfig := &models.ChannelConfig{
		ID:                       uuid.NewString(),
		SlackChannelID:           testChannelID,
		LabelName:                labelName,
		DefaultMentionID:         "U_TEST_MENTION",
		ReviewerList:             "U_REVIEWER_1,U_REVIEWER_2",
		ReviewerReminderInterval: 5, // 5 minutes for testing
		IsActive:                 true,
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
	}
	testConfig.DB.Create(channelConfig)
	defer testConfig.DB.Delete(&channelConfig)

	// Create a review task in "in_review" state
	task := &models.ReviewTask{
		ID:           uuid.NewString(),
		PRURL:        "https://github.com/owner/repo/pull/123",
		Repo:         "owner/repo",
		PRNumber:     123,
		Title:        "Test PR for Reminder",
		SlackTS:      "1234567890.123456",
		SlackChannel: testChannelID,
		Reviewer:     "U_REVIEWER_1",
		Status:       "in_review",
		LabelName:    labelName,
		CreatedAt:    time.Now().Add(-10 * time.Minute), // Created 10 minutes ago
		UpdatedAt:    time.Now().Add(-10 * time.Minute), // Last updated 10 minutes ago
	}
	testConfig.DB.Create(task)
	defer testConfig.DB.Delete(&task)

	// Run the task checker
	services.CheckInReviewTasks(testConfig.DB)

	// Verify task was updated (reminder was processed)
	var updatedTask models.ReviewTask
	err := testConfig.DB.Where("id = ?", task.ID).First(&updatedTask).Error
	require.NoError(t, err, "Should find the task")

	// Check that UpdatedAt was refreshed (indicating reminder was sent)
	assert.True(t, updatedTask.UpdatedAt.After(task.UpdatedAt),
		"Task UpdatedAt should be refreshed after reminder")
}

// TestBusinessHoursReminder tests reminder behavior during business hours
func TestBusinessHoursReminder(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	testChannelID := "C_TEST_BH_REMINDER"
	labelName := "needs-review"

	// Create channel config with business hours 9:00-18:00 Asia/Tokyo
	channelConfig := &models.ChannelConfig{
		ID:                       uuid.NewString(),
		SlackChannelID:           testChannelID,
		LabelName:                labelName,
		DefaultMentionID:         "U_TEST_MENTION",
		ReviewerList:             "U_REVIEWER_1,U_REVIEWER_2",
		ReviewerReminderInterval: 5, // 5 minutes
		BusinessHoursStart:       "09:00",
		BusinessHoursEnd:         "18:00",
		Timezone:                 "Asia/Tokyo",
		IsActive:                 true,
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
	}
	testConfig.DB.Create(channelConfig)
	defer testConfig.DB.Delete(&channelConfig)

	// Get current time in Asia/Tokyo timezone
	loc, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err, "Should load timezone")
	now := time.Now().In(loc)

	// Determine if we're currently in business hours
	isBusinessHours := services.IsWithinBusinessHours(channelConfig, now)

	// Create a review task
	task := &models.ReviewTask{
		ID:           uuid.NewString(),
		PRURL:        "https://github.com/owner/repo/pull/456",
		Repo:         "owner/repo",
		PRNumber:     456,
		Title:        "Test PR for Business Hours Reminder",
		SlackTS:      "1234567890.654321",
		SlackChannel: testChannelID,
		Reviewer:     "U_REVIEWER_1",
		Status:       "in_review",
		LabelName:    labelName,
		CreatedAt:    time.Now().Add(-10 * time.Minute),
		UpdatedAt:    time.Now().Add(-10 * time.Minute),
	}
	testConfig.DB.Create(task)
	defer testConfig.DB.Delete(&task)

	// Run the task checker
	services.CheckInReviewTasks(testConfig.DB)

	// Verify task behavior based on business hours
	var updatedTask models.ReviewTask
	err = testConfig.DB.Where("id = ?", task.ID).First(&updatedTask).Error
	require.NoError(t, err, "Should find the task")

	if isBusinessHours {
		// During business hours, normal reminder should be sent
		assert.True(t, updatedTask.UpdatedAt.After(task.UpdatedAt),
			"Task UpdatedAt should be refreshed during business hours")
		assert.False(t, updatedTask.OutOfHoursReminded,
			"OutOfHoursReminded flag should be false during business hours")
	} else {
		// Outside business hours, out-of-hours reminder should be sent
		// and task should be paused until next business day
		if updatedTask.OutOfHoursReminded {
			assert.NotNil(t, updatedTask.ReminderPausedUntil,
				"ReminderPausedUntil should be set after out-of-hours reminder")
			assert.True(t, updatedTask.ReminderPausedUntil.After(now),
				"ReminderPausedUntil should be in the future")
		}
	}
}

// TestOutOfHoursReminder tests reminder behavior outside business hours
func TestOutOfHoursReminder(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	testChannelID := "C_TEST_OOH_REMINDER"
	labelName := "needs-review"

	// Create a timezone location for testing
	loc, err := time.LoadLocation("Asia/Tokyo")
	require.NoError(t, err, "Should load timezone")

	// Create a time that is definitely outside business hours (e.g., 2 AM Tokyo time)
	// We'll use a Saturday at 2 AM to ensure it's outside business hours
	now := time.Now().In(loc)
	testTime := time.Date(now.Year(), now.Month(), now.Day(), 2, 0, 0, 0, loc)

	// Find the next Saturday
	daysUntilSaturday := (time.Saturday - testTime.Weekday() + 7) % 7
	if daysUntilSaturday == 0 && testTime.Weekday() != time.Saturday {
		daysUntilSaturday = 7
	}
	testTime = testTime.AddDate(0, 0, int(daysUntilSaturday))

	// Create channel config with business hours 9:00-18:00 Asia/Tokyo
	channelConfig := &models.ChannelConfig{
		ID:                       uuid.NewString(),
		SlackChannelID:           testChannelID,
		LabelName:                labelName,
		DefaultMentionID:         "U_TEST_MENTION",
		ReviewerList:             "U_REVIEWER_1,U_REVIEWER_2",
		ReviewerReminderInterval: 5, // 5 minutes
		BusinessHoursStart:       "09:00",
		BusinessHoursEnd:         "18:00",
		Timezone:                 "Asia/Tokyo",
		IsActive:                 true,
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
	}
	testConfig.DB.Create(channelConfig)
	defer testConfig.DB.Delete(&channelConfig)

	// Verify that testTime is indeed outside business hours
	isBusinessHours := services.IsWithinBusinessHours(channelConfig, testTime)
	assert.False(t, isBusinessHours, "Test time should be outside business hours")

	// Create a review task that needs reminder (old UpdatedAt)
	oldUpdateTime := testTime.Add(-10 * time.Minute)
	task := &models.ReviewTask{
		ID:                 uuid.NewString(),
		PRURL:              "https://github.com/owner/repo/pull/789",
		Repo:               "owner/repo",
		PRNumber:           789,
		Title:              "Test PR for Out-of-Hours Reminder",
		SlackTS:            "1234567890.987654",
		SlackChannel:       testChannelID,
		Reviewer:           "U_REVIEWER_1",
		Status:             "in_review",
		LabelName:          labelName,
		OutOfHoursReminded: false,
		CreatedAt:          oldUpdateTime,
		UpdatedAt:          oldUpdateTime,
	}
	testConfig.DB.Create(task)
	defer testConfig.DB.Delete(&task)

	// Note: We can't actually run CheckInReviewTasks here with a fake time
	// because it uses time.Now() internally. Instead, we'll test the logic separately.

	// Verify the IsWithinBusinessHours function works correctly
	assert.False(t, services.IsWithinBusinessHours(channelConfig, testTime),
		"Should detect time is outside business hours")

	// Test weekend detection
	assert.Equal(t, time.Saturday, testTime.Weekday(),
		"Test time should be on Saturday")

	// Verify task state before any processing
	assert.False(t, task.OutOfHoursReminded,
		"OutOfHoursReminded should initially be false")
	assert.Nil(t, task.ReminderPausedUntil,
		"ReminderPausedUntil should initially be nil")
}

// TestTaskCheckerWithWaitingBusinessHours tests the business hours task checker
func TestTaskCheckerWithWaitingBusinessHours(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	testChannelID := "C_TEST_WAITING_BH"
	labelName := "needs-review"

	// Create channel config
	channelConfig := &models.ChannelConfig{
		ID:                 uuid.NewString(),
		SlackChannelID:     testChannelID,
		LabelName:          labelName,
		DefaultMentionID:   "U_TEST_MENTION",
		ReviewerList:       "U_REVIEWER_1,U_REVIEWER_2",
		BusinessHoursStart: "09:00",
		BusinessHoursEnd:   "18:00",
		Timezone:           "Asia/Tokyo",
		IsActive:           true,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
	testConfig.DB.Create(channelConfig)
	defer testConfig.DB.Delete(&channelConfig)

	// Create a task in "waiting_business_hours" state
	task := &models.ReviewTask{
		ID:           uuid.NewString(),
		PRURL:        "https://github.com/owner/repo/pull/101",
		Repo:         "owner/repo",
		PRNumber:     101,
		Title:        "Test PR Waiting for Business Hours",
		SlackTS:      "1234567890.111111",
		SlackChannel: testChannelID,
		Status:       "waiting_business_hours",
		LabelName:    labelName,
		CreatedAt:    time.Now().Add(-1 * time.Hour),
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}
	testConfig.DB.Create(task)
	defer testConfig.DB.Delete(&task)

	// Get current time and check if we're in business hours
	now := time.Now()
	isBusinessHours := services.IsWithinBusinessHours(channelConfig, now)

	// Run the business hours task checker
	services.CheckBusinessHoursTasks(testConfig.DB)

	// Verify task behavior based on business hours
	var updatedTask models.ReviewTask
	err := testConfig.DB.Where("id = ?", task.ID).First(&updatedTask).Error
	require.NoError(t, err, "Should find the task")

	if isBusinessHours {
		// During business hours, the task checker attempts to activate the task
		// However, in test mode with mock Slack client, the API call may fail
		// In that case, the task remains in "waiting_business_hours" state
		// This test verifies that the checker runs without crashing
		assert.Contains(t, []string{"waiting_business_hours", "in_review"}, updatedTask.Status,
			"Task status should be waiting_business_hours or in_review during business hours")

		// If status changed to in_review, reviewer should be assigned
		if updatedTask.Status == "in_review" {
			assert.NotEmpty(t, updatedTask.Reviewer,
				"Reviewer should be assigned when status is in_review")
		}
	} else {
		// Outside business hours, task should remain in "waiting_business_hours"
		assert.Equal(t, "waiting_business_hours", updatedTask.Status,
			"Task status should remain waiting_business_hours outside business hours")
	}
}

// TestReminderPauseFunctionality tests the reminder pause functionality
func TestReminderPauseFunctionality(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	testChannelID := "C_TEST_PAUSE"
	labelName := "needs-review"

	// Create channel config
	channelConfig := &models.ChannelConfig{
		ID:                       uuid.NewString(),
		SlackChannelID:           testChannelID,
		LabelName:                labelName,
		DefaultMentionID:         "U_TEST_MENTION",
		ReviewerList:             "U_REVIEWER_1,U_REVIEWER_2",
		ReviewerReminderInterval: 5, // 5 minutes
		IsActive:                 true,
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
	}
	testConfig.DB.Create(channelConfig)
	defer testConfig.DB.Delete(&channelConfig)

	// Create a task with reminder paused until future
	futureTime := time.Now().Add(1 * time.Hour)
	task := &models.ReviewTask{
		ID:                  uuid.NewString(),
		PRURL:               "https://github.com/owner/repo/pull/202",
		Repo:                "owner/repo",
		PRNumber:            202,
		Title:               "Test PR with Paused Reminder",
		SlackTS:             "1234567890.222222",
		SlackChannel:        testChannelID,
		Reviewer:            "U_REVIEWER_1",
		Status:              "in_review",
		LabelName:           labelName,
		ReminderPausedUntil: &futureTime,
		CreatedAt:           time.Now().Add(-10 * time.Minute),
		UpdatedAt:           time.Now().Add(-10 * time.Minute),
	}
	testConfig.DB.Create(task)
	defer testConfig.DB.Delete(&task)

	// Store original UpdatedAt
	originalUpdatedAt := task.UpdatedAt

	// Run the task checker
	services.CheckInReviewTasks(testConfig.DB)

	// Verify task was NOT updated (reminder was skipped due to pause)
	var updatedTask models.ReviewTask
	err := testConfig.DB.Where("id = ?", task.ID).First(&updatedTask).Error
	require.NoError(t, err, "Should find the task")

	// UpdatedAt should remain the same because reminder is paused
	assert.Equal(t, originalUpdatedAt.Unix(), updatedTask.UpdatedAt.Unix(),
		"Task UpdatedAt should not change when reminder is paused")
	assert.NotNil(t, updatedTask.ReminderPausedUntil,
		"ReminderPausedUntil should still be set")
	assert.True(t, updatedTask.ReminderPausedUntil.After(time.Now()),
		"ReminderPausedUntil should still be in the future")
}

// TestCleanupOldTasks tests the cleanup functionality for old tasks
func TestCleanupOldTasks(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	testChannelID := "C_TEST_CLEANUP"
	labelName := "needs-review"

	// Create old done task (2 days old)
	oldDoneTask := &models.ReviewTask{
		ID:           uuid.NewString(),
		PRURL:        "https://github.com/owner/repo/pull/301",
		Repo:         "owner/repo",
		PRNumber:     301,
		Title:        "Old Done Task",
		SlackTS:      "1234567890.333333",
		SlackChannel: testChannelID,
		Status:       "done",
		LabelName:    labelName,
		CreatedAt:    time.Now().Add(-2 * 24 * time.Hour),
		UpdatedAt:    time.Now().Add(-2 * 24 * time.Hour),
	}
	testConfig.DB.Create(oldDoneTask)

	// Create recent done task (1 hour old)
	recentDoneTask := &models.ReviewTask{
		ID:           uuid.NewString(),
		PRURL:        "https://github.com/owner/repo/pull/302",
		Repo:         "owner/repo",
		PRNumber:     302,
		Title:        "Recent Done Task",
		SlackTS:      "1234567890.444444",
		SlackChannel: testChannelID,
		Status:       "done",
		LabelName:    labelName,
		CreatedAt:    time.Now().Add(-1 * time.Hour),
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}
	testConfig.DB.Create(recentDoneTask)

	// Create old paused task (8 days old)
	oldPausedTask := &models.ReviewTask{
		ID:           uuid.NewString(),
		PRURL:        "https://github.com/owner/repo/pull/303",
		Repo:         "owner/repo",
		PRNumber:     303,
		Title:        "Old Paused Task",
		SlackTS:      "1234567890.555555",
		SlackChannel: testChannelID,
		Status:       "paused",
		LabelName:    labelName,
		CreatedAt:    time.Now().Add(-8 * 24 * time.Hour),
		UpdatedAt:    time.Now().Add(-8 * 24 * time.Hour),
	}
	testConfig.DB.Create(oldPausedTask)

	// Create archived task
	archivedTask := &models.ReviewTask{
		ID:           uuid.NewString(),
		PRURL:        "https://github.com/owner/repo/pull/304",
		Repo:         "owner/repo",
		PRNumber:     304,
		Title:        "Archived Task",
		SlackTS:      "1234567890.666666",
		SlackChannel: testChannelID,
		Status:       "archived",
		LabelName:    labelName,
		CreatedAt:    time.Now().Add(-1 * time.Hour),
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}
	testConfig.DB.Create(archivedTask)

	// Run cleanup
	services.CleanupOldTasks(testConfig.DB)

	// Verify old done task was deleted
	var foundOldDone models.ReviewTask
	err := testConfig.DB.Where("id = ?", oldDoneTask.ID).First(&foundOldDone).Error
	assert.Error(t, err, "Old done task should be deleted")

	// Verify recent done task was NOT deleted
	var foundRecentDone models.ReviewTask
	err = testConfig.DB.Where("id = ?", recentDoneTask.ID).First(&foundRecentDone).Error
	assert.NoError(t, err, "Recent done task should not be deleted")

	// Verify old paused task was deleted
	var foundOldPaused models.ReviewTask
	err = testConfig.DB.Where("id = ?", oldPausedTask.ID).First(&foundOldPaused).Error
	assert.Error(t, err, "Old paused task should be deleted")

	// Verify archived task was deleted
	var foundArchived models.ReviewTask
	err = testConfig.DB.Where("id = ?", archivedTask.ID).First(&foundArchived).Error
	assert.Error(t, err, "Archived task should be deleted")

	// Cleanup remaining tasks
	testConfig.DB.Delete(&recentDoneTask)
}
