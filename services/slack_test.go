package services

import (
	"fmt"
	"os"
	"slack-review-notify/models"
	"testing"
	"time"

	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
)

// MockSlackMessage はSlackメッセージのモックを表す構造体
type MockSlackMessage struct {
	Channel string
	TS      string
	Message string
}

// モックメッセージを保持する変数
var mockSlackMessage *MockSlackMessage

// SetMockSlackMessage はSlackメッセージのモックを設定する関数
func SetMockSlackMessage(channel, ts, message string) {
	mockSlackMessage = &MockSlackMessage{
		Channel: channel,
		TS:      ts,
		Message: message,
	}
}

// GetMockSlackMessage は設定されたモックメッセージを取得する関数
func GetMockSlackMessage() *MockSlackMessage {
	return mockSlackMessage
}

// ClearMockSlackMessage はモックメッセージをクリアする関数
func ClearMockSlackMessage() {
	mockSlackMessage = nil
}

func TestSendSlackMessage(t *testing.T) {
	// テスト前の環境変数を保存し、テスト後に復元
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer os.Setenv("SLACK_BOT_TOKEN", originalToken)
	
	// テスト用の環境変数を設定
	os.Setenv("SLACK_BOT_TOKEN", "test-token")
	
	// モックの設定
	defer gock.Off() // テスト終了時にモックをクリア
	defer ClearMockSlackMessage() // モックメッセージをクリア
	
	// 成功ケースのモック
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		MatchHeader("Content-Type", "application/json").
		Reply(200).
		JSON(map[string]interface{}{
			"ok":      true,
			"channel": "C12345",
			"ts":      "1234.5678",
		})
	
	// 関数を実行
	ts, channel, err := SendSlackMessage(
		"https://github.com/owner/repo/pull/1",
		"Test PR Title",
		"C12345",
		"U12345",
	)
	
	// アサーション
	assert.NoError(t, err)
	assert.Equal(t, "1234.5678", ts)
	assert.Equal(t, "C12345", channel)
	assert.True(t, gock.IsDone(), "すべてのモックが使用されていません")
	
	// エラーケースのテスト
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{
			"ok":    false,
			"error": "channel_not_found",
		})
	
	// 関数を実行
	_, _, err = SendSlackMessage(
		"https://github.com/owner/repo/pull/1",
		"Test PR Title",
		"INVALID",
		"U12345",
	)
	
	// アサーション
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "channel_not_found")
	assert.True(t, gock.IsDone(), "すべてのモックが使用されていません")
}

func TestPostToThread(t *testing.T) {
	// テスト前の環境変数を保存し、テスト後に復元
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer os.Setenv("SLACK_BOT_TOKEN", originalToken)
	
	// テスト用の環境変数を設定
	os.Setenv("SLACK_BOT_TOKEN", "test-token")
	
	// モックの設定
	defer gock.Off() // テスト終了時にモックをクリア
	defer ClearMockSlackMessage() // モックメッセージをクリア
	
	// 成功ケースのモック
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		MatchHeader("Content-Type", "application/json").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
		})
	
	// 関数を実行
	err := PostToThread("C12345", "1234.5678", "テストメッセージ")
	
	// アサーション
	assert.NoError(t, err)
	assert.True(t, gock.IsDone(), "すべてのモックが使用されていません")
	
	// エラーケースのテスト
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{
			"ok":    false,
			"error": "invalid_thread_ts",
		})
	
	// 関数を実行
	err = PostToThread("C12345", "invalid", "テストメッセージ")
	
	// アサーション
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid_thread_ts")
	assert.True(t, gock.IsDone(), "すべてのモックが使用されていません")
}

func TestIsChannelArchived(t *testing.T) {
	// テスト前の環境変数を保存し、テスト後に復元
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer os.Setenv("SLACK_BOT_TOKEN", originalToken)
	
	// テスト用の環境変数を設定
	os.Setenv("SLACK_BOT_TOKEN", "test-token")
	
	// モックの設定
	defer gock.Off() // テスト終了時にモックをクリア
	defer ClearMockSlackMessage() // モックメッセージをクリア
	
	// チャンネル情報取得のモック
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
	
	// チャンネルがアーカイブされていないことを確認
	isArchived, err := IsChannelArchived("C12345")
	assert.NoError(t, err)
	assert.False(t, isArchived)
	assert.True(t, gock.IsDone(), "すべてのモックが使用されていません")
	
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
	
	// チャンネルがアーカイブされていることを確認
	isArchived, err = IsChannelArchived("C67890")
	assert.NoError(t, err)
	assert.True(t, isArchived)
	assert.True(t, gock.IsDone(), "すべてのモックが使用されていません")
	
	// エラーの場合のモック
	gock.New("https://slack.com").
		Post("/api/conversations.info").
		MatchParam("channel", "C99999").
		Reply(200).
		JSON(map[string]interface{}{
			"ok":    false,
			"error": "channel_not_found",
		})
	
	// エラーが返されることを確認
	isArchived, err = IsChannelArchived("C99999")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "channel_not_found")
	assert.True(t, gock.IsDone(), "すべてのモックが使用されていません")
}

