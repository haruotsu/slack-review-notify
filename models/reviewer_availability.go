package models

import (
	"time"

	"gorm.io/gorm"
)

type ReviewerAvailability struct {
	ID          string         `gorm:"primaryKey"`
	SlackUserID string         `gorm:"uniqueIndex"`
	AwayFrom    *time.Time     // If nil, away starts immediately
	AwayUntil   *time.Time     // If nil, the user is away indefinitely
	Reason      string         // Reason for being away (optional)
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}
