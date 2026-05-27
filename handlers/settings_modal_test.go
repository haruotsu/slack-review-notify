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
	"gorm.io/gorm"
)

func setupActionRouter(db *gorm.DB) *gin.Engine {
	services.IsTestMode = true
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/slack/actions", HandleSlackAction(db))
	return r
}

func postPayload(t *testing.T, router http.Handler, payloadJSON string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{}
	form.Add("payload", payloadJSON)
	req := httptest.NewRequest("POST", "/slack/actions", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// TestHandleSlackCommand_HelpHasOpenSettingsButton verifies the help response
// includes a button with action_id=open_settings so users can launch the modal.
func TestHandleSlackCommand_HelpHasOpenSettingsButton(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	form := url.Values{}
	form.Add("command", "/slack-review-notify")
	form.Add("text", "help")
	form.Add("channel_id", "C12345")
	form.Add("user_id", "U12345")

	req := httptest.NewRequest("POST", "/slack/command", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "open_settings", "help response must contain the open_settings action_id")
	assert.Contains(t, body, "Review通知Bot設定コマンド", "help body must still include the help text")
}

// TestHandleSlackAction_OpenSettings verifies the block_actions payload with action_id=open_settings
// is accepted (would call views.open in production; IsTestMode skips the call).
func TestHandleSlackAction_OpenSettings(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)

	// Existing config so the modal can be pre-filled
	db.Create(&models.ChannelConfig{
		ID:               "c1",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U99999",
		IsActive:         true,
	})

	payload := `{
		"type": "block_actions",
		"trigger_id": "trigger-xyz",
		"user": {"id": "U12345"},
		"actions": [{"action_id": "open_settings", "value": "needs-review"}],
		"container": {"channel_id": "C12345"},
		"message": {"ts": "1234.5"}
	}`

	w := postPayload(t, router, payload)
	assert.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
}

// TestHandleSlackAction_ViewSubmission_CreatesConfig verifies a view_submission payload
// upserts the ChannelConfig identified by (channel_id, label_name).
func TestHandleSlackAction_ViewSubmission_CreatesConfig(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)

	payload := `{
		"type": "view_submission",
		"user": {"id": "U12345"},
		"view": {
			"callback_id": "settings_modal",
			"private_metadata": "C99999",
			"state": {
				"values": {
					"label_name": {"label_name": {"type": "plain_text_input", "value": "new-label"}},
					"default_mention_id": {"default_mention_id": {"type": "plain_text_input", "value": "U777"}},
					"reviewer_list": {"reviewer_list": {"type": "plain_text_input", "value": "U1, U2"}},
					"repository_list": {"repository_list": {"type": "plain_text_input", "value": "owner/repo"}},
					"reviewer_reminder_interval": {"reviewer_reminder_interval": {"type": "plain_text_input", "value": "60"}},
					"business_hours_start": {"business_hours_start": {"type": "plain_text_input", "value": "10:00"}},
					"business_hours_end": {"business_hours_end": {"type": "plain_text_input", "value": "19:00"}},
					"timezone": {"timezone": {"type": "plain_text_input", "value": "UTC"}},
					"required_approvals": {"required_approvals": {"type": "plain_text_input", "value": "2"}},
					"language": {"language": {"type": "static_select", "selected_option": {"value": "en", "text": {"text": "English"}}}},
					"is_active": {"is_active": {"type": "static_select", "selected_option": {"value": "true", "text": {"text": "Active"}}}}
				}
			}
		}
	}`

	w := postPayload(t, router, payload)
	assert.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var cfg models.ChannelConfig
	if err := db.Where("slack_channel_id = ? AND label_name = ?", "C99999", "new-label").First(&cfg).Error; err != nil {
		t.Fatalf("expected config to be created, got err: %v", err)
	}
	assert.Equal(t, "U777", cfg.DefaultMentionID)
	assert.Equal(t, "U1,U2", cfg.ReviewerList)
	assert.Equal(t, "owner/repo", cfg.RepositoryList)
	assert.Equal(t, 60, cfg.ReviewerReminderInterval)
	assert.Equal(t, "10:00", cfg.BusinessHoursStart)
	assert.Equal(t, "19:00", cfg.BusinessHoursEnd)
	assert.Equal(t, "UTC", cfg.Timezone)
	assert.Equal(t, 2, cfg.RequiredApprovals)
	assert.Equal(t, "en", cfg.Language)
	assert.True(t, cfg.IsActive)
}

