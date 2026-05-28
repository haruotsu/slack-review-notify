package services

import (
	"encoding/json"
	"slack-review-notify/models"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildUserMappingModalView_HasAllFields(t *testing.T) {
	view := BuildUserMappingModalView(UserMappingModalInputs{
		ChannelID: "C12345",
		UserID:    "U99999",
		Lang:      "ja",
	})

	if view["type"] != "modal" {
		t.Errorf("type = %v, want modal", view["type"])
	}
	if view["callback_id"] != UserMappingModalCallbackID {
		t.Errorf("callback_id = %v, want %s", view["callback_id"], UserMappingModalCallbackID)
	}

	// The user picker uses users_select so Slack guarantees a resolved U-id.
	body, err := json.Marshal(view)
	assert.NoError(t, err)
	assert.Contains(t, string(body), `"type":"users_select"`)
	assert.Contains(t, string(body), `"block_id":"user_mapping_slack_user"`)
	assert.Contains(t, string(body), `"block_id":"user_mapping_github_username"`)
	assert.Contains(t, string(body), `"block_id":"user_mapping_delete"`)
}

func TestBuildUserMappingModalView_RendersExistingMappings(t *testing.T) {
	view := BuildUserMappingModalView(UserMappingModalInputs{
		ChannelID: "C12345",
		UserID:    "U99999",
		Lang:      "ja",
		Mappings: []models.UserMapping{
			{GithubUsername: "octocat", SlackUserID: "U01ABCDE234"},
			{GithubUsername: "legacy-plain", SlackUserID: "legacy-plain"},
		},
	})

	body, err := json.Marshal(view)
	assert.NoError(t, err)
	// Both github usernames must surface; the legacy non-U-id row must be
	// rendered with a flag so the operator notices it needs re-registration.
	assert.Contains(t, string(body), "octocat")
	assert.Contains(t, string(body), "legacy-plain")
	assert.Contains(t, string(body), "U01ABCDE234")
}

func TestBuildUserMappingModalView_PrivateMetadata(t *testing.T) {
	view := BuildUserMappingModalView(UserMappingModalInputs{
		ChannelID: "C_META",
		UserID:    "U_META",
		Lang:      "ja",
	})
	pm, _ := view["private_metadata"].(string)
	if pm == "" {
		t.Fatal("private_metadata is empty")
	}
	meta, err := DecodeUserMappingModalMetadata(pm)
	assert.NoError(t, err)
	assert.Equal(t, "C_META", meta.ChannelID)
	assert.Equal(t, "U_META", meta.UserID)
}

func TestParseUserMappingModalSubmission_Upsert(t *testing.T) {
	values := map[string]map[string]ViewStateValue{
		"user_mapping_github_username": {
			"user_mapping_github_username": {Value: "  octocat  "},
		},
		"user_mapping_slack_user": {
			"user_mapping_slack_user": {SelectedUser: "U01ABCDE234"},
		},
	}
	form, err := ParseUserMappingModalSubmission(values, "ja")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assert.Equal(t, "octocat", form.GithubUsername)
	assert.Equal(t, "U01ABCDE234", form.SlackUserID)
	assert.False(t, form.Delete)
}

func TestParseUserMappingModalSubmission_Delete(t *testing.T) {
	values := map[string]map[string]ViewStateValue{
		"user_mapping_github_username": {
			"user_mapping_github_username": {Value: "octocat"},
		},
		"user_mapping_delete": {
			"user_mapping_delete": {
				SelectedOptions: []ViewSelectedOption{{Value: "yes"}},
			},
		},
	}
	form, err := ParseUserMappingModalSubmission(values, "ja")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assert.True(t, form.Delete)
	assert.Equal(t, "octocat", form.GithubUsername)
	// SlackUserID isn't required for delete, so an empty value is fine.
	assert.Equal(t, "", form.SlackUserID)
}

func TestParseUserMappingModalSubmission_GithubRequired(t *testing.T) {
	values := map[string]map[string]ViewStateValue{
		"user_mapping_github_username": {
			"user_mapping_github_username": {Value: " "},
		},
		"user_mapping_slack_user": {
			"user_mapping_slack_user": {SelectedUser: "U01ABCDE234"},
		},
	}
	_, err := ParseUserMappingModalSubmission(values, "ja")
	if err == nil {
		t.Fatal("expected validation error")
	}
	ve, ok := err.(*ModalValidationError)
	if !ok {
		t.Fatalf("expected *ModalValidationError, got %T", err)
	}
	if _, found := ve.Errors["user_mapping_github_username"]; !found {
		t.Errorf("expected error for github_username, got %v", ve.Errors)
	}
}

func TestParseUserMappingModalSubmission_SlackUserRequiredWhenNotDeleting(t *testing.T) {
	values := map[string]map[string]ViewStateValue{
		"user_mapping_github_username": {
			"user_mapping_github_username": {Value: "octocat"},
		},
		// no user_mapping_slack_user value at all
	}
	_, err := ParseUserMappingModalSubmission(values, "ja")
	if err == nil {
		t.Fatal("expected validation error")
	}
	ve, ok := err.(*ModalValidationError)
	if !ok {
		t.Fatalf("expected *ModalValidationError, got %T", err)
	}
	if _, found := ve.Errors["user_mapping_slack_user"]; !found {
		t.Errorf("expected error for slack_user, got %v", ve.Errors)
	}
}

// The picker always returns a resolved U-id, but if Slack ever fed us a
// non-resolved value we'd want to reject it rather than rewrite legacy data.
func TestParseUserMappingModalSubmission_RejectsNonResolvedSlackID(t *testing.T) {
	values := map[string]map[string]ViewStateValue{
		"user_mapping_github_username": {
			"user_mapping_github_username": {Value: "octocat"},
		},
		"user_mapping_slack_user": {
			"user_mapping_slack_user": {SelectedUser: "not-a-user-id"},
		},
	}
	_, err := ParseUserMappingModalSubmission(values, "ja")
	if err == nil {
		t.Fatal("expected validation error for non-resolved slack id")
	}
}

// TestUserMappingModalConstants pins the action_id / callback_id strings so
// the slack action router never silently breaks when refactoring.
func TestUserMappingModalConstants(t *testing.T) {
	assert.Equal(t, "user_mapping_modal", UserMappingModalCallbackID)
	assert.Equal(t, "open_user_mapping", OpenUserMappingActionID)
	// Modal view title — should be derivable; just sanity check the kind of
	// string we expect.
	view := BuildUserMappingModalView(UserMappingModalInputs{Lang: "ja"})
	title, _ := view["title"].(map[string]any)
	if title == nil || !strings.Contains(title["text"].(string), "ユーザーマッピング") {
		t.Errorf("modal title missing expected ja string: %v", view["title"])
	}
}
