package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestLookupSlackSubteamID_ResolvesByHandle ensures usergroups.list lookup
// matches by handle and by name, and skips deleted (date_delete != 0) groups.
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

// TestLookupSlackSubteamID_NotFound returns a typed NotFound error so callers
// can surface validation messages without sniffing string content.
func TestLookupSlackSubteamID_NotFound(t *testing.T) {
	prev := IsTestMode
	IsTestMode = false
	defer func() { IsTestMode = prev }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"usergroups":[]}`))
	}))
	defer ts.Close()
	t.Setenv("SLACK_API_BASE_URL", ts.URL)
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	ResetSlackLookupCache()

	_, err := LookupSlackSubteamID("@nobody")
	if err == nil {
		t.Fatalf("expected NotFound error")
	}
	if _, ok := err.(*SlackLookupNotFoundError); !ok {
		t.Errorf("expected *SlackLookupNotFoundError, got %T: %v", err, err)
	}
}
