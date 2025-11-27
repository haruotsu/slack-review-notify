package handlers

import (
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

func TestHandleSlackAction_ChangeReviewer_MultipleLabels(t *testing.T) {
	// テストモードを有効化
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()

	// テスト用のDB作成
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/action", HandleSlackAction(db))

	// 複数のラベルの設定を作成
	// label-1用（レビュワー2人）
	config1 := models.ChannelConfig{
		ID:               "config-1",
		SlackChannelID:   "C12345",
		LabelName:        "label-1",
		DefaultMentionID: "U00000",
		ReviewerList:     "U11111,U22222",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&config1)

	// label-2用（レビュワー2人）
	config2 := models.ChannelConfig{
		ID:               "config-2",
		SlackChannelID:   "C12345",
		LabelName:        "label-2",
		DefaultMentionID: "U00000",
		ReviewerList:     "U33333,U44444",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&config2)

	// テスト用のタスクを作成（label-2ラベル）
	task := models.ReviewTask{
		ID:           "test-task-1",
		PRURL:        "https://github.com/test/repo/pull/1",
		Repo:         "test/repo",
		PRNumber:     1,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Status:       "in_review",
		Reviewer:     "U33333", // label-2のレビュワー
		LabelName:    "label-2",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// Slackアクションペイロードを作成
	payload := SlackActionPayload{
		Type: "block_actions",
		User: struct {
			ID string `json:"id"`
		}{ID: "U99999"},
		Actions: []struct {
			ActionID       string `json:"action_id"`
			Value          string `json:"value,omitempty"`
			SelectedOption struct {
				Value string `json:"value"`
				Text  struct {
					Text string `json:"text"`
				} `json:"text"`
			} `json:"selected_option,omitempty"`
		}{
			{
				ActionID: "change_reviewer",
				Value:    "test-task-1",
			},
		},
		Container: struct {
			ChannelID string `json:"channel_id"`
		}{ChannelID: "C12345"},
		Message: struct {
			Ts string `json:"ts"`
		}{Ts: "1234.5678"},
	}

	payloadJSON, _ := json.Marshal(payload)
	form := url.Values{}
	form.Add("payload", string(payloadJSON))

	// リクエストを作成
	req := httptest.NewRequest("POST", "/slack/action", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// リクエストを実行
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// レスポンスを確認
	assert.Equal(t, http.StatusOK, w.Code)

	// DBからタスクを取得して確認
	var updatedTask models.ReviewTask
	db.Where("id = ?", "test-task-1").First(&updatedTask)

	// レビュワーが変更されていることを確認
	assert.NotEqual(t, "U33333", updatedTask.Reviewer)
	// label-2のレビュワーリストから選ばれていることを確認
	assert.Contains(t, []string{"U33333", "U44444"}, updatedTask.Reviewer)
	// label-1のレビュワーリストからは選ばれていないことを確認
	assert.NotContains(t, []string{"U11111", "U22222"}, updatedTask.Reviewer)
}

func TestHandleSlackAction_ChangeReviewer_SingleReviewer(t *testing.T) {
	// テストモードを有効化
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()

	// テスト用のDB作成
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/action", HandleSlackAction(db))

	// レビュワーが1人しかいない設定を作成
	config := models.ChannelConfig{
		ID:               "config-3",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U00000",
		ReviewerList:     "U11111", // 1人だけ
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&config)

	// テスト用のタスクを作成
	task := models.ReviewTask{
		ID:           "test-task-2",
		PRURL:        "https://github.com/test/repo/pull/2",
		Repo:         "test/repo",
		PRNumber:     2,
		Title:        "Test PR",
		SlackTS:      "2345.6789",
		SlackChannel: "C12345",
		Status:       "in_review",
		Reviewer:     "U11111",
		LabelName:    "needs-review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// Slackアクションペイロードを作成
	payload := SlackActionPayload{
		Type: "block_actions",
		User: struct {
			ID string `json:"id"`
		}{ID: "U99999"},
		Actions: []struct {
			ActionID       string `json:"action_id"`
			Value          string `json:"value,omitempty"`
			SelectedOption struct {
				Value string `json:"value"`
				Text  struct {
					Text string `json:"text"`
				} `json:"text"`
			} `json:"selected_option,omitempty"`
		}{
			{
				ActionID: "change_reviewer",
				Value:    "test-task-2",
			},
		},
		Container: struct {
			ChannelID string `json:"channel_id"`
		}{ChannelID: "C12345"},
		Message: struct {
			Ts string `json:"ts"`
		}{Ts: "2345.6789"},
	}

	payloadJSON, _ := json.Marshal(payload)
	form := url.Values{}
	form.Add("payload", string(payloadJSON))

	// リクエストを作成
	req := httptest.NewRequest("POST", "/slack/action", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// リクエストを実行
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// レスポンスを確認
	assert.Equal(t, http.StatusOK, w.Code)

	// DBからタスクを取得して確認
	var updatedTask models.ReviewTask
	db.Where("id = ?", "test-task-2").First(&updatedTask)

	// レビュワーが変更されていない（1人しかいないため）
	assert.Equal(t, "U11111", updatedTask.Reviewer)
}

func TestHandleSlackAction_ChangeReviewer_NoLabelName(t *testing.T) {
	// テストモードを有効化
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()

	// テスト用のDB作成
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/action", HandleSlackAction(db))

	// デフォルトラベル用の設定を作成
	config := models.ChannelConfig{
		ID:               "config-4",
		SlackChannelID:   "C12345",
		LabelName:        "needs-review",
		DefaultMentionID: "U00000",
		ReviewerList:     "U11111,U22222",
		IsActive:         true,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	db.Create(&config)

	// LabelNameが空のタスクを作成（古いタスク）
	task := models.ReviewTask{
		ID:           "test-task-3",
		PRURL:        "https://github.com/test/repo/pull/3",
		Repo:         "test/repo",
		PRNumber:     3,
		Title:        "Test PR",
		SlackTS:      "3456.7890",
		SlackChannel: "C12345",
		Status:       "in_review",
		Reviewer:     "U11111",
		LabelName:    "", // 空のラベル名
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// Slackアクションペイロードを作成
	payload := SlackActionPayload{
		Type: "block_actions",
		User: struct {
			ID string `json:"id"`
		}{ID: "U99999"},
		Actions: []struct {
			ActionID       string `json:"action_id"`
			Value          string `json:"value,omitempty"`
			SelectedOption struct {
				Value string `json:"value"`
				Text  struct {
					Text string `json:"text"`
				} `json:"text"`
			} `json:"selected_option,omitempty"`
		}{
			{
				ActionID: "change_reviewer",
				Value:    "test-task-3",
			},
		},
		Container: struct {
			ChannelID string `json:"channel_id"`
		}{ChannelID: "C12345"},
		Message: struct {
			Ts string `json:"ts"`
		}{Ts: "3456.7890"},
	}

	payloadJSON, _ := json.Marshal(payload)
	form := url.Values{}
	form.Add("payload", string(payloadJSON))

	// リクエストを作成
	req := httptest.NewRequest("POST", "/slack/action", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// リクエストを実行
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// レスポンスを確認
	assert.Equal(t, http.StatusOK, w.Code)

	// DBからタスクを取得して確認
	var updatedTask models.ReviewTask
	db.Where("id = ?", "test-task-3").First(&updatedTask)

	// レビュワーが変更されていることを確認
	assert.NotEqual(t, "U11111", updatedTask.Reviewer)
	assert.Contains(t, []string{"U11111", "U22222"}, updatedTask.Reviewer)
	// LabelNameがデフォルト値に更新されていることを確認
	assert.Equal(t, "needs-review", updatedTask.LabelName)
}
