package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"slack-review-notify/models"
	"slack-review-notify/services"

	"github.com/gin-gonic/gin"
	"github.com/google/go-github/v71/github"
	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
)

func TestUnlabeledEventWithExistingTask(t *testing.T) {
	// セットアップ
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	// チャンネル設定を作成
	config := models.ChannelConfig{
		SlackChannelID:   "C1234567890",
		LabelName:        "needs-review",
		DefaultMentionID: "@here",
		RepositoryList:   "",
		IsActive:         true,
	}
	db.Create(&config)

	// 既存のレビュータスクを作成
	task := models.ReviewTask{
		ID:           "test-task-id",
		PRURL:        "https://github.com/test/repo/pull/123",
		Repo:         "test/repo",
		PRNumber:     123,
		Title:        "Test PR",
		SlackTS:      "1234567890.123456",
		SlackChannel: "C1234567890",
		Status:       "in_review",
		LabelName:    "needs-review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// unlabeledイベントのペイロードを作成
	prNumber := 123
	repoName := "repo"
	ownerLogin := "test"
	prTitle := "Test PR"
	prHTMLURL := "https://github.com/test/repo/pull/123"
	labelName := "needs-review"
	action := "unlabeled"
	
	payload := github.PullRequestEvent{
		Action: &action,
		Number: &prNumber,
		Label: &github.Label{
			Name: &labelName,
		},
		PullRequest: &github.PullRequest{
			Number:  &prNumber,
			Title:   &prTitle,
			HTMLURL: &prHTMLURL,
		},
		Repo: &github.Repository{
			Name: &repoName,
			Owner: &github.User{
				Login: &ownerLogin,
			},
		},
	}

	// ハンドラーをテスト
	router := gin.New()
	router.POST("/webhook", HandleGitHubWebhook(db))

	jsonPayload, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(jsonPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// アサーション
	assert.Equal(t, http.StatusOK, w.Code)

	// タスクがcompletedに更新されているか確認
	var updatedTask models.ReviewTask
	db.First(&updatedTask, "id = ?", "test-task-id")
	assert.Equal(t, "completed", updatedTask.Status)
}

func TestUnlabeledEventWithoutExistingTask(t *testing.T) {
	// セットアップ
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	// unlabeledイベントのペイロードを作成（対応するタスクなし）
	prNumber := 456
	repoName := "repo"
	ownerLogin := "test"
	prTitle := "Another Test PR"
	prHTMLURL := "https://github.com/test/repo/pull/456"
	labelName := "needs-review"
	action := "unlabeled"
	
	payload := github.PullRequestEvent{
		Action: &action,
		Number: &prNumber,
		Label: &github.Label{
			Name: &labelName,
		},
		PullRequest: &github.PullRequest{
			Number:  &prNumber,
			Title:   &prTitle,
			HTMLURL: &prHTMLURL,
		},
		Repo: &github.Repository{
			Name: &repoName,
			Owner: &github.User{
				Login: &ownerLogin,
			},
		},
	}

	// ハンドラーをテスト
	router := gin.New()
	router.POST("/webhook", HandleGitHubWebhook(db))

	jsonPayload, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(jsonPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// アサーション - 特にエラーが発生しないことを確認
	assert.Equal(t, http.StatusOK, w.Code)

	// タスクが作成されていないことを確認
	var taskCount int64
	db.Model(&models.ReviewTask{}).Where("pr_number = ?", 456).Count(&taskCount)
	assert.Equal(t, int64(0), taskCount)
}

func TestUnlabeledEventWithWaitingBusinessHoursTask(t *testing.T) {
	// セットアップ
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	// チャンネル設定を作成
	config := models.ChannelConfig{
		SlackChannelID:   "C1234567890",
		LabelName:        "needs-review",
		DefaultMentionID: "@here",
		RepositoryList:   "",
		IsActive:         true,
	}
	db.Create(&config)

	// 営業時間外待機中のレビュータスクを作成
	task := models.ReviewTask{
		ID:           "waiting-task-id",
		PRURL:        "https://github.com/test/repo/pull/789",
		Repo:         "test/repo",
		PRNumber:     789,
		Title:        "Waiting Business Hours PR",
		SlackTS:      "1234567890.789012",
		SlackChannel: "C1234567890",
		Status:       "waiting_business_hours", // 営業時間外待機中
		LabelName:    "needs-review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// unlabeledイベントのペイロードを作成
	prNumber := 789
	repoName := "repo"
	ownerLogin := "test"
	prTitle := "Waiting Business Hours PR"
	prHTMLURL := "https://github.com/test/repo/pull/789"
	labelName := "needs-review"
	action := "unlabeled"
	
	payload := github.PullRequestEvent{
		Action: &action,
		Number: &prNumber,
		Label: &github.Label{
			Name: &labelName,
		},
		PullRequest: &github.PullRequest{
			Number:  &prNumber,
			Title:   &prTitle,
			HTMLURL: &prHTMLURL,
		},
		Repo: &github.Repository{
			Name: &repoName,
			Owner: &github.User{
				Login: &ownerLogin,
			},
		},
	}

	// HTTPリクエストを作成
	jsonData, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request")

	// レスポンスレコーダー作成
	w := httptest.NewRecorder()

	// Ginルーターを作成してリクエスト実行
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	// HTTPステータスが200であることを確認
	assert.Equal(t, http.StatusOK, w.Code)

	// タスクが完了状態に更新されたことを確認
	var updatedTask models.ReviewTask
	result := db.Where("id = ?", "waiting-task-id").First(&updatedTask)
	assert.NoError(t, result.Error)
	assert.Equal(t, "completed", updatedTask.Status)
	assert.True(t, updatedTask.UpdatedAt.After(task.UpdatedAt))
}

func TestHandleReviewSubmittedEvent(t *testing.T) {
	// テスト用DB
	db := setupTestDB(t)
	
	// テスト前の環境変数を保存し、テスト後に復元
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer os.Setenv("SLACK_BOT_TOKEN", originalToken)

	// テスト用の環境変数を設定
	os.Setenv("SLACK_BOT_TOKEN", "test-token")

	// モックの設定
	defer gock.Off() // テスト終了時にモックをクリア

	// Slack API成功レスポンスのモック
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
		})

	// テスト用タスクを作成
	task := models.ReviewTask{
		ID:           "test-task-review",
		PRURL:        "https://github.com/owner/repo/pull/123",
		Repo:         "owner/repo",
		PRNumber:     123,
		Title:        "Test PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C12345",
		Status:       "in_review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// テスト用の PullRequestReviewEvent JSON
	payload := `{
		"action": "submitted",
		"pull_request": {
			"number": 123,
			"html_url": "https://github.com/owner/repo/pull/123"
		},
		"repository": {
			"full_name": "owner/repo",
			"owner": {
				"login": "owner"
			},
			"name": "repo"
		},
		"review": {
			"state": "approved",
			"user": {
				"login": "reviewer123"
			}
		}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()

	r := gin.Default()
	r.POST("/webhook", HandleGitHubWebhook(db))
	r.ServeHTTP(w, req)

	// ステータスコード確認
	assert.Equal(t, http.StatusOK, w.Code)

	// DBが更新されたことを確認
	var updatedTask models.ReviewTask
	db.Where("id = ?", "test-task-review").First(&updatedTask)
	assert.Equal(t, "completed", updatedTask.Status)
	
	// モックが使用されたことを確認
	assert.True(t, gock.IsDone(), "すべてのモックが使用されていません")
}

func TestHandleReviewSubmittedEventWithWaitingBusinessHoursTask(t *testing.T) {
	// テスト用DB
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true
	
	// テスト前の環境変数を保存し、テスト後に復元
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer os.Setenv("SLACK_BOT_TOKEN", originalToken)

	// テスト用の環境変数を設定
	os.Setenv("SLACK_BOT_TOKEN", "test-token")

	// モックの設定
	defer gock.Off() // テスト終了時にモックをクリア

	// Slack API成功レスポンスのモック
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		Reply(200).
		JSON(map[string]interface{}{
			"ok": true,
			"ts": "1234567890.123456",
			"channel": "C1234567890",
		})

	// 営業時間外待機中のレビュータスクを作成
	task := models.ReviewTask{
		ID:           "waiting-review-task-id",
		PRURL:        "https://github.com/test/repo/pull/999",
		Repo:         "test/repo",
		PRNumber:     999,
		Title:        "Waiting Business Hours Review PR",
		SlackTS:      "1234567890.999888",
		SlackChannel: "C1234567890",
		Status:       "waiting_business_hours", // 営業時間外待機中
		LabelName:    "needs-review",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	db.Create(&task)

	// レビュー投稿イベントのペイロードを作成
	prNumber := 999
	repoName := "repo"
	ownerLogin := "test"
	reviewerLogin := "reviewer"
	reviewState := "approved"
	reviewBody := "LGTM!"
	
	payload := github.PullRequestReviewEvent{
		Action: github.Ptr("submitted"),
		PullRequest: &github.PullRequest{
			Number: &prNumber,
		},
		Repo: &github.Repository{
			Name: &repoName,
			Owner: &github.User{
				Login: &ownerLogin,
			},
		},
		Review: &github.PullRequestReview{
			User: &github.User{
				Login: &reviewerLogin,
			},
			State: &reviewState,
			Body:  &reviewBody,
		},
	}

	// HTTPリクエストを作成
	jsonData, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	// レスポンスレコーダー作成
	w := httptest.NewRecorder()

	// Ginルーターを作成してリクエスト実行
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	// HTTPステータスが200であることを確認
	assert.Equal(t, http.StatusOK, w.Code)

	// タスクが完了状態に更新されたことを確認
	var updatedTask models.ReviewTask
	result := db.Where("id = ?", "waiting-review-task-id").First(&updatedTask)
	assert.NoError(t, result.Error)
	assert.Equal(t, "completed", updatedTask.Status)
	assert.True(t, updatedTask.UpdatedAt.After(task.UpdatedAt))
	
	// モックが使用されたことを確認（Slack API呼び出しが行われた）
	assert.True(t, gock.IsDone(), "すべてのモックが使用されていません")
}

func TestMultipleLabelMatching(t *testing.T) {
	tests := []struct {
		name         string
		configLabel  string
		prLabels     []*github.Label
		shouldNotify bool
	}{
		{
			name:        "単一ラベル設定で複数ラベルPRがマッチ",
			configLabel: "needs-review",
			prLabels: []*github.Label{
				{Name: github.Ptr("needs-review")},
				{Name: github.Ptr("bug")},
			},
			shouldNotify: true,
		},
		{
			name:        "複数ラベル設定で全ラベル存在時にマッチ",
			configLabel: "hoge-project,needs-review",
			prLabels: []*github.Label{
				{Name: github.Ptr("hoge-project")},
				{Name: github.Ptr("needs-review")},
				{Name: github.Ptr("bug")},
			},
			shouldNotify: true,
		},
		{
			name:        "複数ラベル設定で一部ラベル不足時にマッチしない",
			configLabel: "hoge-project,needs-review",
			prLabels: []*github.Label{
				{Name: github.Ptr("hoge-project")},
				{Name: github.Ptr("bug")},
			},
			shouldNotify: false,
		},
		{
			name:        "スペース付きカンマ区切りラベルでマッチ",
			configLabel: "project-a, needs-review, urgent",
			prLabels: []*github.Label{
				{Name: github.Ptr("project-a")},
				{Name: github.Ptr("needs-review")},
				{Name: github.Ptr("urgent")},
				{Name: github.Ptr("feature")},
			},
			shouldNotify: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// セットアップ
			db := setupTestDB(t)
			gin.SetMode(gin.TestMode)
			services.IsTestMode = true

			// Slack API呼び出しをモック
			// チャンネル状態確認のモック
			gock.New("https://slack.com").
				Get("/api/conversations.info").
				Reply(200).
				JSON(map[string]interface{}{
					"ok":      true,
					"channel": map[string]interface{}{"is_archived": false},
				})

			if tt.shouldNotify {
				gock.New("https://slack.com").
					Post("/api/chat.postMessage").
					Reply(200).
					JSON(map[string]interface{}{
						"ok":      true,
						"channel": "C1234567890",
						"ts":      "1234567890.123456",
					})
			}

			// チャンネル設定を作成
			config := models.ChannelConfig{
				SlackChannelID:   "C1234567890",
				LabelName:        tt.configLabel,
				DefaultMentionID: "@here",
				RepositoryList:   "test/repo", // リポジトリを設定
				IsActive:         true,
			}
			db.Create(&config)

			// labeled イベントのペイロード作成
			action := "labeled"
			prNumber := 123

			payload := github.PullRequestEvent{
				Action: &action,
				Number: &prNumber,
				Label: &github.Label{
					Name: github.Ptr("needs-review"), // トリガーとなるラベル
				},
				PullRequest: &github.PullRequest{
					Number:  &prNumber,
					HTMLURL: github.Ptr("https://github.com/test/repo/pull/123"),
					Title:   github.Ptr("Test PR"),
					Labels:  tt.prLabels, // PRに付いている全ラベル
				},
				Repo: &github.Repository{
					FullName: github.Ptr("test/repo"),
					Owner: &github.User{
						Login: github.Ptr("test"),
					},
					Name: github.Ptr("repo"),
				},
			}

			payloadJSON, _ := json.Marshal(payload)

			// リクエスト作成
			req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(payloadJSON))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-GitHub-Event", "pull_request")

			w := httptest.NewRecorder()

			// Ginルーターを作成してリクエスト実行
			router := gin.Default()
			router.POST("/webhook", HandleGitHubWebhook(db))
			router.ServeHTTP(w, req)

			// HTTPステータスが200であることを確認
			assert.Equal(t, http.StatusOK, w.Code)

			// タスクが作成されたかチェック
			var taskCount int64
			db.Model(&models.ReviewTask{}).Where("repo = ? AND pr_number = ?", "test/repo", 123).Count(&taskCount)

			if tt.shouldNotify {
				assert.Equal(t, int64(1), taskCount, "通知すべき場合にタスクが作成されていません")

				// モックが使用されたことを確認
				if gock.HasUnmatchedRequest() {
					t.Log("未マッチのリクエスト:", gock.GetUnmatchedRequests())
				}
			} else {
				assert.Equal(t, int64(0), taskCount, "通知しない場合にタスクが作成されています")
			}

			// クリーンアップ
			gock.Off()
		})
	}
}
