package models

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// insertWithRawOffset writes a row the way the pre-UTC code did: go-sqlite3
// formats the bound time.Time with that value's own offset, so passing a JST
// value stores "...+09:00" TEXT. GORM's model API is bypassed on purpose so the
// BeforeSave hook cannot normalize the value and the legacy encoding survives.
func insertWithRawOffset(t *testing.T, db *gorm.DB, id, userID string, from, until time.Time) {
	t.Helper()
	err := db.Exec(
		"INSERT INTO reviewer_availabilities (id, slack_user_id, away_from, away_until, reason, created_at, updated_at) "+
			"VALUES (?, ?, ?, ?, '', ?, ?)",
		id, userID, from, until, from, from,
	).Error
	if err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
}

// storedText returns the raw TEXT SQLite holds for a column, or "" when the
// column is NULL. The CAST keeps go-sqlite3 from converting a `datetime` column
// back into a time.Time, which would hide the very offset this test is about.
func storedText(t *testing.T, db *gorm.DB, id, column string) string {
	t.Helper()
	var got sql.NullString
	if err := db.Raw("SELECT CAST("+column+" AS TEXT) FROM reviewer_availabilities WHERE id = ?", id).
		Scan(&got).Error; err != nil {
		t.Fatalf("read %s: %v", column, err)
	}
	return got.String
}

