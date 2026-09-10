package services

import (
	"errors"
	"testing"
	"time"
)

// TestBuildAwayManagementModalView_HasAllFields verifies the view contains
// every block_id the submission handler will read back. Slack drops unknown
// fields silently, so a missing block_id here would make the corresponding
// form value invisible at parse time — a class of bug that's worth pinning
// down structurally.
func TestBuildAwayManagementModalView_HasAllFields(t *testing.T) {
	// AllDay off is the widest shape: every block the parser knows is rendered.
	view := BuildAwayManagementModalView(AwayManagementModalInputs{
		ChannelID: "C12345",
		UserID:    "U777",
		Lang:      "ja",
		Prefill:   AwayModalPrefill{AllDay: false},
	})

	if view["type"] != "modal" {
		t.Errorf("type = %v, want modal", view["type"])
	}
	if view["callback_id"] != AwayManagementModalCallbackID {
		t.Errorf("callback_id = %v, want %s", view["callback_id"], AwayManagementModalCallbackID)
	}

	blocks, ok := view["blocks"].([]map[string]any)
	if !ok {
		t.Fatalf("blocks is not a slice: %T", view["blocks"])
	}

	required := []string{
		"away_user",
		"away_from",
		"away_from_time",
		"away_until",
		"away_until_time",
		"away_all_day",
		"away_reason",
		"away_delete_all",
	}
	found := make(map[string]bool)
	for _, b := range blocks {
		if id, ok := b["block_id"].(string); ok {
			found[id] = true
		}
	}
	for _, want := range required {
		if !found[want] {
			t.Errorf("missing block_id: %s", want)
		}
	}
}

// TestBuildAwayManagementModalView_PrivateMetadata: channel ID and user ID
// must round-trip through private_metadata so the submission handler can post
// the confirmation back into the same channel — a visible channel message for
// an actual change, or an ephemeral notice (to the user) when nothing changed.
func TestBuildAwayManagementModalView_PrivateMetadata(t *testing.T) {
	view := BuildAwayManagementModalView(AwayManagementModalInputs{
		ChannelID: "C12345",
		UserID:    "U777",
		Lang:      "ja",
	})
	pm, ok := view["private_metadata"].(string)
	if !ok {
		t.Fatalf("private_metadata not a string: %T", view["private_metadata"])
	}
	meta, err := DecodeAwayModalMetadata(pm)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if meta.ChannelID != "C12345" || meta.UserID != "U777" {
		t.Errorf("metadata = %+v, want {C12345, U777}", meta)
	}
}

func minimalAwayValues() map[string]map[string]ViewStateValue {
	return map[string]map[string]ViewStateValue{
		"away_user":       {"away_user": {SelectedUser: "U999"}},
		"away_from":       {"away_from": {Value: ""}},
		"away_from_time":  {"away_from_time": {SelectedTime: ""}},
		"away_until":      {"away_until": {Value: ""}},
		"away_until_time": {"away_until_time": {SelectedTime: ""}},
		"away_reason":     {"away_reason": {Value: ""}},
		"away_delete_all": {"away_delete_all": {SelectedOptions: nil}},
	}
}

// TestParseAwayModalSubmission_UserRequired: SlackUserID is the only truly
// required field — without a target, there's nothing to operate on, so we
// surface a per-field validation error rather than silently no-op.
func TestParseAwayModalSubmission_UserRequired(t *testing.T) {
	v := minimalAwayValues()
	v["away_user"] = map[string]ViewStateValue{
		"away_user": {SelectedUser: ""},
	}
	_, err := ParseAwayModalSubmission(v, time.UTC, "ja")
	if err == nil {
		t.Fatalf("expected validation error")
	}
	ve, ok := err.(*ModalValidationError)
	if !ok {
		t.Fatalf("want *ModalValidationError, got %T", err)
	}
	if _, has := ve.Errors["away_user"]; !has {
		t.Errorf("want error on away_user, got %+v", ve.Errors)
	}
}

