package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slack-review-notify/models"
	"slack-review-notify/services"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// TestHelpRendersUserMappingButton verifies the help command surfaces the new
// "open user mapping modal" button alongside the existing settings buttons.
// Without this button, operators have no in-Slack path to fix the legacy
// non-U-id rows that caused the original bug.
func TestHelpRendersUserMappingButton(t *testing.T) {
	db := setupTestDB(t)
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()

	router := setupTestRouter(db)

	form := url.Values{}
	form.Add("command", "/slack-review-notify")
	form.Add("text", "help")
	form.Add("channel_id", "C12345")
	form.Add("user_id", "U12345")

	req, _ := http.NewRequest("POST", "/slack/command", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, services.OpenUserMappingActionID,
		"help must include the user-mapping action id so the button routes to the modal")
	assert.Contains(t, body, "ユーザーマッピング",
		"help must include the user-mapping button label")
}

// TestUserMappingModal_Submission_Upsert exercises the view_submission code
// path end-to-end: a fresh DB receives an upsert and a follow-up modify.
func TestUserMappingModal_Submission_Upsert(t *testing.T) {
	db := setupTestDB(t)
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/actions", HandleSlackAction(db))

	payload := buildUserMappingSubmissionPayload(t, "octocat", "U01ABCDE234", false)
	body := url.Values{}
	body.Set("payload", payload)

	req, _ := http.NewRequest("POST", "/slack/actions", strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var got models.UserMapping
	err := db.Where("github_username = ?", "octocat").First(&got).Error
	assert.NoError(t, err)
	assert.Equal(t, "U01ABCDE234", got.SlackUserID)

	// Second submission for the same github user should overwrite the slack id.
	payload = buildUserMappingSubmissionPayload(t, "octocat", "U02XYZ", false)
	body.Set("payload", payload)
	req, _ = http.NewRequest("POST", "/slack/actions", strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	err = db.Where("github_username = ?", "octocat").First(&got).Error
	assert.NoError(t, err)
	assert.Equal(t, "U02XYZ", got.SlackUserID)
}

// TestUserMappingModal_Submission_Delete confirms the delete checkbox path
// removes the row instead of upserting it.
func TestUserMappingModal_Submission_Delete(t *testing.T) {
	db := setupTestDB(t)
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()

	// Seed an existing legacy row (slack_user_id is a plain handle, not a U-id).
	db.Create(&models.UserMapping{
		ID:             "seed",
		GithubUsername: "octocat",
		SlackUserID:    "octocat",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/actions", HandleSlackAction(db))

	payload := buildUserMappingSubmissionPayload(t, "octocat", "", true)
	body := url.Values{}
	body.Set("payload", payload)
	req, _ := http.NewRequest("POST", "/slack/actions", strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var count int64
	db.Model(&models.UserMapping{}).Where("github_username = ?", "octocat").Count(&count)
	assert.Equal(t, int64(0), count, "delete should hard-remove the row so re-mapping the same github user later doesn't collide on the unique index")
}

// TestUserMappingModal_Submission_ValidationError returns response_action=errors
// when the parser rejects the payload. We verify the response shape so Slack
// renders the field-level error rather than silently closing the modal.
func TestUserMappingModal_Submission_ValidationError(t *testing.T) {
	db := setupTestDB(t)
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/actions", HandleSlackAction(db))

	// Empty github_username + no slack user → validation errors.
	payload := buildUserMappingSubmissionPayload(t, "", "", false)
	body := url.Values{}
	body.Set("payload", payload)
	req, _ := http.NewRequest("POST", "/slack/actions", strings.NewReader(body.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"response_action":"errors"`)
}

// buildUserMappingSubmissionPayload assembles a view_submission JSON payload
// in the shape Slack would actually post when the user mapping modal is
// submitted. Kept locally — the away modal tests have a very similar helper.
func buildUserMappingSubmissionPayload(t *testing.T, github, slackUser string, del bool) string {
	t.Helper()

	values := map[string]map[string]any{
		services.UserMappingGithubBlockID: {
			services.UserMappingGithubBlockID: map[string]any{
				"type":  "plain_text_input",
				"value": github,
			},
		},
		services.UserMappingSlackUserBlockID: {
			services.UserMappingSlackUserBlockID: map[string]any{
				"type":          "users_select",
				"selected_user": slackUser,
			},
		},
	}
	if del {
		values[services.UserMappingDeleteBlockID] = map[string]any{
			services.UserMappingDeleteBlockID: map[string]any{
				"type": "checkboxes",
				"selected_options": []map[string]any{
					{"value": "yes"},
				},
			},
		}
	}

	view := map[string]any{
		"id":               "V123",
		"callback_id":      services.UserMappingModalCallbackID,
		"private_metadata": services.EncodeUserMappingModalMetadata(services.UserMappingModalMetadata{ChannelID: "C12345", UserID: "U12345"}),
		"state":            map[string]any{"values": values},
	}

	payload := map[string]any{
		"type": "view_submission",
		"user": map[string]any{"id": "U12345"},
		"view": view,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("payload marshal failed: %v", err)
	}
	// Sanity check
	if !bytes.Contains(b, []byte(services.UserMappingModalCallbackID)) {
		t.Fatal("callback id missing from payload")
	}
	return string(b)
}
