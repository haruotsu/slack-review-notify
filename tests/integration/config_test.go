package integration

import (
	"os"
	"slack-review-notify/models"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetupTestDB_InMemory(t *testing.T) {
	// Test in-memory database setup
	db := SetupTestDB(t, "")
	assert.NotNil(t, db, "Database should be created")

	// Verify migrations ran successfully by creating a record
	config := &models.ChannelConfig{
		ID:             "test-1",
		SlackChannelID: "C12345",
		LabelName:      "needs-review",
		IsActive:       true,
	}

	result := db.Create(config)
	assert.NoError(t, result.Error, "Should be able to create a channel config")

	// Verify record was created
	var found models.ChannelConfig
	err := db.Where("id = ?", "test-1").First(&found).Error
	assert.NoError(t, err, "Should be able to find created config")
	assert.Equal(t, "C12345", found.SlackChannelID)
}

func TestSetupTestDB_FileDB(t *testing.T) {
	// Test file-based database setup
	dbPath := "test_db_file.db"
	defer func() {
		_ = os.Remove(dbPath)
	}()

	db := SetupTestDB(t, dbPath)
	assert.NotNil(t, db, "Database should be created")

	// Verify file was created
	_, err := os.Stat(dbPath)
	assert.NoError(t, err, "Database file should exist")

	// Verify migrations ran successfully
	task := &models.ReviewTask{
		ID:           "task-1",
		PRURL:        "https://github.com/test/repo/pull/1",
		Repo:         "test/repo",
		PRNumber:     1,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Status:       "pending",
		LabelName:    "needs-review",
	}

	result := db.Create(task)
	assert.NoError(t, result.Error, "Should be able to create a review task")
}

func TestCleanupTestDB(t *testing.T) {
	// Setup database with test data
	db := SetupTestDB(t, "")

	// Create test data
	config := &models.ChannelConfig{
		ID:             "test-1",
		SlackChannelID: "C12345",
		LabelName:      "needs-review",
		IsActive:       true,
	}
	db.Create(config)

	task := &models.ReviewTask{
		ID:           "task-1",
		PRURL:        "https://github.com/test/repo/pull/1",
		Repo:         "test/repo",
		PRNumber:     1,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Status:       "pending",
		LabelName:    "needs-review",
	}
	db.Create(task)

	// Verify data exists
	var configCount, taskCount int64
	db.Model(&models.ChannelConfig{}).Count(&configCount)
	db.Model(&models.ReviewTask{}).Count(&taskCount)
	assert.Equal(t, int64(1), configCount, "Should have 1 config before cleanup")
	assert.Equal(t, int64(1), taskCount, "Should have 1 task before cleanup")

	// Cleanup
	err := CleanupTestDB(db)
	assert.NoError(t, err, "Cleanup should succeed")

	// Verify data was removed
	db.Model(&models.ChannelConfig{}).Count(&configCount)
	db.Model(&models.ReviewTask{}).Count(&taskCount)
	assert.Equal(t, int64(0), configCount, "Should have 0 configs after cleanup")
	assert.Equal(t, int64(0), taskCount, "Should have 0 tasks after cleanup")
}

func TestSetupTestEnvironment_InMemory(t *testing.T) {
	// Setup test environment
	testConfig := SetupTestEnvironment(t, false)
	defer testConfig.Cleanup()

	// Verify database was created
	assert.NotNil(t, testConfig.DB, "Database should be initialized")

	// Verify environment variables were set
	slackToken := os.Getenv("SLACK_BOT_TOKEN")
	assert.Equal(t, "xoxb-test-token-integration", slackToken, "SLACK_BOT_TOKEN should be set")

	// Verify test mode flag
	assert.True(t, testConfig.IsSlackMockMode, "Should be in mock mode")
}

func TestSetupTestEnvironment_FileDB(t *testing.T) {
	// Save original env vars
	originalSlackToken := os.Getenv("SLACK_BOT_TOKEN")

	// Setup test environment with file DB
	testConfig := SetupTestEnvironment(t, true)
	defer testConfig.Cleanup()

	// Verify database was created
	assert.NotNil(t, testConfig.DB, "Database should be initialized")

	// Verify DB file exists
	assert.NotEmpty(t, testConfig.TestDBPath, "DB path should be set")
	_, err := os.Stat(testConfig.TestDBPath)
	assert.NoError(t, err, "Database file should exist")

	// Verify environment variables
	dbPath := os.Getenv("DB_PATH")
	assert.Equal(t, testConfig.TestDBPath, dbPath, "DB_PATH should be set")

	// Cleanup should restore original env vars
	testConfig.Cleanup()

	// Verify cleanup removed the DB file
	_, err = os.Stat(testConfig.TestDBPath)
	assert.True(t, os.IsNotExist(err), "Database file should be removed after cleanup")

	// Verify env vars were restored
	currentSlackToken := os.Getenv("SLACK_BOT_TOKEN")
	assert.Equal(t, originalSlackToken, currentSlackToken, "SLACK_BOT_TOKEN should be restored")
}

func TestCreateTestChannelConfig(t *testing.T) {
	db := SetupTestDB(t, "")

	// Create test config
	config := CreateTestChannelConfig(db, "C12345", "needs-review")
	assert.NotNil(t, config, "Config should be created")

	// Verify config was saved to DB
	var found models.ChannelConfig
	err := db.Where("slack_channel_id = ? AND label_name = ?", "C12345", "needs-review").First(&found).Error
	assert.NoError(t, err, "Should find created config")
	assert.Equal(t, "C12345", found.SlackChannelID)
	assert.Equal(t, "needs-review", found.LabelName)
	assert.Equal(t, "U12345", found.DefaultMentionID)
	assert.Equal(t, "U12345,U67890,U99999", found.ReviewerList)
	assert.Equal(t, "owner/test-repo", found.RepositoryList)
	assert.True(t, found.IsActive)
	assert.Equal(t, 30, found.ReminderInterval)
	assert.Equal(t, 30, found.ReviewerReminderInterval)
	assert.Equal(t, "09:00", found.BusinessHoursStart)
	assert.Equal(t, "18:00", found.BusinessHoursEnd)
	assert.Equal(t, "Asia/Tokyo", found.Timezone)
}

func TestCreateTestReviewTask(t *testing.T) {
	db := SetupTestDB(t, "")

	// Create test task
	task := CreateTestReviewTask(db, "task-1", "C12345", "needs-review", "pending")
	assert.NotNil(t, task, "Task should be created")

	// Verify task was saved to DB
	var found models.ReviewTask
	err := db.Where("id = ?", "task-1").First(&found).Error
	assert.NoError(t, err, "Should find created task")
	assert.Equal(t, "task-1", found.ID)
	assert.Equal(t, "https://github.com/owner/test-repo/pull/1", found.PRURL)
	assert.Equal(t, "owner/test-repo", found.Repo)
	assert.Equal(t, 1, found.PRNumber)
	assert.Equal(t, "Test PR", found.Title)
	assert.Equal(t, "1234.5678", found.SlackTS)
	assert.Equal(t, "C12345", found.SlackChannel)
	assert.Equal(t, "pending", found.Status)
	assert.Equal(t, "needs-review", found.LabelName)
}
