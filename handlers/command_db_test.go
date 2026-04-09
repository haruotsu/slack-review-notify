package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"slack-review-notify/models"
	"slack-review-notify/services"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupCommandIntegrationTestDB creates an in-memory database for integration tests
func setupCommandIntegrationTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("fail to open test db: %v", err)
	}

	if err := db.AutoMigrate(&models.ChannelConfig{}, &models.ReviewTask{}, &models.UserMapping{}, &models.ReviewerAvailability{}); err != nil {
		t.Fatalf("fail to migrate test db: %v", err)
	}

	return db
}

func setupHTTPRequest(t *testing.T, text, channelID string) *http.Request {
	data := url.Values{}
	data.Set("command", "/slack-review-notify")
	data.Set("text", text)
	data.Set("channel_id", channelID)
	data.Set("user_id", "U12345")

	req, err := http.NewRequest("POST", "/slack/command", strings.NewReader(data.Encode()))
	if err != nil {
		t.Fatalf("fail to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return req
}

func TestSetBusinessHoursStartCommand_Integration(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	tests := []struct {
		name           string
		text           string
		channelID      string
		expectedStart  string
		expectedStatus int
		expectsError   bool
	}{
		{
			name:           "Valid business hours start setting",
			text:           "needs-review set-business-hours-start 09:00",
			channelID:      "C12345",
			expectedStart:  "09:00",
			expectedStatus: 200,
			expectsError:   false,
		},
		{
			name:           "Business hours start setting with default label",
			text:           "set-business-hours-start 10:00",
			channelID:      "C67890",
			expectedStart:  "10:00",
			expectedStatus: 200,
			expectsError:   false,
		},
		{
			name:           "Invalid time format",
			text:           "needs-review set-business-hours-start 25:00",
			channelID:      "C12345",
			expectedStatus: 200,
			expectsError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			req := setupHTTPRequest(t, tt.text, tt.channelID)
			w := httptest.NewRecorder()

			router := gin.New()
			router.POST("/slack/command", HandleSlackCommand(db))
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectsError {
				var config models.ChannelConfig
				labelName := "needs-review"

				err := db.Where("slack_channel_id = ? AND label_name = ?", tt.channelID, labelName).First(&config).Error
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStart, config.BusinessHoursStart)
			}
		})
	}
}

func TestSetTimezoneCommand_Integration(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	tests := []struct {
		name             string
		text             string
		channelID        string
		expectedTimezone string
		expectedStatus   int
		expectsError     bool
	}{
		{
			name:             "Valid timezone setting (JST)",
			text:             "needs-review set-timezone Asia/Tokyo",
			channelID:        "C12345",
			expectedTimezone: "Asia/Tokyo",
			expectedStatus:   200,
			expectsError:     false,
		},
		{
			name:             "Valid timezone setting (UTC)",
			text:             "needs-review set-timezone UTC",
			channelID:        "C67890",
			expectedTimezone: "UTC",
			expectedStatus:   200,
			expectsError:     false,
		},
		{
			name:           "Invalid timezone",
			text:           "needs-review set-timezone Invalid/Timezone",
			channelID:      "C12345",
			expectedStatus: 200,
			expectsError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			req := setupHTTPRequest(t, tt.text, tt.channelID)
			w := httptest.NewRecorder()

			router := gin.New()
			router.POST("/slack/command", HandleSlackCommand(db))
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectsError {
				var config models.ChannelConfig
				labelName := "needs-review"

				err := db.Where("slack_channel_id = ? AND label_name = ?", tt.channelID, labelName).First(&config).Error
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedTimezone, config.Timezone)
			}
		})
	}
}

func TestSetRequiredApprovals_Integration(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	tests := []struct {
		name              string
		text              string
		channelID         string
		expectedApprovals int
		expectedStatus    int
		expectsError      bool
	}{
		{
			name:              "Valid required approvals setting",
			text:              "needs-review set-required-approvals 2",
			channelID:         "C_APPROVALS",
			expectedApprovals: 2,
			expectedStatus:    200,
			expectsError:      false,
		},
		{
			name:              "Required approvals setting with default label",
			text:              "set-required-approvals 3",
			channelID:         "C_APPROVALS2",
			expectedApprovals: 3,
			expectedStatus:    200,
			expectsError:      false,
		},
		{
			name:           "Out of range value (0)",
			text:           "needs-review set-required-approvals 0",
			channelID:      "C_APPROVALS",
			expectedStatus: 200,
			expectsError:   true,
		},
		{
			name:           "Out of range value (11)",
			text:           "needs-review set-required-approvals 11",
			channelID:      "C_APPROVALS",
			expectedStatus: 200,
			expectsError:   true,
		},
		{
			name:           "Invalid value (string)",
			text:           "needs-review set-required-approvals abc",
			channelID:      "C_APPROVALS",
			expectedStatus: 200,
			expectsError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			req := setupHTTPRequest(t, tt.text, tt.channelID)
			w := httptest.NewRecorder()

			router := gin.New()
			router.POST("/slack/command", HandleSlackCommand(db))
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if !tt.expectsError {
				var config models.ChannelConfig
				labelName := "needs-review"

				err := db.Where("slack_channel_id = ? AND label_name = ?", tt.channelID, labelName).First(&config).Error
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedApprovals, config.RequiredApprovals)
			}

			if tt.expectsError {
				body := w.Body.String()
				assert.Contains(t, body, "1〜10の整数")
			}
		})
	}
}

