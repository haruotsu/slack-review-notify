package fixtures

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadChannelConfigs(t *testing.T) {
	configs, err := LoadChannelConfigs()
	require.NoError(t, err, "Failed to load channel configs")
	require.NotEmpty(t, configs, "Channel configs should not be empty")

	// Check that we have expected number of configs
	assert.GreaterOrEqual(t, len(configs), 1, "Should have at least one config")

	// Validate first config structure
	firstConfig := configs[0]
	assert.NotEmpty(t, firstConfig.SlackChannelID, "SlackChannelID should not be empty")
	assert.NotEmpty(t, firstConfig.LabelName, "LabelName should not be empty")
	assert.Greater(t, firstConfig.ReminderInterval, 0, "ReminderInterval should be positive")
}

func TestLoadReviewTasks(t *testing.T) {
	tasks, err := LoadReviewTasks()
	require.NoError(t, err, "Failed to load review tasks")
	require.NotEmpty(t, tasks, "Review tasks should not be empty")

	// Check that we have expected number of tasks
	assert.GreaterOrEqual(t, len(tasks), 1, "Should have at least one task")

	// Validate first task structure
	firstTask := tasks[0]
	assert.Greater(t, firstTask.PRNumber, 0, "PRNumber should be positive")
	assert.NotEmpty(t, firstTask.PRURL, "PRURL should not be empty")
	assert.NotEmpty(t, firstTask.Title, "Title should not be empty")
	assert.NotEmpty(t, firstTask.Repo, "Repo should not be empty")
	assert.NotEmpty(t, firstTask.LabelName, "LabelName should not be empty")
	assert.NotEmpty(t, firstTask.Status, "Status should not be empty")
	assert.NotEmpty(t, firstTask.SlackChannel, "SlackChannel should not be empty")
}

func TestLoadWebhookPayloads(t *testing.T) {
	payloads, err := LoadWebhookPayloads()
	require.NoError(t, err, "Failed to load webhook payloads")
	require.NotEmpty(t, payloads, "Webhook payloads should not be empty")

	// Check that we have expected payloads
	assert.Contains(t, payloads, "pull_request_labeled", "Should contain pull_request_labeled payload")

	// Validate payload structure
	payload := payloads["pull_request_labeled"]
	assert.NotNil(t, payload, "Payload should not be nil")
	assert.Contains(t, payload, "action", "Payload should contain action")
	assert.Contains(t, payload, "pull_request", "Payload should contain pull_request")
	assert.Contains(t, payload, "repository", "Payload should contain repository")
}

func TestLoadWebhookPayload(t *testing.T) {
	t.Run("existing payload", func(t *testing.T) {
		payload, err := LoadWebhookPayload("pull_request_labeled")
		require.NoError(t, err, "Failed to load pull_request_labeled payload")
		require.NotNil(t, payload, "Payload should not be nil")

		// Validate basic structure
		assert.Equal(t, "labeled", payload["action"], "Action should be 'labeled'")
		assert.NotNil(t, payload["pull_request"], "pull_request should not be nil")
	})

	t.Run("non-existing payload", func(t *testing.T) {
		payload, err := LoadWebhookPayload("non_existing_payload")
		assert.Error(t, err, "Should return error for non-existing payload")
		assert.ErrorIs(t, err, os.ErrNotExist, "Error should be ErrNotExist")
		assert.Nil(t, payload, "Payload should be nil")
	})
}

func TestGetChannelConfigByID(t *testing.T) {
	configs, err := LoadChannelConfigs()
	require.NoError(t, err)
	require.NotEmpty(t, configs)

	t.Run("existing channel ID", func(t *testing.T) {
		channelID := configs[0].SlackChannelID
		config := GetChannelConfigByID(configs, channelID)
		require.NotNil(t, config, "Config should not be nil")
		assert.Equal(t, channelID, config.SlackChannelID, "Channel ID should match")
	})

	t.Run("non-existing channel ID", func(t *testing.T) {
		config := GetChannelConfigByID(configs, "C99NONEXIST")
		assert.Nil(t, config, "Config should be nil for non-existing channel ID")
	})
}

func TestGetChannelConfigByLabel(t *testing.T) {
	configs, err := LoadChannelConfigs()
	require.NoError(t, err)
	require.NotEmpty(t, configs)

	t.Run("existing label", func(t *testing.T) {
		labelName := configs[0].LabelName
		config := GetChannelConfigByLabel(configs, labelName)
		require.NotNil(t, config, "Config should not be nil")
		assert.Equal(t, labelName, config.LabelName, "Label name should match")
	})

	t.Run("non-existing label", func(t *testing.T) {
		config := GetChannelConfigByLabel(configs, "non-existing-label")
		assert.Nil(t, config, "Config should be nil for non-existing label")
	})
}

func TestGetReviewTaskByPR(t *testing.T) {
	tasks, err := LoadReviewTasks()
	require.NoError(t, err)
	require.NotEmpty(t, tasks)

	t.Run("existing PR", func(t *testing.T) {
		prNumber := tasks[0].PRNumber
		repository := tasks[0].Repo
		task := GetReviewTaskByPR(tasks, prNumber, repository)
		require.NotNil(t, task, "Task should not be nil")
		assert.Equal(t, prNumber, task.PRNumber, "PR number should match")
		assert.Equal(t, repository, task.Repo, "Repository should match")
	})

	t.Run("non-existing PR", func(t *testing.T) {
		task := GetReviewTaskByPR(tasks, 99999, "owner/nonexistent")
		assert.Nil(t, task, "Task should be nil for non-existing PR")
	})
}

func TestGetReviewTasksByStatus(t *testing.T) {
	tasks, err := LoadReviewTasks()
	require.NoError(t, err)
	require.NotEmpty(t, tasks)

	t.Run("existing status", func(t *testing.T) {
		// Find a status that exists in the fixtures
		status := tasks[0].Status
		filtered := GetReviewTasksByStatus(tasks, status)
		assert.NotEmpty(t, filtered, "Should return tasks with the status")

		// All returned tasks should have the requested status
		for _, task := range filtered {
			assert.Equal(t, status, task.Status, "All tasks should have the requested status")
		}
	})

	t.Run("non-existing status", func(t *testing.T) {
		filtered := GetReviewTasksByStatus(tasks, "non-existing-status")
		assert.Empty(t, filtered, "Should return empty slice for non-existing status")
	})
}

func TestGetReviewTasksByChannel(t *testing.T) {
	tasks, err := LoadReviewTasks()
	require.NoError(t, err)
	require.NotEmpty(t, tasks)

	t.Run("existing channel", func(t *testing.T) {
		channelID := tasks[0].SlackChannel
		filtered := GetReviewTasksByChannel(tasks, channelID)
		assert.NotEmpty(t, filtered, "Should return tasks for the channel")

		// All returned tasks should have the requested channel ID
		for _, task := range filtered {
			assert.Equal(t, channelID, task.SlackChannel, "All tasks should have the requested channel ID")
		}
	})

	t.Run("non-existing channel", func(t *testing.T) {
		filtered := GetReviewTasksByChannel(tasks, "C99NONEXIST")
		assert.Empty(t, filtered, "Should return empty slice for non-existing channel")
	})
}

func TestGetFixturesDir(t *testing.T) {
	dir := GetFixturesDir()
	assert.NotEmpty(t, dir, "Fixtures directory should not be empty")

	// Verify directory exists
	info, err := os.Stat(dir)
	require.NoError(t, err, "Fixtures directory should exist")
	assert.True(t, info.IsDir(), "Path should be a directory")
}
