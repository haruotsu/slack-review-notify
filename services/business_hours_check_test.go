package services

import (
	"slack-review-notify/models"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIsWithinBusinessHours(t *testing.T) {
	tests := []struct {
		name         string
		config       *models.ChannelConfig
		currentTime  time.Time
		expected     bool
	}{
		{
			name: "営業時間内（通常時間）",
			config: &models.ChannelConfig{
				BusinessHoursStart: "09:00",
				BusinessHoursEnd:   "18:00",
				Timezone:           "UTC",
			},
			currentTime: time.Date(2023, 12, 15, 12, 30, 0, 0, time.UTC), // 12:30
			expected:    true,
		},
		{
			name: "営業時間外（早朝）",
			config: &models.ChannelConfig{
				BusinessHoursStart: "09:00",
				BusinessHoursEnd:   "18:00",
				Timezone:           "UTC",
			},
			currentTime: time.Date(2023, 12, 15, 8, 30, 0, 0, time.UTC), // 8:30
			expected:    false,
		},
		{
			name: "営業時間外（夜間）",
			config: &models.ChannelConfig{
				BusinessHoursStart: "09:00",
				BusinessHoursEnd:   "18:00",
				Timezone:           "UTC",
			},
			currentTime: time.Date(2023, 12, 15, 19, 30, 0, 0, time.UTC), // 19:30
			expected:    false,
		},
		{
			name: "営業時間開始時刻ちょうど",
			config: &models.ChannelConfig{
				BusinessHoursStart: "09:00",
				BusinessHoursEnd:   "18:00",
				Timezone:           "UTC",
			},
			currentTime: time.Date(2023, 12, 15, 9, 0, 0, 0, time.UTC), // 9:00
			expected:    true,
		},
		{
			name: "営業時間終了時刻ちょうど",
			config: &models.ChannelConfig{
				BusinessHoursStart: "09:00",
				BusinessHoursEnd:   "18:00",
				Timezone:           "UTC",
			},
			currentTime: time.Date(2023, 12, 15, 18, 0, 0, 0, time.UTC), // 18:00
			expected:    false, // 終了時刻は含まない
		},
		{
			name: "営業時間設定がない場合（デフォルト値）",
			config: &models.ChannelConfig{
				BusinessHoursStart: "",
				BusinessHoursEnd:   "",
			},
			currentTime: time.Date(2023, 12, 15, 12, 30, 0, 0, time.UTC), // 12:30
			expected:    true, // 営業時間設定がない場合は常に通知
		},
		{
			name: "深夜営業（22:00-06:00）の営業時間内",
			config: &models.ChannelConfig{
				BusinessHoursStart: "22:00",
				BusinessHoursEnd:   "06:00",
				Timezone:           "UTC",
			},
			currentTime: time.Date(2023, 12, 15, 23, 30, 0, 0, time.UTC), // 23:30
			expected:    true,
		},
		{
			name: "深夜営業（22:00-06:00）の営業時間内（早朝）",
			config: &models.ChannelConfig{
				BusinessHoursStart: "22:00",
				BusinessHoursEnd:   "06:00",
				Timezone:           "UTC",
			},
			currentTime: time.Date(2023, 12, 15, 3, 30, 0, 0, time.UTC), // 3:30
			expected:    true,
		},
		{
			name: "深夜営業（22:00-06:00）の営業時間外",
			config: &models.ChannelConfig{
				BusinessHoursStart: "22:00",
				BusinessHoursEnd:   "06:00",
				Timezone:           "UTC",
			},
			currentTime: time.Date(2023, 12, 15, 12, 30, 0, 0, time.UTC), // 12:30
			expected:    false,
		},
		{
			name: "UTC設定での営業時間内",
			config: &models.ChannelConfig{
				BusinessHoursStart: "09:00",
				BusinessHoursEnd:   "18:00",
				Timezone:           "UTC",
			},
			currentTime: time.Date(2023, 12, 15, 12, 30, 0, 0, time.UTC), // 12:30 UTC
			expected:    true,
		},
		{
			name: "UTC設定での営業時間外",
			config: &models.ChannelConfig{
				BusinessHoursStart: "09:00",
				BusinessHoursEnd:   "18:00",
				Timezone:           "UTC",
			},
			currentTime: time.Date(2023, 12, 15, 19, 30, 0, 0, time.UTC), // 19:30 UTC
			expected:    false,
		},
		{
			name: "JST設定での営業時間内（UTC時刻で入力）",
			config: &models.ChannelConfig{
				BusinessHoursStart: "09:00",
				BusinessHoursEnd:   "18:00",
				Timezone:           "Asia/Tokyo",
			},
			currentTime: time.Date(2023, 12, 15, 3, 30, 0, 0, time.UTC), // 3:30 UTC = 12:30 JST
			expected:    true,
		},
		{
			name: "JST設定での営業時間外（UTC時刻で入力）",
			config: &models.ChannelConfig{
				BusinessHoursStart: "09:00",
				BusinessHoursEnd:   "18:00",
				Timezone:           "Asia/Tokyo",
			},
			currentTime: time.Date(2023, 12, 15, 10, 30, 0, 0, time.UTC), // 10:30 UTC = 19:30 JST
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

func TestParseBusinessHoursTime(t *testing.T) {
	tests := []struct {
		name         string
		timeStr      string
		expectedHour int
		expectedMin  int
		expectError  bool
	}{
		{
			name:         "正常な時間（09:00）",
			timeStr:      "09:00",
			expectedHour: 9,
			expectedMin:  0,
			expectError:  false,
		},
		{
			name:         "正常な時間（23:59）",
			timeStr:      "23:59",
			expectedHour: 23,
			expectedMin:  59,
			expectError:  false,
		},
		{
			name:         "無効な形式（時間のみ）",
			timeStr:      "09",
			expectedHour: 0,
			expectedMin:  0,
			expectError:  true,
		},
		{
			name:         "無効な時間（25:00）",
			timeStr:      "25:00",
			expectedHour: 0,
			expectedMin:  0,
			expectError:  true,
		},
		{
			name:         "無効な分（09:60）",
			timeStr:      "09:60",
			expectedHour: 0,
			expectedMin:  0,
			expectError:  true,
		},
		{
			name:         "空文字列",
			timeStr:      "",
			expectedHour: 0,
			expectedMin:  0,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hour, min, err := parseBusinessHoursTime(tt.timeStr)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedHour, hour)
				assert.Equal(t, tt.expectedMin, min)
			}
		})
	}
}