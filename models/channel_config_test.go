package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupChannelConfigTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("fail to open test db: %v", err)
	}

	// Run migration
	if err := db.AutoMigrate(&ChannelConfig{}); err != nil {
		t.Fatalf("fail to migrate test db: %v", err)
	}

	return db
}

func TestChannelConfig_BusinessHoursFields(t *testing.T) {
	db := setupChannelConfigTestDB(t)

	// Create a ChannelConfig with business hours fields
	config := ChannelConfig{
		ID:                 "test-config",
		SlackChannelID:     "C12345",
		LabelName:          "needs-review",
		DefaultMentionID:   "U12345",
		IsActive:           true,
		BusinessHoursStart: "09:00",
		BusinessHoursEnd:   "18:00",
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	// Save to database
	err := db.Create(&config).Error
	assert.NoError(t, err)

	// Read from database
	var savedConfig ChannelConfig
	err = db.Where("id = ?", "test-config").First(&savedConfig).Error
	assert.NoError(t, err)

	// Verify that business hours fields are correctly saved and retrieved
	assert.Equal(t, "09:00", savedConfig.BusinessHoursStart)
	assert.Equal(t, "18:00", savedConfig.BusinessHoursEnd)
}

func TestChannelConfig_DefaultBusinessHours(t *testing.T) {
	db := setupChannelConfigTestDB(t)

	// Create a ChannelConfig without specifying business hours
	config := ChannelConfig{
		ID:               "test-config-default",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U12345",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	// Save to database
	err := db.Create(&config).Error
	assert.NoError(t, err)

	// Read from database
	var savedConfig ChannelConfig
	err = db.Where("id = ?", "test-config-default").First(&savedConfig).Error
	assert.NoError(t, err)

	// Verify that default values are set
	assert.Equal(t, "09:00", savedConfig.BusinessHoursStart)
	assert.Equal(t, "18:00", savedConfig.BusinessHoursEnd)
}
