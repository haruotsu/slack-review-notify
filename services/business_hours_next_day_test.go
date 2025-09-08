package services

import (
	"slack-review-notify/models"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// GetNextBusinessDayMorningWithConfig関数のテスト
func TestGetNextBusinessDayMorningWithConfig(t *testing.T) {
	// JST タイムゾーンを取得
	jst, err := time.LoadLocation("Asia/Tokyo")
	assert.NoError(t, err)

	// 営業時間設定
	config := &models.ChannelConfig{
		BusinessHoursStart: "09:00",
		BusinessHoursEnd:   "18:00",
		Timezone:          "Asia/Tokyo",
	}

	testCases := []struct {
		name        string
		currentTime time.Time
		config      *models.ChannelConfig
		expected    time.Time
	}{
		{
			name:        "月曜8時_同日9時",
			currentTime: time.Date(2024, 1, 15, 8, 0, 0, 0, jst), // 月曜8時（JST）
			config:      config,
			expected:    time.Date(2024, 1, 15, 9, 0, 0, 0, jst), // 同日9時
		},
		{
			name:        "月曜12時_翌日9時",
			currentTime: time.Date(2024, 1, 15, 12, 0, 0, 0, jst), // 月曜12時（JST）
			config:      config,
			expected:    time.Date(2024, 1, 16, 9, 0, 0, 0, jst), // 翌日9時
		},
		{
			name:        "金曜20時_翌週月曜9時",
			currentTime: time.Date(2024, 1, 19, 20, 0, 0, 0, jst), // 金曜20時（JST）
			config:      config,
			expected:    time.Date(2024, 1, 22, 9, 0, 0, 0, jst), // 翌週月曜9時
		},
		{
			name:        "土曜14時_月曜9時",
			currentTime: time.Date(2024, 1, 20, 14, 0, 0, 0, jst), // 土曜14時（JST）
			config:      config,
			expected:    time.Date(2024, 1, 22, 9, 0, 0, 0, jst), // 月曜9時
		},
		{
			name:        "日曜14時_月曜9時",
			currentTime: time.Date(2024, 1, 21, 14, 0, 0, 0, jst), // 日曜14時（JST）
			config:      config,
			expected:    time.Date(2024, 1, 22, 9, 0, 0, 0, jst), // 月曜9時
		},
		{
			name:        "設定がnil_デフォルト10時",
			currentTime: time.Date(2024, 1, 15, 8, 0, 0, 0, jst), // 月曜8時（JST）
			config:      nil,
			expected:    time.Date(2024, 1, 15, 10, 0, 0, 0, jst), // 同日10時（デフォルト）
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GetNextBusinessDayMorningWithConfig(tc.currentTime, tc.config)
			assert.Equal(t, tc.expected, result,
				"currentTime: %s, expected: %s, got: %s",
				tc.currentTime.Format("2006-01-02 15:04:05"),
				tc.expected.Format("2006-01-02 15:04:05"),
				result.Format("2006-01-02 15:04:05"))
		})
	}
}

// 異なる営業時間設定のテスト
func TestGetNextBusinessDayMorningWithConfig_DifferentBusinessHours(t *testing.T) {
	// JST タイムゾーンを取得
	jst, err := time.LoadLocation("Asia/Tokyo")
	assert.NoError(t, err)

	testCases := []struct {
		name          string
		businessStart string
		currentTime   time.Time
		expectedHour  int
		expectedMin   int
	}{
		{
			name:          "営業開始8時半",
			businessStart: "08:30",
			currentTime:   time.Date(2024, 1, 15, 8, 0, 0, 0, jst), // 月曜8時
			expectedHour:  8,
			expectedMin:   30,
		},
		{
			name:          "営業開始11時",
			businessStart: "11:00",
			currentTime:   time.Date(2024, 1, 15, 10, 0, 0, 0, jst), // 月曜10時
			expectedHour:  11,
			expectedMin:   0,
		},
		{
			name:          "営業開始7時",
			businessStart: "07:00",
			currentTime:   time.Date(2024, 1, 15, 6, 30, 0, 0, jst), // 月曜6時半
			expectedHour:  7,
			expectedMin:   0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := &models.ChannelConfig{
				BusinessHoursStart: tc.businessStart,
				BusinessHoursEnd:   "18:00",
				Timezone:          "Asia/Tokyo",
			}

			result := GetNextBusinessDayMorningWithConfig(tc.currentTime, config)
			
			assert.Equal(t, tc.expectedHour, result.Hour(), 
				"Expected hour %d, got %d", tc.expectedHour, result.Hour())
			assert.Equal(t, tc.expectedMin, result.Minute(), 
				"Expected minute %d, got %d", tc.expectedMin, result.Minute())
		})
	}
}