// TestParseAwayModalSubmission_SetIndefinite: only the user picked, no dates,
// no checkbox. This is the analogue of `/slack-review-notify set-away @user`
// with no period — an immediate, indefinite leave.
func TestParseAwayModalSubmission_SetIndefinite(t *testing.T) {
	form, err := ParseAwayModalSubmission(minimalAwayValues(), time.UTC, "ja")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if form.SlackUserID != "U999" {
		t.Errorf("SlackUserID = %q, want U999", form.SlackUserID)
	}
	if form.AwayFrom != nil || form.AwayUntil != nil {
		t.Errorf("dates = %v / %v, want nil / nil (indefinite)", form.AwayFrom, form.AwayUntil)
	}
	if form.DeleteAll {
		t.Errorf("DeleteAll = true, want false")
	}
}

// TestParseAwayModalSubmission_SetWithDatesAndReason: full happy path.
// Dates flow through as parsed time.Time pointers; reason is trimmed.
func TestParseAwayModalSubmission_SetWithDatesAndReason(t *testing.T) {
	v := minimalAwayValues()
	v["away_from"] = map[string]ViewStateValue{
		"away_from": {Value: "2030-01-15"},
	}
	v["away_until"] = map[string]ViewStateValue{
		"away_until": {Value: "2030-01-20"},
	}
	v["away_reason"] = map[string]ViewStateValue{
		"away_reason": {Value: "  vacation  "},
	}
	form, err := ParseAwayModalSubmission(v, time.UTC, "ja")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if form.AwayFrom == nil || form.AwayFrom.Year() != 2030 || form.AwayFrom.Month() != time.January || form.AwayFrom.Day() != 15 {
		t.Errorf("AwayFrom = %v, want 2030-01-15", form.AwayFrom)
	}
	if form.AwayUntil == nil || form.AwayUntil.Day() != 20 {
		t.Errorf("AwayUntil = %v, want 2030-01-20", form.AwayUntil)
	}
	if form.Reason != "vacation" {
		t.Errorf("Reason = %q, want %q (trimmed)", form.Reason, "vacation")
	}
}

// TestParseAwayModalSubmission_FromAfterUntil: from > until is a user error
// that should be caught at the modal layer, not after a DB write.
func TestParseAwayModalSubmission_FromAfterUntil(t *testing.T) {
	v := minimalAwayValues()
	v["away_from"] = map[string]ViewStateValue{
		"away_from": {Value: "2030-02-10"},
	}
	v["away_until"] = map[string]ViewStateValue{
		"away_until": {Value: "2030-02-01"},
	}
	_, err := ParseAwayModalSubmission(v, time.UTC, "ja")
	if err == nil {
		t.Fatalf("expected validation error")
	}
	ve, ok := err.(*ModalValidationError)
	if !ok {
		t.Fatalf("want *ModalValidationError, got %T", err)
	}
	if _, has := ve.Errors["away_until"]; !has {
		t.Errorf("want error on away_until, got %+v", ve.Errors)
	}
}

// TestParseAwayModalSubmission_InvalidDate: malformed date strings (in case a
// non-datepicker client somehow submits text) surface a validation error
// rather than crashing time.Parse downstream.
func TestParseAwayModalSubmission_InvalidDate(t *testing.T) {
	v := minimalAwayValues()
	v["away_from"] = map[string]ViewStateValue{
		"away_from": {Value: "not-a-date"},
	}
	_, err := ParseAwayModalSubmission(v, time.UTC, "ja")
	if err == nil {
		t.Fatalf("expected validation error")
	}
	ve, ok := err.(*ModalValidationError)
	if !ok {
		t.Fatalf("want *ModalValidationError, got %T", err)
	}
	if _, has := ve.Errors["away_from"]; !has {
		t.Errorf("want error on away_from, got %+v", ve.Errors)
	}
}

