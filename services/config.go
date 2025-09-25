package services

import (
	"log"
	"slack-review-notify/models"
	"strings"

	"github.com/google/go-github/v71/github"
	"gorm.io/gorm"
)

// チャンネル設定を取得する関数
func GetChannelConfig(db *gorm.DB, channelID string, labelName string) (*models.ChannelConfig, error) {
	var config models.ChannelConfig

	err := db.Where("slack_channel_id = ? AND label_name = ? AND is_active = ?", channelID, labelName, true).First(&config).Error
	if err != nil {
		return nil, err
	}

	return &config, nil
}

// チャンネル設定が存在するか確認する関数
func HasChannelConfig(db *gorm.DB, channelID string, labelName string) bool {
	var count int64
	db.Model(&models.ChannelConfig{}).Where("slack_channel_id = ? AND label_name = ? AND is_active = ?", channelID, labelName, true).Count(&count)
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

// ラベルが設定条件にマッチするかチェックする関数
func IsLabelMatched(config *models.ChannelConfig, prLabels []*github.Label) bool {
	if config == nil {
		log.Printf("channel config is nil")
		return false
	}

	// 空文字列の場合はマッチしない
	if config.LabelName == "" {
		log.Printf("no label configured for channel")
		return false
	}

	// PRのラベル名をセットに変換
	prLabelNames := make(map[string]bool)
	for _, label := range prLabels {
		if label.Name != nil {
			prLabelNames[*label.Name] = true
		}
	}

	// 設定されたラベル（カンマ区切り）を分割
	requiredLabels := strings.Split(config.LabelName, ",")

	// 全ての必要なラベルがPRに存在するかチェック（AND条件）
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

// GetMissingLabels は設定で必要なラベルのうち、PRに存在しないラベルを返す
func GetMissingLabels(config *models.ChannelConfig, prLabels []*github.Label) []string {
	if config == nil || config.LabelName == "" {
		return []string{}
	}

	// PRのラベル名をセットに変換
	prLabelNames := make(map[string]bool)
	for _, label := range prLabels {
		if label.Name != nil {
			prLabelNames[*label.Name] = true
		}
	}

	// 設定されたラベル（カンマ区切り）を分割
	requiredLabels := strings.Split(config.LabelName, ",")
	missingLabels := make([]string, 0)

	// 存在しない必要なラベルを収集
	for _, label := range requiredLabels {
		trimmedLabel := strings.TrimSpace(label)
		if trimmedLabel != "" && !prLabelNames[trimmedLabel] {
			missingLabels = append(missingLabels, trimmedLabel)
		}
	}

	return missingLabels
}
