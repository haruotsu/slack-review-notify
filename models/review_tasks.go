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
	Reviewers           string // Comma-separated: Slack IDs of all assigned reviewers
	ApprovedBy          string // Comma-separated: Slack IDs of reviewers who approved
	PRAuthorSlackID     string // Slack ID of the PR author (used for excluding from reviewers)
	Status              string // "pending", "in_review", "paused", "archived", "done", "waiting_business_hours"
	LabelName           string
	WatchingUntil       *time.Time
	ReminderPausedUntil *time.Time
	OutOfHoursReminded  bool   // Flag indicating whether reminders were automatically paused outside business hours
	Language            string // Language for messages (copied from ChannelConfig)
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DeletedAt           gorm.DeletedAt `gorm:"index"`
}
