package models

import (
	"time"

	"gorm.io/gorm"
)

type ReviewerAvailability struct {
	ID string `gorm:"primaryKey"`
	// A user can have multiple leave periods at once (e.g. a pre-booked
	// vacation plus an unexpected sick day), so this is a non-unique index.
	SlackUserID string     `gorm:"index"`
	AwayFrom    *time.Time // If nil, away starts immediately
	AwayUntil   *time.Time // If nil, the user is away indefinitely
	Reason      string     // Reason for being away (optional)
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

const reviewerAvailabilitySlackUserIndex = "idx_reviewer_availabilities_slack_user_id"

// MigrateReviewerAvailabilityIndex relaxes the slack_user_id index from UNIQUE
// to non-unique on databases created by older schemas.
//
// SlackUserID used to carry a uniqueIndex tag, which allowed only one leave
// record per user. Now that a reviewer can hold multiple periods at once,
// the column must be a plain index. AutoMigrate does not alter the uniqueness
// of an existing index, so an old database keeps the UNIQUE constraint and
// rejects the second period with "UNIQUE constraint failed". This drops the
// stale unique index and recreates it as a non-unique one. It is idempotent:
// it does nothing once the index is already non-unique (including fresh DBs).
func MigrateReviewerAvailabilityIndex(db *gorm.DB) error {
	if !db.Migrator().HasTable(&ReviewerAvailability{}) {
		return nil
	}

	var indexes []struct {
		Name   string
		Unique int
	}
	if err := db.Raw("PRAGMA index_list(reviewer_availabilities)").Scan(&indexes).Error; err != nil {
		return err
	}

	for _, idx := range indexes {
		if idx.Name == reviewerAvailabilitySlackUserIndex && idx.Unique == 1 {
			if err := db.Migrator().DropIndex(&ReviewerAvailability{}, "SlackUserID"); err != nil {
				return err
			}
			return db.Migrator().CreateIndex(&ReviewerAvailability{}, "SlackUserID")
		}
	}
	return nil
}
