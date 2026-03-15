package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"slack-review-notify/models"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Test 1: set-language en updates existing config to English
func TestSetLanguage_UpdateToEnglish(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	// Create existing config with default language (ja)
	db.Create(&models.ChannelConfig{
		ID:             "test-lang-1",
		SlackChannelID: "C12345",
		LabelName:      "needs-review",
		Language:        "ja",
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	})

	form := url.Values{}
	form.Add("command", "/slack-review-notify")
	form.Add("text", "set-language en")
	form.Add("channel_id", "C12345")
	form.Add("user_id", "U12345")

	req, _ := http.NewRequest("POST", "/slack/command", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	// Response should be in English (the newly set language)
	assert.Contains(t, w.Body.String(), "Updated language")
	assert.Contains(t, w.Body.String(), "en")

	// Verify DB
	var config models.ChannelConfig
	db.Where("slack_channel_id = ? AND label_name = ?", "C12345", "needs-review").First(&config)
	assert.Equal(t, "en", config.Language)
}

// Test 2: set-language ja updates existing config to Japanese
func TestSetLanguage_UpdateToJapanese(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	// Create existing config with English
	db.Create(&models.ChannelConfig{
		ID:             "test-lang-2",
		SlackChannelID: "C12345",
		LabelName:      "needs-review",
		Language:        "en",
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	})

	form := url.Values{}
	form.Add("command", "/slack-review-notify")
	form.Add("text", "set-language ja")
	form.Add("channel_id", "C12345")
	form.Add("user_id", "U12345")

	req, _ := http.NewRequest("POST", "/slack/command", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	// Response should be in Japanese (the newly set language)
	assert.Contains(t, w.Body.String(), "言語を ja に更新しました")

	// Verify DB
	var config models.ChannelConfig
	db.Where("slack_channel_id = ? AND label_name = ?", "C12345", "needs-review").First(&config)
	assert.Equal(t, "ja", config.Language)
}

// Test 3: set-language fr rejects invalid language
func TestSetLanguage_InvalidLanguage(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	form := url.Values{}
	form.Add("command", "/slack-review-notify")
	form.Add("text", "set-language fr")
	form.Add("channel_id", "C12345")
	form.Add("user_id", "U12345")

	req, _ := http.NewRequest("POST", "/slack/command", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	// Should show error about unsupported language (default ja)
	assert.Contains(t, w.Body.String(), "対応していない言語です")
}

// Test 4: set-language en creates new config when none exists
func TestSetLanguage_CreatesNewConfig(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	form := url.Values{}
	form.Add("command", "/slack-review-notify")
	form.Add("text", "set-language en")
	form.Add("channel_id", "C99999")
	form.Add("user_id", "U12345")

	req, _ := http.NewRequest("POST", "/slack/command", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	// Response should be in English (the set language)
	assert.Contains(t, w.Body.String(), "Set language")
	assert.Contains(t, w.Body.String(), "en")

	// Verify new config created in DB
	var config models.ChannelConfig
	result := db.Where("slack_channel_id = ? AND label_name = ?", "C99999", "needs-review").First(&config)
	assert.NoError(t, result.Error)
	assert.Equal(t, "en", config.Language)
	assert.True(t, config.IsActive)
}

// Test 5: set-language with no argument shows usage
func TestSetLanguage_NoArgument(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	form := url.Values{}
	form.Add("command", "/slack-review-notify")
	form.Add("text", "set-language")
	form.Add("channel_id", "C12345")
	form.Add("user_id", "U12345")

	req, _ := http.NewRequest("POST", "/slack/command", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	// Should show usage message (default ja)
	assert.Contains(t, w.Body.String(), "言語を指定してください")
}

// Test 6: set-language with specific label
func TestSetLanguage_WithSpecificLabel(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	// Create existing config for "bug" label
	db.Create(&models.ChannelConfig{
		ID:             "test-lang-6",
		SlackChannelID: "C12345",
		LabelName:      "bug",
		Language:        "ja",
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	})

	form := url.Values{}
	form.Add("command", "/slack-review-notify")
	form.Add("text", "bug set-language en")
	form.Add("channel_id", "C12345")
	form.Add("user_id", "U12345")

	req, _ := http.NewRequest("POST", "/slack/command", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "bug")
	assert.Contains(t, w.Body.String(), "en")

	// Verify DB - only "bug" label updated
	var config models.ChannelConfig
	db.Where("slack_channel_id = ? AND label_name = ?", "C12345", "bug").First(&config)
	assert.Equal(t, "en", config.Language)
}

// Test 7: help command returns English when channel is set to English
func TestHelp_ReturnsEnglishWhenLanguageIsEn(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	// Create config with English language
	db.Create(&models.ChannelConfig{
		ID:             "test-lang-7",
		SlackChannelID: "C12345",
		LabelName:      "needs-review",
		Language:        "en",
		IsActive:       true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	})

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
	// Should return English help
	assert.Contains(t, w.Body.String(), "Review Notification Bot Configuration")
	assert.Contains(t, w.Body.String(), "Initial Setup (Required)")
	// Should NOT contain Japanese
	assert.NotContains(t, w.Body.String(), "Review通知Bot設定コマンド")
}

// Test 8: show command returns English when channel is set to English
func TestShow_ReturnsEnglishWhenLanguageIsEn(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	// Create config with English language
	db.Create(&models.ChannelConfig{
		ID:               "test-lang-8",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U12345",
		RepositoryList:   "owner/repo1",
		Language:          "en",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	})

	form := url.Values{}
	form.Add("command", "/slack-review-notify")
	form.Add("text", "show")
	form.Add("channel_id", "C12345")
	form.Add("user_id", "U12345")

	req, _ := http.NewRequest("POST", "/slack/command", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	// Should return English
	assert.Contains(t, w.Body.String(), "Configured labels for this channel")
	assert.Contains(t, w.Body.String(), "Active")
	// Should NOT contain Japanese
	assert.NotContains(t, w.Body.String(), "このチャンネルで設定済みのラベル")
}
