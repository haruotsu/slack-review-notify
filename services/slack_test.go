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
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()

	// テスト用の環境変数を設定
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

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
		"", // PR作成者のSlack ID (テストでは空)
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
		"", // PR作成者のSlack ID (テストでは空)
	)

	// アサーション
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "channel_not_found")
	assert.True(t, gock.IsDone(), "すべてのモックが使用されていません")
}

func TestPostToThread(t *testing.T) {
	// テスト前の環境変数を保存し、テスト後に復元
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()

	// テスト用の環境変数を設定
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

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
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()

	// テスト用の環境変数を設定
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

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
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()

	// テスト用の環境変数を設定
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

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
	err := SendReviewerReminderMessage(db, task)

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

func TestSendReviewerReminderMessage(t *testing.T) {
	// テスト用DBのセットアップ
	db := setupTestDB(t)

	// テスト前の環境変数を保存し、テスト後に復元
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()

	// テスト用の環境変数を設定
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

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
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()

	// テスト用の環境変数を設定
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

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
	// JST タイムゾーンを定義
	jst, _ := time.LoadLocation("Asia/Tokyo")

	testCases := []struct {
		name     string
		baseTime time.Time
		expected time.Time
	}{
		{
			name:     "月曜日朝9時_当日10時を期待",
			baseTime: time.Date(2024, 1, 8, 9, 0, 0, 0, jst),  // 月曜日 9:00 JST
			expected: time.Date(2024, 1, 8, 10, 0, 0, 0, jst), // 月曜日 10:00 JST
		},
		{
			name:     "月曜日午後2時_火曜日10時を期待",
			baseTime: time.Date(2024, 1, 8, 14, 0, 0, 0, jst), // 月曜日 14:00 JST
			expected: time.Date(2024, 1, 9, 10, 0, 0, 0, jst), // 火曜日 10:00 JST
		},
		{
			name:     "金曜日午後2時_月曜日10時を期待",
			baseTime: time.Date(2024, 1, 12, 14, 0, 0, 0, jst), // 金曜日 14:00 JST
			expected: time.Date(2024, 1, 15, 10, 0, 0, 0, jst), // 月曜日 10:00 JST
		},
		{
			name:     "土曜日午後2時_月曜日10時を期待",
			baseTime: time.Date(2024, 1, 13, 14, 0, 0, 0, jst), // 土曜日 14:00 JST
			expected: time.Date(2024, 1, 15, 10, 0, 0, 0, jst), // 月曜日 10:00 JST
		},
		{
			name:     "日曜日午後2時_月曜日10時を期待",
			baseTime: time.Date(2024, 1, 14, 14, 0, 0, 0, jst), // 日曜日 14:00 JST
			expected: time.Date(2024, 1, 15, 10, 0, 0, 0, jst), // 月曜日 10:00 JST
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GetNextBusinessDayMorningWithConfig(tc.baseTime, nil)
			assert.Equal(t, tc.expected, result)
		})
	}

	result := GetNextBusinessDayMorningWithConfig(time.Now(), nil)

	// 結果は10:00に設定されている
	assert.Equal(t, 10, result.Hour(), "時刻は10時に設定されていること")
	assert.Equal(t, 0, result.Minute(), "分は0分に設定されていること")
	assert.Equal(t, 0, result.Second(), "秒は0秒に設定されていること")

	// 現在時刻以降であることのチェック
	assert.True(t, result.After(time.Now().Add(-time.Second)), "結果は現在時刻以降であること")
}