// TestParseAwayModalSubmission_TimezoneAware: dates picked in the modal must
// be interpreted in the channel's timezone (`from` at 00:00 +loc, `until` at
// 23:59:59 +loc) so that records line up with what the slash command writes.
// Without this, upserts against slash-command-created rows never match and
// `until` expires hours earlier than the user expects.
func TestParseAwayModalSubmission_TimezoneAware(t *testing.T) {
	jst, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("load JST: %v", err)
	}
	v := minimalAwayValues()
	v["away_from"] = map[string]ViewStateValue{
		"away_from": {Value: "2030-04-01"},
	}
	v["away_until"] = map[string]ViewStateValue{
		"away_until": {Value: "2030-04-05"},
	}
	form, err := ParseAwayModalSubmission(v, jst, "ja")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	wantFrom := time.Date(2030, 4, 1, 0, 0, 0, 0, jst)
	if form.AwayFrom == nil || !form.AwayFrom.Equal(wantFrom) {
		t.Errorf("AwayFrom = %v, want %v", form.AwayFrom, wantFrom)
	}
	wantUntil := time.Date(2030, 4, 5, 23, 59, 59, 0, jst)
	if form.AwayUntil == nil || !form.AwayUntil.Equal(wantUntil) {
		t.Errorf("AwayUntil = %v, want %v", form.AwayUntil, wantUntil)
	}
}

// TestParseAwayModalSubmission_SameDayAllowed: `from == until` is a legitimate
// "out for one day" leave, equivalent to the slash command's `on YYYY-MM-DD`.
// Combined with the end-of-day stretch for `until`, the resulting row covers
// the whole selected day instead of being a zero-length window.
func TestParseAwayModalSubmission_SameDayAllowed(t *testing.T) {
	jst, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("load JST: %v", err)
	}
	v := minimalAwayValues()
	v["away_from"] = map[string]ViewStateValue{
		"away_from": {Value: "2030-04-05"},
	}
	v["away_until"] = map[string]ViewStateValue{
		"away_until": {Value: "2030-04-05"},
	}
	form, err := ParseAwayModalSubmission(v, jst, "ja")
	if err != nil {
		t.Fatalf("expected success on same-day leave, got: %v", err)
	}
	if form.AwayFrom == nil || form.AwayUntil == nil {
		t.Fatalf("dates must be set, got from=%v until=%v", form.AwayFrom, form.AwayUntil)
	}
	if !form.AwayFrom.Before(*form.AwayUntil) {
		t.Errorf("expected from < until after end-of-day stretch, got from=%v until=%v", form.AwayFrom, form.AwayUntil)
	}
}

// TestParseAwayModalSubmission_DeleteAll: checkbox checked → DeleteAll=true.
// Per the field semantics (slash command `unset-away @user` without dates),
// this wipes every record for the user, so we don't require the date fields.
func TestParseAwayModalSubmission_DeleteAll(t *testing.T) {
	v := minimalAwayValues()
	v["away_delete_all"] = map[string]ViewStateValue{
		"away_delete_all": {SelectedOptions: []ViewSelectedOption{{Value: "yes"}}},
	}
	form, err := ParseAwayModalSubmission(v, time.UTC, "ja")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !form.DeleteAll {
		t.Errorf("DeleteAll = false, want true")
	}
	if form.SlackUserID != "U999" {
		t.Errorf("SlackUserID = %q, want U999", form.SlackUserID)
	}
}

func TestParseAwayModalSubmission_WithTimePicker(t *testing.T) {
	jst, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("load JST: %v", err)
	}
	v := minimalAwayValues()
	v["away_from"] = map[string]ViewStateValue{
		"away_from": {Value: "2030-04-01"},
	}
	v["away_from_time"] = map[string]ViewStateValue{
		"away_from_time": {SelectedTime: "06:00"},
	}
	v["away_until"] = map[string]ViewStateValue{
		"away_until": {Value: "2030-04-01"},
	}
	v["away_until_time"] = map[string]ViewStateValue{
		"away_until_time": {SelectedTime: "14:00"},
	}
	form, err := ParseAwayModalSubmission(v, jst, "ja")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if form.AwayFrom == nil || form.AwayFrom.Hour() != 6 || form.AwayFrom.Minute() != 0 {
		t.Errorf("AwayFrom = %v, want 06:00", form.AwayFrom)
	}
	if form.AwayUntil == nil || form.AwayUntil.Hour() != 14 || form.AwayUntil.Minute() != 0 {
		t.Errorf("AwayUntil = %v, want 14:00", form.AwayUntil)
	}
}