func TestSetAway_Integration(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	tests := []struct {
		name           string
		text           string
		channelID      string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "Indefinite leave setting",
			text:           "set-away <@U12345>",
			channelID:      "C12345",
			expectedStatus: 200,
			expectedBody:   "休暇に設定しました",
		},
		{
			name:           "Leave setting with date",
			text:           "set-away <@U67890> until 2099-12-31 reason 休暇",
			channelID:      "C12345",
			expectedStatus: 200,
			expectedBody:   "2099-12-31 まで",
		},
		{
			name:           "Leave setting with reason only",
			text:           "set-away <@U11111> reason 育児休業",
			channelID:      "C12345",
			expectedStatus: 200,
			expectedBody:   "育児休業",
		},
		{
			name:           "Leave setting with from and until",
			text:           "set-away <@U22222> from 2099-11-01 until 2099-12-31 reason 長期休暇",
			channelID:      "C12345",
			expectedStatus: 200,
			expectedBody:   "2099-11-01 ~ 2099-12-31",
		},
		{
			name:           "Leave setting with on (single day)",
			text:           "set-away <@U33333> on 2099-06-15 reason 有給休暇",
			channelID:      "C12345",
			expectedStatus: 200,
			expectedBody:   "2099-06-15",
		},
		{
			name:           "Past date is rejected",
			text:           "set-away <@U99999> until 2020-01-01",
			channelID:      "C12345",
			expectedStatus: 200,
			expectedBody:   "過去の日付は指定できません",
		},
		{
			name:           "Past from date is rejected",
			text:           "set-away <@U99999> from 2020-01-01 until 2099-12-31",
			channelID:      "C12345",
			expectedStatus: 200,
			expectedBody:   "過去の日付は指定できません",
		},
		{
			name:           "from after until is rejected",
			text:           "set-away <@U99999> from 2099-12-31 until 2099-01-01",
			channelID:      "C12345",
			expectedStatus: 200,
			expectedBody:   "開始日（from）は終了日（until）より前に指定してください",
		},
		{
			name:           "on combined with from is rejected",
			text:           "set-away <@U99999> on 2099-06-01 from 2099-05-01",
			channelID:      "C12345",
			expectedStatus: 200,
			expectedBody:   "同時に使えません",
		},
		{
			name:           "from without date value is rejected",
			text:           "set-away <@U99999> from",
			channelID:      "C12345",
			expectedStatus: 200,
			expectedBody:   "キーワードの後に日付を指定してください",
		},
		{
			name:           "No user specified",
			text:           "set-away",
			channelID:      "C12345",
			expectedStatus: 200,
			expectedBody:   "休暇に設定するユーザーを指定してください",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			req := setupHTTPRequest(t, tt.text, tt.channelID)
			w := httptest.NewRecorder()

			router := gin.New()
			router.POST("/slack/command", HandleSlackCommand(db))
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Contains(t, w.Body.String(), tt.expectedBody)
		})
	}

	// Upsert test: setting the same user again should result in 1 record
	var count int64
	db.Model(&models.ReviewerAvailability{}).Where("slack_user_id = ?", "U12345").Count(&count)
	assert.Equal(t, int64(1), count)
}

func TestUnsetAway_Integration(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	// First, set the leave status
	gin.SetMode(gin.TestMode)
	req := setupHTTPRequest(t, "set-away <@UAWAY>", "C12345")
	w := httptest.NewRecorder()
	router := gin.New()
	router.POST("/slack/command", HandleSlackCommand(db))
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)

	// Remove leave status
	req2 := setupHTTPRequest(t, "unset-away <@UAWAY>", "C12345")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, 200, w2.Code)
	assert.Contains(t, w2.Body.String(), "休暇を解除しました")

	// Verify no record exists in the DB
	var count int64
	db.Model(&models.ReviewerAvailability{}).Where("slack_user_id = ?", "UAWAY").Count(&count)
	assert.Equal(t, int64(0), count)

	// Removing leave for a non-existent user
	req3 := setupHTTPRequest(t, "unset-away <@UNOTEXIST>", "C12345")
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	assert.Equal(t, 200, w3.Code)
	assert.Contains(t, w3.Body.String(), "休暇に設定されていません")
}

func TestShowAvailability_Integration(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)

	services.IsTestMode = true
	defer func() {
		services.IsTestMode = false
	}()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", HandleSlackCommand(db))

	// When no one is on leave
	req := setupHTTPRequest(t, "show-availability", "C12345")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "休暇中・予約中のユーザーはいません")

	// Add users on leave
	req2 := setupHTTPRequest(t, "set-away <@USHOW1> reason 休暇", "C12345")
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, 200, w2.Code)

	req3 := setupHTTPRequest(t, "set-away <@USHOW2> until 2099-12-31 reason 育児休業", "C12345")
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	assert.Equal(t, 200, w3.Code)

	// Add a scheduled (future) leave
	req3b := setupHTTPRequest(t, "set-away <@USHOW3> from 2099-06-01 until 2099-06-15 reason 予定休暇", "C12345")
	w3b := httptest.NewRecorder()
	router.ServeHTTP(w3b, req3b)
	assert.Equal(t, 200, w3b.Code)

	// Verify the leave list
	req4 := setupHTTPRequest(t, "show-availability", "C12345")
	w4 := httptest.NewRecorder()
	router.ServeHTTP(w4, req4)
	assert.Equal(t, 200, w4.Code)
	body := w4.Body.String()
	assert.Contains(t, body, "休暇中・予約中のユーザー")
	assert.Contains(t, body, "USHOW1")
	assert.Contains(t, body, "USHOW2")
	assert.Contains(t, body, "休暇中")   // status label for currently away
	assert.Contains(t, body, "予約中")   // status label for scheduled
	assert.Contains(t, body, "USHOW3")
	assert.Contains(t, body, "予定休暇")
}
