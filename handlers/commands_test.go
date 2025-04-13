package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"slack-review-notify/models"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestRouter(db *gorm.DB) *gin.Engine {
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
	
	// マイグレーションを実行
	if err := db.AutoMigrate(&models.ChannelConfig{}, &models.ReviewTask{}); err != nil {
		t.Fatalf("fail to migrate test db: %v", err)
	}
	
	return db
}

func TestHandleSlackCommand_Help(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)
	
	// helpコマンドのテスト
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
}

func TestHandleSlackCommand_Show(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)
	
	// テスト用のチャンネル設定を作成
	testConfig := models.ChannelConfig{
		ID:               "test-id",
		SlackChannelID:   "C12345",
		DefaultMentionID: "U12345",
		RepositoryList:   "owner/repo1,owner/repo2",
		LabelName:        "needs-review",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&testConfig)
	
	// showコマンドのテスト
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
	assert.Contains(t, w.Body.String(), "このチャンネルのレビュー通知設定")
	assert.Contains(t, w.Body.String(), "有効")
}

func TestHandleSlackCommand_SetMention(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)
	
	// set-mentionコマンドのテスト
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
	
	// データベース内の値を確認
	var config models.ChannelConfig
	db.Where("slack_channel_id = ?", "C12345").First(&config)
	assert.Equal(t, "U67890", config.DefaultMentionID)
}

func TestHandleSlackCommand_AddRepo(t *testing.T) {
	db := setupTestDB(t)
	router := setupTestRouter(db)
	
	// テスト用のチャンネル設定を作成
	testConfig := models.ChannelConfig{
		ID:               "test-id",
		SlackChannelID:   "C12345",
		DefaultMentionID: "U12345",
		RepositoryList:   "owner/repo1",
		LabelName:        "needs-review",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&testConfig)
	
	// add-repoコマンドのテスト
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
	
	// データベース内の値を確認
	var config models.ChannelConfig
	db.Where("slack_channel_id = ?", "C12345").First(&config)
	assert.Contains(t, config.RepositoryList, "owner/repo1")
	assert.Contains(t, config.RepositoryList, "owner/repo2")
}
