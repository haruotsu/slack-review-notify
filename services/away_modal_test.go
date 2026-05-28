package services

import (
	"testing"
	"time"
)

// TestBuildAwayManagementModalView_HasAllFields verifies the view contains
// every block_id the submission handler will read back. Slack drops unknown
// fields silently, so a missing block_id here would make the corresponding
// form value invisible at parse time — a class of bug that's worth pinning
// down structurally.
func TestBuildAwayManagementModalView_HasAllFields(t *testing.T) {
	view := BuildAwayManagementModalView(AwayManagementModalInputs{
		ChannelID: "C12345",
		UserID:    "U777",
		Lang:      "ja",
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
		"away_until",
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
// must round-trip through private_metadata so the submission handler can
// post an ephemeral confirmation back into the same channel.
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
		"away_until":      {"away_until": {Value: ""}},
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
	_, err := ParseAwayModalSubmission(v)
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
	form, err := ParseAwayModalSubmission(minimalAwayValues())
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
	form, err := ParseAwayModalSubmission(v)
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
	_, err := ParseAwayModalSubmission(v)
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
	_, err := ParseAwayModalSubmission(v)
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

// TestParseAwayModalSubmission_DeleteAll: checkbox checked → DeleteAll=true.
// Per the field semantics (slash command `unset-away @user` without dates),
// this wipes every record for the user, so we don't require the date fields.
func TestParseAwayModalSubmission_DeleteAll(t *testing.T) {
	v := minimalAwayValues()
	v["away_delete_all"] = map[string]ViewStateValue{
		"away_delete_all": {SelectedOptions: []ViewSelectedOption{{Value: "yes"}}},
	}
	form, err := ParseAwayModalSubmission(v)
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
