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
	CreatorSlackID      string // PR作成者のSlack ID
	Status              string // "pending", "in_review", "paused", "archived", "done", "waiting_business_hours"
	LabelName           string
	WatchingUntil       *time.Time
	ReminderPausedUntil *time.Time
	OutOfHoursReminded  bool // 営業時間外に自動でリマインドを一時停止したかのフラグ
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DeletedAt           gorm.DeletedAt `gorm:"index"`
}
