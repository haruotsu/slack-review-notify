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

// UTCTime returns a copy of t normalized to UTC, or nil when t is nil. The
// pointee is never mutated: callers keep their own local-time value for
// rendering while only the stored/bound representation changes.
//
// Why every stored and compared timestamp must be UTC: go-sqlite3 binds a
// time.Time as TEXT formatted with *that value's own* offset
// ("2006-01-02 15:04:05.999999999-07:00"), and away_from/away_until are
// declared datetime (NUMERIC affinity), so SQLite keeps the TEXT and compares
// it lexicographically. A row written as "2026-08-05 06:00:00+09:00" therefore
// fails to match a bind of "2026-08-05 01:00:00+00:00" even though both denote
// the same instant — which is exactly what happened when a channel's
// set-timezone differed from the process TZ. Once every offset is "+00:00",
// lexicographic order equals chronological order again.
func UTCTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}

// InLocation returns a copy of t rendered in loc, or nil when t is nil. This is
// the read-side counterpart of UTCTime: values come back from SQLite in UTC and
// must be converted to the channel's timezone before being formatted for Slack.
func InLocation(t *time.Time, loc *time.Location) *time.Time {
	if t == nil {
		return nil
	}
	local := t.In(loc)
	return &local
}

// BeforeSave normalizes the leave bounds to UTC on every GORM write path
// (Create/Save/Updates on the struct), so no write site has to remember to do
// it. See UTCTime for why UTC is mandatory.
func (r *ReviewerAvailability) BeforeSave(*gorm.DB) error {
	r.AwayFrom = UTCTime(r.AwayFrom)
	r.AwayUntil = UTCTime(r.AwayUntil)
	return nil
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
// Bounds are bound as UTC because that is how they are stored (see UTCTime);
// binding a local-offset value would never match.
func MatchPeriod(q *gorm.DB, from, until *time.Time) *gorm.DB {
	if from == nil {
		q = q.Where("away_from IS NULL")
	} else {
		q = q.Where("away_from = ?", UTCTime(from))
	}
	if until == nil {
		q = q.Where("away_until IS NULL")
	} else {
		q = q.Where("away_until = ?", UTCTime(until))
	}
	return q
}

// nonUTCAvailabilityBounds matches rows whose away_from/away_until TEXT was
// written with an offset other than UTC. go-sqlite3 renders a UTC value's
// offset as the literal "+00:00" (the "-07:00" layout element never emits "Z"),
// so the suffix test is exact rather than heuristic.
const nonUTCAvailabilityBounds = "(away_from IS NOT NULL AND away_from NOT LIKE '%+00:00') OR " +
	"(away_until IS NOT NULL AND away_until NOT LIKE '%+00:00')"

// MigrateAvailabilityTimestampsToUTC rewrites leave bounds that an older
// version stored with a non-UTC offset (the channel's set-timezone, e.g.
// "+09:00") into the UTC representation the code now writes and binds.
//
// Without this pass a database would hold both representations at once, and
// SQLite's lexicographic TEXT comparison breaks across that boundary: a
// "+09:00" row is invisible to a "+00:00" bind, so a legacy leave period would
// stop excluding its reviewer and set-away would insert a duplicate row instead
// of updating the existing one. The instant each row denotes is unchanged —
// only its textual encoding is.
//
// The whole pass runs in one transaction so a mid-loop failure cannot leave the
// table half-normalized, matching MigrateNormalizeSlackUserIDs' style. Only
// non-UTC rows are loaded, so once normalization is complete a later startup
// scans zero rows. Columns are written with UpdateColumns so this
// representation-only change does not bump updated_at.
func MigrateAvailabilityTimestampsToUTC(db *gorm.DB) error {
	if !db.Migrator().HasTable(&ReviewerAvailability{}) {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var records []ReviewerAvailability
		if err := tx.Unscoped().Where(nonUTCAvailabilityBounds).Find(&records).Error; err != nil {
			return err
		}

		for _, r := range records {
			if err := tx.Unscoped().Model(&ReviewerAvailability{}).
				Where("id = ?", r.ID).
				UpdateColumns(map[string]any{
					"away_from":  UTCTime(r.AwayFrom),
					"away_until": UTCTime(r.AwayUntil),
				}).Error; err != nil {
				return err
			}
		}

		if len(records) > 0 {
			log.Printf("normalized %d leave period(s) to UTC", len(records))
		}
		return nil
	})
}