func TestSelectRandomReviewer(t *testing.T) {
	db := setupTestDB(t)

	// テストデータ作成
	testConfig := models.ChannelConfig{
		ID:               "test-id",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U12345",
		ReviewerList:     "U23456,U34567",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	db.Create(&testConfig)

	// 関数を実行
	reviewerID := SelectRandomReviewer(db, "C12345", "needs-review")

	// アサーション
	assert.Contains(t, []string{"U23456", "U34567"}, reviewerID)

	// レビュワーリストが空の場合のテスト
	emptyConfig := models.ChannelConfig{
		ID:               "empty-id",
		SlackChannelID:   "C67890",
		LabelName:        "needs-review",
		DefaultMentionID: "U12345",
		ReviewerList:     "",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	db.Create(&emptyConfig)
	defaultReviewer := SelectRandomReviewer(db, "C67890", "needs-review")
	assert.Equal(t, "U12345", defaultReviewer)

	// 存在しないチャンネル/ラベルのテスト
	nonExistentReviewer := SelectRandomReviewer(db, "nonexistent", "needs-review")
	assert.Equal(t, "", nonExistentReviewer)
}

func TestSendReviewCompletedAutoNotification(t *testing.T) {
	// テスト前の環境変数を保存し、テスト後に復元
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()

	// テスト用の環境変数を設定
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	// モックの設定
	defer gock.Off() // テスト終了時にモックをクリア

	testCases := []struct {
		name          string
		reviewerLogin string
		reviewState   string
		expectedMsg   string
	}{
		{"承認", "reviewer1", "approved", "✅ reviewer1さんがレビューを承認しました！感謝！👏"},
		{"変更要求", "reviewer2", "changes_requested", "🔄 reviewer2さんが変更を要求しました 感謝！👏"},
		{"コメント", "reviewer3", "commented", "💬 reviewer3さんがレビューコメントを残しました 感謝！👏"},
		{"その他", "reviewer4", "other", "👀 reviewer4さんがレビューしました 感謝！👏"},
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
				Status:       "in_review",
			}

			// 関数を実行
			err := SendReviewCompletedAutoNotification(task, tc.reviewerLogin, tc.reviewState)

			// アサーション
			assert.NoError(t, err)
			assert.True(t, gock.IsDone(), "すべてのモックが使用されていません")
		})
	}
}

