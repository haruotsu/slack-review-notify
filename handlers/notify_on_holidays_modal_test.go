package handlers

import (
	"net/http"
	"slack-review-notify/models"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandleSlackAction_ViewSubmission_SavesNotifyOnHolidays(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)
	db.Create(&models.ChannelConfig{
		ID:                 "existing",
		SlackChannelID:     "C99999",
		LabelName:          "needs-review",
		BusinessHoursStart: "09:00",
		BusinessHoursEnd:   "18:00",
		Timezone:           "Asia/Tokyo",
		RequiredApprovals:  1,
		Language:           "ja",
		IsActive:           true,
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
					"reminder_interval": {"reminder_interval": {"value": "30"}},
					"reviewer_reminder_interval": {"reviewer_reminder_interval": {"value": "30"}},
					"business_hours_start": {"business_hours_start": {"value": "09:00"}},
					"business_hours_end": {"business_hours_end": {"value": "18:00"}},
					"timezone": {"timezone": {"value": "Asia/Tokyo"}},
					"notify_on_holidays": {"notify_on_holidays": {"selected_option": {"value": "true"}}},
					"required_approvals": {"required_approvals": {"value": "1"}},
					"language": {"language": {"selected_option": {"value": "ja"}}},
					"is_active": {"is_active": {"selected_option": {"value": "true"}}}
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
	assert.True(t, cfg.NotifyOnHolidays)
}

func TestHandleSlackAction_ViewSubmission_KeepsNotifyOnHolidaysWhenFieldMissing(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)
	db.Create(&models.ChannelConfig{
		ID:                 "existing",
		SlackChannelID:     "C99999",
		LabelName:          "needs-review",
		BusinessHoursStart: "09:00",
		BusinessHoursEnd:   "18:00",
		Timezone:           "Asia/Tokyo",
		RequiredApprovals:  1,
		Language:           "ja",
		NotifyOnHolidays:   true,
		IsActive:           true,
	})

	// The field is absent when the modal was opened before it was added.
	payload := `{
		"type": "view_submission",
		"user": {"id": "U12345"},
		"view": {
			"callback_id": "settings_modal",
			"private_metadata": "{\"c\":\"C99999\",\"u\":\"U12345\"}",
			"state": {
				"values": {
					"label_select": {"label_select": {"selected_option": {"value": "needs-review"}}},
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
	assert.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var cfg models.ChannelConfig
	if err := db.Where("id = ?", "existing").First(&cfg).Error; err != nil {
		t.Fatalf("config not found: %v", err)
	}
	assert.True(t, cfg.NotifyOnHolidays)
}
