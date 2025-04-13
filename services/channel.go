package services

import (
	"log"
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
			log.Printf("チャンネル状態確認エラー（チャンネル: %s）: %v", config.SlackChannelID, err)
			continue
		}
		
		if isArchived {
			log.Printf("⚠️ チャンネル %s はアーカイブされています", config.SlackChannelID)
			
			// 非アクティブに更新
			config.IsActive = false
			config.UpdatedAt = time.Now()
			if err := db.Save(&config).Error; err != nil {
				log.Printf("チャンネル設定更新エラー: %v", err)
			} else {
				log.Printf("✅ アーカイブされたチャンネル %s の設定を非アクティブにしました", config.SlackChannelID)
			}
		}
	}
} 
