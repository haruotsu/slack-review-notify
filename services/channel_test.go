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
	
	// テスト用のチャンネル設定を作成
	configs := []models.ChannelConfig{
		{
			ID:               "config-id-1",
			SlackChannelID:   "C12345",
			DefaultMentionID: "U12345",
			RepositoryList:   "owner/repo1",
			LabelName:        "needs-review",
			IsActive:         true,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		},
		{
			ID:               "config-id-2",
			SlackChannelID:   "C67890",
			DefaultMentionID: "U67890",
			RepositoryList:   "owner/repo2",
			LabelName:        "needs-review",
			IsActive:         true,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		},
	}
	
	for _, config := range configs {
		db.Create(&config)
	}
	
	// チャンネル情報取得のモック (1つ目のチャンネルはアクティブ)
	gock.New("https://slack.com").
		Get("/api/conversations.info").
		MatchParam("channel", "C12345").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
			"channel": map[string]interface{}{
				"id":          "C12345",
				"is_archived": false,
			},
		})
	
	// チャンネル情報取得のモック (2つ目のチャンネルはアーカイブ済み)
	gock.New("https://slack.com").
		Get("/api/conversations.info").
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
	
	// DBが更新されたことを確認
	var config1 models.ChannelConfig
	db.Where("slack_channel_id = ?", "C12345").First(&config1)
	assert.True(t, config1.IsActive, "アクティブなチャンネルは有効のままであるべき")
	
	var config2 models.ChannelConfig
	db.Where("slack_channel_id = ?", "C67890").First(&config2)
	assert.False(t, config2.IsActive, "アーカイブされたチャンネルは無効になるべき")
	
	assert.True(t, gock.IsDone(), "すべてのモックが使用されていません")
	
	// APIエラーのケースもテスト
	gock.New("https://slack.com").
		Get("/api/conversations.info").
		MatchParam("channel", "C12345").
		Reply(200).
		JSON(map[string]interface{}{
			"ok":    false,
			"error": "channel_not_found",
		})
	
	// 関数を再度実行 (エラーが適切に処理されることを確認)
	CleanupArchivedChannels(db)
	
	// APIエラーがあっても処理が継続することを確認
	assert.True(t, gock.IsDone(), "すべてのモックが使用されていません")
} 