// 無効な営業時間設定のテスト
func TestGetNextBusinessDayMorningWithConfig_InvalidConfig(t *testing.T) {
	jst, err := time.LoadLocation("Asia/Tokyo")
	assert.NoError(t, err)

	currentTime := time.Date(2024, 1, 15, 8, 0, 0, 0, jst) // 月曜8時

	testCases := []struct {
		name   string
		config *models.ChannelConfig
	}{
		{
			name: "無効な時刻形式",
			config: &models.ChannelConfig{
				BusinessHoursStart: "invalid",
				BusinessHoursEnd:   "18:00",
				Timezone:          "Asia/Tokyo",
			},
		},
		{
			name: "空の営業開始時刻",
			config: &models.ChannelConfig{
				BusinessHoursStart: "",
				BusinessHoursEnd:   "18:00",
				Timezone:          "Asia/Tokyo",
			},
		},
		{
			name: "無効な時間",
			config: &models.ChannelConfig{
				BusinessHoursStart: "25:00",
				BusinessHoursEnd:   "18:00",
				Timezone:          "Asia/Tokyo",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GetNextBusinessDayMorningWithConfig(currentTime, tc.config)
			
			// 無効な設定の場合、デフォルト（10:00）が使用されることを確認
			assert.Equal(t, 10, result.Hour(), 
				"Invalid config should fallback to default hour 10, got %d", result.Hour())
			assert.Equal(t, 0, result.Minute(), 
				"Invalid config should fallback to default minute 0, got %d", result.Minute())
		})
	}
}

// 異なるタイムゾーンのテスト
func TestGetNextBusinessDayMorningWithConfig_DifferentTimezones(t *testing.T) {
	// UTC タイムゾーンでの時刻
	utc, err := time.LoadLocation("UTC")
	assert.NoError(t, err)
	
	currentTimeUTC := time.Date(2024, 1, 15, 23, 0, 0, 0, utc) // UTC 23:00（JST 8:00）

	// JST設定
	configJST := &models.ChannelConfig{
		BusinessHoursStart: "09:00",
		BusinessHoursEnd:   "18:00",
		Timezone:          "Asia/Tokyo",
	}

	result := GetNextBusinessDayMorningWithConfig(currentTimeUTC, configJST)

	// UTC 23:00はJST 8:00に相当（翌日）
	// 1月15日 UTC 23:00 = 1月16日 JST 8:00
	// 営業開始時刻9:00より前なので、同日（1月16日）の9:00が返されるべき
	expectedJST, _ := time.LoadLocation("Asia/Tokyo")
	expected := time.Date(2024, 1, 16, 9, 0, 0, 0, expectedJST) // JST 9:00

	// デバッグ情報
	jstTime := currentTimeUTC.In(expectedJST)
	t.Logf("UTC input: %s", currentTimeUTC.Format("2006-01-02 15:04:05 MST"))
	t.Logf("JST time: %s", jstTime.Format("2006-01-02 15:04:05 MST"))
	t.Logf("Expected: %s", expected.Format("2006-01-02 15:04:05 MST"))
	t.Logf("Got:      %s", result.Format("2006-01-02 15:04:05 MST"))

	assert.Equal(t, expected, result,
		"Expected JST time %s, got %s", 
		expected.Format("2006-01-02 15:04:05 MST"), 
		result.Format("2006-01-02 15:04:05 MST"))
}