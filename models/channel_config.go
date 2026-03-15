package models

import (
	"time"

	"gorm.io/gorm"
)

type ChannelConfig struct {
	ID                       string `gorm:"primaryKey"`
	SlackChannelID           string `gorm:"index:idx_channel_label,unique:true"` // Composite unique index on channel ID and label name
	LabelName                string `gorm:"index:idx_channel_label,unique:true"` // Label name to trigger notifications
	DefaultMentionID         string // Default mention target (user ID)
	ReviewerList             string // Reviewer list (comma-separated)
	RepositoryList           string // List of repositories to notify for (comma-separated)
	IsActive                 bool   // Active/inactive flag
	ReminderInterval         int    // Reminder frequency (in minutes, default 30 minutes)
	ReviewerReminderInterval int    // Reminder frequency after reviewer assignment (in minutes, default 30 minutes)
	RequiredApprovals        int    `gorm:"default:1"` // Required number of approvals (default: 1)
	BusinessHoursStart       string `gorm:"default:'09:00'"`      // Business hours start (HH:MM format)
	BusinessHoursEnd         string `gorm:"default:'18:00'"`      // Business hours end (HH:MM format)
	Timezone                 string `gorm:"default:'Asia/Tokyo'"` // Timezone (default: JST)
	Language                 string `gorm:"default:'ja'"`         // Language for messages (ja, en)
	CreatedAt                time.Time
	UpdatedAt                time.Time
	DeletedAt                gorm.DeletedAt `gorm:"index"`
}
