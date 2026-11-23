package services

import (
	"testing"
	"time"

	"slack-review-notify/models"
)

func TestIsJapaneseHoliday(t *testing.T) {
	jst, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("failed to load Asia/Tokyo: %v", err)
	}

	tests := []struct {
		name string
		date time.Time
		want bool
	}{
		{
			name: "2025年以降の祝日（2026年の成人の日）を祝日と判定する",
			date: time.Date(2026, 1, 12, 12, 0, 0, 0, jst),
			want: true,
		},
		{
			name: "振替休日（2025年の勤労感謝の日 振替休日）を祝日と判定する",
			date: time.Date(2025, 11, 24, 12, 0, 0, 0, jst),
			want: true,
		},
		{
			name: "国民の休日（2026年9月22日）を祝日と判定する",
			date: time.Date(2026, 9, 22, 12, 0, 0, 0, jst),
			want: true,
		},
		{
			name: "祝日ではない平日（2026年11月24日）は祝日と判定しない",
			date: time.Date(2026, 11, 24, 12, 0, 0, 0, jst),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isJapaneseHoliday(tt.date); got != tt.want {
				t.Errorf("isJapaneseHoliday(%s) = %v, want %v", tt.date.Format("2006-01-02"), got, tt.want)
			}
		})
	}
}

func TestIsWithinBusinessHours_HolidayAcrossUTCDateBoundary(t *testing.T) {
	config := &models.ChannelConfig{
		BusinessHoursStart: "22:00",
		BusinessHoursEnd:   "06:00",
		Timezone:           "Asia/Tokyo",
	}

	// 2026-01-11 15:30 UTC は JST で 2026-01-12（成人の日）00:30
	current := time.Date(2026, 1, 11, 15, 30, 0, 0, time.UTC)

	if IsWithinBusinessHours(config, current) {
		t.Errorf("UTC では前日でも JST で祝日なら営業時間外として扱うべき")
	}
}