// TestParseAwayModalSubmission_TimeWithoutDateRejected pins that a time picked
// without its date is reported instead of dropped. Both pickers are optional, so
// this is an ordinary slip; accepting it left away_until nil, which means "away
// indefinitely" — the reviewer would stay excluded until somebody noticed.
func TestParseAwayModalSubmission_TimeWithoutDateRejected(t *testing.T) {
	for _, tc := range []struct{ name, timeBlock string }{
		{"start", "away_from_time"},
		{"end", "away_until_time"},
	} {
		v := minimalAwayValues()
		v[tc.timeBlock] = map[string]ViewStateValue{
			tc.timeBlock: {SelectedTime: "09:00"},
		}
		_, err := ParseAwayModalSubmission(v, time.UTC, "ja")
		var ve *ModalValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("%s: want a validation error, got %v", tc.name, err)
		}
		if _, ok := ve.Errors[tc.timeBlock]; !ok {
			t.Errorf("%s: want the error on %q, got %v", tc.name, tc.timeBlock, ve.Errors)
		}
	}
}

// TestParseAwayModalSubmission_MalformedTimeRejected pins the other half of the
// same rule. Slack's timepicker always sends HH:MM, so these values only arrive
// in a malformed payload — but falling through silently turned the requested
// slot into a whole day.
func TestParseAwayModalSubmission_MalformedTimeRejected(t *testing.T) {
	for _, bad := range []string{"9", "25:00", "ab:cd", "12:60", ":30"} {
		v := minimalAwayValues()
		v["away_from"] = map[string]ViewStateValue{
			"away_from": {Value: "2030-04-01"},
		}
		v["away_from_time"] = map[string]ViewStateValue{
			"away_from_time": {SelectedTime: bad},
		}
		_, err := ParseAwayModalSubmission(v, time.UTC, "ja")
		var ve *ModalValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("%q: want a validation error, got %v", bad, err)
		}
		if _, ok := ve.Errors["away_from_time"]; !ok {
			t.Errorf("%q: want the error on away_from_time, got %v", bad, ve.Errors)
		}
	}
}

func TestParseAwayModalSubmission_SameTimeRejected(t *testing.T) {
	v := minimalAwayValues()
	v["away_from"] = map[string]ViewStateValue{
		"away_from": {Value: "2030-04-01"},
	}
	v["away_from_time"] = map[string]ViewStateValue{
		"away_from_time": {SelectedTime: "06:00"},
	}
	v["away_until"] = map[string]ViewStateValue{
		"away_until": {Value: "2030-04-01"},
	}
	v["away_until_time"] = map[string]ViewStateValue{
		"away_until_time": {SelectedTime: "06:00"},
	}
	_, err := ParseAwayModalSubmission(v, time.UTC, "ja")
	if err == nil {
		t.Fatalf("expected validation error for zero-length leave (same from and until time)")
	}
	ve, ok := err.(*ModalValidationError)
	if !ok {
		t.Fatalf("want *ModalValidationError, got %T", err)
	}
	if _, has := ve.Errors["away_until_time"]; !has {
		t.Errorf("want error on away_until_time, got %+v", ve.Errors)
	}
}

