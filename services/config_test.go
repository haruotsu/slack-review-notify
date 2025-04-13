package services

import (
	"slack-review-notify/models"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("fail to open test db: %v", err)
	}
	
	// マイグレーションを実行
	if err := db.AutoMigrate(&models.ChannelConfig{}, &models.ReviewTask{}); err != nil {
		t.Fatalf("fail to migrate test db: %v", err)
	}
	
	return db
}

func TestGetChannelConfig(t *testing.T) {
	db := setupTestDB(t)
	
	// テストデータ作成
	testConfig := models.ChannelConfig{
		ID:               "test-id",
		SlackChannelID:   "C12345",
		DefaultMentionID: "U12345",
		RepositoryList:   "owner/repo1,owner/repo2",
		LabelName:        "needs-review",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	
	db.Create(&testConfig)
	
	// テスト実行
	config, err := GetChannelConfig(db, "C12345")
	
	// アサーション
	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, "C12345", config.SlackChannelID)
	assert.Equal(t, "U12345", config.DefaultMentionID)
	assert.Equal(t, "needs-review", config.LabelName)
	assert.True(t, config.IsActive)
	
	// 存在しないチャンネルIDのテスト
	_, err = GetChannelConfig(db, "nonexistent")
	assert.Error(t, err)
}

func TestHasChannelConfig(t *testing.T) {
	db := setupTestDB(t)
	
	// テストデータ作成
	testConfig := models.ChannelConfig{
		ID:               "test-id",
		SlackChannelID:   "C12345",
		DefaultMentionID: "U12345",
		RepositoryList:   "owner/repo1,owner/repo2",
		LabelName:        "needs-review",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	
	db.Create(&testConfig)
	
	// テスト実行とアサーション
	assert.True(t, HasChannelConfig(db, "C12345"))
	assert.False(t, HasChannelConfig(db, "nonexistent"))
}

func TestIsRepositoryWatched(t *testing.T) {
	// テスト用のconfig構造体を作成
	config := &models.ChannelConfig{
		SlackChannelID: "C12345",
		RepositoryList: "owner/repo1,owner/repo2, owner/repo3",
	}
	
	// テストケース
	testCases := []struct {
		repoName string
		expected bool
	}{
		{"owner/repo1", true},
		{"owner/repo2", true},
		{"owner/repo3", true},
		{"owner/repo4", false},
		{"different/repo", false},
	}
	
	for _, tc := range testCases {
		t.Run(tc.repoName, func(t *testing.T) {
			result := IsRepositoryWatched(config, tc.repoName)
			assert.Equal(t, tc.expected, result)
		})
	}
	
	// 空のリポジトリリストのテスト
	emptyConfig := &models.ChannelConfig{
		SlackChannelID: "C12345",
		RepositoryList: "",
	}
	assert.False(t, IsRepositoryWatched(emptyConfig, "owner/repo1"))
	
	// nilのconfigのテスト
	assert.False(t, IsRepositoryWatched(nil, "owner/repo1"))
}
