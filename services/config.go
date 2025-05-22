package services

import (
	"log"
	"slack-review-notify/models"
	"strings"

	"gorm.io/gorm"
)

// チャンネル設定を取得する関数（後方互換性のため）
// 注: これは過去互換性のための関数で、複数ラベル対応後は GetChannelConfigByLabel を使用すべき
func GetChannelConfig(db *gorm.DB, channelID string) (*models.ChannelConfig, error) {
	var config models.ChannelConfig

	err := db.Where("slack_channel_id = ? AND is_active = ?", channelID, true).First(&config).Error
	if err != nil {
		return nil, err
	}

	return &config, nil
}

// 特定のラベル名を持つチャンネル設定を取得する関数
func GetChannelConfigByLabel(db *gorm.DB, channelID string, labelName string) (*models.ChannelConfig, error) {
	var config models.ChannelConfig

	err := db.Where("slack_channel_id = ? AND label_name = ? AND is_active = ?",
		channelID, labelName, true).First(&config).Error
	if err != nil {
		return nil, err
	}

	return &config, nil
}

// チャンネルの全設定を取得する関数
func GetAllChannelConfigs(db *gorm.DB, channelID string) ([]models.ChannelConfig, error) {
	var configs []models.ChannelConfig

	err := db.Where("slack_channel_id = ? AND is_active = ?", channelID, true).Find(&configs).Error
	if err != nil {
		return nil, err
	}

	return configs, nil
}

// チャンネル設定が存在するか確認する関数
func HasChannelConfig(db *gorm.DB, channelID string) bool {
	var count int64
	db.Model(&models.ChannelConfig{}).Where("slack_channel_id = ? AND is_active = ?", channelID, true).Count(&count)
	return count > 0
}

// 特定のラベルのチャンネル設定が存在するか確認する関数
func HasChannelConfigWithLabel(db *gorm.DB, channelID string, labelName string) bool {
	var count int64
	db.Model(&models.ChannelConfig{}).Where(
		"slack_channel_id = ? AND label_name = ? AND is_active = ?",
		channelID, labelName, true).Count(&count)
	return count > 0
}

// リポジトリがチャンネルで通知対象かチェックする関数
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