func TestParseAwayModalSubmission_SameDayFromTimeAfterUntilTime(t *testing.T) {
	v := minimalAwayValues()
	v["away_from"] = map[string]ViewStateValue{
		"away_from": {Value: "2030-04-01"},
	}
	v["away_from_time"] = map[string]ViewStateValue{
		"away_from_time": {SelectedTime: "14:00"},
	}
	v["away_until"] = map[string]ViewStateValue{
		"away_until": {Value: "2030-04-01"},
	}
	v["away_until_time"] = map[string]ViewStateValue{
		"away_until_time": {SelectedTime: "06:00"},
	}
	_, err := ParseAwayModalSubmission(v, time.UTC, "ja")
	if err == nil {
		t.Fatalf("expected validation error for from_time > until_time on same day")
	}
	ve, ok := err.(*ModalValidationError)
	if !ok {
		t.Fatalf("want *ModalValidationError, got %T", err)
	}
	if _, has := ve.Errors["away_until_time"]; !has {
		t.Errorf("want error on away_until_time, got %+v", ve.Errors)
	}
}

// TestBuildAwayManagementModalView_AllDayHidesTimePickers pins the default
// shape of the modal: the all-day box is ticked and the time pickers are not
// rendered at all. Most leave is whole days, so the form should not ask for
// times unless the user opts in by unticking the box. dispatch_action is what
// makes the untick reach the server so the pickers can be added on re-render.
func TestBuildAwayManagementModalView_AllDayHidesTimePickers(t *testing.T) {
	view := BuildAwayManagementModalView(AwayManagementModalInputs{
		ChannelID: "C12345",
		UserID:    "U777",
		Lang:      "ja",
		Prefill:   DefaultAwayModalPrefill(),
	})
	blocks := view["blocks"].([]map[string]any)

	byID := map[string]map[string]any{}
	for _, b := range blocks {
		if id, ok := b["block_id"].(string); ok {
			byID[id] = b
		}
	}

	for _, hidden := range []string{"away_from_time", "away_until_time"} {
		if _, ok := byID[hidden]; ok {
			t.Errorf("%s must not be rendered while all-day is ticked", hidden)
		}
	}

	allDay, ok := byID["away_all_day"]
	if !ok {
		t.Fatalf("missing block_id away_all_day")
	}
	if allDay["dispatch_action"] != true {
		t.Errorf("away_all_day must set dispatch_action so ticking re-renders the modal")
	}
	element := allDay["element"].(map[string]any)
	if element["type"] != "checkboxes" || element["action_id"] != AwayAllDayActionID {
		t.Errorf("element = %v, want checkboxes with action_id %s", element, AwayAllDayActionID)
	}
	initial, ok := element["initial_options"].([]map[string]any)
	if !ok || len(initial) != 1 || initial[0]["value"] != "yes" {
		t.Errorf("initial_options = %v, want the all-day option ticked", element["initial_options"])
	}
}

// TestBuildAwayManagementModalView_AllDayOffShowsTimePickers is the other
// half of the toggle: once the box is unticked the re-rendered modal must
// carry both time pickers, directly under the checkbox, and the checkbox
// itself must come back unticked rather than snapping to its default.
func TestBuildAwayManagementModalView_AllDayOffShowsTimePickers(t *testing.T) {
	view := BuildAwayManagementModalView(AwayManagementModalInputs{
		ChannelID: "C12345",
		UserID:    "U777",
		Lang:      "ja",
		Prefill:   AwayModalPrefill{AllDay: false},
	})
	blocks := view["blocks"].([]map[string]any)

	order := []string{}
	byID := map[string]map[string]any{}
	for _, b := range blocks {
		if id, ok := b["block_id"].(string); ok {
			order = append(order, id)
			byID[id] = b
		}
	}

	want := []string{"away_user", "away_from", "away_until", "away_all_day", "away_from_time", "away_until_time", "away_reason", "away_delete_all"}
	if len(order) != len(want) {
		t.Fatalf("block order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("block[%d] = %s, want %s (full order %v)", i, order[i], want[i], order)
		}
	}

	element := byID["away_all_day"]["element"].(map[string]any)
	if _, ticked := element["initial_options"]; ticked {
		t.Errorf("away_all_day must render unticked when AllDay is false, got %v", element["initial_options"])
	}
}

