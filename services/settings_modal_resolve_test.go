package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestParseSettingsModalSubmission_ResolvesSubteamHandle confirms that an
// @team-handle entered in the subteam field is resolved to an S-prefixed
// subteam ID via usergroups.list. Individual users come pre-resolved from the
// users_select Block Kit picker so no users.list call is needed.
func TestParseSettingsModalSubmission_ResolvesSubteamHandle(t *testing.T) {
	prev := IsTestMode
	IsTestMode = false
	defer func() { IsTestMode = prev }()

	mux := http.NewServeMux()
	mux.HandleFunc("/usergroups.list", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"usergroups":[
			{"id":"SDEVS","handle":"devs","name":"Devs"}
		]}`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	t.Setenv("SLACK_API_BASE_URL", ts.URL)
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	ResetSlackLookupCache()

	values := minimalValidParseValues()
	// Clear the user picker so the subteam field is the only mention source.
	values["default_mention_user"] = map[string]ViewStateValue{
		"default_mention_user": {SelectedUser: ""},
	}
	values["default_mention_subteam"] = map[string]ViewStateValue{
		"default_mention_subteam": {Value: "@devs"},
	}
	// Reviewer pool comes from multi_users_select → already-resolved IDs.
	values["reviewer_list"] = map[string]ViewStateValue{
		"reviewer_list": {SelectedUsers: []string{"UHARU", "UALICE", "UBOB"}},
	}

	form, err := ParseSettingsModalSubmission(values)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if form.DefaultMentionID != "SDEVS" {
		t.Errorf("DefaultMentionID = %q, want SDEVS (subteam resolved)", form.DefaultMentionID)
	}
	if form.ReviewerList != "UHARU,UALICE,UBOB" {
		t.Errorf("ReviewerList = %q, want UHARU,UALICE,UBOB", form.ReviewerList)
	}
}

// TestParseSettingsModalSubmission_UserBeatsSubteam: when both the users_select
// (individual) and the subteam plain_text are filled, the individual wins.
// This makes the precedence predictable when a user keeps the subteam field
// pre-filled but later adds a user to the picker.
func TestParseSettingsModalSubmission_UserBeatsSubteam(t *testing.T) {
	prev := IsTestMode
	IsTestMode = true // bypass usergroups.list resolution
	defer func() { IsTestMode = prev }()

	values := minimalValidParseValues()
	values["default_mention_user"] = map[string]ViewStateValue{
		"default_mention_user": {SelectedUser: "UPICKER"},
	}
	values["default_mention_subteam"] = map[string]ViewStateValue{
		"default_mention_subteam": {Value: "SLEFTOVER"},
	}

	form, err := ParseSettingsModalSubmission(values)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if form.DefaultMentionID != "UPICKER" {
		t.Errorf("DefaultMentionID = %q, want UPICKER (user must win over subteam)", form.DefaultMentionID)
	}
}

// TestParseSettingsModalSubmission_UnknownSubteamHandleFails surfaces a
// per-field validation error pointing at default_mention_subteam when the
// entered @handle does not exist.
func TestParseSettingsModalSubmission_UnknownSubteamHandleFails(t *testing.T) {
	prev := IsTestMode
	IsTestMode = false
	defer func() { IsTestMode = prev }()

	mux := http.NewServeMux()
	mux.HandleFunc("/usergroups.list", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"usergroups":[]}`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	t.Setenv("SLACK_API_BASE_URL", ts.URL)
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	ResetSlackLookupCache()

	values := minimalValidParseValues()
	values["default_mention_user"] = map[string]ViewStateValue{
		"default_mention_user": {SelectedUser: ""},
	}
	values["default_mention_subteam"] = map[string]ViewStateValue{
		"default_mention_subteam": {Value: "@ghost"},
	}

	_, err := ParseSettingsModalSubmission(values)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	ve, ok := err.(*ModalValidationError)
	if !ok {
		t.Fatalf("expected ModalValidationError, got %T", err)
	}
	if msg, has := ve.Errors["default_mention_subteam"]; !has || !strings.Contains(msg, "ghost") {
		t.Errorf("expected default_mention_subteam error mentioning ghost, got %+v", ve.Errors)
	}
}

// minimalValidParseValues returns a payload representing the new Block Kit
// shape: users_select for default_mention_user, multi_users_select for
// reviewer_list, plain_text for default_mention_subteam.
func minimalValidParseValues() map[string]map[string]ViewStateValue {
	return map[string]map[string]ViewStateValue{
		"label_select":               {"label_select": {SelectedOption: &ViewSelectedOption{Value: "needs-review"}}},
		"default_mention_user":       {"default_mention_user": {SelectedUser: "U99999"}},
		"default_mention_subteam":    {"default_mention_subteam": {Value: ""}},
		"reviewer_list":              {"reviewer_list": {SelectedUsers: nil}},
		"repository_list":            {"repository_list": {Value: ""}},
		"reminder_interval":          {"reminder_interval": {Value: "30"}},
		"reviewer_reminder_interval": {"reviewer_reminder_interval": {Value: "30"}},
		"business_hours_start":       {"business_hours_start": {Value: "09:00"}},
		"business_hours_end":         {"business_hours_end": {Value: "18:00"}},
		"timezone":                   {"timezone": {Value: "Asia/Tokyo"}},
		"required_approvals":         {"required_approvals": {Value: "1"}},
		"language":                   {"language": {SelectedOption: &ViewSelectedOption{Value: "ja"}}},
		"is_active":                  {"is_active": {SelectedOption: &ViewSelectedOption{Value: "true"}}},
	}
}
