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

	if err := db.AutoMigrate(&models.ChannelConfig{}, &models.ReviewTask{}); err != nil {
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
			name:           "正常な営業開始時間設定",
			text:           "needs-review set-business-hours-start 09:00",
			channelID:      "C12345",
			expectedStart:  "09:00",
			expectedStatus: 200,
			expectsError:   false,
		},
		{
			name:           "デフォルトラベルでの営業開始時間設定",
			text:           "set-business-hours-start 10:00",
			channelID:      "C67890",
			expectedStart:  "10:00",
			expectedStatus: 200,
			expectsError:   false,
		},
		{
			name:           "無効な時間形式",
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
			name:             "正常なタイムゾーン設定（JST）",
			text:             "needs-review set-timezone Asia/Tokyo",
			channelID:        "C12345",
			expectedTimezone: "Asia/Tokyo",
			expectedStatus:   200,
			expectsError:     false,
		},
		{
			name:             "正常なタイムゾーン設定（UTC）",
			text:             "needs-review set-timezone UTC",
			channelID:        "C67890",
			expectedTimezone: "UTC",
			expectedStatus:   200,
			expectsError:     false,
		},
		{
			name:           "無効なタイムゾーン",
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
