package models

import (
	"time"

	"gorm.io/gorm"
)

type ReviewTask struct {
	ID                  string `gorm:"primaryKey"`
	PRURL               string
	Repo                string
	PRNumber            int
	Title               string
	SlackTS             string
	SlackChannel        string
	Reviewer            string
	Status              string // "pending", "in_review", "paused", "archived", "done"
	LabelName           string
	WatchingUntil       *time.Time
	ReminderPausedUntil *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DeletedAt           gorm.DeletedAt `gorm:"index"`
}
