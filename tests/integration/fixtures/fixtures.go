package fixtures

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"slack-review-notify/models"
)

// GetFixturesDir returns the absolute path to the fixtures directory
func GetFixturesDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Dir(filename)
}

// LoadChannelConfigs loads channel configuration fixtures
func LoadChannelConfigs() ([]models.ChannelConfig, error) {
	fixturesDir := GetFixturesDir()
	filePath := filepath.Join(fixturesDir, "channel_configs.json")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var rawConfigs []map[string]interface{}
	if err := json.Unmarshal(data, &rawConfigs); err != nil {
		return nil, err
	}

	configs := make([]models.ChannelConfig, len(rawConfigs))
	for i, rawConfig := range rawConfigs {
		configs[i] = models.ChannelConfig{
			ID:                       rawConfig["id"].(string),
			SlackChannelID:           rawConfig["slack_channel_id"].(string),
			LabelName:                rawConfig["label_name"].(string),
			DefaultMentionID:         rawConfig["default_mention_id"].(string),
			ReviewerList:             rawConfig["reviewer_list"].(string),
			RepositoryList:           rawConfig["repository_list"].(string),
			IsActive:                 rawConfig["is_active"].(bool),
			ReminderInterval:         int(rawConfig["reminder_interval"].(float64)),
			ReviewerReminderInterval: int(rawConfig["reviewer_reminder_interval"].(float64)),
			BusinessHoursStart:       rawConfig["business_hours_start"].(string),
			BusinessHoursEnd:         rawConfig["business_hours_end"].(string),
			Timezone:                 rawConfig["timezone"].(string),
		}
	}

	return configs, nil
}

// LoadReviewTasks loads review task fixtures
func LoadReviewTasks() ([]models.ReviewTask, error) {
	fixturesDir := GetFixturesDir()
	filePath := filepath.Join(fixturesDir, "review_tasks.json")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var rawTasks []map[string]interface{}
	if err := json.Unmarshal(data, &rawTasks); err != nil {
		return nil, err
	}

	tasks := make([]models.ReviewTask, len(rawTasks))
	for i, rawTask := range rawTasks {
		tasks[i] = models.ReviewTask{
			ID:                 rawTask["id"].(string),
			PRURL:              rawTask["pr_url"].(string),
			Repo:               rawTask["repo"].(string),
			PRNumber:           int(rawTask["pr_number"].(float64)),
			Title:              rawTask["title"].(string),
			SlackTS:            rawTask["slack_ts"].(string),
			SlackChannel:       rawTask["slack_channel"].(string),
			Reviewer:           rawTask["reviewer"].(string),
			Status:             rawTask["status"].(string),
			LabelName:          rawTask["label_name"].(string),
			OutOfHoursReminded: rawTask["out_of_hours_reminded"].(bool),
		}

		// Handle nullable timestamps
		if watchingUntil, ok := rawTask["watching_until"].(string); ok && watchingUntil != "" {
			if t, err := time.Parse(time.RFC3339, watchingUntil); err == nil {
				tasks[i].WatchingUntil = &t
			}
		}

		if reminderPausedUntil, ok := rawTask["reminder_paused_until"].(string); ok && reminderPausedUntil != "" {
			if t, err := time.Parse(time.RFC3339, reminderPausedUntil); err == nil {
				tasks[i].ReminderPausedUntil = &t
			}
		}
	}

	return tasks, nil
}

// WebhookPayload represents a generic webhook payload structure
type WebhookPayload map[string]interface{}

// LoadWebhookPayloads loads webhook payload fixtures
func LoadWebhookPayloads() (map[string]WebhookPayload, error) {
	fixturesDir := GetFixturesDir()
	filePath := filepath.Join(fixturesDir, "webhook_payloads.json")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var payloads map[string]WebhookPayload
	if err := json.Unmarshal(data, &payloads); err != nil {
		return nil, err
	}

	return payloads, nil
}

// LoadWebhookPayload loads a specific webhook payload by name
func LoadWebhookPayload(name string) (WebhookPayload, error) {
	payloads, err := LoadWebhookPayloads()
	if err != nil {
		return nil, err
	}

	payload, ok := payloads[name]
	if !ok {
		return nil, os.ErrNotExist
	}

	return payload, nil
}

// GetChannelConfigByID returns a channel config by Slack channel ID
func GetChannelConfigByID(configs []models.ChannelConfig, channelID string) *models.ChannelConfig {
	for i := range configs {
		if configs[i].SlackChannelID == channelID {
			return &configs[i]
		}
	}
	return nil
}

// GetChannelConfigByLabel returns a channel config by label name
func GetChannelConfigByLabel(configs []models.ChannelConfig, labelName string) *models.ChannelConfig {
	for i := range configs {
		if configs[i].LabelName == labelName {
			return &configs[i]
		}
	}
	return nil
}

// GetReviewTaskByPR returns a review task by PR number and repository
func GetReviewTaskByPR(tasks []models.ReviewTask, prNumber int, repository string) *models.ReviewTask {
	for i := range tasks {
		if tasks[i].PRNumber == prNumber && tasks[i].Repo == repository {
			return &tasks[i]
		}
	}
	return nil
}

// GetReviewTasksByStatus returns all review tasks with a specific status
func GetReviewTasksByStatus(tasks []models.ReviewTask, status string) []models.ReviewTask {
	var filtered []models.ReviewTask
	for _, task := range tasks {
		if task.Status == status {
			filtered = append(filtered, task)
		}
	}
	return filtered
}

// GetReviewTasksByChannel returns all review tasks for a specific Slack channel
func GetReviewTasksByChannel(tasks []models.ReviewTask, channelID string) []models.ReviewTask {
	var filtered []models.ReviewTask
	for _, task := range tasks {
		if task.SlackChannel == channelID {
			filtered = append(filtered, task)
		}
	}
	return filtered
}
