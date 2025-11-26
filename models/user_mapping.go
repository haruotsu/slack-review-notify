package models

import (
	"time"

	"gorm.io/gorm"
)

// UserMapping は GitHub username と Slack User ID のマッピングを保持する
type UserMapping struct {
	ID             string `gorm:"primaryKey"`
	GithubUsername string `gorm:"uniqueIndex"` // GitHub のユーザー名
	SlackUserID    string                      // Slack のユーザーID
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}
