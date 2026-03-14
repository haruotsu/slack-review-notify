package services

import (
	"errors"
	"slack-review-notify/models"
	"strconv"
	"strings"
	"time"

	"github.com/haruotsu/go-jpholiday/holiday"
)

// IsWithinBusinessHours determines whether the given time falls within business hours
func IsWithinBusinessHours(config *models.ChannelConfig, currentTime time.Time) bool {
	// If no business hours are configured, always return true (send notifications)
	if config == nil || config.BusinessHoursStart == "" || config.BusinessHoursEnd == "" {
		return true
	}

	timezone := config.Timezone
	if timezone == "" {
		timezone = "Asia/Tokyo"
	}

	// Convert to the configured timezone
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc, _ = time.LoadLocation("Asia/Tokyo")
	}

	localTime := currentTime.In(loc)

	// Check for weekends
	weekday := localTime.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return false
	}

	// Check for Japanese holidays if the timezone is Asia/Tokyo
	if timezone == "Asia/Tokyo" && isJapaneseHoliday(localTime) {
		return false
	}

	currentHour := localTime.Hour()
	currentMin := localTime.Minute()
	currentMinutes := currentHour*60 + currentMin

	// Parse business hours start and end times
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

// parseBusinessHoursTime parses a time string (HH:MM) into hours and minutes
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

// isJapaneseHoliday determines whether the given date is a Japanese public holiday
func isJapaneseHoliday(t time.Time) bool {
	return holiday.IsHoliday(t)
}
