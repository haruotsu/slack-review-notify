package models

import (
	"time"

	"gorm.io/gorm"
)

type ReviewTask struct {
	ID                 string `gorm:"primaryKey"`
	PRURL              string
	Repo               string
	PRNumber           int
	Title              string
	SlackTS            string
	SlackChannel       string
	Reviewer           string
	Status             string     // "pending", "in_review", "paused", "done"
	WatchingUntil      *time.Time // nullable
	ReminderPausedUntil *time.Time // リマインダー一時停止期限
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          gorm.DeletedAt `gorm:"index"`
}
