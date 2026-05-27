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

// TestHandleSlackAction_OpenSettings_NoFallback verifies that when the target label
// has no config but ANOTHER label in the same channel does, we do NOT silently
// fall back to it — the modal must open fresh for the requested label only.
// (Regression guard for the previous "fallback pre-fills another label" UX bug.)
func TestHandleSlackAction_OpenSettings_NoFallback(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)

	// Unrelated label exists; the modal must NOT pre-fill from it.
	db.Create(&models.ChannelConfig{
		ID:               "other",
		SlackChannelID:   "C12345",
		LabelName:        "other-label",
		DefaultMentionID: "Uother",
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
	// Functional verification (that the modal shows empty defaults rather than the
	// other-label config) lives in services/settings_modal_test.go where the view
	// payload is inspectable; here we just assert the request is accepted.
}

// TestHandleSlackAction_ViewSubmission_CreatesConfig verifies a view_submission payload
// upserts a new ChannelConfig when the dropdown is on the create-new sentinel
// and a label name is provided in the new_label_name input.
func TestHandleSlackAction_ViewSubmission_CreatesConfig(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)

	payload := `{
		"type": "view_submission",
		"user": {"id": "U12345"},
		"view": {
			"callback_id": "settings_modal",
			"private_metadata": "{\"c\":\"C99999\",\"u\":\"U12345\"}",
			"state": {
				"values": {
					"label_select": {"label_select": {"type": "static_select", "selected_option": {"value": "__create_new__"}}},
					"new_label_name": {"new_label_name": {"type": "plain_text_input", "value": "new-label"}},
					"default_mention_user": {"default_mention_user": {"type": "users_select", "selected_user": "U777"}},
					"default_mention_subteam": {"default_mention_subteam": {"type": "plain_text_input", "value": ""}},
					"reviewer_list": {"reviewer_list": {"type": "multi_users_select", "selected_users": ["U1", "U2"]}},
					"repository_list": {"repository_list": {"type": "plain_text_input", "value": "owner/repo"}},
					"reminder_interval": {"reminder_interval": {"type": "plain_text_input", "value": "20"}},
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
	assert.Equal(t, 20, cfg.ReminderInterval)
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
			"private_metadata": "{\"c\":\"C99999\",\"u\":\"U12345\"}",
			"state": {
				"values": {
					"label_select": {"label_select": {"selected_option": {"value": "needs-review"}}},
					"default_mention_user": {"default_mention_user": {"selected_user": "Unew"}},
					"default_mention_subteam": {"default_mention_subteam": {"value": ""}},
					"reviewer_list": {"reviewer_list": {"selected_users": []}},
					"repository_list": {"repository_list": {"value": ""}},
					"reminder_interval": {"reminder_interval": {"value": "30"}},
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
			"private_metadata": "{\"c\":\"C99999\",\"u\":\"U12345\"}",
			"state": {
				"values": {
					"label_select": {"label_select": {"selected_option": {"value": "needs-review"}}},
					"default_mention_user": {"default_mention_user": {"selected_user": ""}},
					"default_mention_subteam": {"default_mention_subteam": {"value": ""}},
					"reviewer_list": {"reviewer_list": {"selected_users": []}},
					"repository_list": {"repository_list": {"value": ""}},
					"reminder_interval": {"reminder_interval": {"value": "30"}},
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
	// the bad field should appear in the inline errors
	assert.Contains(t, body, "reviewer_reminder_interval")
}

// TestHandleSlackAction_ViewSubmission_DeletesConfig verifies the delete
// checkbox path: when the user checks "delete this configuration" on an
// existing label and submits, the corresponding ChannelConfig row must be
// removed (soft-deleted via gorm).
func TestHandleSlackAction_ViewSubmission_DeletesConfig(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)

	db.Create(&models.ChannelConfig{
		ID:               "victim",
		SlackChannelID:   "C99999",
		LabelName:        "needs-review",
		DefaultMentionID: "Uold",
		IsActive:         true,
	})

	payload := `{
		"type": "view_submission",
		"user": {"id": "U12345"},
		"view": {
			"callback_id": "settings_modal",
			"private_metadata": "{\"c\":\"C99999\",\"u\":\"U12345\"}",
			"state": {
				"values": {
					"label_select": {"label_select": {"selected_option": {"value": "needs-review"}}},
					"delete_config": {"delete_config": {"type": "checkboxes", "selected_options": [{"value": "yes"}]}},
					"language": {"language": {"selected_option": {"value": "ja"}}},
					"is_active": {"is_active": {"selected_option": {"value": "true"}}}
				}
			}
		}
	}`

	w := postPayload(t, router, payload)
	assert.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var cfg models.ChannelConfig
	err := db.Where("slack_channel_id = ? AND label_name = ?", "C99999", "needs-review").First(&cfg).Error
	assert.Error(t, err, "config row must be deleted")
}

// TestHandleSlackAction_LabelSelectDispatch verifies the label dropdown's
// dispatch_action fires a views.update flow without erroring. The actual
// views.update call is short-circuited by IsTestMode; here we just confirm the
// block_actions branch is routed correctly and acknowledges with 200.
func TestHandleSlackAction_LabelSelectDispatch(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)

	db.Create(&models.ChannelConfig{
		ID:               "c1",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U99999",
		IsActive:         true,
	})

	payload := `{
		"type": "block_actions",
		"user": {"id": "U12345"},
		"actions": [{"action_id": "label_select", "selected_option": {"value": "__create_new__"}}],
		"container": {"channel_id": "C12345"},
		"view": {
			"id": "V1",
			"callback_id": "settings_modal",
			"private_metadata": "{\"c\":\"C12345\",\"u\":\"U12345\"}",
			"state": {"values": {}}
		}
	}`

	w := postPayload(t, router, payload)
	assert.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
}

// TestHandleSlackAction_ViewSubmission_CreateNewDuplicateRejected verifies
// that picking the create-new sentinel but naming an already-configured label
// fails with a per-field error, so the user doesn't silently overwrite an
// existing row through the rename footgun.
func TestHandleSlackAction_ViewSubmission_CreateNewDuplicateRejected(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)

	db.Create(&models.ChannelConfig{
		ID:             "existing",
		SlackChannelID: "C99999",
		LabelName:      "needs-review",
		IsActive:       true,
	})

	payload := `{
		"type": "view_submission",
		"user": {"id": "U12345"},
		"view": {
			"callback_id": "settings_modal",
			"private_metadata": "{\"c\":\"C99999\",\"u\":\"U12345\"}",
			"state": {
				"values": {
					"label_select": {"label_select": {"selected_option": {"value": "__create_new__"}}},
					"new_label_name": {"new_label_name": {"value": "needs-review"}},
					"default_mention_user": {"default_mention_user": {"selected_user": "U1"}},
					"default_mention_subteam": {"default_mention_subteam": {"value": ""}},
					"reviewer_list": {"reviewer_list": {"selected_users": []}},
					"repository_list": {"repository_list": {"value": ""}},
					"reminder_interval": {"reminder_interval": {"value": "30"}},
					"reviewer_reminder_interval": {"reviewer_reminder_interval": {"value": "30"}},
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
	assert.Contains(t, body, "new_label_name")

	// Confirm we did NOT create a duplicate row.
	var count int64
	db.Model(&models.ChannelConfig{}).Where("slack_channel_id = ? AND label_name = ?", "C99999", "needs-review").Count(&count)
	assert.Equal(t, int64(1), count)
}

// TestHandleSlackCommand_HelpListsPerLabelButtons verifies that the help
// response renders one open_settings button per existing label config plus
// the create-new button — so users with multiple labels can edit each one
// from the UI rather than being stuck on the previously hardcoded "needs-review".
func TestHandleSlackCommand_HelpListsPerLabelButtons(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	db.Create(&models.ChannelConfig{ID: "a", SlackChannelID: "C12345", LabelName: "needs-review", IsActive: true})
	db.Create(&models.ChannelConfig{ID: "b", SlackChannelID: "C12345", LabelName: "urgent", IsActive: true})

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
	// Each label's name appears in its own button.
	assert.Contains(t, body, "needs-review")
	assert.Contains(t, body, "urgent")
	// Create-new button uses the sentinel as its value.
	assert.Contains(t, body, "__create_new__")
}

// TestHandleSlackAction_ViewSubmission_DeleteThenRecreate verifies the user can
// delete a label configuration and then immediately recreate one with the same
// label name through the create-new flow. A naive soft-delete leaves a row in
// place that, combined with the (slack_channel_id, label_name) unique index,
// blocks the recreate INSERT at the DB layer.
func TestHandleSlackAction_ViewSubmission_DeleteThenRecreate(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)

	db.Create(&models.ChannelConfig{
		ID:               "victim",
		SlackChannelID:   "C99999",
		LabelName:        "needs-review",
		DefaultMentionID: "Uold",
		IsActive:         true,
	})

	deletePayload := `{
		"type": "view_submission",
		"user": {"id": "U12345"},
		"view": {
			"callback_id": "settings_modal",
			"private_metadata": "{\"c\":\"C99999\",\"u\":\"U12345\"}",
			"state": {
				"values": {
					"label_select": {"label_select": {"selected_option": {"value": "needs-review"}}},
					"delete_config": {"delete_config": {"type": "checkboxes", "selected_options": [{"value": "yes"}]}},
					"language": {"language": {"selected_option": {"value": "ja"}}},
					"is_active": {"is_active": {"selected_option": {"value": "true"}}}
				}
			}
		}
	}`
	w := postPayload(t, router, deletePayload)
	assert.Equal(t, http.StatusOK, w.Code, "delete body: %s", w.Body.String())

	recreatePayload := `{
		"type": "view_submission",
		"user": {"id": "U12345"},
		"view": {
			"callback_id": "settings_modal",
			"private_metadata": "{\"c\":\"C99999\",\"u\":\"U12345\"}",
			"state": {
				"values": {
					"label_select": {"label_select": {"selected_option": {"value": "__create_new__"}}},
					"new_label_name": {"new_label_name": {"value": "needs-review"}},
					"default_mention_user": {"default_mention_user": {"selected_user": "Unew"}},
					"default_mention_subteam": {"default_mention_subteam": {"value": ""}},
					"reviewer_list": {"reviewer_list": {"selected_users": []}},
					"repository_list": {"repository_list": {"value": ""}},
					"reminder_interval": {"reminder_interval": {"value": "30"}},
					"reviewer_reminder_interval": {"reviewer_reminder_interval": {"value": "30"}},
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
	w = postPayload(t, router, recreatePayload)
	assert.Equal(t, http.StatusOK, w.Code, "recreate body: %s", w.Body.String())
	assert.NotContains(t, w.Body.String(), "errors", "recreate must not return a validation error")

	var cfg models.ChannelConfig
	if err := db.Where("slack_channel_id = ? AND label_name = ?", "C99999", "needs-review").First(&cfg).Error; err != nil {
		t.Fatalf("recreated config not found: %v", err)
	}
	assert.Equal(t, "Unew", cfg.DefaultMentionID)
}

// TestHandleSlackAction_ViewSubmission_MissingPrivateMetadata verifies the handler
// rejects a submission whose private_metadata is missing/invalid, so a bug that
// loses modal context can't silently no-op or write to the wrong row.
func TestHandleSlackAction_ViewSubmission_MissingPrivateMetadata(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)

	payload := `{
		"type": "view_submission",
		"user": {"id": "U12345"},
		"view": {
			"callback_id": "settings_modal",
			"private_metadata": "",
			"state": {"values": {}}
		}
	}`

	w := postPayload(t, router, payload)
	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "response_action")
	assert.Contains(t, body, "errors")
}
