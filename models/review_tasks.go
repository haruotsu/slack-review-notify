package models

import (
	"time"

	"gorm.io/gorm"
)

type ReviewTask struct {
	ID            string `gorm:"primaryKey"`
	PRURL         string
	Repo          string
	PRNumber      int
	Title         string
	SlackTS       string
	SlackChannel  string
	Reviewer      string
	Status        string     // "pending", "done", "watching"
	WatchingUntil *time.Time // nullable
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     gorm.DeletedAt `gorm:"index"`
}
