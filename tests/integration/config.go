package integration

import (
	"os"
	"slack-review-notify/models"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestConfig holds configuration for integration tests
type TestConfig struct {
	DB                 *gorm.DB
	SlackBotToken      string
	SlackSigningSecret string
	OriginalDBPath     string
	TestDBPath         string
	CleanupFuncs       []func()
	IsSlackMockMode    bool
}

// SetupTestDB creates an in-memory or file-based test database
// If dbPath is empty, it uses in-memory database
func SetupTestDB(t *testing.T, dbPath string) *gorm.DB {
	var dialector gorm.Dialector

	if dbPath == "" {
		// Use in-memory database for fast unit tests
		dialector = sqlite.Open(":memory:")
	} else {
		// Use file-based database for integration tests
		dialector = sqlite.Open(dbPath)
	}

	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent), // Suppress DB logs in tests
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Run migrations
	if err := db.AutoMigrate(&models.ChannelConfig{}, &models.ReviewTask{}); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}

	return db
}

// CleanupTestDB removes all data from the database
func CleanupTestDB(db *gorm.DB) error {
	// Delete all records from tables
	if err := db.Exec("DELETE FROM review_tasks").Error; err != nil {
		return err
	}
	if err := db.Exec("DELETE FROM channel_configs").Error; err != nil {
		return err
	}
	return nil
}

// SetupTestEnvironment sets up the complete test environment
// including database, environment variables, and cleanup functions
func SetupTestEnvironment(t *testing.T, useFileDB bool) *TestConfig {
	config := &TestConfig{
		CleanupFuncs:    make([]func(), 0),
		IsSlackMockMode: true,
	}

	// Setup database
	var dbPath string
	if useFileDB {
		dbPath = "test_integration.db"
		config.TestDBPath = dbPath
		// Add cleanup function to remove DB file
		config.CleanupFuncs = append(config.CleanupFuncs, func() {
			_ = os.Remove(dbPath)
		})
	}
	config.DB = SetupTestDB(t, dbPath)

	// Setup environment variables
	config.OriginalDBPath = os.Getenv("DB_PATH")
	config.SlackBotToken = os.Getenv("SLACK_BOT_TOKEN")
	config.SlackSigningSecret = os.Getenv("SLACK_SIGNING_SECRET")

	// Set test environment variables
	testSlackToken := "xoxb-test-token-integration"
	if err := os.Setenv("SLACK_BOT_TOKEN", testSlackToken); err != nil {
		t.Fatalf("failed to set SLACK_BOT_TOKEN: %v", err)
	}

	testSigningSecret := "test_signing_secret_12345"
	if err := os.Setenv("SLACK_SIGNING_SECRET", testSigningSecret); err != nil {
		t.Fatalf("failed to set SLACK_SIGNING_SECRET: %v", err)
	}

	if dbPath != "" {
		if err := os.Setenv("DB_PATH", dbPath); err != nil {
			t.Fatalf("failed to set DB_PATH: %v", err)
		}
	}

	// Add cleanup function to restore environment variables
	config.CleanupFuncs = append(config.CleanupFuncs, func() {
		if config.OriginalDBPath != "" {
			_ = os.Setenv("DB_PATH", config.OriginalDBPath)
		} else {
			_ = os.Unsetenv("DB_PATH")
		}

		if config.SlackBotToken != "" {
			_ = os.Setenv("SLACK_BOT_TOKEN", config.SlackBotToken)
		} else {
			_ = os.Unsetenv("SLACK_BOT_TOKEN")
		}

		if config.SlackSigningSecret != "" {
			_ = os.Setenv("SLACK_SIGNING_SECRET", config.SlackSigningSecret)
		} else {
			_ = os.Unsetenv("SLACK_SIGNING_SECRET")
		}
	})

	return config
}

// Cleanup runs all registered cleanup functions
func (c *TestConfig) Cleanup() {
	// Clean up database first
	if c.DB != nil {
		_ = CleanupTestDB(c.DB)
	}

	// Run cleanup functions in reverse order
	for i := len(c.CleanupFuncs) - 1; i >= 0; i-- {
		c.CleanupFuncs[i]()
	}
}

// CreateTestChannelConfig creates a test channel configuration
func CreateTestChannelConfig(db *gorm.DB, channelID, labelName string) *models.ChannelConfig {
	config := &models.ChannelConfig{
		ID:                       "test-" + channelID + "-" + labelName,
		SlackChannelID:           channelID,
		LabelName:                labelName,
		DefaultMentionID:         "U12345",
		ReviewerList:             "U12345,U67890,U99999",
		RepositoryList:           "owner/test-repo",
		IsActive:                 true,
		ReminderInterval:         30,
		ReviewerReminderInterval: 30,
		BusinessHoursStart:       "09:00",
		BusinessHoursEnd:         "18:00",
		Timezone:                 "Asia/Tokyo",
	}

	db.Create(config)
	return config
}

// CreateTestReviewTask creates a test review task
func CreateTestReviewTask(db *gorm.DB, taskID, channelID, labelName, status string) *models.ReviewTask {
	task := &models.ReviewTask{
		ID:           taskID,
		PRURL:        "https://github.com/owner/test-repo/pull/1",
		Repo:         "owner/test-repo",
		PRNumber:     1,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: channelID,
		Status:       status,
		LabelName:    labelName,
	}

	db.Create(task)
	return task
}
