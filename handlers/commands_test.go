package handlers

import (
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
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestRouter(db *gorm.DB) *gin.Engine {
	// Enable test mode
	services.IsTestMode = true

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/slack/command", HandleSlackCommand(db))
	return r
}

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("fail to open test db: %v", err)
	}

	// Run migrations
	if err := db.AutoMigrate(&models.ChannelConfig{}, &models.ReviewTask{}, &models.UserMapping{}, &models.ReviewerAvailability{}); err != nil {
		t.Fatalf("fail to migrate test db: %v", err)
	}

	return db
}

func TestHandleSlackCommand_Help(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	// Test the help command
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
	assert.Contains(t, w.Body.String(), "Review通知Bot設定コマンド")
	assert.Contains(t, w.Body.String(), "複数ラベル設定の使い方")
	assert.Contains(t, w.Body.String(), "初期設定（必須）")
	assert.Contains(t, w.Body.String(), "この2つを設定しないと通知は送信されません")
	assert.Contains(t, w.Body.String(), "このチャンネルの全ラベル設定を表示")
	assert.Contains(t, w.Body.String(), "指定ラベルの詳細設定を表示")
}

func TestHandleSlackCommand_Show(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	// Create a test channel config
	testConfig := models.ChannelConfig{
		ID:               "test-id",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U12345",
		RepositoryList:   "owner/repo1,owner/repo2",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&testConfig)

	// Test the show command
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
	assert.Contains(t, w.Body.String(), "このチャンネルで設定済みのラベル")
	assert.Contains(t, w.Body.String(), "needs-review")
	assert.Contains(t, w.Body.String(), "有効")
}

func TestHandleSlackCommand_ShowSpecificLabel(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	// Create a test channel config
	testConfig := models.ChannelConfig{
		ID:               "test-id",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U12345",
		RepositoryList:   "owner/repo1,owner/repo2",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&testConfig)

	// Test displaying detailed settings for a specific label
	form := url.Values{}
	form.Add("command", "/slack-review-notify")
	form.Add("text", "needs-review show")
	form.Add("channel_id", "C12345")
	form.Add("user_id", "U12345")

	req, _ := http.NewRequest("POST", "/slack/command", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "このチャンネルのラベル「needs-review」のレビュー通知設定")
	assert.Contains(t, w.Body.String(), "有効")
	assert.Contains(t, w.Body.String(), "owner/repo1,owner/repo2")
}

func TestHandleSlackCommand_SetMention(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	// Test the set-mention command
	form := url.Values{}
	form.Add("command", "/slack-review-notify")
	form.Add("text", "set-mention U67890")
	form.Add("channel_id", "C12345")
	form.Add("user_id", "U12345")

	req, _ := http.NewRequest("POST", "/slack/command", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "メンション先を")

	// Verify the value in the database
	var config models.ChannelConfig
	db.Where("slack_channel_id = ? AND label_name = ?", "C12345", "needs-review").First(&config)
	assert.Equal(t, "U67890", config.DefaultMentionID)
}

func TestHandleSlackCommand_AddRepo(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	// Create a test channel config
	testConfig := models.ChannelConfig{
		ID:               "test-id",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U12345",
		RepositoryList:   "owner/repo1",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&testConfig)

	// Test the add-repo command
	form := url.Values{}
	form.Add("command", "/slack-review-notify")
	form.Add("text", "add-repo owner/repo2")
	form.Add("channel_id", "C12345")
	form.Add("user_id", "U12345")

	req, _ := http.NewRequest("POST", "/slack/command", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "通知対象リポジトリに")

	// Verify the value in the database
	var config models.ChannelConfig
	db.Where("slack_channel_id = ? AND label_name = ?", "C12345", "needs-review").First(&config)
	assert.Contains(t, config.RepositoryList, "owner/repo1")
	assert.Contains(t, config.RepositoryList, "owner/repo2")
}

// Test with label specification
func TestHandleSlackCommand_WithLabel(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	// Create a test channel config
	testConfig := models.ChannelConfig{
		ID:               "test-id",
		SlackChannelID:   "C12345",
		LabelName:        "bug",
		DefaultMentionID: "U12345",
		RepositoryList:   "owner/repo1",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&testConfig)

	// Test show command with label specification
	form := url.Values{}
	form.Add("command", "/slack-review-notify")
	form.Add("text", "bug show")
	form.Add("channel_id", "C12345")
	form.Add("user_id", "U12345")

	req, _ := http.NewRequest("POST", "/slack/command", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "このチャンネルのラベル「bug」のレビュー通知設定")
}

func TestHandleSlackCommand_ShowAllLabels(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)

	// Create multiple test channel configs
	testConfigs := []models.ChannelConfig{
		{
			ID:               "test-id-1",
			SlackChannelID:   "C12345",
			LabelName:        "needs-review",
			DefaultMentionID: "U12345",
			IsActive:         true,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		},
		{
			ID:               "test-id-2",
			SlackChannelID:   "C12345",
			LabelName:        "bug",
			DefaultMentionID: "U67890",
			IsActive:         false,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		},
		{
			ID:               "test-id-3",
			SlackChannelID:   "C12345",
			LabelName:        "feature",
			DefaultMentionID: "U11111",
			IsActive:         true,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		},
		{
			ID:               "test-id-4",
			SlackChannelID:   "C67890", // Different channel
			LabelName:        "security",
			DefaultMentionID: "U22222",
			IsActive:         true,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		},
	}

	for _, config := range testConfigs {
		db.Create(&config)
	}

	// Test the show command without arguments (display all labels)
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
	responseBody := w.Body.String()

	// Verify that only labels from the current channel are displayed
	assert.Contains(t, responseBody, "このチャンネルで設定済みのラベル")
	assert.Contains(t, responseBody, "needs-review")
	assert.Contains(t, responseBody, "bug")
	assert.Contains(t, responseBody, "feature")
	assert.Contains(t, responseBody, "有効")
	assert.Contains(t, responseBody, "無効")

	// Verify that labels from other channels are not displayed
	assert.NotContains(t, responseBody, "security")
}
