package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// SlackLookupNotFoundError is returned when a handle cannot be resolved to a
// Slack subteam. Callers (modal validators) match on this type to surface
// field-level errors without sniffing strings.
type SlackLookupNotFoundError struct {
	Handle string
}

func (e *SlackLookupNotFoundError) Error() string {
	return fmt.Sprintf("could not resolve %q to a Slack subteam", e.Handle)
}

// lookupCacheTTL is short enough that newly-added subteams become visible
// quickly while still avoiding hitting usergroups.list on every form submit.
const lookupCacheTTL = 5 * time.Minute

type lookupCache struct {
	mu                sync.Mutex
	subteamsFetchedAt time.Time
	subteamsByName    map[string]string // lowercased handle/name → S-prefixed ID
}

var slackLookup = &lookupCache{}

// ResetSlackLookupCache wipes the in-memory cache. Tests call this to isolate runs.
func ResetSlackLookupCache() {
	slackLookup.mu.Lock()
	slackLookup.subteamsFetchedAt = time.Time{}
	slackLookup.subteamsByName = nil
	slackLookup.mu.Unlock()
}

// looksLikeUserID returns true when the input is already a Slack user ID
// (U… or W… prefix followed by uppercase alphanumeric). Used to choose which
// modal field (user picker vs. subteam input) to pre-fill from a stored value.
func looksLikeUserID(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return false
	}
	if s[0] != 'U' && s[0] != 'W' {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	return true
}

// looksLikeSubteamID returns true for S-prefixed IDs.
func looksLikeSubteamID(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != 'S' {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	return true
}

// LookupSlackSubteamID resolves a handle like "@backend" or "Backend Team" to
// an S-prefixed subteam ID via usergroups.list. Deleted groups (date_delete!=0)
// are ignored. Pre-resolved S-IDs pass through.
func LookupSlackSubteamID(handle string) (string, error) {
	handle = strings.TrimSpace(handle)
	if handle == "" {
		return "", &SlackLookupNotFoundError{Handle: handle}
	}
	if looksLikeSubteamID(handle) {
		return handle, nil
	}
	if IsTestMode {
		return handle, nil
	}

	idx, err := getSubteamsIndex()
	if err != nil {
		return "", err
	}
	key := strings.ToLower(strings.TrimPrefix(handle, "@"))
	if id, ok := idx[key]; ok {
		return id, nil
	}
	return "", &SlackLookupNotFoundError{Handle: handle}
}

func getSubteamsIndex() (map[string]string, error) {
	slackLookup.mu.Lock()
	if slackLookup.subteamsByName != nil && time.Since(slackLookup.subteamsFetchedAt) < lookupCacheTTL {
		idx := slackLookup.subteamsByName
		slackLookup.mu.Unlock()
		return idx, nil
	}
	slackLookup.mu.Unlock()

	idx, err := fetchSubteamsIndex()
	if err != nil {
		return nil, err
	}
	slackLookup.mu.Lock()
	slackLookup.subteamsByName = idx
	slackLookup.subteamsFetchedAt = time.Now()
	slackLookup.mu.Unlock()
	return idx, nil
}

type slackSubteam struct {
	ID         string `json:"id"`
	Handle     string `json:"handle"`
	Name       string `json:"name"`
	DateDelete int64  `json:"date_delete"`
}

type usergroupsListResponse struct {
	OK         bool           `json:"ok"`
	Error      string         `json:"error"`
	Usergroups []slackSubteam `json:"usergroups"`
}

func fetchSubteamsIndex() (map[string]string, error) {
	req, err := http.NewRequest("GET", SlackAPIBaseURL()+"/usergroups.list", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("usergroups.list call failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	var parsed usergroupsListResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("usergroups.list parse error: %w (body=%s)", err, string(body))
	}
	if !parsed.OK {
		return nil, fmt.Errorf("usergroups.list error: %s", parsed.Error)
	}
	idx := map[string]string{}
	for _, g := range parsed.Usergroups {
		if g.DateDelete != 0 || g.ID == "" {
			continue
		}
		for _, name := range []string{g.Handle, g.Name} {
			key := strings.ToLower(strings.TrimSpace(name))
			if key == "" {
				continue
			}
			if _, exists := idx[key]; !exists {
				idx[key] = g.ID
			}
		}
	}
	return idx, nil
}
