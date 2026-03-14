package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"slack-review-notify/models"
	"slack-review-notify/services"

	"github.com/gin-gonic/gin"
	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
)

func TestMultipleLabelCommandHandler(t *testing.T) {
	tests := []struct {
		name           string
		command        string
		text           string
		expectedStatus int
		expectedBody   string
		setupConfig    *models.ChannelConfig
	}{
		{
			name:           "set-mention command with multiple labels",
			command:        "/slack-review-notify",
			text:           `"hoge-project,needs-review" set-mention @team`,
			expectedStatus: 200,
			expectedBody:   "ラベル「hoge-project,needs-review」のメンション先を <@team> に設定しました。",
		},
		{
			name:           "add-repo command with multiple labels",
			command:        "/slack-review-notify",
			text:           `"frontend,urgent,needs-review" add-repo owner/webapp`,
			expectedStatus: 200,
			expectedBody:   "ラベル「frontend,urgent,needs-review」の通知対象リポジトリに `owner/webapp` を追加しました。",
		},
		{
			name:           "Display multiple label settings",
			command:        "/slack-review-notify",
			text:           `"project-a,needs-review" show`,
			expectedStatus: 200,
			expectedBody:   "*このチャンネルのラベル「project-a,needs-review」のレビュー通知設定*",
			setupConfig: &models.ChannelConfig{
				SlackChannelID:   "C1234567890",
				LabelName:        "project-a,needs-review",
				DefaultMentionID: "@team",
				RepositoryList:   "owner/repo",
				IsActive:         true,
				CreatedAt:        time.Now(),
				UpdatedAt:        time.Now(),
			},
		},
		{
			name:           "Multiple label settings with spaces",
			command:        "/slack-review-notify",
			text:           `"project a, needs review, urgent" set-mention @team`,
			expectedStatus: 200,
			expectedBody:   "ラベル「project a, needs review, urgent」のメンション先を <@team> に設定しました。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			db := setupTestDB(t)
			gin.SetMode(gin.TestMode)
			services.IsTestMode = true

			// Create existing config if provided
			if tt.setupConfig != nil {
				tt.setupConfig.ID = "test-config-id"
				db.Create(tt.setupConfig)
			}

			// Create request body
			data := url.Values{}
			data.Set("command", tt.command)
			data.Set("text", tt.text)
			data.Set("channel_id", "C1234567890")
			data.Set("user_id", "U1234567890")

			// Create request
			req, _ := http.NewRequest("POST", "/slack/command", strings.NewReader(data.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			// Mock Slack signature verification (skipped in test mode)
			req.Header.Set("X-Slack-Signature", "test")
			req.Header.Set("X-Slack-Request-Timestamp", "1234567890")

			w := httptest.NewRecorder()

			// Create Gin router and execute request
			router := gin.Default()
			router.POST("/slack/command", HandleSlackCommand(db))
			router.ServeHTTP(w, req)

			// Verify HTTP status
			assert.Equal(t, tt.expectedStatus, w.Code)

			// Verify response content
			assert.Contains(t, w.Body.String(), tt.expectedBody)

			// Verify saved in database
			if strings.Contains(tt.text, "set-mention") || strings.Contains(tt.text, "add-repo") {
				var config models.ChannelConfig
				labelName := extractLabelFromText(tt.text)
				result := db.Where("slack_channel_id = ? AND label_name = ?", "C1234567890", labelName).First(&config)
				assert.NoError(t, result.Error, "Config was not saved to database")
			}

			// Cleanup
			gock.Off()
		})
	}
}

// extractLabelFromText is a helper function that extracts the label name from text
func extractLabelFromText(text string) string {
	parts := parseCommand(text)
	if len(parts) > 0 {
		return parts[0]
	}
	return "needs-review"
}

func TestMultipleLabelHelp(t *testing.T) {
	// Setup
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	// Request for the help command
	data := url.Values{}
	data.Set("command", "/slack-review-notify")
	data.Set("text", "help")
	data.Set("channel_id", "C1234567890")
	data.Set("user_id", "U1234567890")

	req, _ := http.NewRequest("POST", "/slack/command", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Slack-Signature", "test")
	req.Header.Set("X-Slack-Request-Timestamp", "1234567890")

	w := httptest.NewRecorder()

	router := gin.Default()
	router.POST("/slack/command", HandleSlackCommand(db))
	router.ServeHTTP(w, req)

	// Verify HTTP status
	assert.Equal(t, 200, w.Code)

	// Verify that the help message contains the multiple labels explanation
	body := w.Body.String()
	assert.Contains(t, body, "*複数ラベルAND条件の設定*")
	assert.Contains(t, body, "カンマ区切りで複数のラベルを指定")
	assert.Contains(t, body, "hoge-project,needs-review")
	assert.Contains(t, body, "全てのラベルが付いている場合のみ通知")
}
