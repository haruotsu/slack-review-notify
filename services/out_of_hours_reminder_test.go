package services

import (
	"slack-review-notify/models"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// テスト用のデータベースを作成
func setupOutOfHoursTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	assert.NoError(t, err)

	// マイグレーション
	err = db.AutoMigrate(&models.ReviewTask{}, &models.ChannelConfig{})
	assert.NoError(t, err)

	return db
}

// 営業時間外リマインドのテスト（シンプル版）
func TestCheckInReviewTasks_OutOfHoursReminder(t *testing.T) {
	db := setupOutOfHoursTestDB(t)

	// チャンネル設定を作成
	config := models.ChannelConfig{
		SlackChannelID:           "C123456",
		LabelName:                "needs-review",
		DefaultMentionID:         "U123456",
		ReviewerReminderInterval: 30,
		IsActive:                 true,
	}
	db.Create(&config)

	// 古いUpdatedAtでテストタスクを作成（確実にリマインドが送信される）
	oldTime := time.Now().Add(-2 * time.Hour)
	task := models.ReviewTask{
		ID:           "task-1",
		PRURL:        "https://github.com/test/repo/pull/1",
		SlackTS:      "1234567890.123456",
		SlackChannel: "C123456",
		Reviewer:     "U123456",
		Status:       "in_review",
		LabelName:    "needs-review",
		UpdatedAt:    oldTime,
	}
	db.Create(&task)

	// CheckInReviewTasksを実行
	CheckInReviewTasks(db)

	// タスクの状態を確認
	var updatedTask models.ReviewTask
	db.First(&updatedTask, "id = ?", "task-1")

	// UpdatedAtが更新されていることを確認（何らかのリマインドが送信された）
	assert.True(t, updatedTask.UpdatedAt.After(oldTime),
		"UpdatedAtが更新されていない（リマインドが送信されていない）")

	// 現在の時刻で営業時間外判定を確認してログ出力
	now := time.Now()
	isOutOfHours := IsOutsideBusinessHours(now)
	t.Logf("現在時刻: %v", now)
	t.Logf("営業時間外: %v", isOutOfHours)
	t.Logf("OutOfHoursReminded: %v", updatedTask.OutOfHoursReminded)
	
	if updatedTask.ReminderPausedUntil != nil {
		t.Logf("ReminderPausedUntil: %v", *updatedTask.ReminderPausedUntil)
	}

	// 営業時間外の場合、OutOfHoursRemindedがtrueになり、ReminderPausedUntilが設定されることを確認
	if isOutOfHours {
		assert.True(t, updatedTask.OutOfHoursReminded,
			"営業時間外でOutOfHoursRemindedフラグがtrueになっていない")
		assert.NotNil(t, updatedTask.ReminderPausedUntil,
			"営業時間外でReminderPausedUntilが設定されていない")
	}
}

// 翌営業日10時の計算テスト
func TestGetNextBusinessDayMorningWithTime_OutOfHours(t *testing.T) {
	// JSTタイムゾーンを取得
	jst, _ := time.LoadLocation("Asia/Tokyo")

	tests := []struct {
		name     string
		input    time.Time
		expected time.Time
	}{
		{
			name:     "月曜9時_同日10時",
			input:    time.Date(2024, 1, 15, 0, 0, 0, 0, jst), // 月曜9時（JST）
			expected: time.Date(2024, 1, 15, 10, 0, 0, 0, jst),
		},
		{
			name:     "月曜20時_翌日10時",
			input:    time.Date(2024, 1, 15, 11, 0, 0, 0, jst), // 月曜20時（JST）
			expected: time.Date(2024, 1, 16, 10, 0, 0, 0, jst),
		},
		{
			name:     "金曜20時_月曜10時",
			input:    time.Date(2024, 1, 19, 11, 0, 0, 0, jst), // 金曜20時（JST）
			expected: time.Date(2024, 1, 22, 10, 0, 0, 0, jst),
		},
		{
			name:     "土曜14時_月曜10時",
			input:    time.Date(2024, 1, 20, 14, 0, 0, 0, jst), // 土曜14時（JST）
			expected: time.Date(2024, 1, 22, 10, 0, 0, 0, jst),
		},
		{
			name:     "日曜14時_月曜10時",
			input:    time.Date(2024, 1, 21, 14, 0, 0, 0, jst), // 日曜14時（JST）
			expected: time.Date(2024, 1, 22, 10, 0, 0, 0, jst),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetNextBusinessDayMorningWithTime(tt.input)
			assert.Equal(t, tt.expected, result,
				"翌営業日10時の計算が正しくない: got %v, want %v", result, tt.expected)
		})
	}
}

// 営業時間外判定のテスト
func TestIsOutsideBusinessHours_OutOfHours(t *testing.T) {
	// JSTタイムゾーンを取得
	jst, _ := time.LoadLocation("Asia/Tokyo")

	tests := []struct {
		name               string
		input              time.Time
		expectedOutOfHours bool
	}{
		{
			name:               "月曜10時_営業時間内",
			input:              time.Date(2024, 1, 15, 10, 0, 0, 0, jst),
			expectedOutOfHours: false,
		},
		{
			name:               "月曜18時_営業時間内",
			input:              time.Date(2024, 1, 15, 18, 59, 0, 0, jst),
			expectedOutOfHours: false,
		},
		{
			name:               "月曜19時_営業時間外",
			input:              time.Date(2024, 1, 15, 19, 0, 0, 0, jst),
			expectedOutOfHours: true,
		},
		{
			name:               "月曜9時_営業時間外",
			input:              time.Date(2024, 1, 15, 9, 59, 0, 0, jst),
			expectedOutOfHours: true,
		},
		{
			name:               "土曜14時_営業時間外",
			input:              time.Date(2024, 1, 20, 14, 0, 0, 0, jst),
			expectedOutOfHours: true,
		},
		{
			name:               "日曜10時_営業時間外",
			input:              time.Date(2024, 1, 21, 10, 0, 0, 0, jst),
			expectedOutOfHours: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsOutsideBusinessHours(tt.input)
			assert.Equal(t, tt.expectedOutOfHours, result,
				"営業時間外判定が正しくない: got %v, want %v", result, tt.expectedOutOfHours)
		})
	}
}
