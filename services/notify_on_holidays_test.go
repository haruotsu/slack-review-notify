package services

import (
	"slack-review-notify/models"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestIsWithinBusinessHours_NotifyOnHolidays(t *testing.T) {
	jst, err := time.LoadLocation("Asia/Tokyo")
	assert.NoError(t, err)

	config := &models.ChannelConfig{
		BusinessHoursStart: "09:00",
		BusinessHoursEnd:   "18:00",
		Timezone:           "Asia/Tokyo",
		NotifyOnHolidays:   true,
	}

	tests := []struct {
		name        string
		currentTime time.Time
		expected    bool
	}{
		{
			name:        "土曜でも営業時間内なら通知対象になる",
			currentTime: time.Date(2024, 1, 13, 12, 0, 0, 0, jst), // Saturday
			expected:    true,
		},
		{
			name:        "日曜でも営業時間内なら通知対象になる",
			currentTime: time.Date(2024, 1, 14, 12, 0, 0, 0, jst), // Sunday
			expected:    true,
		},
		{
			name:        "日本の祝日でも営業時間内なら通知対象になる",
			currentTime: time.Date(2024, 1, 8, 12, 0, 0, 0, jst), // Coming of Age Day
			expected:    true,
		},
		{
			name:        "休日でも営業時間外は通知対象にならない",
			currentTime: time.Date(2024, 1, 13, 20, 0, 0, 0, jst), // Saturday night
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsWithinBusinessHours(config, tt.currentTime))
		})
	}
}

func TestIsWithinBusinessHours_NotifyOnHolidaysOvernight(t *testing.T) {
	jst, err := time.LoadLocation("Asia/Tokyo")
	assert.NoError(t, err)

	config := &models.ChannelConfig{
		BusinessHoursStart: "22:00",
		BusinessHoursEnd:   "06:00",
		Timezone:           "Asia/Tokyo",
		NotifyOnHolidays:   true,
	}

	tests := []struct {
		name        string
		currentTime time.Time
		expected    bool
	}{
		{
			name:        "日をまたぐ営業時間なら土曜の深夜も通知対象になる",
			currentTime: time.Date(2024, 1, 13, 2, 0, 0, 0, jst), // Saturday
			expected:    true,
		},
		{
			name:        "日をまたぐ営業時間なら祝日の夜も通知対象になる",
			currentTime: time.Date(2024, 1, 8, 23, 0, 0, 0, jst), // Coming of Age Day
			expected:    true,
		},
		{
			name:        "日をまたぐ営業時間でも休日の昼は通知対象にならない",
			currentTime: time.Date(2024, 1, 14, 12, 0, 0, 0, jst), // Sunday
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsWithinBusinessHours(config, tt.currentTime))
		})
	}
}

func TestGetNextBusinessDayMorningWithConfig_NotifyOnHolidays(t *testing.T) {
	jst, err := time.LoadLocation("Asia/Tokyo")
	assert.NoError(t, err)

	config := &models.ChannelConfig{
		BusinessHoursStart: "09:00",
		Timezone:           "Asia/Tokyo",
		NotifyOnHolidays:   true,
	}

	tests := []struct {
		name        string
		currentTime time.Time
		expected    time.Time
	}{
		{
			name:        "金曜の営業開始後は翌日の土曜朝になる",
			currentTime: time.Date(2024, 1, 12, 19, 0, 0, 0, jst), // Friday
			expected:    time.Date(2024, 1, 13, 9, 0, 0, 0, jst),
		},
		{
			name:        "土曜の営業開始前は当日の朝になる",
			currentTime: time.Date(2024, 1, 13, 7, 0, 0, 0, jst), // Saturday
			expected:    time.Date(2024, 1, 13, 9, 0, 0, 0, jst),
		},
		{
			name:        "日曜の営業開始後は翌日の月曜朝になる",
			currentTime: time.Date(2024, 1, 14, 19, 0, 0, 0, jst), // Sunday
			expected:    time.Date(2024, 1, 15, 9, 0, 0, 0, jst),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, GetNextBusinessDayMorningWithConfig(tt.currentTime, config))
		})
	}
}
