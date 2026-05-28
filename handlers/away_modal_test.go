package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slack-review-notify/models"
	"slack-review-notify/services"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// TestHelpCommand_IncludesAwayManagementButton: the help message renders a
// button that opens the away modal. Without this entry point the modal is
// only reachable via API and the feature is dead UX-wise.
func TestHelpCommand_IncludesAwayManagementButton(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	form := url.Values{}
	form.Add("command", "/slack-review-notify")
	form.Add("text", "help")
	form.Add("channel_id", "C12345")
	form.Add("user_id", "U12345")

	req, _ := http.NewRequest("POST", "/slack/command", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, services.OpenAwayManagementActionID,
		"help response must include the open_away_management action_id")
}

// TestHandleSlackAction_OpenAwayManagement: block_actions with
// action_id=open_away_management must be accepted (would call views.open in
// production; IsTestMode skips the network call).
func TestHandleSlackAction_OpenAwayManagement(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)

	payload := `{
		"type": "block_actions",
		"trigger_id": "trigger-xyz",
		"user": {"id": "U12345"},
		"actions": [{"action_id": "` + services.OpenAwayManagementActionID + `"}],
		"container": {"channel_id": "C12345"},
		"message": {"ts": "1234.5"}
	}`

	w := postPayload(t, router, payload)
	assert.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
}

// TestAwayModal_ViewSubmission_CreatesRecord: happy-path save.
// Submitting a user + period + reason persists exactly one ReviewerAvailability
// row matching the input.
func TestAwayModal_ViewSubmission_CreatesRecord(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)

	payload := buildAwayViewSubmission(t, awaySubmission{
		channelID: "C12345",
		userID:    "U_ADMIN",
		awayUser:  "U_TARGET",
		from:      "2030-04-01",
		until:     "2030-04-05",
		reason:    "vacation",
	})
	w := postPayload(t, router, payload)
	assert.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var rows []models.ReviewerAvailability
	db.Where("slack_user_id = ?", "U_TARGET").Find(&rows)
	if assert.Len(t, rows, 1) {
		got := rows[0]
		assert.NotNil(t, got.AwayFrom)
		assert.NotNil(t, got.AwayUntil)
		assert.Equal(t, "vacation", got.Reason)
	}
}

// TestAwayModal_ViewSubmission_Indefinite: no dates → indefinite immediate
// leave. Mirrors the slash-command `set-away @user` (no period) behavior.
func TestAwayModal_ViewSubmission_Indefinite(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)

	payload := buildAwayViewSubmission(t, awaySubmission{
		channelID: "C12345",
		userID:    "U_ADMIN",
		awayUser:  "U_TARGET",
	})
	w := postPayload(t, router, payload)
	assert.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var rows []models.ReviewerAvailability
	db.Where("slack_user_id = ?", "U_TARGET").Find(&rows)
	if assert.Len(t, rows, 1) {
		assert.Nil(t, rows[0].AwayFrom)
		assert.Nil(t, rows[0].AwayUntil)
	}
}

// TestAwayModal_ViewSubmission_DeleteAll: checkbox=yes wipes every record for
// the target user, regardless of the date inputs.
func TestAwayModal_ViewSubmission_DeleteAll(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)

	now := time.Now()
	later := now.Add(72 * time.Hour)
	db.Create(&models.ReviewerAvailability{
		ID: uuid.NewString(), SlackUserID: "U_TARGET", AwayFrom: &now, AwayUntil: &later,
	})
	db.Create(&models.ReviewerAvailability{
		ID: uuid.NewString(), SlackUserID: "U_TARGET",
	})

	payload := buildAwayViewSubmission(t, awaySubmission{
		channelID: "C12345",
		userID:    "U_ADMIN",
		awayUser:  "U_TARGET",
		deleteAll: true,
	})
	w := postPayload(t, router, payload)
	assert.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var count int64
	db.Model(&models.ReviewerAvailability{}).Where("slack_user_id = ?", "U_TARGET").Count(&count)
	assert.EqualValues(t, 0, count)
}

// TestAwayModal_ViewSubmission_FromAfterUntil: bad period → response_action
// errors map with the offending block_id, no DB write.
func TestAwayModal_ViewSubmission_FromAfterUntil(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)

	payload := buildAwayViewSubmission(t, awaySubmission{
		channelID: "C12345",
		userID:    "U_ADMIN",
		awayUser:  "U_TARGET",
		from:      "2030-04-10",
		until:     "2030-04-01",
	})
	w := postPayload(t, router, payload)
	assert.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var resp struct {
		ResponseAction string            `json:"response_action"`
		Errors         map[string]string `json:"errors"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not JSON: %v\nbody=%s", err, w.Body.String())
	}
	assert.Equal(t, "errors", resp.ResponseAction)
	assert.Contains(t, resp.Errors, "away_until")

	var count int64
	db.Model(&models.ReviewerAvailability{}).Count(&count)
	assert.EqualValues(t, 0, count, "no row may be written on validation failure")
}

// awaySubmission is a tiny DSL for assembling test payloads. Empty string
// fields are rendered as empty values; deleteAll=true ticks the checkbox.
type awaySubmission struct {
	channelID string
	userID    string
	awayUser  string
	from      string
	until     string
	reason    string
	deleteAll bool
}

func buildAwayViewSubmission(t *testing.T, s awaySubmission) string {
	t.Helper()
	deleteOpts := "[]"
	if s.deleteAll {
		deleteOpts = `[{"value":"yes"}]`
	}
	meta := services.EncodeAwayModalMetadata(services.AwayModalMetadata{
		ChannelID: s.channelID,
		UserID:    s.userID,
	})
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}

	payload := `{
		"type": "view_submission",
		"user": {"id": "` + s.userID + `"},
		"view": {
			"id": "V_TEST",
			"callback_id": "` + services.AwayManagementModalCallbackID + `",
			"private_metadata": ` + string(metaJSON) + `,
			"state": {
				"values": {
					"away_user":       {"away_user":       {"type": "users_select",     "selected_user": "` + s.awayUser + `"}},
					"away_from":       {"away_from":       {"type": "datepicker",       "selected_date": "` + s.from + `", "value": "` + s.from + `"}},
					"away_until":      {"away_until":      {"type": "datepicker",       "selected_date": "` + s.until + `", "value": "` + s.until + `"}},
					"away_reason":     {"away_reason":     {"type": "plain_text_input", "value": "` + s.reason + `"}},
					"away_delete_all": {"away_delete_all": {"type": "checkboxes",       "selected_options": ` + deleteOpts + `}}
				}
			}
		},
		"actions": []
	}`
	return payload
}
