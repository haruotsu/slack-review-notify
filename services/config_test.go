package services

import (
	"slack-review-notify/models"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("fail to open test db: %v", err)
	}

	// Run migrations
	if err := db.AutoMigrate(&models.ChannelConfig{}, &models.ReviewTask{}, &models.UserMapping{}, &models.ReviewerAvailability{}); err != nil {
		t.Fatalf("fail to migrate test db: %v", err)
	}

	return db
}

func TestGetChannelConfig(t *testing.T) {
	db := setupTestDB(t)

	// Create test data
	testConfig := models.ChannelConfig{
		ID:               "test-id",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U12345",
		RepositoryList:   "owner/repo1,owner/repo2",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	db.Create(&testConfig)

	// Execute test
	config, err := GetChannelConfig(db, "C12345", "needs-review")

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, "C12345", config.SlackChannelID)
	assert.Equal(t, "U12345", config.DefaultMentionID)
	assert.Equal(t, "needs-review", config.LabelName)
	assert.True(t, config.IsActive)

	// Test with a non-existent channel ID
	_, err = GetChannelConfig(db, "nonexistent", "needs-review")
	assert.Error(t, err)
}

func TestHasChannelConfig(t *testing.T) {
	db := setupTestDB(t)

	// Create test data
	testConfig := models.ChannelConfig{
		ID:               "test-id",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U12345",
		RepositoryList:   "owner/repo1,owner/repo2",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	db.Create(&testConfig)

	// Execute test and assertions
	assert.True(t, HasChannelConfig(db, "C12345", "needs-review"))
	assert.False(t, HasChannelConfig(db, "nonexistent", "needs-review"))
	assert.False(t, HasChannelConfig(db, "C12345", "other-label"))
}

func TestIsRepositoryWatched(t *testing.T) {
	// Create a config struct for testing
	config := &models.ChannelConfig{
		SlackChannelID: "C12345",
		RepositoryList: "owner/repo1,owner/repo2, owner/repo3",
	}

	// Test cases
	testCases := []struct {
		repoName string
		expected bool
	}{
		{"owner/repo1", true},
		{"owner/repo2", true},
		{"owner/repo3", true},
		{"owner/repo4", false},
		{"different/repo", false},
	}

	for _, tc := range testCases {
		t.Run(tc.repoName, func(t *testing.T) {
			result := IsRepositoryWatched(config, tc.repoName)
			assert.Equal(t, tc.expected, result)
		})
	}

	// Test with an empty repository list
	emptyConfig := &models.ChannelConfig{
		SlackChannelID: "C12345",
		RepositoryList: "",
	}
	assert.False(t, IsRepositoryWatched(emptyConfig, "owner/repo1"))

	// Test with nil config
	assert.False(t, IsRepositoryWatched(nil, "owner/repo1"))
}
