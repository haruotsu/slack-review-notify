package services

import (
	"slack-review-notify/models"
	"testing"
	"time"

	"os"

	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
)

func TestCleanupArchivedChannels(t *testing.T) {
	// テスト用DBのセットアップ
	db := setupTestDB(t)
	
	// テスト前の環境変数を保存し、テスト後に復元
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer os.Setenv("SLACK_BOT_TOKEN", originalToken)
	
	// テスト用の環境変数を設定
	os.Setenv("SLACK_BOT_TOKEN", "test-token")
	
	// モックの設定
	defer gock.Off() // テスト終了時にモックをクリア
	defer ClearMockSlackMessage() // モックメッセージをクリア
	
	// アクティブなチャンネル設定を作成
	activeConfig := models.ChannelConfig{
		ID:               "active-config",
		SlackChannelID:   "C12345",
		DefaultMentionID: "U12345",
		RepositoryList:   "owner/repo",
		LabelName:        "needs-review",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	
	// アーカイブされたチャンネル設定を作成
	archivedConfig := models.ChannelConfig{
		ID:               "archived-config",
		SlackChannelID:   "C67890",
		DefaultMentionID: "U67890",
		RepositoryList:   "owner/repo",
		LabelName:        "needs-review",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	
	db.Create(&activeConfig)
	db.Create(&archivedConfig)
	
	// アクティブなチャンネルのモック
	gock.New("https://slack.com").
		Post("/api/conversations.info").
		MatchParam("channel", "C12345").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
			"channel": map[string]interface{}{
				"id":          "C12345",
				"is_archived": false,
			},
		})
	
	// アーカイブされたチャンネルのモック
	gock.New("https://slack.com").
		Post("/api/conversations.info").
		MatchParam("channel", "C67890").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
			"channel": map[string]interface{}{
				"id":          "C67890",
				"is_archived": true,
			},
		})
	
	// 関数を実行
	CleanupArchivedChannels(db)
	
	// アクティブなチャンネル設定が変更されていないことを確認
	var updatedActiveConfig models.ChannelConfig
	db.First(&updatedActiveConfig, "id = ?", activeConfig.ID)
	assert.True(t, updatedActiveConfig.IsActive)
	
	// アーカイブされたチャンネル設定が非アクティブになっていることを確認
	var updatedArchivedConfig models.ChannelConfig
	db.First(&updatedArchivedConfig, "id = ?", archivedConfig.ID)
	assert.False(t, updatedArchivedConfig.IsActive)
	
	assert.True(t, gock.IsDone(), "すべてのモックが使用されていません")
} 
