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

// TestAwayModal_ViewSubmission_DeleteAllIgnoresPickers: delete-all must not be
// blocked by whatever the date and time pickers happen to hold. A time selected
// without its date is a validation error on the set path, and running that check
// under delete-all refused a deletion that uses no dates at all.
func TestAwayModal_ViewSubmission_DeleteAllIgnoresPickers(t *testing.T) {
	cases := []struct {
		name      string
		from      string
		fromTime  string
		untilTime string
	}{
		{name: "time without date", fromTime: "09:00"},
		{name: "end time without date", untilTime: "17:00"},
		{name: "malformed time", fromTime: "25:00"},
		{name: "date and time", from: "2030-04-01", fromTime: "09:00"},
	}

	for _, tc := range cases {
		db := setupTestDB(t)
		router := setupActionRouter(db)

		db.Create(&models.ReviewerAvailability{
			ID: uuid.NewString(), SlackUserID: "U_TARGET",
		})

		payload := buildAwayViewSubmission(t, awaySubmission{
			channelID: "C12345",
			userID:    "U_ADMIN",
			awayUser:  "U_TARGET",
			from:      tc.from,
			fromTime:  tc.fromTime,
			untilTime: tc.untilTime,
			deleteAll: true,
		})
		w := postPayload(t, router, payload)
		assert.Equal(t, http.StatusOK, w.Code, "%s: body: %s", tc.name, w.Body.String())
		assert.NotContains(t, w.Body.String(), "response_action",
			"%s: delete-all must not be rejected", tc.name)

		var count int64
		db.Model(&models.ReviewerAvailability{}).Where("slack_user_id = ?", "U_TARGET").Count(&count)
		assert.EqualValues(t, 0, count, "%s: the record must be deleted", tc.name)
	}
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

// TestAwayModal_ViewSubmission_UpsertMatchesSlashCommandRecord:
// the modal's upsert must hit an existing row created by the slash command
// when the user picks the same dates. The slash command stores `from` at
// 00:00 +tz and `until` at 23:59:59 +tz; the modal must do the same, otherwise
// a "re-save with new reason" leaks a duplicate row instead of updating in
// place. This is a regression test for the timezone fix.
func TestAwayModal_ViewSubmission_UpsertMatchesSlashCommandRecord(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)

	// Configure the channel's timezone so resolveAwayTimezone selects JST.
	db.Create(&models.ChannelConfig{
		ID:             uuid.NewString(),
		SlackChannelID: "C12345",
		LabelName:      "needs-review",
		Timezone:       "Asia/Tokyo",
		IsActive:       true,
	})

	// Simulate a row written by the slash command path: 00:00 / 23:59:59 in JST.
	jst, _ := time.LoadLocation("Asia/Tokyo")
	from := time.Date(2030, 4, 1, 0, 0, 0, 0, jst)
	until := time.Date(2030, 4, 5, 23, 59, 59, 0, jst)
	preExisting := models.ReviewerAvailability{
		ID:          uuid.NewString(),
		SlackUserID: "U_TARGET",
		AwayFrom:    &from,
		AwayUntil:   &until,
		Reason:      "original",
	}
	if err := db.Create(&preExisting).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	payload := buildAwayViewSubmission(t, awaySubmission{
		channelID: "C12345",
		userID:    "U_ADMIN",
		awayUser:  "U_TARGET",
		from:      "2030-04-01",
		until:     "2030-04-05",
		reason:    "updated reason",
	})
	w := postPayload(t, router, payload)
	assert.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var rows []models.ReviewerAvailability
	db.Where("slack_user_id = ?", "U_TARGET").Find(&rows)
	if assert.Len(t, rows, 1, "modal save must update in place, not insert a duplicate") {
		assert.Equal(t, "updated reason", rows[0].Reason)
		assert.Equal(t, preExisting.ID, rows[0].ID, "primary key must be preserved (UPDATE, not INSERT)")
	}
}

