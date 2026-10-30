package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"slack-review-notify/models"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func postSlackCommand(t *testing.T, router http.Handler, text string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{}
	form.Add("command", "/slack-review-notify")
	form.Add("text", text)
	form.Add("channel_id", "C12345")
	form.Add("user_id", "U12345")

	req, _ := http.NewRequest("POST", "/slack/command", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestSetNotifyOnHolidays_OnEnablesHolidayNotification(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)
	db.Create(&models.ChannelConfig{
		ID:             "holiday-1",
		SlackChannelID: "C12345",
		LabelName:      "needs-review",
		Language:       "ja",
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	})

	w := postSlackCommand(t, router, "set-notify-on-holidays on")

	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "ラベル「needs-review」の土日・祝日の通知を on に更新しました。")
	var config models.ChannelConfig
	db.Where("slack_channel_id = ? AND label_name = ?", "C12345", "needs-review").First(&config)
	assert.True(t, config.NotifyOnHolidays)
}

func TestSetNotifyOnHolidays_OffDisablesHolidayNotification(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)
	db.Create(&models.ChannelConfig{
		ID:               "holiday-2",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		Language:         "en",
		IsActive:         true,
		NotifyOnHolidays: true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	})

	w := postSlackCommand(t, router, "set-notify-on-holidays off")

	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), `Updated weekend and holiday notifications for label "needs-review" to off.`)
	var config models.ChannelConfig
	db.Where("slack_channel_id = ? AND label_name = ?", "C12345", "needs-review").First(&config)
	assert.False(t, config.NotifyOnHolidays)
}

func TestSetNotifyOnHolidays_CreatesConfigForUnknownLabel(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	w := postSlackCommand(t, router, "weekend-team set-notify-on-holidays on")

	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "ラベル「weekend-team」の土日・祝日の通知を on に設定しました。")
	var config models.ChannelConfig
	err := db.Where("slack_channel_id = ? AND label_name = ?", "C12345", "weekend-team").First(&config).Error
	assert.NoError(t, err)
	assert.True(t, config.NotifyOnHolidays)
	assert.True(t, config.IsActive)
	assert.Equal(t, "09:00", config.BusinessHoursStart)
	assert.Equal(t, "18:00", config.BusinessHoursEnd)
	assert.Equal(t, "Asia/Tokyo", config.Timezone)
}

func TestSetNotifyOnHolidays_ShowsUsageWithoutValue(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	w := postSlackCommand(t, router, "set-notify-on-holidays")

	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "例: /slack-review-notify needs-review set-notify-on-holidays on")
	var count int64
	db.Model(&models.ChannelConfig{}).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestSetNotifyOnHolidays_RejectsValueOtherThanOnOff(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	w := postSlackCommand(t, router, "set-notify-on-holidays yes")

	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "on または off で指定してください。")
	var count int64
	db.Model(&models.ChannelConfig{}).Count(&count)
	assert.Equal(t, int64(0), count)
}

func TestShowConfig_DisplaysNotifyOnHolidays(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)
	db.Create(&models.ChannelConfig{
		ID:                 "holiday-3",
		SlackChannelID:     "C12345",
		LabelName:          "needs-review",
		BusinessHoursStart: "09:00",
		BusinessHoursEnd:   "18:00",
		Language:           "ja",
		IsActive:           true,
		NotifyOnHolidays:   true,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	})

	w := postSlackCommand(t, router, "needs-review show")

	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "土日・祝日の通知: する")
}
