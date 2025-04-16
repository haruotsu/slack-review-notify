package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"slack-review-notify/models"
	"time"

	"gorm.io/gorm"
)

// アーカイブされたチャンネルの設定を非アクティブにする
func CleanupArchivedChannels(db *gorm.DB) {
	var configs []models.ChannelConfig
	db.Where("is_active = ?", true).Find(&configs)
	
	for _, config := range configs {
		isArchived, err := IsChannelArchived(config.SlackChannelID)
		if err != nil {
			log.Printf("channel status check error (channel: %s): %v", config.SlackChannelID, err)
			continue
		}
		
		if isArchived {
			log.Printf("channel %s is archived", config.SlackChannelID)
			
			// 非アクティブに更新
			config.IsActive = false
			config.UpdatedAt = time.Now()
			if err := db.Save(&config).Error; err != nil {
				log.Printf("channel config update error: %v", err)
			} else {
				log.Printf("channel %s config is deactivated", config.SlackChannelID)
			}
		}
	}
}

// IsChannelArchived はチャンネルがアーカイブされているかどうかを確認します
func IsChannelArchived(channelID string) (bool, error) {
	body := map[string]interface{}{
		"channel": channelID,
	}

	jsonData, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", "https://slack.com/api/conversations.info", bytes.NewBuffer(jsonData))
	if err != nil {
		return false, err
	}

	req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	var result struct {
		OK      bool `json:"ok"`
		Channel struct {
			IsArchived bool `json:"is_archived"`
		} `json:"channel"`
		Error string `json:"error"`
	}
	
	bodyBytes, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return false, fmt.Errorf("slack API response parse error: %v", err)
	}

	if !result.OK {
		return false, fmt.Errorf("slack error: %s", result.Error)
	}

	return result.Channel.IsArchived, nil
} 
