package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestLookupSlackUserID_ResolvesByName ensures @display_name is resolved to the
// U-prefixed user ID via users.list. The lookup must match against multiple
// name fields Slack exposes (display_name, real_name, name, profile.display_name)
// so users don't have to know which field is which.
func TestLookupSlackUserID_ResolvesByName(t *testing.T) {
	prev := IsTestMode
	IsTestMode = false
	defer func() { IsTestMode = prev }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/users.list") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"ok": true,
			"members": [
				{"id":"UALICE","name":"alice","profile":{"display_name":"Alice","real_name":"Alice Anderson"}},
				{"id":"UHARU","name":"haruotsu","profile":{"display_name":"haruotsu","real_name":"Haruto Yokoyama"}},
				{"id":"UDELETED","deleted":true,"name":"old-haruotsu","profile":{"display_name":"haruotsu"}}
			],
			"response_metadata": {"next_cursor": ""}
		}`))
	}))
	defer ts.Close()

	t.Setenv("SLACK_API_BASE_URL", ts.URL)
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	ResetSlackLookupCache()

	tests := []struct {
		input string
		want  string
	}{
		{"@haruotsu", "UHARU"},
		{"haruotsu", "UHARU"},
		{"@alice", "UALICE"},
		{"Alice Anderson", "UALICE"}, // matches real_name
	}
	for _, tt := range tests {
		got, err := LookupSlackUserID(tt.input)
		if err != nil {
			t.Errorf("LookupSlackUserID(%q) error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("LookupSlackUserID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestLookupSlackUserID_PassThroughIDs ensures already-resolved IDs (U… / W…)
// are returned as-is without an API call, so existing data round-trips cleanly.
func TestLookupSlackUserID_PassThroughIDs(t *testing.T) {
	prev := IsTestMode
	IsTestMode = false
	defer func() { IsTestMode = prev }()

	// Server should never be hit; if it is, we'd error.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("users.list should not be called for pre-resolved IDs")
		w.WriteHeader(500)
	}))
	defer ts.Close()
	t.Setenv("SLACK_API_BASE_URL", ts.URL)
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	ResetSlackLookupCache()

	cases := []string{"U12345", "W67890"}
	for _, in := range cases {
		got, err := LookupSlackUserID(in)
		if err != nil {
			t.Errorf("LookupSlackUserID(%q) err: %v", in, err)
		}
		if got != in {
			t.Errorf("LookupSlackUserID(%q) = %q, want pass-through", in, got)
		}
	}
}

// TestLookupSlackUserID_NotFound returns a typed NotFoundError so callers can
// surface validation messages without sniffing string content.
func TestLookupSlackUserID_NotFound(t *testing.T) {
	prev := IsTestMode
	IsTestMode = false
	defer func() { IsTestMode = prev }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"members":[],"response_metadata":{"next_cursor":""}}`))
	}))
	defer ts.Close()
	t.Setenv("SLACK_API_BASE_URL", ts.URL)
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	ResetSlackLookupCache()

	_, err := LookupSlackUserID("@nobody")
	if err == nil {
		t.Fatalf("expected NotFound error")
	}
	if _, ok := err.(*SlackLookupNotFoundError); !ok {
		t.Errorf("expected *SlackLookupNotFoundError, got %T: %v", err, err)
	}
}

// TestLookupSlackSubteamID_ResolvesByHandle ensures usergroups.list lookup matches by handle.
func TestLookupSlackSubteamID_ResolvesByHandle(t *testing.T) {
	prev := IsTestMode
	IsTestMode = false
	defer func() { IsTestMode = prev }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/usergroups.list") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"ok": true,
			"usergroups": [
				{"id":"S001","handle":"backend","name":"Backend Team","date_delete":0},
				{"id":"S002","handle":"frontend","name":"Frontend Team","date_delete":0},
				{"id":"S099","handle":"old-team","name":"old","date_delete":1700000000}
			]
		}`))
	}))
	defer ts.Close()
	t.Setenv("SLACK_API_BASE_URL", ts.URL)
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	ResetSlackLookupCache()

	tests := []struct {
		in   string
		want string
	}{
		{"@backend", "S001"},
		{"backend", "S001"},
		{"Frontend Team", "S002"}, // matches name
	}
	for _, tt := range tests {
		got, err := LookupSlackSubteamID(tt.in)
		if err != nil {
			t.Errorf("LookupSlackSubteamID(%q) error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("LookupSlackSubteamID(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestLookupSlackSubteamID_PassThroughIDs: S-prefixed IDs bypass the API.
func TestLookupSlackSubteamID_PassThroughIDs(t *testing.T) {
	prev := IsTestMode
	IsTestMode = false
	defer func() { IsTestMode = prev }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("usergroups.list should not be called for pre-resolved S-prefixed IDs")
	}))
	defer ts.Close()
	t.Setenv("SLACK_API_BASE_URL", ts.URL)
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	ResetSlackLookupCache()

	got, err := LookupSlackSubteamID("S001")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "S001" {
		t.Errorf("got %q want pass-through", got)
	}
}

// TestResolveMentionTarget_PrefersUserThenSubteam ensures the combined resolver
// tries user lookup first, then subteam, then returns NotFound. This is what
// the modal submission handler uses for the default-mention field.
func TestResolveMentionTarget_PrefersUserThenSubteam(t *testing.T) {
	prev := IsTestMode
	IsTestMode = false
	defer func() { IsTestMode = prev }()

	mux := http.NewServeMux()
	mux.HandleFunc("/users.list", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"members":[
			{"id":"UHARU","name":"haruotsu","profile":{"display_name":"haruotsu"}}
		]}`))
	})
	mux.HandleFunc("/usergroups.list", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"usergroups":[
			{"id":"S001","handle":"devs","name":"Devs"}
		]}`))
	})
	ts := httptest.NewServer(mux)
	defer ts.Close()

	t.Setenv("SLACK_API_BASE_URL", ts.URL)
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	ResetSlackLookupCache()

	if got, err := ResolveMentionTarget("@haruotsu"); err != nil || got != "UHARU" {
		t.Errorf("user resolve: got %q err %v", got, err)
	}
	if got, err := ResolveMentionTarget("@devs"); err != nil || got != "S001" {
		t.Errorf("subteam resolve: got %q err %v", got, err)
	}
	if _, err := ResolveMentionTarget("@nobody"); err == nil {
		t.Errorf("expected NotFound for unknown handle")
	}
}
