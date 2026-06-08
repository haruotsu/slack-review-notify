package models

import (
	"log"
	"strings"
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
			// Drop and recreate in one transaction so a failure cannot leave
			// the table without any slack_user_id index.
			return db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Migrator().DropIndex(&ReviewerAvailability{}, "SlackUserID"); err != nil {
					return err
				}
				return tx.Migrator().CreateIndex(&ReviewerAvailability{}, "SlackUserID")
			})
		}
	}
	return nil
}

// MigrateNormalizeSlackUserIDs normalizes legacy slack_user_id values that
// contain a pipe-separated display name (e.g. "UABC123|username"). These were
// stored by an older version of cleanUserID that did not strip the
// "|displayname" suffix from Slack's autocomplete-escape format
// (<@UABC123|username>). After this migration every record stores only the
// bare Slack user ID so that exact-match queries in set-away/unset-away work
// correctly regardless of which code version created the record.
//
// The whole pass runs in a single transaction so a mid-loop failure cannot
// leave the table half-normalized, matching MigrateReviewerAvailabilityIndex's
// transactional style. Only rows containing a pipe are loaded, so once
// normalization is complete a later startup scans zero rows instead of the
// whole table.
func MigrateNormalizeSlackUserIDs(db *gorm.DB) error {
	if !db.Migrator().HasTable(&ReviewerAvailability{}) {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var records []ReviewerAvailability
		if err := tx.Unscoped().Where("slack_user_id LIKE ?", "%|%").Find(&records).Error; err != nil {
			return err
		}

		var normalized int
		for _, r := range records {
			idx := strings.Index(r.SlackUserID, "|")
			// idx <= 0 would yield an empty id (a leading pipe is corrupt data
			// that no real code path produces), so leave such rows untouched.
			if idx <= 0 {
				continue
			}
			cleanID := r.SlackUserID[:idx]

			// Dedup: if an identical (clean id, same period) row already exists,
			// normalizing this legacy row would create a duplicate (user, period)
			// pair. Drop the legacy row instead so the "one row per (user, period)"
			// invariant survives. NULL-aware so indefinite periods match correctly.
			var dup int64
			dupQuery := MatchPeriod(
				tx.Unscoped().Model(&ReviewerAvailability{}).Where("id <> ? AND slack_user_id = ?", r.ID, cleanID),
				r.AwayFrom, r.AwayUntil,
			)
			if err := dupQuery.Count(&dup).Error; err != nil {
				return err
			}

			if dup > 0 {
				if err := tx.Unscoped().Delete(&ReviewerAvailability{}, "id = ?", r.ID).Error; err != nil {
					return err
				}
			} else if err := tx.Unscoped().Model(&ReviewerAvailability{}).
				Where("id = ?", r.ID).Update("slack_user_id", cleanID).Error; err != nil {
				return err
			}
			normalized++
		}

		if normalized > 0 {
			log.Printf("normalized %d legacy slack_user_id value(s)", normalized)
		}
		return nil
	})
}

// MatchPeriod narrows a query to rows whose away_from/away_until exactly match
// the given bounds, treating nil as a NULL column (so an indefinite period
// matches only indefinite rows). Shared by the set-away/unset-away handlers and
// the normalization migration so period matching stays consistent everywhere.
func MatchPeriod(q *gorm.DB, from, until *time.Time) *gorm.DB {
	if from == nil {
		q = q.Where("away_from IS NULL")
	} else {
		q = q.Where("away_from = ?", from)
	}
	if until == nil {
		q = q.Where("away_until IS NULL")
	} else {
		q = q.Where("away_until = ?", until)
	}
	return q
}