// TestBuildAwayManagementModalView_PrefillCarriesValues pins that a re-render
// hands every value the user already entered back to Slack as initial_*. The
// modal is rebuilt on each all-day toggle, and a rebuild that forgot the
// picked user or dates would make the checkbox feel like a reset button.
func TestBuildAwayManagementModalView_PrefillCarriesValues(t *testing.T) {
	view := BuildAwayManagementModalView(AwayManagementModalInputs{
		ChannelID: "C12345",
		UserID:    "U777",
		Lang:      "ja",
		Prefill: AwayModalPrefill{
			AllDay:    false,
			UserID:    "U999",
			From:      "2030-04-01",
			FromTime:  "09:00",
			Until:     "2030-04-02",
			UntilTime: "17:30",
			Reason:    "vacation",
			DeleteAll: true,
		},
	})
	elements := map[string]map[string]any{}
	for _, b := range view["blocks"].([]map[string]any) {
		if id, ok := b["block_id"].(string); ok {
			elements[id] = b["element"].(map[string]any)
		}
	}

	checks := []struct {
		block, key string
		want       any
	}{
		{"away_user", "initial_user", "U999"},
		{"away_from", "initial_date", "2030-04-01"},
		{"away_from_time", "initial_time", "09:00"},
		{"away_until", "initial_date", "2030-04-02"},
		{"away_until_time", "initial_time", "17:30"},
		{"away_reason", "initial_value", "vacation"},
	}
	for _, c := range checks {
		if got := elements[c.block][c.key]; got != c.want {
			t.Errorf("%s.%s = %v, want %v", c.block, c.key, got, c.want)
		}
	}
	deleteInitial, ok := elements["away_delete_all"]["initial_options"].([]map[string]any)
	if !ok || len(deleteInitial) != 1 || deleteInitial[0]["value"] != "yes" {
		t.Errorf("away_delete_all initial_options = %v, want ticked", elements["away_delete_all"]["initial_options"])
	}
}

// TestBuildAwayManagementModalView_EmptyPrefillOmitsInitials: Slack rejects
// an empty initial_date / initial_time / initial_user outright, so a blank
// field must leave the key out instead of sending "".
func TestBuildAwayManagementModalView_EmptyPrefillOmitsInitials(t *testing.T) {
	view := BuildAwayManagementModalView(AwayManagementModalInputs{
		ChannelID: "C12345",
		UserID:    "U777",
		Lang:      "ja",
		Prefill:   AwayModalPrefill{AllDay: false},
	})
	for _, b := range view["blocks"].([]map[string]any) {
		element, ok := b["element"].(map[string]any)
		if !ok {
			continue
		}
		for _, key := range []string{"initial_user", "initial_date", "initial_time", "initial_value", "initial_options"} {
			if v, present := element[key]; present {
				t.Errorf("%s.%s = %v, want absent for an empty prefill", b["block_id"], key, v)
			}
		}
	}
}

// TestAwayModalPrefillFromState pins the read side of a re-render: the
// block_actions payload carries view.state.values, and everything the user
// has entered so far must come back out as a prefill. selected_date is what
// Slack's datepicker sends; value is the fallback the parser also honors.
func TestAwayModalPrefillFromState(t *testing.T) {
	values := map[string]map[string]ViewStateValue{
		"away_user":       {"away_user": {SelectedUser: "U999"}},
		"away_from":       {"away_from": {SelectedDate: "2030-04-01"}},
		"away_from_time":  {"away_from_time": {SelectedTime: "09:00"}},
		"away_until":      {"away_until": {Value: "2030-04-02"}},
		"away_until_time": {"away_until_time": {SelectedTime: "17:30"}},
		"away_reason":     {"away_reason": {Value: "  vacation  "}},
		"away_all_day":    {"away_all_day": {SelectedOptions: []ViewSelectedOption{{Value: "yes"}}}},
		"away_delete_all": {"away_delete_all": {SelectedOptions: []ViewSelectedOption{{Value: "yes"}}}},
	}
	got := AwayModalPrefillFromState(values)
	want := AwayModalPrefill{
		AllDay:    true,
		UserID:    "U999",
		From:      "2030-04-01",
		FromTime:  "09:00",
		Until:     "2030-04-02",
		UntilTime: "17:30",
		Reason:    "vacation",
		DeleteAll: true,
	}
	if got != want {
		t.Errorf("prefill = %+v, want %+v", got, want)
	}
}

