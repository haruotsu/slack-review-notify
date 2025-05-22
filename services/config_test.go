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

func TestGetChannelConfigByLabel(t *testing.T) {
	db := setupTestDB(t)

	// テストデータ作成
	testConfig1 := models.ChannelConfig{
		ID:               "test-id-1",
		SlackChannelID:   "C12345",
		DefaultMentionID: "U12345",
		RepositoryList:   "owner/repo1,owner/repo2",
		LabelName:        "needs-review",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	testConfig2 := models.ChannelConfig{
		ID:               "test-id-2",
		SlackChannelID:   "C12345",
		DefaultMentionID: "U12345",
		RepositoryList:   "owner/repo3",
		LabelName:        "priority-high",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	db.Create(&testConfig1)
	db.Create(&testConfig2)

	// テスト実行
	config, err := GetChannelConfigByLabel(db, "C12345", "priority-high")

	// アサーション
	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, "C12345", config.SlackChannelID)
	assert.Equal(t, "priority-high", config.LabelName)
	assert.Equal(t, "owner/repo3", config.RepositoryList)
	assert.True(t, config.IsActive)

	// 存在しないラベルのテスト
	_, err = GetChannelConfigByLabel(db, "C12345", "nonexistent-label")
	assert.Error(t, err)
}

func TestGetAllChannelConfigs(t *testing.T) {
	db := setupTestDB(t)

	// テストデータ作成
	testConfig1 := models.ChannelConfig{
		ID:               "test-id-1",
		SlackChannelID:   "C12345",
		DefaultMentionID: "U12345",
		RepositoryList:   "owner/repo1,owner/repo2",
		LabelName:        "needs-review",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	testConfig2 := models.ChannelConfig{
		ID:               "test-id-2",
		SlackChannelID:   "C12345",
		DefaultMentionID: "U12345",
		RepositoryList:   "owner/repo3",
		LabelName:        "priority-high",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	testConfig3 := models.ChannelConfig{
		ID:               "test-id-3",
		SlackChannelID:   "C12345",
		DefaultMentionID: "U67890",
		RepositoryList:   "owner/repo4",
		LabelName:        "inactive-label",
		IsActive:         false,  // 非アクティブな設定
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	db.Create(&testConfig1)
	db.Create(&testConfig2)
	db.Create(&testConfig3)

	// テスト実行
	configs, err := GetAllChannelConfigs(db, "C12345")

	// アサーション
	assert.NoError(t, err)
	assert.Equal(t, 2, len(configs), "アクティブな設定のみ取得される")

	// ラベル名でソートされていないため、どちらが先に来るかは保証されない
	labelNames := []string{configs[0].LabelName, configs[1].LabelName}
	assert.Contains(t, labelNames, "needs-review")
	assert.Contains(t, labelNames, "priority-high")

	// 存在しないチャンネルIDのテスト
	configs, err = GetAllChannelConfigs(db, "nonexistent")
	assert.NoError(t, err)
	assert.Equal(t, 0, len(configs), "存在しないチャンネルの場合は空配列が返る")
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

func TestHasChannelConfigWithLabel(t *testing.T) {
	db := setupTestDB(t)

	// テストデータ作成
	testConfig1 := models.ChannelConfig{
		ID:               "test-id-1",
		SlackChannelID:   "C12345",
		DefaultMentionID: "U12345",
		RepositoryList:   "owner/repo1,owner/repo2",
		LabelName:        "needs-review",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	testConfig2 := models.ChannelConfig{
		ID:               "test-id-2",
		SlackChannelID:   "C12345",
		DefaultMentionID: "U12345",
		RepositoryList:   "owner/repo3",
		LabelName:        "priority-high",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	testConfig3 := models.ChannelConfig{
		ID:               "test-id-3",
		SlackChannelID:   "C12345",
		DefaultMentionID: "U67890",
		RepositoryList:   "owner/repo4",
		LabelName:        "inactive-label",
		IsActive:         false,  // 非アクティブな設定
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	db.Create(&testConfig1)
	db.Create(&testConfig2)
	db.Create(&testConfig3)

	// テスト実行とアサーション
	assert.True(t, HasChannelConfigWithLabel(db, "C12345", "needs-review"))
	assert.True(t, HasChannelConfigWithLabel(db, "C12345", "priority-high"))
	assert.False(t, HasChannelConfigWithLabel(db, "C12345", "inactive-label"), "非アクティブな設定はfalseを返す")
	assert.False(t, HasChannelConfigWithLabel(db, "C12345", "nonexistent-label"))
	assert.False(t, HasChannelConfigWithLabel(db, "nonexistent", "needs-review"))
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
