package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// SlackLookupNotFoundError is returned when neither a user nor a subteam can be
// resolved from a given handle. Callers (modal validators) match on this type
// to surface field-level errors without sniffing strings.
type SlackLookupNotFoundError struct {
	Handle string
}

func (e *SlackLookupNotFoundError) Error() string {
	return fmt.Sprintf("could not resolve %q to a Slack user or subteam", e.Handle)
}

// lookupCacheTTL is short enough that newly-added users/subteams become visible
// quickly while still avoiding hitting users.list on every form submit.
const lookupCacheTTL = 5 * time.Minute

type lookupCache struct {
	mu                sync.Mutex
	usersFetchedAt    time.Time
	usersByName       map[string]string // lowercased name → user ID
	subteamsFetchedAt time.Time
	subteamsByName    map[string]string // lowercased handle/name → S-prefixed ID
}

var slackLookup = &lookupCache{}

// ResetSlackLookupCache wipes the in-memory cache. Tests call this to isolate
// runs; not exported for production callers.
func ResetSlackLookupCache() {
	slackLookup.mu.Lock()
	slackLookup.usersFetchedAt = time.Time{}
	slackLookup.usersByName = nil
	slackLookup.subteamsFetchedAt = time.Time{}
	slackLookup.subteamsByName = nil
	slackLookup.mu.Unlock()
}

// normalizeHandle trims whitespace, a leading "@", and lowercases the input.
// Returns "" if the resulting string is empty.
func normalizeHandle(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "@")
	return strings.ToLower(s)
}

// looksLikeUserID returns true when the input is already a Slack user ID
// (U… or W… prefix followed by an uppercase alphanumeric). We pass these
// through to preserve existing CSV data and avoid an API roundtrip.
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

// LookupSlackUserID resolves a handle like "@haruotsu", "haruotsu", or
// "Haruto Yokoyama" to a U-prefixed user ID via users.list. Pre-resolved IDs
// pass through. Deleted users are ignored. Returns *SlackLookupNotFoundError
// if no active user matches.
func LookupSlackUserID(handle string) (string, error) {
	handle = strings.TrimSpace(handle)
	if handle == "" {
		return "", &SlackLookupNotFoundError{Handle: handle}
	}
	if looksLikeUserID(handle) {
		return handle, nil
	}
	if IsTestMode {
		// In unit tests covering downstream code, we don't want every save to
		// reach out for Slack lookups; the test-mode shortcut keeps handles as-is.
		return handle, nil
	}

	idx, err := getUsersIndex()
	if err != nil {
		return "", err
	}
	key := normalizeHandle(handle)
	if id, ok := idx[key]; ok {
		return id, nil
	}
	return "", &SlackLookupNotFoundError{Handle: handle}
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
	key := normalizeHandle(handle)
	if id, ok := idx[key]; ok {
		return id, nil
	}
	return "", &SlackLookupNotFoundError{Handle: handle}
}

// ResolveMentionTarget tries user resolution first; on NotFound, falls back to
// subteam resolution. Pre-resolved IDs short-circuit either path. Used for the
// modal's default-mention field, which historically accepted both.
func ResolveMentionTarget(handle string) (string, error) {
	handle = strings.TrimSpace(handle)
	if handle == "" {
		return "", &SlackLookupNotFoundError{Handle: handle}
	}
	if looksLikeUserID(handle) || looksLikeSubteamID(handle) {
		return handle, nil
	}
	id, err := LookupSlackUserID(handle)
	if err == nil {
		return id, nil
	}
	if _, isNotFound := err.(*SlackLookupNotFoundError); !isNotFound {
		return "", err
	}
	// User lookup miss → try subteam.
	id, err = LookupSlackSubteamID(handle)
	if err == nil {
		return id, nil
	}
	return "", &SlackLookupNotFoundError{Handle: handle}
}

func getUsersIndex() (map[string]string, error) {
	slackLookup.mu.Lock()
	if slackLookup.usersByName != nil && time.Since(slackLookup.usersFetchedAt) < lookupCacheTTL {
		idx := slackLookup.usersByName
		slackLookup.mu.Unlock()
		return idx, nil
	}
	slackLookup.mu.Unlock()

	idx, err := fetchUsersIndex()
	if err != nil {
		return nil, err
	}
	slackLookup.mu.Lock()
	slackLookup.usersByName = idx
	slackLookup.usersFetchedAt = time.Now()
	slackLookup.mu.Unlock()
	return idx, nil
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

type slackUser struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Deleted bool   `json:"deleted"`
	Profile struct {
		DisplayName string `json:"display_name"`
		RealName    string `json:"real_name"`
	} `json:"profile"`
}

type usersListResponse struct {
	OK       bool        `json:"ok"`
	Error    string      `json:"error"`
	Members  []slackUser `json:"members"`
	Metadata struct {
		NextCursor string `json:"next_cursor"`
	} `json:"response_metadata"`
}

// maxUsersListPages bounds users.list pagination at 200 users × 100 = 20,000
// active users — well above any realistic workspace size. The cap prevents a
// runaway loop if Slack ever returns a non-empty next_cursor in a degenerate
// response.
const maxUsersListPages = 100

// fetchUsersIndex pages through users.list and builds a name→ID map. Multiple
// name fields map to the same ID so callers can use whichever name they see in
// Slack. Deleted users are skipped.
func fetchUsersIndex() (map[string]string, error) {
	idx := map[string]string{}
	cursor := ""
	for range maxUsersListPages {
		params := url.Values{}
		params.Set("limit", "200")
		if cursor != "" {
			params.Set("cursor", cursor)
		}
		req, err := http.NewRequest("GET", SlackAPIBaseURL()+"/users.list?"+params.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+os.Getenv("SLACK_BOT_TOKEN"))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("users.list call failed: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		var parsed usersListResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("users.list parse error: %w (body=%s)", err, string(body))
		}
		if !parsed.OK {
			return nil, fmt.Errorf("users.list error: %s", parsed.Error)
		}
		for _, m := range parsed.Members {
			if m.Deleted || m.ID == "" {
				continue
			}
			for _, name := range []string{m.Name, m.Profile.DisplayName, m.Profile.RealName} {
				key := strings.ToLower(strings.TrimSpace(name))
				if key == "" {
					continue
				}
				// First write wins so an active user is not shadowed by a same-named deleted one.
				if _, exists := idx[key]; !exists {
					idx[key] = m.ID
				}
			}
		}
		if parsed.Metadata.NextCursor == "" {
			break
		}
		cursor = parsed.Metadata.NextCursor
	}
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
