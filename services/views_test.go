package services

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestOpenView_PostsCorrectPayload(t *testing.T) {
	// Other test files flip IsTestMode globally; explicitly disable for this test.
	prev := IsTestMode
	IsTestMode = false
	defer func() { IsTestMode = prev }()

	var gotPath, gotAuth, gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"view":{"id":"V1"}}`))
	}))
	defer ts.Close()

	t.Setenv("SLACK_API_BASE_URL", ts.URL)
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")

	view := map[string]interface{}{"type": "modal", "callback_id": "x"}
	if err := OpenView("trigger123", view); err != nil {
		t.Fatalf("OpenView returned error: %v", err)
	}
	if gotPath != "/views.open" {
		t.Errorf("path = %s, want /views.open", gotPath)
	}
	if gotAuth != "Bearer xoxb-test" {
		t.Errorf("auth = %s", gotAuth)
	}

	var payload struct {
		TriggerID string                 `json:"trigger_id"`
		View      map[string]interface{} `json:"view"`
	}
	if err := json.Unmarshal([]byte(gotBody), &payload); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, gotBody)
	}
	if payload.TriggerID != "trigger123" {
		t.Errorf("trigger_id = %q", payload.TriggerID)
	}
	if payload.View["callback_id"] != "x" {
		t.Errorf("view.callback_id = %v", payload.View["callback_id"])
	}
}

func TestOpenView_TestModeNoOp(t *testing.T) {
	IsTestMode = true
	defer func() { IsTestMode = false }()

	// Server should not be called; if it is, this will fail because BASE_URL is unreachable.
	_ = os.Unsetenv("SLACK_API_BASE_URL")
	if err := OpenView("trigger123", map[string]interface{}{"type": "modal"}); err != nil {
		t.Fatalf("OpenView in test mode returned error: %v", err)
	}
}

func TestOpenView_SlackErrorReturnsError(t *testing.T) {
	prev := IsTestMode
	IsTestMode = false
	defer func() { IsTestMode = prev }()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"error":"invalid_trigger_id"}`))
	}))
	defer ts.Close()

	t.Setenv("SLACK_API_BASE_URL", ts.URL)
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")

	err := OpenView("trig", map[string]interface{}{})
	if err == nil {
		t.Fatalf("expected error from non-ok response")
	}
}
