package services

import (
	"slack-review-notify/models"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIsWithinBusinessHours_JapaneseHolidays(t *testing.T) {
	// JST タイムゾーンを取得
	jst, err := time.LoadLocation("Asia/Tokyo")
	assert.NoError(t, err)

	tests := []struct {
		name         string
		config       *models.ChannelConfig
		currentTime  time.Time
		expected     bool
	}{
		{
			name: "日本のタイムゾーンで祝日（元日）は営業時間外",
			config: &models.ChannelConfig{
				BusinessHoursStart: "09:00",
				BusinessHoursEnd:   "18:00",
				Timezone:           "Asia/Tokyo",
			},
			currentTime: time.Date(2024, 1, 1, 12, 30, 0, 0, jst), // 2024年1月1日 12:30 JST（元日）
			expected:    false,
		},
		{
			name: "日本のタイムゾーンで祝日（成人の日）は営業時間外",
			config: &models.ChannelConfig{
				BusinessHoursStart: "09:00",
				BusinessHoursEnd:   "18:00",
				Timezone:           "Asia/Tokyo",
			},
			currentTime: time.Date(2024, 1, 8, 12, 30, 0, 0, jst), // 2024年1月8日 12:30 JST（成人の日）
			expected:    false,
		},
		{
			name: "日本のタイムゾーンで土曜日は営業時間外",
			config: &models.ChannelConfig{
				BusinessHoursStart: "09:00",
				BusinessHoursEnd:   "18:00",
				Timezone:           "Asia/Tokyo",
			},
			currentTime: time.Date(2024, 1, 6, 12, 30, 0, 0, jst), // 2024年1月6日 12:30 JST（土曜日）
			expected:    false,
		},
		{
			name: "日本のタイムゾーンで日曜日は営業時間外",
			config: &models.ChannelConfig{
				BusinessHoursStart: "09:00",
				BusinessHoursEnd:   "18:00",
				Timezone:           "Asia/Tokyo",
			},
			currentTime: time.Date(2024, 1, 7, 12, 30, 0, 0, jst), // 2024年1月7日 12:30 JST（日曜日）
			expected:    false,
		},
		{
			name: "日本のタイムゾーンで平日の営業時間内",
			config: &models.ChannelConfig{
				BusinessHoursStart: "09:00",
				BusinessHoursEnd:   "18:00",
				Timezone:           "Asia/Tokyo",
			},
			currentTime: time.Date(2024, 1, 10, 12, 30, 0, 0, jst), // 2024年1月10日 12:30 JST（水曜日）
			expected:    true,
		},
		{
			name: "UTC設定では祝日判定されない（日本の祝日でも営業時間内）",
			config: &models.ChannelConfig{
				BusinessHoursStart: "09:00",
				BusinessHoursEnd:   "18:00",
				Timezone:           "UTC",
			},
			currentTime: time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC), // 2024年1月1日 12:30 UTC（元日だが判定されない）
			expected:    true,
		},
		{
			name: "年末（12/29）は営業時間外",
			config: &models.ChannelConfig{
				BusinessHoursStart: "09:00",
				BusinessHoursEnd:   "18:00",
				Timezone:           "Asia/Tokyo",
			},
			currentTime: time.Date(2023, 12, 29, 12, 30, 0, 0, jst), // 2023年12月29日 12:30 JST
			expected:    false,
		},
		{
			name: "年始（1/3）は営業時間外",
			config: &models.ChannelConfig{
				BusinessHoursStart: "09:00",
				BusinessHoursEnd:   "18:00",
				Timezone:           "Asia/Tokyo",
			},
			currentTime: time.Date(2024, 1, 3, 12, 30, 0, 0, jst), // 2024年1月3日 12:30 JST
			expected:    false,
		},
		{
			name: "ゴールデンウィーク（憲法記念日）は営業時間外",
			config: &models.ChannelConfig{
				BusinessHoursStart: "09:00",
				BusinessHoursEnd:   "18:00",
				Timezone:           "Asia/Tokyo",
			},
			currentTime: time.Date(2024, 5, 3, 12, 30, 0, 0, jst), // 2024年5月3日 12:30 JST（憲法記念日）
			expected:    false,
		},
		{
			name: "振替休日は営業時間外",
			config: &models.ChannelConfig{
				BusinessHoursStart: "09:00",
				BusinessHoursEnd:   "18:00",
				Timezone:           "Asia/Tokyo",
			},
			currentTime: time.Date(2024, 2, 12, 12, 30, 0, 0, jst), // 2024年2月12日 12:30 JST（建国記念の日の振替休日）
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsWithinBusinessHours(tt.config, tt.currentTime)
			assert.Equal(t, tt.expected, result)
		})
	}
}