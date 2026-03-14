package services

import (
	"slack-review-notify/models"
	"testing"
	"time"

	"os"

	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
)

func TestCleanupArchivedChannels(t *testing.T) {
	// Set up test database
	db := setupTestDB(t)

	// Save environment variables before the test and restore them afterwards
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()

	// Set environment variables for testing
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	// Set up mocks
	defer gock.Off() // Clear mocks when the test ends

	// Create channel configurations for testing
	configs := []models.ChannelConfig{
		{
			ID:               "config-id-1",
			SlackChannelID:   "C12345",
			DefaultMentionID: "U12345",
			RepositoryList:   "owner/repo1",
			LabelName:        "needs-review",
			IsActive:         true,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		},
		{
			ID:               "config-id-2",
			SlackChannelID:   "C67890",
			DefaultMentionID: "U67890",
			RepositoryList:   "owner/repo2",
			LabelName:        "needs-review",
			IsActive:         true,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		},
	}

	for _, config := range configs {
		db.Create(&config)
	}

	// Mock for channel info retrieval (first channel is active)
	gock.New("https://slack.com").
		Get("/api/conversations.info").
		MatchParam("channel", "C12345").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
			"channel": map[string]interface{}{
				"id":          "C12345",
				"is_archived": false,
			},
		})

	// Mock for channel info retrieval (second channel is archived)
	gock.New("https://slack.com").
		Get("/api/conversations.info").
		MatchParam("channel", "C67890").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
			"channel": map[string]interface{}{
				"id":          "C67890",
				"is_archived": true,
			},
		})

	// Execute the function
	CleanupArchivedChannels(db)

	// Verify that the DB has been updated
	var config1 models.ChannelConfig
	db.Where("slack_channel_id = ?", "C12345").First(&config1)
	assert.True(t, config1.IsActive, "Active channel should remain enabled")

	var config2 models.ChannelConfig
	db.Where("slack_channel_id = ?", "C67890").First(&config2)
	assert.False(t, config2.IsActive, "Archived channel should be disabled")

	assert.True(t, gock.IsDone(), "Not all mocks have been used")

	// Also test the API error case
	gock.New("https://slack.com").
		Get("/api/conversations.info").
		MatchParam("channel", "C12345").
		Reply(200).
		JSON(map[string]interface{}{
			"ok":    false,
			"error": "channel_not_found",
		})

	// Execute the function again (verify that errors are handled properly)
	CleanupArchivedChannels(db)

	// Verify that processing continues even when there are API errors
	assert.True(t, gock.IsDone(), "Not all mocks have been used")
}
