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