// TestFormatReviewerMentions は複数のレビュワーIDをSlackメンション形式に変換する関数のテスト
func TestFormatReviewerMentions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "単一レビュワーID",
			input:    "haruotsu",
			expected: "<@haruotsu>",
		},
		{
			name:     "複数レビュワーID（スペース区切り）",
			input:    "fuga @hoge",
			expected: "<@fuga> <@hoge>",
		},
		{
			name:     "複数レビュワーID（@付き）",
			input:    "@fuga @hoge",
			expected: "<@fuga> <@hoge>",
		},
		{
			name:     "混在パターン",
			input:    "fuga @hoge",
			expected: "<@fuga> <@hoge>",
		},
		{
			name:     "空文字列",
			input:    "",
			expected: "",
		},
		{
			name:     "3人のレビュワー",
			input:    "fuga hoge piyo",
			expected: "<@fuga> <@hoge> <@piyo>",
		},
		{
			name:     "余分なスペース",
			input:    "  user1   @user2  ",
			expected: "<@user1> <@user2>",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := formatReviewerMentions(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// --- 複数レビュワー対応のテスト ---

func TestSelectRandomReviewers_Basic(t *testing.T) {
	db := setupTestDB(t)

	testConfig := models.ChannelConfig{
		ID:               "multi-rev-id",
		SlackChannelID:   "C_MULTI",
		LabelName:        "needs-review",
		DefaultMentionID: "UDEFAULT",
		ReviewerList:     "U1,U2,U3,U4,U5",
		IsActive:         true,
	}
	db.Create(&testConfig)

	// 2人選択
	result := SelectRandomReviewers(db, "C_MULTI", "needs-review", 2, nil)
	assert.Equal(t, 2, len(result))
	// 重複なし
	assert.NotEqual(t, result[0], result[1])
}

func TestSelectRandomReviewers_ExcludeIDs(t *testing.T) {
	db := setupTestDB(t)

	testConfig := models.ChannelConfig{
		ID:               "exclude-id",
		SlackChannelID:   "C_EXCL",
		LabelName:        "needs-review",
		DefaultMentionID: "UDEFAULT",
		ReviewerList:     "U1,U2,U3",
		IsActive:         true,
	}
	db.Create(&testConfig)

	// U1を除外して2人選択
	result := SelectRandomReviewers(db, "C_EXCL", "needs-review", 2, []string{"U1"})
	assert.Equal(t, 2, len(result))
	for _, id := range result {
		assert.NotEqual(t, "U1", id, "除外対象のU1が含まれている")
	}
}

func TestSelectRandomReviewers_InsufficientAfterExclusion(t *testing.T) {
	db := setupTestDB(t)

	testConfig := models.ChannelConfig{
		ID:               "insuff-id",
		SlackChannelID:   "C_INSUF",
		LabelName:        "needs-review",
		DefaultMentionID: "UDEFAULT",
		ReviewerList:     "U1,U2",
		IsActive:         true,
	}
	db.Create(&testConfig)

	// U1を除外して2人要求 → 候補はU2のみ → 1人だけ返す
	result := SelectRandomReviewers(db, "C_INSUF", "needs-review", 2, []string{"U1"})
	assert.Equal(t, 1, len(result))
	assert.Equal(t, "U2", result[0])
}

func TestSelectRandomReviewers_AllExcluded(t *testing.T) {
	db := setupTestDB(t)

	testConfig := models.ChannelConfig{
		ID:               "allexcl-id",
		SlackChannelID:   "C_ALLX",
		LabelName:        "needs-review",
		DefaultMentionID: "UDEFAULT",
		ReviewerList:     "U1,U2",
		IsActive:         true,
	}
	db.Create(&testConfig)

	// 全員除外 → DefaultMentionIDを返す
	result := SelectRandomReviewers(db, "C_ALLX", "needs-review", 1, []string{"U1", "U2"})
	assert.Equal(t, 1, len(result))
	assert.Equal(t, "UDEFAULT", result[0])
}

func TestGetPendingReviewers(t *testing.T) {
	// 正常ケース: Reviewers設定あり、一部approve済み
	task := models.ReviewTask{
		Reviewers:  "U1,U2,U3",
		ApprovedBy: "U1",
	}
	pending := GetPendingReviewers(task)
	assert.Equal(t, []string{"U2", "U3"}, pending)

	// 全員approve済み
	task2 := models.ReviewTask{
		Reviewers:  "U1,U2",
		ApprovedBy: "U1,U2",
	}
	pending2 := GetPendingReviewers(task2)
	assert.Equal(t, 0, len(pending2))

	// Reviewers空（旧データ）→ Reviewerフォールバック
	task3 := models.ReviewTask{
		Reviewer: "UOLD",
	}
	pending3 := GetPendingReviewers(task3)
	assert.Equal(t, []string{"UOLD"}, pending3)

	// 全て空
	task4 := models.ReviewTask{}
	pending4 := GetPendingReviewers(task4)
	assert.Nil(t, pending4)
}

func TestAddApproval(t *testing.T) {
	// 新規追加
	task := models.ReviewTask{}
	added := AddApproval(&task, "U1")
	assert.True(t, added)
	assert.Equal(t, "U1", task.ApprovedBy)

	// 2人目追加
	added2 := AddApproval(&task, "U2")
	assert.True(t, added2)
	assert.Equal(t, "U1,U2", task.ApprovedBy)

	// 重複追加
	added3 := AddApproval(&task, "U1")
	assert.False(t, added3)
	assert.Equal(t, "U1,U2", task.ApprovedBy)

	// 空文字列
	added4 := AddApproval(&task, "")
	assert.False(t, added4)
}

func TestIsReviewFullyApproved(t *testing.T) {
	// 1人必要、1人approve済み → 完了
	task := models.ReviewTask{ApprovedBy: "U1"}
	assert.True(t, IsReviewFullyApproved(task, 1))

	// 2人必要、1人approve済み → 未完了
	assert.False(t, IsReviewFullyApproved(task, 2))

	// 2人必要、2人approve済み → 完了
	task2 := models.ReviewTask{ApprovedBy: "U1,U2"}
	assert.True(t, IsReviewFullyApproved(task2, 2))

	// 2人必要、3人approve済み → 完了
	task3 := models.ReviewTask{ApprovedBy: "U1,U2,U3"}
	assert.True(t, IsReviewFullyApproved(task3, 2))

	// ApprovedBy空 → 未完了
	task4 := models.ReviewTask{}
	assert.False(t, IsReviewFullyApproved(task4, 1))

	// requiredApprovals 0以下 → 1として扱う
	assert.False(t, IsReviewFullyApproved(task4, 0))

	// 割り当て人数 < requiredApprovals の場合、割り当て人数で判定
	task5 := models.ReviewTask{Reviewers: "U1", ApprovedBy: "U1"}
	assert.True(t, IsReviewFullyApproved(task5, 3), "割り当て1人でapprove済みなら完了")

	task6 := models.ReviewTask{Reviewers: "U1,U2", ApprovedBy: "U1"}
	assert.False(t, IsReviewFullyApproved(task6, 3), "割り当て2人で1人approve済みなら未完了")

	task7 := models.ReviewTask{Reviewers: "U1,U2", ApprovedBy: "U1,U2"}
	assert.True(t, IsReviewFullyApproved(task7, 3), "割り当て2人で2人approve済みなら完了")
}

func TestCountApprovals(t *testing.T) {
	assert.Equal(t, 0, CountApprovals(models.ReviewTask{}))
	assert.Equal(t, 0, CountApprovals(models.ReviewTask{ApprovedBy: ""}))
	assert.Equal(t, 1, CountApprovals(models.ReviewTask{ApprovedBy: "U1"}))
	assert.Equal(t, 2, CountApprovals(models.ReviewTask{ApprovedBy: "U1,U2"}))
}

func TestGetAwayUserIDs(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	future := now.Add(24 * time.Hour)
	past := now.Add(-24 * time.Hour)

	// 無期限休暇
	db.Create(&models.ReviewerAvailability{
		ID:          "away-1",
		SlackUserID: "U_AWAY1",
		AwayUntil:   nil,
		Reason:      "育児休業",
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	// 未来まで休暇
	db.Create(&models.ReviewerAvailability{
		ID:          "away-2",
		SlackUserID: "U_AWAY2",
		AwayUntil:   &future,
		Reason:      "休暇",
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	// 期限切れ（返さない）
	db.Create(&models.ReviewerAvailability{
		ID:          "away-3",
		SlackUserID: "U_EXPIRED",
		AwayUntil:   &past,
		Reason:      "過去の休暇",
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	ids := GetAwayUserIDs(db)
	assert.Contains(t, ids, "U_AWAY1")
	assert.Contains(t, ids, "U_AWAY2")
	assert.NotContains(t, ids, "U_EXPIRED")
}

func TestSelectRandomReviewers_ExcludesAwayUsers(t *testing.T) {
	db := setupTestDB(t)

	now := time.Now()
	future := now.Add(24 * time.Hour)

	// チャンネル設定
	testConfig := models.ChannelConfig{
		ID:               "away-test-id",
		SlackChannelID:   "C_AWAY",
		LabelName:        "needs-review",
		DefaultMentionID: "UDEFAULT",
		ReviewerList:     "U1,U2,U3",
		IsActive:         true,
	}
	db.Create(&testConfig)

	// U2 を休暇に設定
	db.Create(&models.ReviewerAvailability{
		ID:          "away-u2",
		SlackUserID: "U2",
		AwayUntil:   &future,
		Reason:      "休暇",
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	// 100回繰り返して、U2 が選ばれないことを確認
	for i := 0; i < 100; i++ {
		result := SelectRandomReviewers(db, "C_AWAY", "needs-review", 2, nil)
		for _, id := range result {
			assert.NotEqual(t, "U2", id, "休暇中のU2が選択された")
		}
	}
}
