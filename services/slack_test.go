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

func TestSendSlackMessage(t *testing.T) {
	// テスト前の環境変数を保存し、テスト後に復元
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer os.Setenv("SLACK_BOT_TOKEN", originalToken)
	
	// テスト用の環境変数を設定
	os.Setenv("SLACK_BOT_TOKEN", "test-token")
	
	// モックの設定
	defer gock.Off() // テスト終了時にモックをクリア
	
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
	
	// アーカイブされたチャンネルのモック
	gock.New("https://slack.com").
		Get("/api/conversations.info").
		MatchParam("channel", "C12345").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
			"channel": map[string]interface{}{
				"id":          "C12345",
				"is_archived": true,
			},
		})
	
	// 関数を実行
	isArchived, err := IsChannelArchived("C12345")
	
	// アサーション
	assert.NoError(t, err)
	assert.True(t, isArchived)
	assert.True(t, gock.IsDone(), "すべてのモックが使用されていません")
	
	// アーカイブされていないチャンネルのモック
	gock.New("https://slack.com").
		Get("/api/conversations.info").
		MatchParam("channel", "C67890").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
			"channel": map[string]interface{}{
				"id":          "C67890",
				"is_archived": false,
			},
		})
	
	// 関数を実行
	isArchived, err = IsChannelArchived("C67890")
	
	// アサーション
	assert.NoError(t, err)
	assert.False(t, isArchived)
	assert.True(t, gock.IsDone(), "すべてのモックが使用されていません")
	
	// 存在しないチャンネルのモック
	gock.New("https://slack.com").
		Get("/api/conversations.info").
		MatchParam("channel", "INVALID").
		Reply(200).
		JSON(map[string]interface{}{
			"ok":    false,
			"error": "channel_not_found",
		})
	
	// 関数を実行
	isArchived, err = IsChannelArchived("INVALID")
	
	// アサーション
	assert.True(t, isArchived) // チャンネルが存在しない場合もアーカイブされているとみなす
	assert.NoError(t, err)     // エラーではなく、単に結果がtrueになる
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
		Reviewer:     "U12345",
		Status:       "in_review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	
	// 関数を実行
	err := SendReviewerReminderMessage(db, task)
	
	// アサーション
	assert.NoError(t, err)
	assert.True(t, gock.IsDone(), "すべてのモックが使用されていません")
	
	// チャンネルがアーカイブされている場合のテスト
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
		Reviewer:     "U67890",
		Status:       "in_review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	
	config := models.ChannelConfig{
		ID:               "config-id",
		SlackChannelID:   "C67890",
		DefaultMentionID: "U12345",
		RepositoryList:   "owner/repo",
		LabelName:        "needs-review",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	
	db.Create(&config)
	
	// 関数を実行
	err = SendReviewerReminderMessage(db, task2)
	
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

func TestSendReminderPausedMessage(t *testing.T) {
	// テスト前の環境変数を保存し、テスト後に復元
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer os.Setenv("SLACK_BOT_TOKEN", originalToken)
	
	// テスト用の環境変数を設定
	os.Setenv("SLACK_BOT_TOKEN", "test-token")
	
	// モックの設定
	defer gock.Off() // テスト終了時にモックをクリア
	
	testCases := []struct {
		name     string
		duration string
		message  string
	}{
		{"1時間", "1h", "はい！1時間リマインドをストップします！"},
		{"2時間", "2h", "はい！2時間リマインドをストップします！"},
		{"4時間", "4h", "はい！4時間リマインドをストップします！"},
		{"今日", "today", "今日はもうリマインドしません。翌営業日の朝に再開します！"},
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
	testCases := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"not_in_channel", fmt.Errorf("slack error: not_in_channel"), true},
		{"channel_not_found", fmt.Errorf("slack error: channel_not_found"), true},
		{"is_archived", fmt.Errorf("slack error: is_archived"), true},
		{"missing_scope", fmt.Errorf("slack error: missing_scope"), true},
		{"other error", fmt.Errorf("other error"), false},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := IsChannelRelatedError(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}


// GetNextBusinessDayMorning関数のテスト
func TestGetNextBusinessDayMorning(t *testing.T) {
	result := GetNextBusinessDayMorning()
	
	// 結果は必ず翌日以降の日付
	now := time.Now()
	tomorrow := now.AddDate(0, 0, 1)
	
	// 以下をテスト
	// 1. 結果は現在時刻より後
	assert.True(t, result.After(now), "翌営業日は現在時刻より後であること")
	
	// 2. 結果は10:00に設定されている
	assert.Equal(t, 10, result.Hour(), "時刻は10時に設定されていること")
	assert.Equal(t, 0, result.Minute(), "分は0分に設定されていること")
	assert.Equal(t, 0, result.Second(), "秒は0秒に設定されていること")
	
	// 3. 土日に実行した場合は月曜日になる
	switch now.Weekday() {
	case time.Friday:
		// 金曜日に実行した場合、結果は月曜日（3日後）
		expectedDate := tomorrow.AddDate(0, 0, 2)
		assert.Equal(t, time.Monday, result.Weekday(), "金曜日の翌営業日は月曜日")
		assert.Equal(t, expectedDate.Year(), result.Year())
		assert.Equal(t, expectedDate.Month(), result.Month()) 
		assert.Equal(t, expectedDate.Day(), result.Day())
	case time.Saturday:
		// 土曜日に実行した場合、結果は月曜日（2日後）
		expectedDate := tomorrow.AddDate(0, 0, 1)
		assert.Equal(t, time.Monday, result.Weekday(), "土曜日の翌営業日は月曜日")
		assert.Equal(t, expectedDate.Year(), result.Year())
		assert.Equal(t, expectedDate.Month(), result.Month())
		assert.Equal(t, expectedDate.Day(), result.Day())
	case time.Sunday:
		// 日曜日に実行した場合、結果は月曜日（1日後）
		assert.Equal(t, time.Monday, result.Weekday(), "日曜日の翌営業日は月曜日")
		assert.Equal(t, tomorrow.Year(), result.Year())
		assert.Equal(t, tomorrow.Month(), result.Month()) 
		assert.Equal(t, tomorrow.Day(), result.Day())
	default:
		// 平日に実行した場合、結果は翌日
		assert.Equal(t, tomorrow.Year(), result.Year())
		assert.Equal(t, tomorrow.Month(), result.Month())
		assert.Equal(t, tomorrow.Day(), result.Day())
		
		// 翌日が平日であること（月〜金）
		expectedWeekday := (now.Weekday() + 1) % 7
		assert.Equal(t, expectedWeekday, result.Weekday(), "平日の翌営業日は翌日")
	}
}