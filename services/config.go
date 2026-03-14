package services

import (
	"log"
	"slack-review-notify/models"
	"strings"

	"github.com/google/go-github/v71/github"
	"gorm.io/gorm"
)

// GetChannelConfig retrieves the channel configuration
func GetChannelConfig(db *gorm.DB, channelID string, labelName string) (*models.ChannelConfig, error) {
	var config models.ChannelConfig

	err := db.Where("slack_channel_id = ? AND label_name = ? AND is_active = ?", channelID, labelName, true).First(&config).Error
	if err != nil {
		return nil, err
	}

	return &config, nil
}

// HasChannelConfig checks whether a channel configuration exists
func HasChannelConfig(db *gorm.DB, channelID string, labelName string) bool {
	var count int64
	db.Model(&models.ChannelConfig{}).Where("slack_channel_id = ? AND label_name = ? AND is_active = ?", channelID, labelName, true).Count(&count)
	return count > 0
}

// IsRepositoryWatched checks whether a repository is a notification target for the channel
func IsRepositoryWatched(config *models.ChannelConfig, repoFullName string) bool {
	if config == nil {
		log.Printf("channel config is nil")
		return false
	}

	if config.RepositoryList == "" {
		log.Printf("channel %s has no repository list", config.SlackChannelID)
		return false
	}

	repos := strings.Split(config.RepositoryList, ",")
	log.Printf("channel %s repository list: %v (target: %s)",
		config.SlackChannelID, repos, repoFullName)

	for _, repo := range repos {
		trimmedRepo := strings.TrimSpace(repo)
		if trimmedRepo == repoFullName {
			log.Printf("repository %s is watched", repoFullName)
			return true
		}
	}

	log.Printf("repository %s is not watched", repoFullName)
	return false
}

// IsLabelMatched checks whether the labels match the configuration conditions
func IsLabelMatched(config *models.ChannelConfig, prLabels []*github.Label) bool {
	if config == nil {
		log.Printf("channel config is nil")
		return false
	}

	// Empty string does not match
	if config.LabelName == "" {
		log.Printf("no label configured for channel")
		return false
	}

	// Convert PR label names to a set
	prLabelNames := make(map[string]bool)
	for _, label := range prLabels {
		if label.Name != nil {
			prLabelNames[*label.Name] = true
		}
	}

	// Split the configured labels (comma-separated)
	requiredLabels := strings.Split(config.LabelName, ",")

	// Check that all required labels exist on the PR (AND condition)
	for _, label := range requiredLabels {
		trimmedLabel := strings.TrimSpace(label)
		if trimmedLabel != "" && !prLabelNames[trimmedLabel] {
			log.Printf("required label '%s' not found in PR labels", trimmedLabel)
			return false
		}
	}

	log.Printf("all required labels found: %s", config.LabelName)
	return true
}

// IsAddedLabelRelevant checks whether the newly added label is relevant to the configured label set
func IsAddedLabelRelevant(config *models.ChannelConfig, addedLabelName string) bool {
	if config == nil || config.LabelName == "" {
		return false
	}

	// Split the configured labels (comma-separated)
	requiredLabels := strings.Split(config.LabelName, ",")

	// Check whether the added label is included in the configured label set
	for _, label := range requiredLabels {
		trimmedLabel := strings.TrimSpace(label)
		if trimmedLabel != "" && trimmedLabel == addedLabelName {
			log.Printf("added label '%s' is relevant to config: %s", addedLabelName, config.LabelName)
			return true
		}
	}

	log.Printf("added label '%s' is not relevant to config: %s", addedLabelName, config.LabelName)
	return false
}

// GetMissingLabels returns the required labels from the configuration that are not present on the PR
func GetMissingLabels(config *models.ChannelConfig, prLabels []*github.Label) []string {
	if config == nil || config.LabelName == "" {
		return []string{}
	}

	// Convert PR label names to a set
	prLabelNames := make(map[string]bool)
	for _, label := range prLabels {
		if label.Name != nil {
			prLabelNames[*label.Name] = true
		}
	}

	// Split the configured labels (comma-separated)
	requiredLabels := strings.Split(config.LabelName, ",")
	missingLabels := make([]string, 0)

	// Collect required labels that are missing
	for _, label := range requiredLabels {
		trimmedLabel := strings.TrimSpace(label)
		if trimmedLabel != "" && !prLabelNames[trimmedLabel] {
			missingLabels = append(missingLabels, trimmedLabel)
		}
	}

	return missingLabels
}
