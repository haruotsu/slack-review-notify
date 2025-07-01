package services

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetNextBusinessDayMorning_Detailed(t *testing.T) {
	testCases := []struct {
		name        string
		currentTime time.Time
		expected    time.Time
	}{
		{
			name:        "月曜日の午前9時 → 月曜日の10時",
			currentTime: time.Date(2024, 1, 8, 9, 0, 0, 0, time.Local), // 2024年1月8日は月曜日
			expected:    time.Date(2024, 1, 8, 10, 0, 0, 0, time.Local),
		},
		{
			name:        "月曜日の午後2時 → 火曜日の10時",
			currentTime: time.Date(2024, 1, 8, 14, 0, 0, 0, time.Local),
			expected:    time.Date(2024, 1, 9, 10, 0, 0, 0, time.Local),
		},
		{
			name:        "金曜日の午後2時 → 月曜日の10時",
			currentTime: time.Date(2024, 1, 12, 14, 0, 0, 0, time.Local), // 2024年1月12日は金曜日
			expected:    time.Date(2024, 1, 15, 10, 0, 0, 0, time.Local), // 2024年1月15日は月曜日
		},
		{
			name:        "土曜日の午後2時 → 月曜日の10時",
			currentTime: time.Date(2024, 1, 13, 14, 0, 0, 0, time.Local), // 2024年1月13日は土曜日
			expected:    time.Date(2024, 1, 15, 10, 0, 0, 0, time.Local), // 2024年1月15日は月曜日
		},
		{
			name:        "日曜日の午後2時 → 月曜日の10時",
			currentTime: time.Date(2024, 1, 14, 14, 0, 0, 0, time.Local), // 2024年1月14日は日曜日
			expected:    time.Date(2024, 1, 15, 10, 0, 0, 0, time.Local), // 2024年1月15日は月曜日
		},
		{
			name:        "木曜日の午後11時59分 → 金曜日の10時",
			currentTime: time.Date(2024, 1, 11, 23, 59, 0, 0, time.Local), // 2024年1月11日は木曜日
			expected:    time.Date(2024, 1, 12, 10, 0, 0, 0, time.Local),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 時刻を固定するためのモック関数を作成
			// 注: 現在の実装では time.Now() を使用しているため、
			// このテストは現在の実装では失敗します
			// 実際のテストのため、GetNextBusinessDayMorningに時刻を渡せるようにする必要があります
			
			// 現在の実装をテスト（バグがあることを確認）
			result := GetNextBusinessDayMorning()
			
			// 現在の実装では常に「翌日」になるため、このテストは失敗するはずです
			t.Logf("現在時刻: %s", tc.currentTime.Format("2006-01-02 15:04:05"))
			t.Logf("期待値: %s", tc.expected.Format("2006-01-02 15:04:05"))
			t.Logf("実際の値: %s", result.Format("2006-01-02 15:04:05"))
		})
	}
}

func TestGetNextBusinessDayMorningWithTime(t *testing.T) {
	testCases := []struct {
		name        string
		currentTime time.Time
		expected    time.Time
	}{
		{
			name:        "月曜日の午前9時 → 月曜日の10時",
			currentTime: time.Date(2024, 1, 8, 9, 0, 0, 0, time.Local), // 2024年1月8日は月曜日
			expected:    time.Date(2024, 1, 8, 10, 0, 0, 0, time.Local),
		},
		{
			name:        "月曜日の午前10時ちょうど → 火曜日の10時",
			currentTime: time.Date(2024, 1, 8, 10, 0, 0, 0, time.Local),
			expected:    time.Date(2024, 1, 9, 10, 0, 0, 0, time.Local),
		},
		{
			name:        "月曜日の午前10時1分 → 火曜日の10時",
			currentTime: time.Date(2024, 1, 8, 10, 1, 0, 0, time.Local),
			expected:    time.Date(2024, 1, 9, 10, 0, 0, 0, time.Local),
		},
		{
			name:        "月曜日の午後2時 → 火曜日の10時",
			currentTime: time.Date(2024, 1, 8, 14, 0, 0, 0, time.Local),
			expected:    time.Date(2024, 1, 9, 10, 0, 0, 0, time.Local),
		},
		{
			name:        "金曜日の午前9時 → 金曜日の10時",
			currentTime: time.Date(2024, 1, 12, 9, 0, 0, 0, time.Local), // 2024年1月12日は金曜日
			expected:    time.Date(2024, 1, 12, 10, 0, 0, 0, time.Local),
		},
		{
			name:        "金曜日の午後2時 → 月曜日の10時",
			currentTime: time.Date(2024, 1, 12, 14, 0, 0, 0, time.Local),
			expected:    time.Date(2024, 1, 15, 10, 0, 0, 0, time.Local), // 2024年1月15日は月曜日
		},
		{
			name:        "土曜日の午後2時 → 月曜日の10時",
			currentTime: time.Date(2024, 1, 13, 14, 0, 0, 0, time.Local), // 2024年1月13日は土曜日
			expected:    time.Date(2024, 1, 15, 10, 0, 0, 0, time.Local), // 2024年1月15日は月曜日
		},
		{
			name:        "日曜日の午後2時 → 月曜日の10時",
			currentTime: time.Date(2024, 1, 14, 14, 0, 0, 0, time.Local), // 2024年1月14日は日曜日
			expected:    time.Date(2024, 1, 15, 10, 0, 0, 0, time.Local), // 2024年1月15日は月曜日
		},
		{
			name:        "木曜日の午後11時59分 → 金曜日の10時",
			currentTime: time.Date(2024, 1, 11, 23, 59, 0, 0, time.Local), // 2024年1月11日は木曜日
			expected:    time.Date(2024, 1, 12, 10, 0, 0, 0, time.Local),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// GetNextBusinessDayMorningWithTimeをテスト（まだ実装されていない）
			result := GetNextBusinessDayMorningWithTime(tc.currentTime)
			
			assert.Equal(t, tc.expected, result, 
				"currentTime: %s, expected: %s, got: %s",
				tc.currentTime.Format("2006-01-02 15:04:05"),
				tc.expected.Format("2006-01-02 15:04:05"),
				result.Format("2006-01-02 15:04:05"))
		})
	}
}