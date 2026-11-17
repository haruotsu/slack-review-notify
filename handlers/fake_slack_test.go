package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slack-review-notify/services"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const fakeSlackSigningSecret = "test-signing-secret"

// slackCall is one request the bot made to the Slack Web API.
type slackCall struct {
	Path string
	Body map[string]any
	Raw  string
}

// startFakeSlack points the bot at a local stand-in for the Slack Web API and
// leaves test mode, so the calls test mode would skip are really made and can
// be inspected. Requests to the bot must then be signed: use postSignedPayload.
func startFakeSlack(t *testing.T) <-chan slackCall {
	t.Helper()
	calls := make(chan slackCall, 16)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		call := slackCall{Path: r.URL.Path, Raw: string(raw)}
		_ = json.Unmarshal(raw, &call.Body)
		calls <- call
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)

	t.Setenv("SLACK_API_BASE_URL", server.URL)
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("SLACK_SIGNING_SECRET", fakeSlackSigningSecret)
	prev := services.IsTestMode
	services.IsTestMode = false
	t.Cleanup(func() { services.IsTestMode = prev })

	return calls
}

func postSignedPayload(t *testing.T, router http.Handler, payloadJSON string) *httptest.ResponseRecorder {
	t.Helper()
	form := url.Values{}
	form.Add("payload", payloadJSON)
	return postSignedForm(t, router, "/slack/actions", form)
}

func postSignedForm(t *testing.T, router http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	body := form.Encode()

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, []byte(fakeSlackSigningSecret))
	mac.Write([]byte("v0:" + timestamp + ":" + body))

	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Slack-Request-Timestamp", timestamp)
	req.Header.Set("X-Slack-Signature", "v0="+hex.EncodeToString(mac.Sum(nil)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// nextSlackCall takes the bot's next Slack API call. Nothing is waited for:
// TestMain makes runAsync synchronous, so every call a request causes has
// already been made by the time the request returns.
func nextSlackCall(t *testing.T, calls <-chan slackCall) slackCall {
	t.Helper()
	select {
	case call := <-calls:
		return call
	default:
		require.FailNow(t, "the bot made no Slack API call")
		return slackCall{}
	}
}

func requireNoSlackCall(t *testing.T, calls <-chan slackCall) {
	t.Helper()
	// In test mode the bot never calls Slack, so "no call" would prove nothing.
	require.False(t, services.IsTestMode, "startFakeSlack must run after the router is set up")
	select {
	case call := <-calls:
		require.FailNow(t, "unexpected Slack API call", "%s %s", call.Path, call.Raw)
	default:
	}
}

func slackCallView(t *testing.T, call slackCall) map[string]any {
	t.Helper()
	view, ok := call.Body["view"].(map[string]any)
	require.True(t, ok, "call carries no view: %s", call.Raw)
	return view
}
