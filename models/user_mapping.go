package models

import (
	"time"

	"gorm.io/gorm"
)

// UserMapping holds the mapping between GitHub username and Slack User ID
type UserMapping struct {
	ID             string `gorm:"primaryKey"`
	GithubUsername string `gorm:"uniqueIndex"` // GitHub username
	SlackUserID    string // Slack user ID
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}
