package services

import (
	"errors"
	"slack-review-notify/models"
	"strconv"
	"strings"
	"time"

	"github.com/yut-kt/goholiday"
	"github.com/yut-kt/goholiday/nholidays/jp"
)

// IsWithinBusinessHours は指定された時刻が営業時間内かどうかを判定する
func IsWithinBusinessHours(config *models.ChannelConfig, currentTime time.Time) bool {
	// 営業時間設定がない場合は常にtrue（通知する）
	if config == nil || config.BusinessHoursStart == "" || config.BusinessHoursEnd == "" {
		return true
	}

	timezone := config.Timezone
	if timezone == "" {
		timezone = "Asia/Tokyo"
	}

	// タイムゾーンに変換
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc, _ = time.LoadLocation("Asia/Tokyo")
	}

	localTime := currentTime.In(loc)

	// 日本のタイムゾーンの場合は祝日チェックを行う
	if timezone == "Asia/Tokyo" && isJapaneseHoliday(localTime) {
		return false
	}

	currentHour := localTime.Hour()
	currentMin := localTime.Minute()
	currentMinutes := currentHour*60 + currentMin

	// 営業開始・終了時刻を解析
	startHour, startMin, err := parseBusinessHoursTime(config.BusinessHoursStart)
	if err != nil {
		return true
	}

	endHour, endMin, err := parseBusinessHoursTime(config.BusinessHoursEnd)
	if err != nil {
		return true
	}

	startMinutes := startHour*60 + startMin
	endMinutes := endHour*60 + endMin

	if startMinutes < endMinutes {
		return currentMinutes >= startMinutes && currentMinutes < endMinutes
	}
	return currentMinutes >= startMinutes || currentMinutes < endMinutes
}

// parseBusinessHoursTime は時刻文字列（HH:MM）を時間と分に解析する
func parseBusinessHoursTime(timeStr string) (int, int, error) {
	if timeStr == "" {
		return 0, 0, errors.New("empty time string")
	}

	parts := strings.Split(timeStr, ":")
	if len(parts) != 2 {
		return 0, 0, errors.New("invalid time format")
	}

	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, errors.New("invalid hour")
	}

	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, errors.New("invalid minute")
	}

	return hour, minute, nil
}

// isJapaneseHoliday は指定された日付が日本の祝日かどうかを判定する
func isJapaneseHoliday(t time.Time) bool {
	// 日本の祝日スケジュールを使用
	jpSchedule := jp.New()
	jpHoliday := goholiday.New(jpSchedule)
	return jpHoliday.IsHoliday(t)
}
