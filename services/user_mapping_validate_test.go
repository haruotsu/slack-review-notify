package services

import (
	"slack-review-notify/models"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestFindLegacyUserMappings returns every UserMapping whose SlackUserID is
// not a resolved Slack user id (U… / W…). Legacy plain-handle rows stored
// before the modal picker landed silently break PR-author exclusion, so we
// surface them to the operator at startup rather than trusting them.
func TestFindLegacyUserMappings(t *testing.T) {
	db := setupTestDB(t)

	db.Create(&models.UserMapping{
		ID:             "1",
		GithubUsername: "legacy-plain",
		SlackUserID:    "legacy-plain", // legacy: plain handle
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	})
	db.Create(&models.UserMapping{
		ID:             "2",
		GithubUsername: "octocat",
		SlackUserID:    "U01ABCDE234", // canonical
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	})
	db.Create(&models.UserMapping{
		ID:             "3",
		GithubUsername: "enterprise",
		SlackUserID:    "W01ABCDE234", // canonical (enterprise grid)
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	})
	db.Create(&models.UserMapping{
		ID:             "4",
		GithubUsername: "broken",
		SlackUserID:    "U01ABCDE234|broken", // historical bug: leftover display name
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	})

	legacy := FindLegacyUserMappings(db)

	wantGithubs := map[string]bool{
		"legacy-plain": true,
		"broken":       true,
	}
	gotGithubs := map[string]bool{}
	for _, m := range legacy {
		gotGithubs[m.GithubUsername] = true
	}
	assert.Equal(t, wantGithubs, gotGithubs)
}

// TestLooksLikeResolvedSlackUserID — the shared classifier used by both the
// startup audit and the slash-command validator.
func TestLooksLikeResolvedSlackUserID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"U01ABCDE234", true},
		{"W01ABCDE234", true},
		{"octocat", false},
		{"U01ABCDE234|octocat", false},
		{"", false},
		{"<@U01ABCDE234>", false},
		{"S0123ABCDEF", false},
	}
	for _, c := range cases {
		got := LooksLikeResolvedSlackUserID(c.in)
		if got != c.want {
			t.Errorf("LooksLikeResolvedSlackUserID(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
