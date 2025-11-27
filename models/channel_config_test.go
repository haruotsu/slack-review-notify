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

	// マイグレーションを実行
	if err := db.AutoMigrate(&ChannelConfig{}); err != nil {
		t.Fatalf("fail to migrate test db: %v", err)
	}

	return db
}

func TestChannelConfig_BusinessHoursFields(t *testing.T) {
	db := setupChannelConfigTestDB(t)

	// 営業時間フィールドを持つChannelConfigを作成
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

	// データベースに保存
	err := db.Create(&config).Error
	assert.NoError(t, err)

	// データベースから読み取り
	var savedConfig ChannelConfig
	err = db.Where("id = ?", "test-config").First(&savedConfig).Error
	assert.NoError(t, err)

	// 営業時間フィールドが正しく保存・読み取りされていることを確認
	assert.Equal(t, "09:00", savedConfig.BusinessHoursStart)
	assert.Equal(t, "18:00", savedConfig.BusinessHoursEnd)
}

func TestChannelConfig_DefaultBusinessHours(t *testing.T) {
	db := setupChannelConfigTestDB(t)

	// 営業時間を指定しないChannelConfigを作成
	config := ChannelConfig{
		ID:               "test-config-default",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U12345",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	// データベースに保存
	err := db.Create(&config).Error
	assert.NoError(t, err)

	// データベースから読み取り
	var savedConfig ChannelConfig
	err = db.Where("id = ?", "test-config-default").First(&savedConfig).Error
	assert.NoError(t, err)

	// デフォルト値が設定されていることを確認
	assert.Equal(t, "09:00", savedConfig.BusinessHoursStart)
	assert.Equal(t, "18:00", savedConfig.BusinessHoursEnd)
}