// TestHandleSlackAction_ViewSubmission_UpdatesConfig verifies an existing label config is updated.
func TestHandleSlackAction_ViewSubmission_UpdatesConfig(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)

	db.Create(&models.ChannelConfig{
		ID:                       "existing",
		SlackChannelID:           "C99999",
		LabelName:                "needs-review",
		DefaultMentionID:         "Uold",
		ReviewerReminderInterval: 15,
		BusinessHoursStart:       "08:00",
		BusinessHoursEnd:         "17:00",
		Timezone:                 "Asia/Tokyo",
		RequiredApprovals:        1,
		Language:                 "ja",
		IsActive:                 true,
	})

	payload := `{
		"type": "view_submission",
		"user": {"id": "U12345"},
		"view": {
			"callback_id": "settings_modal",
			"private_metadata": "C99999",
			"state": {
				"values": {
					"label_name": {"label_name": {"value": "needs-review"}},
					"default_mention_id": {"default_mention_id": {"value": "Unew"}},
					"reviewer_list": {"reviewer_list": {"value": ""}},
					"repository_list": {"repository_list": {"value": ""}},
					"reviewer_reminder_interval": {"reviewer_reminder_interval": {"value": "45"}},
					"business_hours_start": {"business_hours_start": {"value": "09:30"}},
					"business_hours_end": {"business_hours_end": {"value": "18:30"}},
					"timezone": {"timezone": {"value": "Asia/Tokyo"}},
					"required_approvals": {"required_approvals": {"value": "3"}},
					"language": {"language": {"selected_option": {"value": "ja"}}},
					"is_active": {"is_active": {"selected_option": {"value": "false"}}}
				}
			}
		}
	}`

	w := postPayload(t, router, payload)
	assert.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var cfg models.ChannelConfig
	if err := db.Where("id = ?", "existing").First(&cfg).Error; err != nil {
		t.Fatalf("config not found: %v", err)
	}
	assert.Equal(t, "Unew", cfg.DefaultMentionID)
	assert.Equal(t, 45, cfg.ReviewerReminderInterval)
	assert.Equal(t, "09:30", cfg.BusinessHoursStart)
	assert.Equal(t, "18:30", cfg.BusinessHoursEnd)
	assert.Equal(t, 3, cfg.RequiredApprovals)
	assert.False(t, cfg.IsActive)

	// Only one row should exist (no duplicate from upsert)
	var count int64
	db.Model(&models.ChannelConfig{}).Where("slack_channel_id = ? AND label_name = ?", "C99999", "needs-review").Count(&count)
	assert.Equal(t, int64(1), count)
}

// TestHandleSlackAction_ViewSubmission_ValidationError verifies invalid input returns a
// response_action: errors body so Slack shows inline errors.
func TestHandleSlackAction_ViewSubmission_ValidationError(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)

	payload := `{
		"type": "view_submission",
		"user": {"id": "U12345"},
		"view": {
			"callback_id": "settings_modal",
			"private_metadata": "C99999",
			"state": {
				"values": {
					"label_name": {"label_name": {"value": ""}},
					"default_mention_id": {"default_mention_id": {"value": ""}},
					"reviewer_list": {"reviewer_list": {"value": ""}},
					"repository_list": {"repository_list": {"value": ""}},
					"reviewer_reminder_interval": {"reviewer_reminder_interval": {"value": "abc"}},
					"business_hours_start": {"business_hours_start": {"value": "09:00"}},
					"business_hours_end": {"business_hours_end": {"value": "18:00"}},
					"timezone": {"timezone": {"value": "Asia/Tokyo"}},
					"required_approvals": {"required_approvals": {"value": "1"}},
					"language": {"language": {"selected_option": {"value": "ja"}}},
					"is_active": {"is_active": {"selected_option": {"value": "true"}}}
				}
			}
		}
	}`

	w := postPayload(t, router, payload)
	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "response_action")
	assert.Contains(t, body, "errors")
	// the bad fields should appear
	assert.Contains(t, body, "label_name")
	assert.Contains(t, body, "reviewer_reminder_interval")
}