// awaySubmission is a tiny DSL for assembling test payloads. Empty string
// fields are rendered as empty values; deleteAll=true ticks the checkbox.
type awaySubmission struct {
	channelID string
	userID    string
	awayUser  string
	from      string
	until     string
	fromTime  string
	untilTime string
	reason    string
	allDay    bool
	deleteAll bool
}

func buildAwayViewSubmission(t *testing.T, s awaySubmission) string {
	t.Helper()
	deleteOpts := "[]"
	if s.deleteAll {
		deleteOpts = `[{"value":"yes"}]`
	}
	allDayOpts := "[]"
	if s.allDay {
		allDayOpts = `[{"value":"yes"}]`
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
					"away_from_time":  {"away_from_time":  {"type": "timepicker",       "selected_time": "` + s.fromTime + `"}},
					"away_until_time": {"away_until_time": {"type": "timepicker",       "selected_time": "` + s.untilTime + `"}},
					"away_reason":     {"away_reason":     {"type": "plain_text_input", "value": "` + s.reason + `"}},
					"away_all_day":    {"away_all_day":    {"type": "checkboxes",       "selected_options": ` + allDayOpts + `}},
					"away_delete_all": {"away_delete_all": {"type": "checkboxes",       "selected_options": ` + deleteOpts + `}}
				}
			}
		},
		"actions": []
	}`
	return payload
}

// TestAwayModal_OpenRendersAllDayByDefault: a freshly opened modal is the
// all-day shape. Whole-day leave is the common case, so the form must not
// ask for times until the user unticks the box.
func TestAwayModal_OpenRendersAllDayByDefault(t *testing.T) {
	db := setupTestDB(t)

	var payload SlackActionPayload
	if err := json.Unmarshal([]byte(`{
		"type": "block_actions",
		"trigger_id": "trigger-xyz",
		"user": {"id": "U12345"},
		"actions": [{"action_id": "`+services.OpenAwayManagementActionID+`"}],
		"container": {"channel_id": "C12345"}
	}`), &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}

	view := awayModalViewForOpen(db, payload)
	ids := awayBlockIDs(view)
	assert.Contains(t, ids, services.AwayAllDayActionID)
	assert.NotContains(t, ids, "away_from_time", "the default form must not show time pickers")
	assert.NotContains(t, ids, "away_until_time", "the default form must not show time pickers")
}

// TestHandleSlackAction_AwayAllDayToggle: unticking the box inside the modal
// arrives as block_actions with the view attached; the handler must accept it
// (views.update is skipped under IsTestMode) rather than fall through to the
// reminder-button branches, which would 400 on the missing message.
func TestHandleSlackAction_AwayAllDayToggle(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)

	w := postPayload(t, router, awayAllDayTogglePayload(false))
	assert.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	assert.Empty(t, w.Body.String(), "a modal re-render must ack with an empty body")
}

// TestAwayModal_ToggleRerenderKeepsInputs pins the view the toggle handler
// pushes: unticked shows the time pickers, and what the user already picked
// (here the target user) rides along as initial values so the toggle does not
// double as a reset. The view id must be the one Slack sent, or views.update
// would target nothing.
func TestAwayModal_ToggleRerenderKeepsInputs(t *testing.T) {
	db := setupTestDB(t)

	var payload SlackActionPayload
	if err := json.Unmarshal([]byte(awayAllDayTogglePayload(false)), &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}

	viewID, view, ok := awayModalViewForToggle(db, payload)
	if !ok {
		t.Fatalf("toggle must produce a view")
	}
	assert.Equal(t, "V_AWAY", viewID)

	ids := awayBlockIDs(view)
	assert.Contains(t, ids, "away_from_time", "unticking all-day must reveal the time pickers")
	assert.Contains(t, ids, "away_until_time", "unticking all-day must reveal the time pickers")

	for _, b := range view["blocks"].([]map[string]any) {
		if b["block_id"] == "away_user" {
			element := b["element"].(map[string]any)
			assert.Equal(t, "U_TARGET", element["initial_user"], "the picked user must survive the re-render")
		}
	}

	// Ticking it again hides the pickers; the state Slack sends then carries
	// the option as selected.
	if err := json.Unmarshal([]byte(awayAllDayTogglePayload(true)), &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	_, view, ok = awayModalViewForToggle(db, payload)
	if !ok {
		t.Fatalf("toggle must produce a view")
	}
	ids = awayBlockIDs(view)
	assert.NotContains(t, ids, "away_from_time", "ticking all-day must hide the time pickers")
	assert.NotContains(t, ids, "away_until_time", "ticking all-day must hide the time pickers")
}

// TestAwayModal_ViewSubmission_AllDayWinsOverTimes: with the box ticked the
// stored leave spans the whole selected day in the channel's zone even if a
// time value is still in the payload (two quick toggles can race the
// re-render). Anything else would silently shrink a day off into a slot.
func TestAwayModal_ViewSubmission_AllDayWinsOverTimes(t *testing.T) {
	db := setupTestDB(t)
	router := setupActionRouter(db)

	db.Create(&models.ChannelConfig{
		ID:             uuid.NewString(),
		SlackChannelID: "C12345",
		LabelName:      "needs-review",
		Timezone:       "Asia/Tokyo",
		IsActive:       true,
	})

	payload := buildAwayViewSubmission(t, awaySubmission{
		channelID: "C12345",
		userID:    "U_ADMIN",
		awayUser:  "U_TARGET",
		from:      "2030-04-01",
		until:     "2030-04-01",
		fromTime:  "09:00",
		untilTime: "17:00",
		allDay:    true,
	})
	w := postPayload(t, router, payload)
	assert.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	assert.NotContains(t, w.Body.String(), "response_action", "all-day must not be rejected")

	jst, _ := time.LoadLocation("Asia/Tokyo")
	var rows []models.ReviewerAvailability
	db.Where("slack_user_id = ?", "U_TARGET").Find(&rows)
	if assert.Len(t, rows, 1) {
		got := rows[0]
		if assert.NotNil(t, got.AwayFrom) {
			assert.True(t, got.AwayFrom.Equal(time.Date(2030, 4, 1, 0, 0, 0, 0, jst)),
				"AwayFrom = %v, want start of day", got.AwayFrom)
		}
		if assert.NotNil(t, got.AwayUntil) {
			assert.True(t, got.AwayUntil.Equal(time.Date(2030, 4, 1, 23, 59, 59, 0, jst)),
				"AwayUntil = %v, want end of day", got.AwayUntil)
		}
	}
}

// awayAllDayTogglePayload is the block_actions payload Slack sends when the
// all-day box changes inside the modal. ticked mirrors the resulting state.
func awayAllDayTogglePayload(ticked bool) string {
	opts := "[]"
	if ticked {
		opts = `[{"value":"yes"}]`
	}
	return `{
		"type": "block_actions",
		"user": {"id": "U12345"},
		"actions": [{"action_id": "` + services.AwayAllDayActionID + `", "type": "checkboxes", "selected_options": ` + opts + `}],
		"container": {"type": "view", "view_id": "V_AWAY"},
		"view": {
			"id": "V_AWAY",
			"callback_id": "` + services.AwayManagementModalCallbackID + `",
			"private_metadata": "{\"c\":\"C12345\",\"u\":\"U12345\"}",
			"state": {
				"values": {
					"away_user":    {"away_user":    {"type": "users_select", "selected_user": "U_TARGET"}},
					"away_all_day": {"away_all_day": {"type": "checkboxes",   "selected_options": ` + opts + `}}
				}
			}
		}
	}`
}

func awayBlockIDs(view map[string]any) []string {
	ids := []string{}
	for _, b := range view["blocks"].([]map[string]any) {
		if id, ok := b["block_id"].(string); ok {
			ids = append(ids, id)
		}
	}
	return ids
}
