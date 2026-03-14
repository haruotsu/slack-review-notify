package services

import (
	"slack-review-notify/models"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBusinessHoursWithDefaultConfig(t *testing.T) {
	// JST timezone setup
	jst, err := time.LoadLocation("Asia/Tokyo")
	assert.NoError(t, err)

	// Default business hours configuration (10:00-19:00 JST)
	defaultConfig := &models.ChannelConfig{
		BusinessHoursStart: "10:00",
		BusinessHoursEnd:   "19:00",
		Timezone:           "Asia/Tokyo",
	}

	tests := []struct {
		name        string
		testTime    time.Time
		expected    bool
		description string
	}{
		{
			name:        "平日18時",
			testTime:    time.Date(2024, 8, 27, 18, 0, 0, 0, jst), // Tuesday 18:00 JST
			expected:    true,
			description: "平日18時は営業時間内",
		},
		{
			name:        "平日19時",
			testTime:    time.Date(2024, 8, 27, 19, 0, 0, 0, jst), // Tuesday 19:00 JST
			expected:    false,
			description: "平日19時は営業時間外",
		},
		{
			name:        "平日20時",
			testTime:    time.Date(2024, 8, 27, 20, 30, 0, 0, jst), // Tuesday 20:30 JST
			expected:    false,
			description: "平日夜は営業時間外",
		},
		{
			name:        "土曜日10時",
			testTime:    time.Date(2024, 8, 24, 10, 0, 0, 0, jst), // Saturday 10:00 JST
			expected:    false,
			description: "土曜日は営業時間外",
		},
		{
			name:        "日曜日15時",
			testTime:    time.Date(2024, 8, 25, 15, 0, 0, 0, jst), // Sunday 15:00 JST
			expected:    false,
			description: "日曜日は営業時間外",
		},
		{
			name:        "平日朝9時59分",
			testTime:    time.Date(2024, 8, 26, 9, 59, 0, 0, jst), // Monday 9:59 JST
			expected:    false,
			description: "平日朝9時59分は営業時間外",
		},
		{
			name:        "平日朝10時",
			testTime:    time.Date(2024, 8, 26, 10, 0, 0, 0, jst), // Monday 10:00 JST
			expected:    true,
			description: "平日朝10時は営業時間内",
		},
		{
			name:        "金曜日18時59分",
			testTime:    time.Date(2024, 8, 30, 18, 59, 0, 0, jst), // Friday 18:59 JST
			expected:    true,
			description: "金曜日18時59分は営業時間内",
		},
		{
			name:        "金曜日19時1分",
			testTime:    time.Date(2024, 8, 30, 19, 1, 0, 0, jst), // Friday 19:01 JST
			expected:    false,
			description: "金曜日19時1分は営業時間外",
		},
		{
			name:        "UTC時刻での営業時間外判定",
			testTime:    time.Date(2024, 8, 27, 10, 0, 0, 0, time.UTC), // Tuesday 10:00 UTC = Tuesday 19:00 JST
			expected:    false,
			description: "UTC時刻でも正確にJST基準で営業時間外判定",
		},
		{
			name:        "日本の祝日（元日）の営業時間内",
			testTime:    time.Date(2024, 1, 1, 12, 0, 0, 0, jst), // New Year's Day 12:00 JST
			expected:    false,
			description: "祝日は営業時間内でも営業時間外として扱う",
		},
		{
			name:        "日本の祝日（成人の日）の営業時間内",
			testTime:    time.Date(2024, 1, 8, 15, 0, 0, 0, jst), // Coming of Age Day 15:00 JST
			expected:    false,
			description: "祝日は営業時間内でも営業時間外として扱う",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsWithinBusinessHours(defaultConfig, tt.testTime)
			assert.Equal(t, tt.expected, result, "Test: %s - %s", tt.name, tt.description)
		})
	}
}
