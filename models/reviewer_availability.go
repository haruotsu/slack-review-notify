package models

import (
	"time"

	"gorm.io/gorm"
)

type ReviewerAvailability struct {
	ID          string         `gorm:"primaryKey"`
	SlackUserID string         `gorm:"index"`
	AwayUntil   *time.Time     // nil の場合は無期限休暇
	Reason      string         // 休暇理由（任意）
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}
