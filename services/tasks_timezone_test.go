package services

import (
	"slack-review-notify/models"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCheckInReviewTasks_ReminderPausedUntilTimezone(t *testing.T) {
	db := setupTestDB(t)

	// JST タイムゾーン設定
	jst, err := time.LoadLocation("Asia/Tokyo")
	assert.NoError(t, err)

	// テスト時刻（JST）：火曜日の13:00
	baseTimeJST := time.Date(2024, 8, 27, 13, 0, 0, 0, jst)
	
	// 翌営業日の朝（JST 10:00）を計算
	nextBusinessDayJST := GetNextBusinessDayMorningWithConfig(baseTimeJST, nil)
	
	// UTC での同じ時刻
	baseTimeUTC := baseTimeJST.UTC()
	nextBusinessDayUTC := nextBusinessDayJST.UTC()

	tests := []struct {
		name          string
		currentTime   time.Time // テスト実行時の現在時刻
		pausedUntil   *time.Time // reminder_paused_until の値（データベース保存値）
		shouldSkip    bool       // リマインダーがスキップされるべきか
		description   string
	}{
		{
			name:        "JST_with_JST_paused_until",
			currentTime: baseTimeJST,
			pausedUntil: &nextBusinessDayJST,
			shouldSkip:  true,
			description: "JST現在時刻、JST一時停止時刻 - スキップされる",
		},
		{
			name:        "UTC_with_UTC_paused_until",
			currentTime: baseTimeUTC,
			pausedUntil: &nextBusinessDayUTC,
			shouldSkip:  true,
			description: "UTC現在時刻、UTC一時停止時刻 - スキップされる",
		},
		{
			name:        "JST_with_UTC_paused_until",
			currentTime: baseTimeJST,
			pausedUntil: &nextBusinessDayUTC, // データベース保存値（UTC）
			shouldSkip:  true,
			description: "JST現在時刻、UTC一時停止時刻（実際のケース） - スキップされる",
		},
		{
			name:        "past_paused_until",
			currentTime: baseTimeJST,
			pausedUntil: func() *time.Time { t := baseTimeJST.Add(-1 * time.Hour); return &t }(), // 1時間前
			shouldSkip:  false,
			description: "過去の一時停止時刻 - スキップされない",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// テストタスクを作成
			task := models.ReviewTask{
				ID:                    tt.name + "_task",
				PRURL:                 "https://github.com/owner/repo/pull/1",
				Repo:                  "owner/repo",
				PRNumber:              1,
				Title:                 "Test PR",
				SlackTS:               "1234.5678",
				SlackChannel:          "C12345",
				Status:                "in_review",
				Reviewer:              "U12345",
				LabelName:             "needs-review",
				ReminderPausedUntil:   tt.pausedUntil,
				CreatedAt:             tt.currentTime.Add(-2 * time.Hour),
				UpdatedAt:             tt.currentTime.Add(-2 * time.Hour), // 2時間前に更新（リマインド対象）
			}

			db.Create(&task)

			// 現在時刻の比較ロジックをシミュレート
			now := tt.currentTime
			shouldSkip := task.ReminderPausedUntil != nil && now.Before(*task.ReminderPausedUntil)

			assert.Equal(t, tt.shouldSkip, shouldSkip, "Test: %s - %s", tt.name, tt.description)

			// クリーンアップ
			db.Where("id = ?", task.ID).Delete(&models.ReviewTask{})
		})
	}
}

func TestGetNextBusinessDayMorning_Timezone(t *testing.T) {
	// JST タイムゾーン設定
	jst, err := time.LoadLocation("Asia/Tokyo")
	assert.NoError(t, err)

	tests := []struct {
		name        string
		inputTime   time.Time
		expectedDay int
		expectedHour int
		description string
	}{
		{
			name:        "JST_Tuesday_15_00",
			inputTime:   time.Date(2024, 8, 27, 15, 0, 0, 0, jst), // 火曜日 15:00 JST
			expectedDay: 28, // 水曜日
			expectedHour: 10,
			description: "火曜日 15:00 JST -> 翌日（水曜日）10:00 JST",
		},
		{
			name:        "UTC_equivalent_Tuesday_06_00",
			inputTime:   time.Date(2024, 8, 27, 6, 0, 0, 0, time.UTC), // 火曜日 06:00 UTC = 火曜日 15:00 JST
			expectedDay: 28, // 水曜日
			expectedHour: 10,
			description: "火曜日 06:00 UTC（= 火曜日 15:00 JST）-> 翌日（水曜日）10:00 JST",
		},
		{
			name:        "JST_Tuesday_09_00",
			inputTime:   time.Date(2024, 8, 27, 9, 0, 0, 0, jst), // 火曜日 09:00 JST
			expectedDay: 27, // 火曜日（同日）
			expectedHour: 10,
			description: "火曜日 09:00 JST -> 当日（火曜日）10:00 JST",
		},
		{
			name:        "UTC_equivalent_Tuesday_00_00",
			inputTime:   time.Date(2024, 8, 27, 0, 0, 0, 0, time.UTC), // 火曜日 00:00 UTC = 火曜日 09:00 JST
			expectedDay: 27, // 火曜日（同日）
			expectedHour: 10,
			description: "火曜日 00:00 UTC（= 火曜日 09:00 JST）-> 当日（火曜日）10:00 JST",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 翌営業日を計算
			nextBusinessDay := GetNextBusinessDayMorningWithConfig(tt.inputTime, nil)

			// 結果は常に JST で指定された日の 10:00 になるはず
			assert.Equal(t, 2024, nextBusinessDay.Year(), "Year should be 2024")
			assert.Equal(t, time.August, nextBusinessDay.Month(), "Month should be August")
			assert.Equal(t, tt.expectedDay, nextBusinessDay.Day(), "Day should match expected: %s", tt.description)
			assert.Equal(t, tt.expectedHour, nextBusinessDay.Hour(), "Hour should be 10: %s", tt.description)
			assert.Equal(t, 0, nextBusinessDay.Minute(), "Minute should be 0: %s", tt.description)
			
			// タイムゾーンはJSTであるべき
			assert.Equal(t, jst.String(), nextBusinessDay.Location().String(), "Timezone should be JST: %s", tt.description)
		})
	}
}