// TestAwayModalPrefillFromState_Unticked: an unticked box arrives as an empty
// selected_options list, and a payload with no state at all must not panic.
// Both read as "not all-day"; the re-render then shows the time pickers.
func TestAwayModalPrefillFromState_Unticked(t *testing.T) {
	values := map[string]map[string]ViewStateValue{
		"away_all_day": {"away_all_day": {SelectedOptions: []ViewSelectedOption{}}},
	}
	if got := AwayModalPrefillFromState(values); got.AllDay {
		t.Errorf("empty selected_options must read as unticked, got %+v", got)
	}
	if got := AwayModalPrefillFromState(nil); got != (AwayModalPrefill{}) {
		t.Errorf("nil state must yield the zero prefill, got %+v", got)
	}
}

// TestParseAwayModalSubmission_AllDayIgnoresTimes pins the meaning of the
// all-day box on the submit side. The pickers are normally gone from the view
// while it is ticked, but two quick toggles can race the re-render and leave
// both on screen; the ticked box must then win and the leave must cover the
// whole selected days, exactly as if no time had been picked.
func TestParseAwayModalSubmission_AllDayIgnoresTimes(t *testing.T) {
	jst, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		t.Fatalf("load JST: %v", err)
	}
	v := minimalAwayValues()
	v["away_from"] = map[string]ViewStateValue{"away_from": {Value: "2030-04-01"}}
	v["away_from_time"] = map[string]ViewStateValue{"away_from_time": {SelectedTime: "09:00"}}
	v["away_until"] = map[string]ViewStateValue{"away_until": {Value: "2030-04-01"}}
	v["away_until_time"] = map[string]ViewStateValue{"away_until_time": {SelectedTime: "17:00"}}
	v["away_all_day"] = map[string]ViewStateValue{
		"away_all_day": {SelectedOptions: []ViewSelectedOption{{Value: "yes"}}},
	}

	form, err := ParseAwayModalSubmission(v, jst, "ja")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	wantFrom := time.Date(2030, 4, 1, 0, 0, 0, 0, jst)
	if form.AwayFrom == nil || !form.AwayFrom.Equal(wantFrom) {
		t.Errorf("AwayFrom = %v, want %v (time picker ignored)", form.AwayFrom, wantFrom)
	}
	wantUntil := time.Date(2030, 4, 1, 23, 59, 59, 0, jst)
	if form.AwayUntil == nil || !form.AwayUntil.Equal(wantUntil) {
		t.Errorf("AwayUntil = %v, want %v (time picker ignored)", form.AwayUntil, wantUntil)
	}
}

// TestParseAwayModalSubmission_AllDayTimeWithoutDateAccepted: with the box
// ticked a stray time is discarded, so it must not raise the
// date_required_for_time error that guards the timed path. An indefinite
// all-day leave stays possible whatever a hidden picker still holds.
func TestParseAwayModalSubmission_AllDayTimeWithoutDateAccepted(t *testing.T) {
	v := minimalAwayValues()
	v["away_from_time"] = map[string]ViewStateValue{"away_from_time": {SelectedTime: "09:00"}}
	v["away_all_day"] = map[string]ViewStateValue{
		"away_all_day": {SelectedOptions: []ViewSelectedOption{{Value: "yes"}}},
	}

	form, err := ParseAwayModalSubmission(v, time.UTC, "ja")
	if err != nil {
		t.Fatalf("all-day must discard the stray time, got: %v", err)
	}
	if form.AwayFrom != nil || form.AwayUntil != nil {
		t.Errorf("dates = %v / %v, want nil / nil (indefinite)", form.AwayFrom, form.AwayUntil)
	}
}
