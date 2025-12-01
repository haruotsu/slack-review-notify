package services

import (
	"slack-review-notify/models"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCheckBusinessHoursTasks(t *testing.T) {
	db := setupTestDB(t)

	// JST タイムゾーン設定
	jst, err := time.LoadLocation("Asia/Tokyo")
	assert.NoError(t, err)

	// チャンネル設定を作成
	config := models.ChannelConfig{
		ID:               "test-config",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U12345",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&config)

	// 営業時間外待機状態のタスクを作成
	task := models.ReviewTask{
		ID:           "waiting-task",
		PRURL:        "https://github.com/owner/repo/pull/1",
		Repo:         "owner/repo",
		PRNumber:     1,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Reviewer:     "", // 営業時間外では空
		Status:       "waiting_business_hours",
		LabelName:    "needs-review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// 営業時間外の時刻でテスト（何も起きない）
	outsideHours := time.Date(2024, 8, 27, 20, 0, 0, 0, jst) // 火曜日 20:00 JST

	// ここでは実際の時間をモックするのではなく、ロジックを直接テスト
	// デフォルト設定（10:00-19:00 JST）での営業時間判定のテスト
	defaultConfig := &models.ChannelConfig{
		BusinessHoursStart: "10:00",
		BusinessHoursEnd:   "19:00",
		Timezone:           "Asia/Tokyo",
	}

	// 営業時間外判定のテスト（20時は営業時間外）
	assert.False(t, IsWithinBusinessHours(defaultConfig, outsideHours), "20時は営業時間外")

	// 営業時間内の時刻でテスト
	businessHours := time.Date(2024, 8, 28, 10, 0, 0, 0, jst) // 水曜日 10:00 JST
	assert.True(t, IsWithinBusinessHours(defaultConfig, businessHours), "10時は営業時間内")

	// この時点でタスクはまだ待機状態
	var beforeTask models.ReviewTask
	db.First(&beforeTask, "id = ?", "waiting-task")
	assert.Equal(t, "waiting_business_hours", beforeTask.Status)
	assert.Equal(t, "", beforeTask.Reviewer)
}

func TestBusinessHoursTaskFlow(t *testing.T) {
	db := setupTestDB(t)

	// チャンネル設定を作成
	config := models.ChannelConfig{
		ID:                       "test-config",
		SlackChannelID:           "C12345",
		LabelName:                "needs-review",
		DefaultMentionID:         "U12345",
		ReviewerReminderInterval: 30,
		IsActive:                 true,
		CreatedAt:                time.Now(),
		UpdatedAt:                time.Now(),
	}
	db.Create(&config)

	// レビュワーリストを設定
	config.ReviewerList = "U67890,U11111,U22222"
	db.Save(&config)

	// 営業時間外待機状態のタスクを作成
	task := models.ReviewTask{
		ID:           "waiting-task",
		PRURL:        "https://github.com/owner/repo/pull/1",
		Repo:         "owner/repo",
		PRNumber:     1,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Reviewer:     "",
		Status:       "waiting_business_hours",
		LabelName:    "needs-review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// レビュワー選択のテスト
	selectedReviewer := SelectRandomReviewer(db, "C12345", "needs-review")
	assert.Contains(t, []string{"U67890", "U11111", "U22222"}, selectedReviewer, "レビュワーがリストから正しく選択される")

	// タスクのステータス確認
	var retrievedTask models.ReviewTask
	db.First(&retrievedTask, "id = ?", "waiting-task")
	assert.Equal(t, "waiting_business_hours", retrievedTask.Status)
}

func TestIsWithinBusinessHoursEdgeCases(t *testing.T) {
	jst, err := time.LoadLocation("Asia/Tokyo")
	assert.NoError(t, err)

	// デフォルト設定（10:00-19:00 JST）
	defaultConfig := &models.ChannelConfig{
		BusinessHoursStart: "10:00",
		BusinessHoursEnd:   "19:00",
		Timezone:           "Asia/Tokyo",
	}

	tests := []struct {
		name                   string
		testTime               time.Time
		expectedWithinBusiness bool // 営業時間内かどうか
	}{
		{
			name:                   "金曜日18時59分59秒",
			testTime:               time.Date(2024, 8, 30, 18, 59, 59, 0, jst),
			expectedWithinBusiness: true, // 営業時間内
		},
		{
			name:                   "金曜日19時00分00秒",
			testTime:               time.Date(2024, 8, 30, 19, 0, 0, 0, jst),
			expectedWithinBusiness: false, // 営業時間外
		},
		{
			name:                   "月曜日00時00分00秒",
			testTime:               time.Date(2024, 8, 26, 0, 0, 0, 0, jst),
			expectedWithinBusiness: false, // 平日の深夜は営業時間外（10時未満）
		},
		{
			name:                   "土曜日00時00分00秒",
			testTime:               time.Date(2024, 8, 24, 0, 0, 0, 0, jst),
			expectedWithinBusiness: false, // 土曜日は営業時間外
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsWithinBusinessHours(defaultConfig, tt.testTime)
			assert.Equal(t, tt.expectedWithinBusiness, result, tt.name)
		})
	}
}
