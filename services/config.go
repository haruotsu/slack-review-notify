package services

import (
	"slack-review-notify/models"
	"strings"

	"gorm.io/gorm"
)

// チャンネル設定を取得する関数
func GetChannelConfig(db *gorm.DB, channelID string) (*models.ChannelConfig, error) {
	var config models.ChannelConfig
	
	err := db.Where("slack_channel_id = ? AND is_active = ?", channelID, true).First(&config).Error
	if err != nil {
		return nil, err
	}
	
	return &config, nil
}

// チャンネル設定が存在するか確認する関数
func HasChannelConfig(db *gorm.DB, channelID string) bool {
	var count int64
	db.Model(&models.ChannelConfig{}).Where("slack_channel_id = ? AND is_active = ?", channelID, true).Count(&count)
	return count > 0
}

// リポジトリがチャンネルで監視対象かチェックする関数
func IsRepositoryWatched(config *models.ChannelConfig, repoFullName string) bool {
	if config == nil || config.RepositoryList == "" {
		return false
	}
	
	repos := strings.Split(config.RepositoryList, ",")
	for _, repo := range repos {
		if strings.TrimSpace(repo) == repoFullName {
			return true
		}
	}
	
	return false
} 
