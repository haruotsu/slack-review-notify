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

func TestHandleSlackAction_PauseReminder_Today(t *testing.T) {
	// テスト用のDBとルーターをセットアップ
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/action", HandleSlackAction(db))

	// テスト用のタスクを作成
	task := models.ReviewTask{
		ID:           "test-task-id",
		PRURL:        "https://github.com/owner/repo/pull/1",
		Repo:         "owner/repo",
		PRNumber:     1,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Status:       "in_review",
		Reviewer:     "U12345",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// テスト前の時刻を記録
	beforeUpdate := time.Now()

	// Slackアクションペイロードを作成
	payload := SlackActionPayload{
		Type: "block_actions",
		User: struct {
			ID string `json:"id"`
		}{ID: "U12345"},
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
				ActionID: "pause_reminder",
				SelectedOption: struct {
					Value string `json:"value"`
					Text  struct {
						Text string `json:"text"`
					} `json:"text"`
				}{
					Value: "test-task-id:today",
					Text: struct {
						Text string `json:"text"`
					}{
						Text: "翌営業日の朝まで",
					},
				},
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

	// 署名の検証をモック（実際のテストでは適切に設定する必要があります）
	services.IsTestMode = true
	defer func() { services.IsTestMode = false }()

	// リクエストを実行
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// レスポンスを確認
	assert.Equal(t, http.StatusOK, w.Code)

	// DBからタスクを取得して確認
	var updatedTask models.ReviewTask
	db.Where("id = ?", "test-task-id").First(&updatedTask)

	// ReminderPausedUntilが設定されていることを確認
	assert.NotNil(t, updatedTask.ReminderPausedUntil)

	// 翌営業日の朝に設定されていることを確認
	pausedUntil := *updatedTask.ReminderPausedUntil

	// 現在時刻より後であることを確認
	assert.True(t, pausedUntil.After(beforeUpdate))

	// 10時に設定されていることを確認
	assert.Equal(t, 10, pausedUntil.Hour())
	assert.Equal(t, 0, pausedUntil.Minute())
	assert.Equal(t, 0, pausedUntil.Second())

	// 営業日（月〜金）であることを確認
	assert.NotEqual(t, time.Saturday, pausedUntil.Weekday())
	assert.NotEqual(t, time.Sunday, pausedUntil.Weekday())
}

func TestHandleSlackAction_PauseReminder_Hours(t *testing.T) {
	testCases := []struct {
		name     string
		value    string
		duration time.Duration
	}{
		{"1時間", "test-task-id:1h", 1 * time.Hour},
		{"2時間", "test-task-id:2h", 2 * time.Hour},
		{"4時間", "test-task-id:4h", 4 * time.Hour},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// テスト用のDBとルーターをセットアップ
			db := setupTestDB(t)
			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.POST("/slack/action", HandleSlackAction(db))

			// テスト用のタスクを作成
			task := models.ReviewTask{
				ID:           "test-task-id",
				PRURL:        "https://github.com/owner/repo/pull/1",
				Repo:         "owner/repo",
				PRNumber:     1,
				Title:        "Test PR",
				SlackTS:      "1234.5678",
				SlackChannel: "C12345",
				Status:       "in_review",
				Reviewer:     "U12345",
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			}
			db.Create(&task)

			// テスト前の時刻を記録
			beforeUpdate := time.Now()

			// Slackアクションペイロードを作成
			payload := SlackActionPayload{
				Type: "block_actions",
				User: struct {
					ID string `json:"id"`
				}{ID: "U12345"},
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
						ActionID: "pause_reminder",
						SelectedOption: struct {
							Value string `json:"value"`
							Text  struct {
								Text string `json:"text"`
							} `json:"text"`
						}{
							Value: tc.value,
						},
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

			// 署名の検証をモック
			services.IsTestMode = true
			defer func() { services.IsTestMode = false }()

			// リクエストを実行
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// レスポンスを確認
			assert.Equal(t, http.StatusOK, w.Code)

			// DBからタスクを取得して確認
			var updatedTask models.ReviewTask
			db.Where("id = ?", "test-task-id").First(&updatedTask)

			// ReminderPausedUntilが設定されていることを確認
			assert.NotNil(t, updatedTask.ReminderPausedUntil)

			// 指定された時間だけ後に設定されていることを確認
			pausedUntil := *updatedTask.ReminderPausedUntil
			expectedTime := beforeUpdate.Add(tc.duration)

			// 誤差を考慮して確認（1分以内の誤差を許容）
			diff := pausedUntil.Sub(expectedTime)
			assert.True(t, diff < time.Minute && diff > -time.Minute,
				"Expected pause until around %v, but got %v (diff: %v)",
				expectedTime, pausedUntil, diff)
		})
	}
}

func TestHandleSlackAction_PauseReminderInitial(t *testing.T) {
	// テストモードを有効化（署名検証をスキップ）
	services.IsTestMode = true

	// テスト用のDB作成
	db := setupTestDB(t)

	// テスト用のタスクを作成
	task := models.ReviewTask{
		ID:           "test-task-initial",
		PRURL:        "https://github.com/test/test/pull/1",
		Repo:         "test/test",
		PRNumber:     1,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Status:       "in_review",
		Reviewer:     "U54321",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// モックSlackペイロード
	payload := SlackActionPayload{
		Type: "block_actions",
		User: struct {
			ID string `json:"id"`
		}{ID: "U12345"},
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
				ActionID: "pause_reminder_initial",
				SelectedOption: struct {
					Value string `json:"value"`
					Text  struct {
						Text string `json:"text"`
					} `json:"text"`
				}{
					Value: "test-task-initial:4h",
					Text: struct {
						Text string `json:"text"`
					}{
						Text: "4時間停止",
					},
				},
			},
		},
		Container: struct {
			ChannelID string `json:"channel_id"`
		}{ChannelID: "C12345"},
	}

	// JSON化
	payloadJSON, _ := json.Marshal(payload)

	// リクエスト作成
	form := url.Values{}
	form.Add("payload", string(payloadJSON))

	req, _ := http.NewRequest("POST", "/slack/actions", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// レスポンス記録
	w := httptest.NewRecorder()

	// ハンドラー実行
	handler := HandleSlackAction(db)
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	handler(c)

	// ステータスコード確認
	assert.Equal(t, http.StatusOK, w.Code)

	// DBが更新されたことを確認
	var updatedTask models.ReviewTask
	err := db.Where("id = ?", "test-task-initial").First(&updatedTask).Error
	assert.NoError(t, err)
	assert.NotNil(t, updatedTask.ReminderPausedUntil)

	// 4時間後に設定されているか確認
	expected := time.Now().Add(4 * time.Hour)
	actual := *updatedTask.ReminderPausedUntil
	assert.WithinDuration(t, expected, actual, 10*time.Second)
}
