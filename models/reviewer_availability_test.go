package models

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// legacyReviewerAvailability replicates the pre-migration schema where
// SlackUserID carried a uniqueIndex tag. It maps to the same table so we can
// reproduce a database created by an older version of the app.
type legacyReviewerAvailability struct {
	ID          string `gorm:"primaryKey"`
	SlackUserID string `gorm:"uniqueIndex"`
	AwayFrom    *time.Time
	AwayUntil   *time.Time
	Reason      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

func (legacyReviewerAvailability) TableName() string { return "reviewer_availabilities" }

func openSilent(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	return db
}

// TestMigrateReviewerAvailabilityIndex_LegacyDB reproduces the original bug:
// a database created with the old uniqueIndex schema rejects a second leave
// period for the same user even after AutoMigrate. The migration must relax
// the index so multiple periods can coexist.
func TestMigrateReviewerAvailabilityIndex_LegacyDB(t *testing.T) {
	dbPath := t.TempDir() + "/legacy.db"

	// Create the DB with the old uniqueIndex schema, then close it.
	dbOld := openSilent(t, dbPath)
	if err := dbOld.AutoMigrate(&legacyReviewerAvailability{}); err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}
	sqlOld, _ := dbOld.DB()
	sqlOld.Close()

	// Reopen and apply the current migrations.
	db := openSilent(t, dbPath)
	if err := db.AutoMigrate(&ReviewerAvailability{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	if err := MigrateReviewerAvailabilityIndex(db); err != nil {
		t.Fatalf("migrate index: %v", err)
	}

	now := time.Now()
	later := now.Add(24 * time.Hour)
	r1 := ReviewerAvailability{ID: uuid.NewString(), SlackUserID: "U123", AwayFrom: &now, AwayUntil: &later}
	r2 := ReviewerAvailability{ID: uuid.NewString(), SlackUserID: "U123", AwayFrom: &later, AwayUntil: &later}
	if err := db.Create(&r1).Error; err != nil {
		t.Fatalf("create first period: %v", err)
	}
	if err := db.Create(&r2).Error; err != nil {
		t.Fatalf("create second period (legacy unique index not relaxed): %v", err)
	}

	var count int64
	db.Model(&ReviewerAvailability{}).Where("slack_user_id = ?", "U123").Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 periods, got %d", count)
	}
}

// TestMigrateNormalizeSlackUserIDs verifies that legacy "ID|displayname" values
// are stripped to the bare ID, and that records with clean IDs are untouched.
func TestMigrateNormalizeSlackUserIDs(t *testing.T) {
	dbPath := t.TempDir() + "/normalize.db"
	db := openSilent(t, dbPath)
	if err := db.AutoMigrate(&ReviewerAvailability{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	// Insert one legacy record and one already-clean record.
	legacy := ReviewerAvailability{ID: uuid.NewString(), SlackUserID: "UABC123|username", Reason: "old"}
	clean := ReviewerAvailability{ID: uuid.NewString(), SlackUserID: "UDEF456", Reason: "new"}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatalf("create legacy: %v", err)
	}
	if err := db.Create(&clean).Error; err != nil {
		t.Fatalf("create clean: %v", err)
	}

	if err := MigrateNormalizeSlackUserIDs(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var got ReviewerAvailability
	if err := db.First(&got, "id = ?", legacy.ID).Error; err != nil {
		t.Fatalf("fetch legacy: %v", err)
	}
	if got.SlackUserID != "UABC123" {
		t.Errorf("legacy record: want SlackUserID %q, got %q", "UABC123", got.SlackUserID)
	}

	var gotClean ReviewerAvailability
	if err := db.First(&gotClean, "id = ?", clean.ID).Error; err != nil {
		t.Fatalf("fetch clean: %v", err)
	}
	if gotClean.SlackUserID != "UDEF456" {
		t.Errorf("clean record should be unchanged: want %q, got %q", "UDEF456", gotClean.SlackUserID)
	}
}

// TestMigrateNormalizeSlackUserIDs_Dedup verifies that when a user already has
// a clean-ID record for the same period, normalizing the legacy "ID|name" row
// drops it instead of creating a duplicate (user, period) pair.
func TestMigrateNormalizeSlackUserIDs_Dedup(t *testing.T) {
	dbPath := t.TempDir() + "/dedup.db"
	db := openSilent(t, dbPath)
	if err := db.AutoMigrate(&ReviewerAvailability{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	// Same user, same (indefinite) period: one legacy row and one clean row.
	legacy := ReviewerAvailability{ID: uuid.NewString(), SlackUserID: "UDUP|username", Reason: "legacy"}
	clean := ReviewerAvailability{ID: uuid.NewString(), SlackUserID: "UDUP", Reason: "clean"}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatalf("create legacy: %v", err)
	}
	if err := db.Create(&clean).Error; err != nil {
		t.Fatalf("create clean: %v", err)
	}

	if err := MigrateNormalizeSlackUserIDs(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// Exactly one row must remain for UDUP — the legacy row was deduped away,
	// not normalized into a second identical (user, period) row.
	var count int64
	db.Model(&ReviewerAvailability{}).Where("slack_user_id = ?", "UDUP").Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 row after dedup, got %d", count)
	}
	// The surviving row is the pre-existing clean one.
	var got ReviewerAvailability
	if err := db.First(&got, "slack_user_id = ?", "UDUP").Error; err != nil {
		t.Fatalf("fetch survivor: %v", err)
	}
	if got.ID != clean.ID {
		t.Errorf("dedup should keep the existing clean row %q, kept %q", clean.ID, got.ID)
	}
}

// TestMigrateNormalizeSlackUserIDs_Idempotent confirms the normalization is a
// no-op when re-run (it runs on every startup), matching the index migration's
// idempotency guarantee.
func TestMigrateNormalizeSlackUserIDs_Idempotent(t *testing.T) {
	dbPath := t.TempDir() + "/normalize-idem.db"
	db := openSilent(t, dbPath)
	if err := db.AutoMigrate(&ReviewerAvailability{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	legacy := ReviewerAvailability{ID: uuid.NewString(), SlackUserID: "UIDEM|username"}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatalf("create legacy: %v", err)
	}

	for i := 0; i < 2; i++ {
		if err := MigrateNormalizeSlackUserIDs(db); err != nil {
			t.Fatalf("migrate (run %d): %v", i, err)
		}
		var got ReviewerAvailability
		if err := db.First(&got, "id = ?", legacy.ID).Error; err != nil {
			t.Fatalf("fetch after run %d: %v", i, err)
		}
		if got.SlackUserID != "UIDEM" {
			t.Errorf("run %d: want %q, got %q", i, "UIDEM", got.SlackUserID)
		}
	}
}

// TestMigrateNormalizeSlackUserIDs_MultipleAndMultiPipe verifies that every
// legacy row in a single pass is normalized, and that a value with multiple
// pipes is truncated at the first pipe.
func TestMigrateNormalizeSlackUserIDs_MultipleAndMultiPipe(t *testing.T) {
	dbPath := t.TempDir() + "/normalize-multi.db"
	db := openSilent(t, dbPath)
	if err := db.AutoMigrate(&ReviewerAvailability{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	rows := map[string]string{ // id -> legacy slack_user_id
		uuid.NewString(): "UAAA|alice",
		uuid.NewString(): "UBBB|bob",
		uuid.NewString(): "UCCC|name|extra", // multiple pipes -> truncate at first
	}
	want := map[string]string{ // same id -> expected normalized id
		"UAAA|alice":      "UAAA",
		"UBBB|bob":        "UBBB",
		"UCCC|name|extra": "UCCC",
	}
	ids := map[string]string{} // id -> original legacy value
	for id, sid := range rows {
		if err := db.Create(&ReviewerAvailability{ID: id, SlackUserID: sid}).Error; err != nil {
			t.Fatalf("create %s: %v", sid, err)
		}
		ids[id] = sid
	}

	if err := MigrateNormalizeSlackUserIDs(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	for id, original := range ids {
		var got ReviewerAvailability
		if err := db.First(&got, "id = ?", id).Error; err != nil {
			t.Fatalf("fetch %s: %v", id, err)
		}
		if got.SlackUserID != want[original] {
			t.Errorf("%q: want %q, got %q", original, want[original], got.SlackUserID)
		}
	}
}

// TestMigrateReviewerAvailabilityIndex_Idempotent confirms the migration is a
// no-op on a fresh DB (already non-unique) and safe to run repeatedly.
func TestMigrateReviewerAvailabilityIndex_Idempotent(t *testing.T) {
	dbPath := t.TempDir() + "/fresh.db"
	db := openSilent(t, dbPath)
	if err := db.AutoMigrate(&ReviewerAvailability{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := MigrateReviewerAvailabilityIndex(db); err != nil {
			t.Fatalf("migrate index (run %d): %v", i, err)
		}
	}

	now := time.Now()
	r1 := ReviewerAvailability{ID: uuid.NewString(), SlackUserID: "U999", AwayFrom: &now}
	r2 := ReviewerAvailability{ID: uuid.NewString(), SlackUserID: "U999", AwayFrom: &now}
	if err := db.Create(&r1).Error; err != nil {
		t.Fatalf("create r1: %v", err)
	}
	if err := db.Create(&r2).Error; err != nil {
		t.Fatalf("create r2: %v", err)
	}

	var count int64
	db.Model(&ReviewerAvailability{}).Where("slack_user_id = ?", "U999").Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 rows after idempotent migration, got %d", count)
	}
}