func TestSendReminderMessage(t *testing.T) {
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
	
	// チャンネル情報取得のモック
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
	
	// メッセージ送信のモック
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
		})
	
	// テスト用のタスクを作成
	task := models.ReviewTask{
		ID:           "test-id",
		PRURL:        "https://github.com/owner/repo/pull/1",
		Repo:         "owner/repo",
		PRNumber:     1,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Status:       "pending",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	
	// モックメッセージを設定
	SetMockSlackMessage("C12345", "1234.5678", "⏰ レビューをお願いします！\n<https://github.com/owner/repo/pull/1|Test PR>")
	
	// 関数を実行
	err := SendReminderMessage(db, task)
	
	// アサーション
	assert.NoError(t, err)
	assert.True(t, gock.IsDone(), "すべてのモックが使用されていません")
	
	// アーカイブされたチャンネルの場合
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
	
	// テスト用のタスクとチャンネル設定を作成
	task2 := models.ReviewTask{
		ID:           "test-id-2",
		PRURL:        "https://github.com/owner/repo/pull/2",
		Repo:         "owner/repo",
		PRNumber:     2,
		Title:        "Test PR 2",
		SlackTS:      "1234.5679",
		SlackChannel: "C67890",
		Status:       "pending",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	
	config := models.ChannelConfig{
		ID:              "config-id",
		SlackChannelID:  "C67890",
		DefaultMentionID: "U12345",
		RepositoryList:  "owner/repo",
		LabelName:       "needs-review",
		IsActive:        true,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	
	db.Create(&config)
	
	// 関数を実行
	err = SendReminderMessage(db, task2)
	
	// アサーション
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "channel is archived")
	
	// DBが更新されたことを確認
	var updatedTask models.ReviewTask
	db.Where("id = ?", "test-id-2").First(&updatedTask)
	assert.Equal(t, "archived", updatedTask.Status)
	
	var updatedConfig models.ChannelConfig
	db.Where("slack_channel_id = ?", "C67890").First(&updatedConfig)
	assert.False(t, updatedConfig.IsActive)
	
	assert.True(t, gock.IsDone(), "すべてのモックが使用されていません")
}

func TestSendReviewerAssignedMessage(t *testing.T) {
	// テスト前の環境変数を保存し、テスト後に復元
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer os.Setenv("SLACK_BOT_TOKEN", originalToken)
	
	// テスト用の環境変数を設定
	os.Setenv("SLACK_BOT_TOKEN", "test-token")
	
	// モックの設定
	defer gock.Off() // テスト終了時にモックをクリア
	defer ClearMockSlackMessage() // モックメッセージをクリア
	
	// スレッドメッセージ送信のモック
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		MatchHeader("Content-Type", "application/json").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
		})
	
	// テスト用のタスクを作成
	task := models.ReviewTask{
		ID:           "test-id",
		PRURL:        "https://github.com/owner/repo/pull/1",
		Repo:         "owner/repo",
		PRNumber:     1,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Reviewer:     "U12345",
		Status:       "in_review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	
	// モックメッセージを設定
	SetMockSlackMessage("C12345", "1234.5678", "🤖 レビュアーリストからランダムに選ばれた <@U12345> さんが担当になりました！よろしくお願いします！")
	
	// 関数を実行
	err := SendReviewerAssignedMessage(task)
	
	// アサーション
	assert.NoError(t, err)
	assert.True(t, gock.IsDone(), "すべてのモックが使用されていません")
}