// TestReviewerAvailabilityStoredInUTC pins the storage encoding the rest of the
// leave feature depends on: whatever timezone a period is registered in, the
// row must be written with a "+00:00" offset so SQLite's lexicographic TEXT
// comparison stays chronological.
func TestReviewerAvailabilityStoredInUTC(t *testing.T) {
	db := openSilent(t, t.TempDir()+"/utc.db")
	if err := db.AutoMigrate(&ReviewerAvailability{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	jst := time.FixedZone("JST", 9*60*60)
	from := time.Date(2099, 8, 5, 6, 0, 0, 0, jst)
	until := time.Date(2099, 8, 5, 14, 0, 0, 0, jst)
	rec := ReviewerAvailability{ID: uuid.NewString(), SlackUserID: "UUTC", AwayFrom: &from, AwayUntil: &until}
	if err := db.Create(&rec).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	if got, want := storedText(t, db, rec.ID, "away_from"), "2099-08-04 21:00:00+00:00"; got != want {
		t.Errorf("away_from stored as %q, want %q", got, want)
	}
	if got, want := storedText(t, db, rec.ID, "away_until"), "2099-08-05 05:00:00+00:00"; got != want {
		t.Errorf("away_until stored as %q, want %q", got, want)
	}

	// The caller's own values must survive untouched: setAway renders its
	// confirmation from them, so the hook must not rewrite the pointees.
	if from.Hour() != 6 || from.Location() != jst {
		t.Errorf("caller's from was mutated: %v", from)
	}
	if until.Hour() != 14 || until.Location() != jst {
		t.Errorf("caller's until was mutated: %v", until)
	}
}

// TestMigrateAvailabilityTimestampsToUTC covers the upgrade path: rows written
// by an older version carry the channel's offset, which no longer matches the
// UTC values the code now binds. After the migration the instant is unchanged
// but the encoding is UTC, so a UTC-bound query finds the row again.
func TestMigrateAvailabilityTimestampsToUTC(t *testing.T) {
	db := openSilent(t, t.TempDir()+"/utc-migrate.db")
	if err := db.AutoMigrate(&ReviewerAvailability{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	jst := time.FixedZone("JST", 9*60*60)
	legacyFrom := time.Date(2099, 8, 5, 6, 0, 0, 0, jst)
	legacyUntil := time.Date(2099, 8, 5, 14, 0, 0, 0, jst)
	legacyID := uuid.NewString()
	insertWithRawOffset(t, db, legacyID, "U_LEGACY", legacyFrom, legacyUntil)

	if got := storedText(t, db, legacyID, "away_from"); got != "2099-08-05 06:00:00+09:00" {
		t.Fatalf("precondition: legacy row should carry +09:00, got %q", got)
	}

	// A row already written by the current code must be left alone.
	utcFrom := time.Date(2099, 9, 1, 0, 0, 0, 0, time.UTC)
	utcUntil := time.Date(2099, 9, 2, 0, 0, 0, 0, time.UTC)
	current := ReviewerAvailability{ID: uuid.NewString(), SlackUserID: "U_CURRENT", AwayFrom: &utcFrom, AwayUntil: &utcUntil}
	if err := db.Create(&current).Error; err != nil {
		t.Fatalf("create current: %v", err)
	}

	// An indefinite period (both bounds NULL) must survive the pass.
	indefinite := ReviewerAvailability{ID: uuid.NewString(), SlackUserID: "U_INDEF"}
	if err := db.Create(&indefinite).Error; err != nil {
		t.Fatalf("create indefinite: %v", err)
	}

	if err := MigrateAvailabilityTimestampsToUTC(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if got, want := storedText(t, db, legacyID, "away_from"), "2099-08-04 21:00:00+00:00"; got != want {
		t.Errorf("legacy away_from: got %q, want %q", got, want)
	}
	if got, want := storedText(t, db, legacyID, "away_until"), "2099-08-05 05:00:00+00:00"; got != want {
		t.Errorf("legacy away_until: got %q, want %q", got, want)
	}

	// Same instant, new encoding.
	var migrated ReviewerAvailability
	if err := db.First(&migrated, "id = ?", legacyID).Error; err != nil {
		t.Fatalf("fetch migrated: %v", err)
	}
	if !migrated.AwayFrom.Equal(legacyFrom) || !migrated.AwayUntil.Equal(legacyUntil) {
		t.Errorf("migration changed the instant: got %v..%v, want %v..%v",
			migrated.AwayFrom, migrated.AwayUntil, legacyFrom, legacyUntil)
	}

	// The point of the migration: a UTC-bound "is this user away now?" query
	// must see the legacy row. Before the rewrite this returns nothing.
	during := time.Date(2099, 8, 5, 10, 0, 0, 0, jst).UTC()
	var ids []string
	if err := db.Model(&ReviewerAvailability{}).
		Where("(away_from IS NULL OR away_from <= ?) AND (away_until IS NULL OR away_until > ?)", during, during).
		Pluck("slack_user_id", &ids).Error; err != nil {
		t.Fatalf("query away users: %v", err)
	}
	if len(ids) != 2 || (ids[0] != "U_LEGACY" && ids[1] != "U_LEGACY") {
		t.Errorf("migrated legacy row must match a UTC-bound query, got %v", ids)
	}

	if got, want := storedText(t, db, current.ID, "away_from"), "2099-09-01 00:00:00+00:00"; got != want {
		t.Errorf("already-UTC row should be untouched: got %q, want %q", got, want)
	}
	if got := storedText(t, db, indefinite.ID, "away_from"); got != "" {
		t.Errorf("indefinite row should keep NULL bounds, got %q", got)
	}
}

// TestMigrateAvailabilityTimestampsToUTC_Idempotent verifies a second run is a
// no-op: the filter matches nothing once every offset is "+00:00", which is
// what keeps this migration cheap on every startup.
func TestMigrateAvailabilityTimestampsToUTC_Idempotent(t *testing.T) {
	db := openSilent(t, t.TempDir()+"/utc-idempotent.db")
	if err := db.AutoMigrate(&ReviewerAvailability{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	jst := time.FixedZone("JST", 9*60*60)
	id := uuid.NewString()
	insertWithRawOffset(t, db, id, "U_LEGACY",
		time.Date(2099, 8, 5, 6, 0, 0, 0, jst), time.Date(2099, 8, 5, 14, 0, 0, 0, jst))

	for i := range 2 {
		if err := MigrateAvailabilityTimestampsToUTC(db); err != nil {
			t.Fatalf("migrate run %d: %v", i+1, err)
		}
	}

	if got, want := storedText(t, db, id, "away_from"), "2099-08-04 21:00:00+00:00"; got != want {
		t.Errorf("after two runs: got %q, want %q", got, want)
	}

	var remaining int64
	if err := db.Model(&ReviewerAvailability{}).Where(nonUTCAvailabilityBounds).Count(&remaining).Error; err != nil {
		t.Fatalf("count non-UTC rows: %v", err)
	}
	if remaining != 0 {
		t.Errorf("a completed migration must scan zero rows, got %d", remaining)
	}
}

// TestMigrationOrder_UTCBeforeSlackUserIDs covers the interaction between the
// two data migrations on a database that predates both: legacy
// "ID|displayname" rows whose periods are still stored with the channel's
// offset.
//
// MigrateNormalizeSlackUserIDs deduplicates through MatchPeriod, which binds
// UTC. Against not-yet-migrated "+09:00" rows that comparison never matches, so
// the legacy row would be renamed into a duplicate of the clean row instead of
// being dropped. Running the UTC pass first (as main.go does) keeps the "one
// row per (user, period)" invariant.
func TestMigrationOrder_UTCBeforeSlackUserIDs(t *testing.T) {
	db := openSilent(t, t.TempDir()+"/utc-order.db")
	if err := db.AutoMigrate(&ReviewerAvailability{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	jst := time.FixedZone("JST", 9*60*60)
	from := time.Date(2099, 8, 5, 0, 0, 0, 0, jst)
	until := time.Date(2099, 8, 5, 23, 59, 59, 0, jst)
	insertWithRawOffset(t, db, uuid.NewString(), "UDUP|username", from, until)
	insertWithRawOffset(t, db, uuid.NewString(), "UDUP", from, until)

	if err := MigrateAvailabilityTimestampsToUTC(db); err != nil {
		t.Fatalf("utc migrate: %v", err)
	}
	if err := MigrateNormalizeSlackUserIDs(db); err != nil {
		t.Fatalf("slack user id migrate: %v", err)
	}

	var rows []ReviewerAvailability
	if err := db.Find(&rows).Error; err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("legacy duplicate should have been dropped, got %d rows", len(rows))
	}
	if rows[0].SlackUserID != "UDUP" {
		t.Errorf("surviving row: want SlackUserID %q, got %q", "UDUP", rows[0].SlackUserID)
	}
}

// TestMatchPeriodBindsUTC verifies the read side of the same invariant: the
// exact-period lookup used by set-away/unset-away must find a stored row even
// when the caller passes the period in the channel's local timezone.
func TestMatchPeriodBindsUTC(t *testing.T) {
	db := openSilent(t, t.TempDir()+"/match.db")
	if err := db.AutoMigrate(&ReviewerAvailability{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	jst := time.FixedZone("JST", 9*60*60)
	from := time.Date(2099, 8, 5, 6, 0, 0, 0, jst)
	until := time.Date(2099, 8, 5, 14, 0, 0, 0, jst)
	if err := db.Create(&ReviewerAvailability{
		ID: uuid.NewString(), SlackUserID: "UMATCH", AwayFrom: &from, AwayUntil: &until,
	}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var count int64
	if err := MatchPeriod(db.Model(&ReviewerAvailability{}).Where("slack_user_id = ?", "UMATCH"), &from, &until).
		Count(&count).Error; err != nil {
		t.Fatalf("match: %v", err)
	}
	if count != 1 {
		t.Errorf("local-timezone bounds must match the stored UTC row, got %d rows", count)
	}
}
