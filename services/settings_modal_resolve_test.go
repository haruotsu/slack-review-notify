package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestParseSettingsModalSubmission_ResolvesAtHandles confirms that @display_name
// entries in the mention and reviewer fields are resolved to U-prefixed IDs
// before storage. Mention also accepts subteam handles.
func TestParseSettingsModalSubmission_ResolvesAtHandles(t *testing.T) {
	prev := IsTestMode
	IsTestMode = false
	defer func() { IsTestMode = prev }()

	mux := http.NewServeMux()
	mux.HandleFunc("/users.list", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"members":[
			{"id":"UHARU","name":"haruotsu","profile":{"display_name":"haruotsu"}},
			{"id":"UALICE","name":"alice","profile":{"display_name":"alice"}},
			{"id":"UBOB","name":"bob","profile":{"display_name":"bob"}}
		],"response_metadata":{"next_cursor":""}}`))
	})
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
	values["default_mention_id"] = map[string]ViewStateValue{"default_mention_id": {Value: "@devs"}}
	values["reviewer_list"] = map[string]ViewStateValue{"reviewer_list": {Value: "@haruotsu, @alice ,UBOB"}}

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

// TestParseSettingsModalSubmission_UnknownHandleFails surfaces a per-field
// validation error pointing at the field that couldn't be resolved.
func TestParseSettingsModalSubmission_UnknownHandleFails(t *testing.T) {
	prev := IsTestMode
	IsTestMode = false
	defer func() { IsTestMode = prev }()

	mux := http.NewServeMux()
	mux.HandleFunc("/users.list", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"members":[],"response_metadata":{"next_cursor":""}}`))
	})
	mux.HandleFunc("/usergroups.list", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"usergroups":[]}`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()
	t.Setenv("SLACK_API_BASE_URL", ts.URL)
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	ResetSlackLookupCache()

	values := minimalValidParseValues()
	values["reviewer_list"] = map[string]ViewStateValue{"reviewer_list": {Value: "@ghost"}}

	_, err := ParseSettingsModalSubmission(values)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	ve, ok := err.(*ModalValidationError)
	if !ok {
		t.Fatalf("expected ModalValidationError, got %T", err)
	}
	if msg, has := ve.Errors["reviewer_list"]; !has || !strings.Contains(msg, "ghost") {
		t.Errorf("expected reviewer_list error mentioning ghost, got %+v", ve.Errors)
	}
}

func minimalValidParseValues() map[string]map[string]ViewStateValue {
	return map[string]map[string]ViewStateValue{
		"label_select":               {"label_select": {SelectedOption: &ViewSelectedOption{Value: "needs-review"}}},
		"default_mention_id":         {"default_mention_id": {Value: "U99999"}},
		"reviewer_list":              {"reviewer_list": {Value: ""}},
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