func TestSendReviewerReminderMessage(t *testing.T) {
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
	
	// チャンネル情報取得のモック
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
	
	// メッセージ送信のモック
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
		})
	
	// テスト用のタスクを作成
	task := models.ReviewTask{
		ID:           "test-id",
		PRURL:        "https://github.com/owner/repo/pull/1",
		Repo:         "owner/repo",
		PRNumber:     1,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Reviewer:     "U12345",
		Status:       "in_review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	
	// モックメッセージを設定
	SetMockSlackMessage("C12345", "1234.5678", "⏰ <@U12345> さん、レビューをお願いします！\n<https://github.com/owner/repo/pull/1|Test PR>")
	
	// 関数を実行
	err := SendReviewerReminderMessage(db, task)
	
	// アサーション
	assert.NoError(t, err)
	assert.True(t, gock.IsDone(), "すべてのモックが使用されていません")
	
	// アーカイブされたチャンネルのテスト
	gock.New("https://slack.com").
		Post("/api/conversations.info").
		MatchParam("channel", "C12345").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
			"channel": map[string]interface{}{
				"id":          "C12345",
				"is_archived": true,
			},
		})
	
	// アーカイブされたチャンネルにメッセージを送信しようとする
	err = SendReviewerReminderMessage(db, task)
	
	// エラーが返されることを確認
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "channel is archived")
	
	// タスクのステータスが更新されていることを確認
	var updatedTask models.ReviewTask
	db.First(&updatedTask, "id = ?", task.ID)
	assert.Equal(t, "archived", updatedTask.Status)
	
	// チャンネル設定が非アクティブになっていることを確認
	var config models.ChannelConfig
	db.First(&config, "slack_channel_id = ?", task.SlackChannel)
	assert.False(t, config.IsActive)
	
	assert.True(t, gock.IsDone(), "すべてのモックが使用されていません")
}

func TestSendReminderPausedMessage(t *testing.T) {
	// テスト前の環境変数を保存し、テスト後に復元
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer os.Setenv("SLACK_BOT_TOKEN", originalToken)
	
	// テスト用の環境変数を設定
	os.Setenv("SLACK_BOT_TOKEN", "test-token")
	
	// モックの設定
	defer gock.Off() // テスト終了時にモックをクリア
	defer ClearMockSlackMessage() // モックメッセージをクリア
	
	testCases := []struct {
		name     string
		duration string
		message  string
	}{
		{"1時間", "1h", "はい！1時間リマインドをストップします！"},
		{"2時間", "2h", "はい！2時間リマインドをストップします！"},
		{"4時間", "4h", "はい！4時間リマインドをストップします！"},
		{"今日", "today", "今日はもうリマインドしません。24時間後に再開します！"},
		{"完全停止", "stop", "リマインダーを完全に停止しました。レビュー担当者が決まるまで通知しません。"},
		{"デフォルト", "unknown", "リマインドをストップします！"},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// スレッドメッセージ送信のモック
			gock.New("https://slack.com").
				Post("/api/chat.postMessage").
				MatchHeader("Authorization", "Bearer test-token").
				Reply(200).
				JSON(map[string]interface{}{
					"ok": true,
				})
			
			// テスト用のタスクを作成
			task := models.ReviewTask{
				ID:           "test-id",
				SlackTS:      "1234.5678",
				SlackChannel: "C12345",
				Status:       "pending",
			}
			
			// 関数を実行
			err := SendReminderPausedMessage(task, tc.duration)
			
			// アサーション
			assert.NoError(t, err)
			assert.True(t, gock.IsDone(), "すべてのモックが使用されていません")
		})
	}
}

// IsChannelRelatedErrorのテスト
func TestIsChannelRelatedError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		want    bool
	}{
		{
			name:    "channel_not_found",
			err:     fmt.Errorf("slack error: channel_not_found"),
			want:    true,
		},
		{
			name:    "not_in_channel",
			err:     fmt.Errorf("slack error: not_in_channel"),
			want:    true,
		},
		{
			name:    "is_archived",
			err:     fmt.Errorf("slack error: is_archived"),
			want:    true,
		},
		{
			name:    "channel is archived",
			err:     fmt.Errorf("slack error: channel is archived"),
			want:    true,
		},
		{
			name:    "not accessible",
			err:     fmt.Errorf("slack error: not accessible"),
			want:    true,
		},
		{
			name:    "missing_scope",
			err:     fmt.Errorf("slack error: missing_scope"),
			want:    true,
		},
		{
			name:    "other error",
			err:     fmt.Errorf("slack error: other error"),
			want:    false,
		},
		{
			name:    "nil error",
			err:     nil,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsChannelRelatedError(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}
