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
	"gorm.io/gorm"
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
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()

	// テスト用の環境変数を設定
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

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
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()

	// テスト用の環境変数を設定
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	// モックの設定
	defer gock.Off() // テスト終了時にモックをクリア

	// Slack API成功レスポンスのモック
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		Reply(200).
		JSON(map[string]interface{}{
			"ok":      true,
			"ts":      "1234567890.123456",
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

func TestMultipleLabelUnlabeling(t *testing.T) {
	tests := []struct {
		name               string
		configLabel        string
		prLabelsAfterEvent []*github.Label // unlabeled後のPRラベル状態
		removedLabel       string          // 削除されたラベル
		shouldComplete     bool            // タスクが完了すべきかどうか
	}{
		{
			name:        "単一ラベル設定でラベル削除時にタスク完了",
			configLabel: "needs-review",
			prLabelsAfterEvent: []*github.Label{
				{Name: github.Ptr("bug")},
			},
			removedLabel:   "needs-review",
			shouldComplete: true,
		},
		{
			name:        "複数ラベル設定で必要ラベル削除時にタスク完了",
			configLabel: "hoge-project,needs-review",
			prLabelsAfterEvent: []*github.Label{
				{Name: github.Ptr("hoge-project")}, // needs-reviewが削除された
				{Name: github.Ptr("bug")},
			},
			removedLabel:   "needs-review",
			shouldComplete: true,
		},
		{
			name:        "複数ラベル設定で不要ラベル削除時はタスク継続",
			configLabel: "hoge-project,needs-review",
			prLabelsAfterEvent: []*github.Label{
				{Name: github.Ptr("hoge-project")},
				{Name: github.Ptr("needs-review")}, // 両方とも残っている
			},
			removedLabel:   "bug", // 関係ないラベルが削除
			shouldComplete: false,
		},
		{
			name:        "複数ラベル設定で複数の必要ラベルのうち1つ削除",
			configLabel: "project-a,needs-review,urgent",
			prLabelsAfterEvent: []*github.Label{
				{Name: github.Ptr("needs-review")},
				{Name: github.Ptr("urgent")}, // project-aが削除された
			},
			removedLabel:   "project-a",
			shouldComplete: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// セットアップ
			db := setupTestDB(t)
			gin.SetMode(gin.TestMode)
			services.IsTestMode = true

			// チャンネル設定を作成
			config := models.ChannelConfig{
				SlackChannelID:   "C1234567890",
				LabelName:        tt.configLabel,
				DefaultMentionID: "@here",
				RepositoryList:   "test/repo",
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
				LabelName:    tt.configLabel,
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			}
			db.Create(&task)

			// Slack API呼び出しをモック
			if tt.shouldComplete {
				// タスク完了時のメッセージ更新
				gock.New("https://slack.com").
					Post("/api/chat.update").
					Reply(200).
					JSON(map[string]interface{}{
						"ok": true,
					})

				// ラベル削除による完了をスレッドに通知
				gock.New("https://slack.com").
					Post("/api/chat.postMessage").
					Reply(200).
					JSON(map[string]interface{}{
						"ok": true,
						"ts": "1234567890.123457",
					})
			}

			// unlabeled イベントのペイロード作成
			action := "unlabeled"
			prNumber := 123

			payload := github.PullRequestEvent{
				Action: &action,
				Number: &prNumber,
				Label: &github.Label{
					Name: github.Ptr(tt.removedLabel),
				},
				PullRequest: &github.PullRequest{
					Number:  &prNumber,
					HTMLURL: github.Ptr("https://github.com/test/repo/pull/123"),
					Title:   github.Ptr("Test PR"),
					Labels:  tt.prLabelsAfterEvent, // 削除後のラベル状態
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

			// タスクのステータスを確認
			var updatedTask models.ReviewTask
			result := db.Where("id = ?", "test-task-id").First(&updatedTask)
			assert.NoError(t, result.Error)

			if tt.shouldComplete {
				assert.Equal(t, "completed", updatedTask.Status, "タスクが完了状態になっていません")
			} else {
				assert.Equal(t, "in_review", updatedTask.Status, "タスクが継続状態でありません")
			}

			// クリーンアップ
			gock.Off()
		})
	}
}

// 初回レビュー後、2回目のレビューでもスレッドに通知が送られることを確認するテスト
func TestHandleReviewSubmittedEvent_SecondReviewAfterCompletion(t *testing.T) {
	// テスト用DB
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	// テスト前の環境変数を保存し、テスト後に復元
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()

	// テスト用の環境変数を設定
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	// モックの設定
	defer gock.Off() // テスト終了時にモックをクリア

	// Slack API成功レスポンスのモック（2回目のレビュー通知用）
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		Reply(200).
		JSON(map[string]interface{}{
			"ok":      true,
			"ts":      "1234567890.999999",
			"channel": "C1234567890",
		})

	// 既にレビュー済み（completed状態）のタスクを作成
	task := models.ReviewTask{
		ID:           "completed-task-id",
		PRURL:        "https://github.com/test/repo/pull/888",
		Repo:         "test/repo",
		PRNumber:     888,
		Title:        "Already Reviewed PR",
		SlackTS:      "1234567890.888888",
		SlackChannel: "C1234567890",
		Status:       "completed", // 既に完了済み
		Reviewer:     "U1234567890",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-1 * time.Hour),    // 1時間前に作成
		UpdatedAt:    time.Now().Add(-30 * time.Minute), // 30分前に更新
	}
	db.Create(&task)

	// 2回目のレビュー投稿イベントのペイロードを作成
	prNumber := 888
	repoName := "repo"
	ownerLogin := "test"
	reviewerLogin := "second-reviewer"
	reviewState := "changes_requested"
	reviewBody := "いくつか修正をお願いします"

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

	// タスクのステータスはcompletedのまま維持されていることを確認
	var updatedTask models.ReviewTask
	result := db.Where("id = ?", "completed-task-id").First(&updatedTask)
	assert.NoError(t, result.Error)
	assert.Equal(t, "completed", updatedTask.Status, "タスクステータスはcompletedのまま維持されるべき")

	// モックが使用されたことを確認（Slack API呼び出しが行われた = 通知が送られた）
	assert.True(t, gock.IsDone(), "2回目のレビューでもSlack通知が送られるべき")
}

// 同一PRに複数のタスクがある場合、最新のスレッドにのみ通知が送られることを確認するテスト
func TestHandleReviewSubmittedEvent_OnlyLatestTaskReceivesNotification(t *testing.T) {
	// テスト用DB
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off() // テスト終了時にモックをクリア

	// Slack API成功レスポンスのモック
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		Reply(200).
		JSON(map[string]interface{}{
			"ok":      true,
			"ts":      "1234567890.999999",
			"channel": "C1234567890",
		})

	// 同一PRに対して、古いタスク（2時間前に作成）を作成
	oldTask := models.ReviewTask{
		ID:           "old-task-id",
		PRURL:        "https://github.com/test/repo/pull/777",
		Repo:         "test/repo",
		PRNumber:     777,
		Title:        "Test PR with Multiple Tasks",
		SlackTS:      "1234567890.111111",
		SlackChannel: "C1234567890",
		Status:       "completed",
		Reviewer:     "U1111111111",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-2 * time.Hour), // 2時間前に作成
		UpdatedAt:    time.Now().Add(-2 * time.Hour),
	}
	db.Create(&oldTask)

	// 同一PRに対して、新しいタスク（1時間前に作成）を作成
	newTask := models.ReviewTask{
		ID:           "new-task-id",
		PRURL:        "https://github.com/test/repo/pull/777",
		Repo:         "test/repo",
		PRNumber:     777,
		Title:        "Test PR with Multiple Tasks",
		SlackTS:      "1234567890.222222",
		SlackChannel: "C1234567890",
		Status:       "completed",
		Reviewer:     "U2222222222",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-1 * time.Hour), // 1時間前に作成（より新しい）
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}
	db.Create(&newTask)

	// レビュー投稿イベントのペイロードを作成
	prNumber := 777
	repoName := "repo"
	ownerLogin := "test"
	reviewerLogin := "reviewer"
	reviewState := "approved"
	reviewBody := "LGTM"

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

	// モックが1回だけ使用されたことを確認（最新のタスクにのみ通知が送られた）
	assert.True(t, gock.IsDone(), "最新のタスクにのみSlack通知が送られるべき")

	// 未消費のモックがないことを確認（2回呼ばれていないことの確認）
	pendingMocks := gock.Pending()
	assert.Equal(t, 0, len(pendingMocks), "Slack APIは1回だけ呼ばれるべき（古いタスクには通知しない）")
}

// 異なるチャンネルのタスクには両方通知が送られることを確認するテスト
func TestHandleReviewSubmittedEvent_DifferentChannelsReceiveNotifications(t *testing.T) {
	// テスト用DB
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off() // テスト終了時にモックをクリア

	// チャンネル1への通知用モック
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		Reply(200).
		JSON(map[string]interface{}{
			"ok":      true,
			"ts":      "1234567890.111111",
			"channel": "C1111111111",
		})

	// チャンネル2への通知用モック
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		Reply(200).
		JSON(map[string]interface{}{
			"ok":      true,
			"ts":      "1234567890.222222",
			"channel": "C2222222222",
		})

	// チャンネル1のタスク（needs-review ラベル）
	task1 := models.ReviewTask{
		ID:           "task-channel-1",
		PRURL:        "https://github.com/test/repo/pull/999",
		Repo:         "test/repo",
		PRNumber:     999,
		Title:        "Test PR with Multiple Channels",
		SlackTS:      "1234567890.111111",
		SlackChannel: "C1111111111", // チャンネル1
		Status:       "completed",
		Reviewer:     "U1111111111",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-1 * time.Hour),
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}
	db.Create(&task1)

	// チャンネル2のタスク（needs-design-review ラベル）
	task2 := models.ReviewTask{
		ID:           "task-channel-2",
		PRURL:        "https://github.com/test/repo/pull/999",
		Repo:         "test/repo",
		PRNumber:     999,
		Title:        "Test PR with Multiple Channels",
		SlackTS:      "1234567890.222222",
		SlackChannel: "C2222222222", // チャンネル2（異なるチャンネル）
		Status:       "completed",
		Reviewer:     "U2222222222",
		LabelName:    "needs-design-review",
		CreatedAt:    time.Now().Add(-1 * time.Hour),
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}
	db.Create(&task2)

	// レビュー投稿イベントのペイロードを作成
	prNumber := 999
	repoName := "repo"
	ownerLogin := "test"
	reviewerLogin := "reviewer"
	reviewState := "approved"
	reviewBody := "LGTM"

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

	// モックが2回使用されたことを確認（両方のチャンネルに通知が送られた）
	assert.True(t, gock.IsDone(), "両方のチャンネルにSlack通知が送られるべき")

	// 未消費のモックがないことを確認
	pendingMocks := gock.Pending()
	assert.Equal(t, 0, len(pendingMocks), "Slack APIは2回呼ばれるべき（各チャンネルに1回ずつ）")
}

// 同一チャンネル・同一PRの古いタスクもcompletedになることを確認するテスト
func TestHandleReviewSubmittedEvent_OldTasksAlsoCompleted(t *testing.T) {
	// テスト用DB
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	// テスト前の環境変数を保存し、テスト後に復元
	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()

	// テスト用の環境変数を設定
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	// モックの設定
	defer gock.Off() // テスト終了時にモックをクリア

	// Slack API成功レスポンスのモック（1回だけ呼ばれる - 最新タスクのみ）
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		Reply(200).
		JSON(map[string]interface{}{
			"ok":      true,
			"ts":      "1234567890.999999",
			"channel": "C1234567890",
		})

	// 同一PR・同一チャンネルに古いタスク（in_review）を作成
	oldTask := models.ReviewTask{
		ID:           "old-in-review-task",
		PRURL:        "https://github.com/test/repo/pull/555",
		Repo:         "test/repo",
		PRNumber:     555,
		Title:        "Test PR with Old Task",
		SlackTS:      "1234567890.111111",
		SlackChannel: "C1234567890",
		Status:       "in_review", // 古いタスクはin_review状態
		Reviewer:     "U1111111111",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-2 * time.Hour), // 2時間前
		UpdatedAt:    time.Now().Add(-2 * time.Hour),
	}
	db.Create(&oldTask)

	// 同一PR・同一チャンネルに新しいタスク（in_review）を作成
	newTask := models.ReviewTask{
		ID:           "new-in-review-task",
		PRURL:        "https://github.com/test/repo/pull/555",
		Repo:         "test/repo",
		PRNumber:     555,
		Title:        "Test PR with Old Task",
		SlackTS:      "1234567890.222222",
		SlackChannel: "C1234567890",
		Status:       "in_review", // 新しいタスクもin_review状態
		Reviewer:     "U2222222222",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-1 * time.Hour), // 1時間前（より新しい）
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}
	db.Create(&newTask)

	// レビュー投稿イベントのペイロードを作成
	prNumber := 555
	repoName := "repo"
	ownerLogin := "test"
	reviewerLogin := "reviewer"
	reviewState := "approved"
	reviewBody := "LGTM"

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

	// 古いタスクもcompletedになっていることを確認
	var updatedOldTask models.ReviewTask
	db.Where("id = ?", "old-in-review-task").First(&updatedOldTask)
	assert.Equal(t, "completed", updatedOldTask.Status, "古いタスクもcompletedになるべき（リマインド防止）")

	// 新しいタスクもcompletedになっていることを確認
	var updatedNewTask models.ReviewTask
	db.Where("id = ?", "new-in-review-task").First(&updatedNewTask)
	assert.Equal(t, "completed", updatedNewTask.Status, "新しいタスクもcompletedになるべき")

	// モックが使用されたことを確認
	assert.True(t, gock.IsDone(), "最新のタスクにのみSlack通知が送られるべき")
}

// --- レビューコメント/変更要求時にタスクをcompletedにしてリマインドを停止するテスト ---

func TestHandleReviewSubmittedEvent_CommentedCompletesTask(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	// Slack API通知モック
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	task := models.ReviewTask{
		ID:           "commented-task",
		PRURL:        "https://github.com/owner/repo/pull/300",
		Repo:         "owner/repo",
		PRNumber:     300,
		Title:        "Commented PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C_COMMENT",
		Reviewer:     "UREVIEWER1",
		Status:       "in_review",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-1 * time.Hour),
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}
	db.Create(&task)

	payload := `{
		"action": "submitted",
		"pull_request": {"number": 300, "html_url": "https://github.com/owner/repo/pull/300"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "commented", "user": {"login": "reviewer1"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "commented-task").First(&updatedTask)
	assert.Equal(t, "completed", updatedTask.Status, "commentedレビュー後はcompletedになりリマインドが止まるべき")
}

func TestHandleReviewSubmittedEvent_ChangesRequestedCompletesTask(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	// Slack API通知モック
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		MatchHeader("Authorization", "Bearer test-token").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	task := models.ReviewTask{
		ID:           "changes-requested-task",
		PRURL:        "https://github.com/owner/repo/pull/301",
		Repo:         "owner/repo",
		PRNumber:     301,
		Title:        "Changes Requested PR",
		SlackTS:      "1234.5679",
		SlackChannel: "C_CHANGES",
		Reviewer:     "UREVIEWER1",
		Status:       "in_review",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-1 * time.Hour),
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}
	db.Create(&task)

	payload := `{
		"action": "submitted",
		"pull_request": {"number": 301, "html_url": "https://github.com/owner/repo/pull/301"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "changes_requested", "user": {"login": "reviewer1"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "changes-requested-task").First(&updatedTask)
	assert.Equal(t, "completed", updatedTask.Status, "changes_requestedレビュー後はcompletedになりリマインドが止まるべき")
}

func TestHandleReviewSubmittedEvent_CommentedCompletesOldTasksToo(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// 同一PR・同一チャンネルに古いタスク（in_review）
	oldTask := models.ReviewTask{
		ID:           "old-comment-task",
		PRURL:        "https://github.com/owner/repo/pull/302",
		Repo:         "owner/repo",
		PRNumber:     302,
		Title:        "Comment PR",
		SlackTS:      "1234.1111",
		SlackChannel: "C_COMMENT2",
		Status:       "in_review",
		Reviewer:     "UOLD",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-2 * time.Hour),
		UpdatedAt:    time.Now().Add(-2 * time.Hour),
	}
	db.Create(&oldTask)

	newTask := models.ReviewTask{
		ID:           "new-comment-task",
		PRURL:        "https://github.com/owner/repo/pull/302",
		Repo:         "owner/repo",
		PRNumber:     302,
		Title:        "Comment PR",
		SlackTS:      "1234.2222",
		SlackChannel: "C_COMMENT2",
		Status:       "in_review",
		Reviewer:     "UNEW",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-1 * time.Hour),
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
	}
	db.Create(&newTask)

	payload := `{
		"action": "submitted",
		"pull_request": {"number": 302, "html_url": "https://github.com/owner/repo/pull/302"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "commented", "user": {"login": "reviewer1"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// 古いタスクもcompletedになっていること（リマインド防止）
	var updatedOld models.ReviewTask
	db.Where("id = ?", "old-comment-task").First(&updatedOld)
	assert.Equal(t, "completed", updatedOld.Status)

	var updatedNew models.ReviewTask
	db.Where("id = ?", "new-comment-task").First(&updatedNew)
	assert.Equal(t, "completed", updatedNew.Status)
}

func TestHandleReviewSubmittedEvent_SnoozedTaskCompletedOnComment(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// snoozed状態のタスク
	task := models.ReviewTask{
		ID:           "snoozed-comment-task",
		PRURL:        "https://github.com/owner/repo/pull/400",
		Repo:         "owner/repo",
		PRNumber:     400,
		Title:        "Snoozed PR",
		SlackTS:      "1234.9999",
		SlackChannel: "C_SNOOZED",
		Reviewer:     "UREVIEWER1",
		Status:       "snoozed",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-2 * time.Hour),
		UpdatedAt:    time.Now().Add(-2 * time.Hour),
	}
	db.Create(&task)

	payload := `{
		"action": "submitted",
		"pull_request": {"number": 400, "html_url": "https://github.com/owner/repo/pull/400"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "commented", "user": {"login": "reviewer1"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "snoozed-comment-task").First(&updatedTask)
	assert.Equal(t, "completed", updatedTask.Status, "snoozed状態のタスクもcommentedレビュー後にcompletedになるべき")
}

func TestHandleReviewSubmittedEvent_SnoozedTaskCompletedOnChangesRequested(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// snoozed状態のタスク
	task := models.ReviewTask{
		ID:           "snoozed-changes-task",
		PRURL:        "https://github.com/owner/repo/pull/401",
		Repo:         "owner/repo",
		PRNumber:     401,
		Title:        "Snoozed Changes PR",
		SlackTS:      "1234.8888",
		SlackChannel: "C_SNOOZED2",
		Reviewer:     "UREVIEWER1",
		Status:       "snoozed",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-2 * time.Hour),
		UpdatedAt:    time.Now().Add(-2 * time.Hour),
	}
	db.Create(&task)

	payload := `{
		"action": "submitted",
		"pull_request": {"number": 401, "html_url": "https://github.com/owner/repo/pull/401"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "changes_requested", "user": {"login": "reviewer1"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "snoozed-changes-task").First(&updatedTask)
	assert.Equal(t, "completed", updatedTask.Status, "snoozed状態のタスクもchanges_requestedレビュー後にcompletedになるべき")
}

// --- snoozed状態のタスクがfully approved時にcompletedになるテスト ---

func TestHandleReviewSubmittedEvent_SnoozedTaskCompletedOnFullApproval(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	// レビュー通知 + 完了メッセージ + approve通知
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// RequiredApprovals=1のチャンネル設定
	config := models.ChannelConfig{
		ID:                "config-snoozed-approve",
		SlackChannelID:    "C_SNOOZED_APPROVE",
		LabelName:         "needs-review",
		DefaultMentionID:  "UDEFAULT",
		RequiredApprovals: 1,
		IsActive:          true,
	}
	db.Create(&config)

	userMapping := models.UserMapping{
		ID:             "mapping-snoozed",
		GithubUsername: "reviewer1",
		SlackUserID:    "UREVIEWER1",
	}
	db.Create(&userMapping)

	// snoozed状態のタスク
	task := models.ReviewTask{
		ID:           "snoozed-approve-task",
		PRURL:        "https://github.com/owner/repo/pull/600",
		Repo:         "owner/repo",
		PRNumber:     600,
		Title:        "Snoozed Approve PR",
		SlackTS:      "1234.6000",
		SlackChannel: "C_SNOOZED_APPROVE",
		Reviewer:     "UREVIEWER1",
		Reviewers:    "UREVIEWER1",
		Status:       "snoozed",
		LabelName:    "needs-review",
		CreatedAt:    time.Now().Add(-2 * time.Hour),
		UpdatedAt:    time.Now().Add(-2 * time.Hour),
	}
	db.Create(&task)

	payload := `{
		"action": "submitted",
		"pull_request": {"number": 600, "html_url": "https://github.com/owner/repo/pull/600"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "approved", "user": {"login": "reviewer1"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var updatedTask models.ReviewTask
	db.Where("id = ?", "snoozed-approve-task").First(&updatedTask)
	assert.Equal(t, "completed", updatedTask.Status, "snoozed状態でもfully approved時はcompletedになるべき")
}

// --- 複数レビュワー対応のWebhookテスト ---

func TestHandleReviewSubmittedEvent_PartialApproval(t *testing.T) {
	// テスト用DB
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	// レビュー通知のモック
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// 部分approve時の進捗メッセージのモック
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// RequiredApprovals=2のチャンネル設定
	config := models.ChannelConfig{
		ID:                "config-partial",
		SlackChannelID:    "C_PARTIAL",
		LabelName:         "needs-review",
		DefaultMentionID:  "UDEFAULT",
		RequiredApprovals: 2,
		IsActive:          true,
	}
	db.Create(&config)

	// UserMapping
	userMapping := models.UserMapping{
		ID:             "mapping-1",
		GithubUsername: "reviewer1",
		SlackUserID:    "UREVIEWER1",
	}
	db.Create(&userMapping)

	// 2人アサインされたタスク
	task := models.ReviewTask{
		ID:           "partial-task",
		PRURL:        "https://github.com/owner/repo/pull/200",
		Repo:         "owner/repo",
		PRNumber:     200,
		Title:        "Partial Approval PR",
		SlackTS:      "1234.5678",
		SlackChannel: "C_PARTIAL",
		Reviewer:     "UREVIEWER1",
		Reviewers:    "UREVIEWER1,UREVIEWER2",
		Status:       "in_review",
		LabelName:    "needs-review",
	}
	db.Create(&task)

	// 1人目がapprove
	payload := `{
		"action": "submitted",
		"pull_request": {"number": 200, "html_url": "https://github.com/owner/repo/pull/200"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "approved", "user": {"login": "reviewer1"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// タスクはまだin_reviewのまま（1/2 approve）
	var updatedTask models.ReviewTask
	db.Where("id = ?", "partial-task").First(&updatedTask)
	assert.Equal(t, "in_review", updatedTask.Status, "1/2 approveなのでまだin_review")
	assert.Contains(t, updatedTask.ApprovedBy, "UREVIEWER1")
}

func TestHandleReviewSubmittedEvent_FullApproval(t *testing.T) {
	// テスト用DB
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	// レビュー通知のモック
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// RequiredApprovals=2のチャンネル設定
	config := models.ChannelConfig{
		ID:                "config-full",
		SlackChannelID:    "C_FULL",
		LabelName:         "needs-review",
		DefaultMentionID:  "UDEFAULT",
		RequiredApprovals: 2,
		IsActive:          true,
	}
	db.Create(&config)

	// UserMapping
	userMapping := models.UserMapping{
		ID:             "mapping-2",
		GithubUsername: "reviewer2",
		SlackUserID:    "UREVIEWER2",
	}
	db.Create(&userMapping)

	// 既に1人approveされたタスク
	task := models.ReviewTask{
		ID:           "full-task",
		PRURL:        "https://github.com/owner/repo/pull/201",
		Repo:         "owner/repo",
		PRNumber:     201,
		Title:        "Full Approval PR",
		SlackTS:      "1234.5679",
		SlackChannel: "C_FULL",
		Reviewer:     "UREVIEWER1",
		Reviewers:    "UREVIEWER1,UREVIEWER2",
		ApprovedBy:   "UREVIEWER1",
		Status:       "in_review",
		LabelName:    "needs-review",
	}
	db.Create(&task)

	// 2人目がapprove
	payload := `{
		"action": "submitted",
		"pull_request": {"number": 201, "html_url": "https://github.com/owner/repo/pull/201"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "approved", "user": {"login": "reviewer2"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// タスクがcompletedになっている（2/2 approve）
	var updatedTask models.ReviewTask
	db.Where("id = ?", "full-task").First(&updatedTask)
	assert.Equal(t, "completed", updatedTask.Status, "2/2 approveなのでcompleted")
	assert.Contains(t, updatedTask.ApprovedBy, "UREVIEWER1")
	assert.Contains(t, updatedTask.ApprovedBy, "UREVIEWER2")
}

func TestHandleReviewSubmittedEvent_BackwardCompat(t *testing.T) {
	// テスト用DB - RequiredApprovals=1（デフォルト）で既存動作
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	originalToken := os.Getenv("SLACK_BOT_TOKEN")
	defer func() {
		_ = os.Setenv("SLACK_BOT_TOKEN", originalToken)
	}()
	_ = os.Setenv("SLACK_BOT_TOKEN", "test-token")

	defer gock.Off()

	// レビュー通知のモック
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// RequiredApprovals未設定（デフォルト=1）のチャンネル設定
	config := models.ChannelConfig{
		ID:               "config-compat",
		SlackChannelID:   "C_COMPAT",
		LabelName:        "needs-review",
		DefaultMentionID: "UDEFAULT",
		IsActive:         true,
	}
	db.Create(&config)

	// 旧データ形式のタスク（Reviewers空）
	task := models.ReviewTask{
		ID:           "compat-task",
		PRURL:        "https://github.com/owner/repo/pull/202",
		Repo:         "owner/repo",
		PRNumber:     202,
		Title:        "Backward Compat PR",
		SlackTS:      "1234.5680",
		SlackChannel: "C_COMPAT",
		Reviewer:     "UOLD",
		Status:       "in_review",
		LabelName:    "needs-review",
	}
	db.Create(&task)

	// approveイベント
	payload := `{
		"action": "submitted",
		"pull_request": {"number": 202, "html_url": "https://github.com/owner/repo/pull/202"},
		"repository": {"full_name": "owner/repo", "owner": {"login": "owner"}, "name": "repo"},
		"review": {"state": "approved", "user": {"login": "unknownUser"}}
	}`

	req, _ := http.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "pull_request_review")

	w := httptest.NewRecorder()
	router := gin.Default()
	router.POST("/webhook", HandleGitHubWebhook(db))
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// RequiredApprovals=1なので1回のapproveでcompleted
	var updatedTask models.ReviewTask
	db.Where("id = ?", "compat-task").First(&updatedTask)
	assert.Equal(t, "completed", updatedTask.Status, "RequiredApprovals=1で1回approveなのでcompleted")
}

// IssueCommentEventのテスト用ペイロードを構築するヘルパー
func buildIssueCommentPayload(action string, prNumber int, repoOwner, repoName, prAuthorLogin, senderLogin, commentURL string) github.IssueCommentEvent {
	commentBody := "test comment"
	prURL := "https://github.com/" + repoOwner + "/" + repoName + "/pull/1"
	return github.IssueCommentEvent{
		Action: &action,
		Issue: &github.Issue{
			Number: &prNumber,
			User:   &github.User{Login: &prAuthorLogin},
			PullRequestLinks: &github.PullRequestLinks{
				URL: &prURL,
			},
		},
		Comment: &github.IssueComment{
			Body:    &commentBody,
			HTMLURL: &commentURL,
			User:    &github.User{Login: &senderLogin},
		},
		Repo: &github.Repository{
			Name:  &repoName,
			Owner: &github.User{Login: &repoOwner},
		},
		Sender: &github.User{Login: &senderLogin},
	}
}

func sendIssueCommentWebhook(t *testing.T, db *gorm.DB, payload github.IssueCommentEvent) *httptest.ResponseRecorder {
	router := gin.New()
	router.POST("/webhook", HandleGitHubWebhook(db))

	jsonPayload, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/webhook", bytes.NewBuffer(jsonPayload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "issue_comment")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestIssueCommentEvent_PRAuthorReply(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	defer gock.Off()
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true, "ts": "111.222", "channel": "C1234567890"})

	db.Create(&models.ReviewTask{
		ID: "issue-comment-task", PRURL: "https://github.com/test/repo/pull/10",
		Repo: "test/repo", PRNumber: 10, Title: "Test PR",
		SlackTS: "1234567890.123456", SlackChannel: "C1234567890",
		Status: "in_review", LabelName: "needs-review",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	payload := buildIssueCommentPayload("created", 10, "test", "repo", "pr-author", "pr-author",
		"https://github.com/test/repo/pull/10#issuecomment-123")

	w := sendIssueCommentWebhook(t, db, payload)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, gock.IsDone(), "Slack APIが呼ばれるべき")
}

func TestIssueCommentEvent_NonAuthorSkipped(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	defer gock.Off()
	// Slack APIが呼ばれた場合にテストが失敗するよう明示的にモックを設定
	mock := gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	db.Create(&models.ReviewTask{
		ID: "issue-comment-task-2", PRURL: "https://github.com/test/repo/pull/11",
		Repo: "test/repo", PRNumber: 11, Title: "Test PR 2",
		SlackTS: "1234567890.654321", SlackChannel: "C1234567890",
		Status: "in_review", LabelName: "needs-review",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	payload := buildIssueCommentPayload("created", 11, "test", "repo", "pr-author", "reviewer",
		"https://github.com/test/repo/pull/11#issuecomment-456")

	w := sendIssueCommentWebhook(t, db, payload)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, gock.IsPending(), "Slack APIは呼ばれないべき")
	_ = mock // gockモックが未使用であることを明示
}

func TestIssueCommentEvent_NoActiveTask(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	payload := buildIssueCommentPayload("created", 99, "test", "repo", "pr-author", "pr-author",
		"https://github.com/test/repo/pull/99#issuecomment-789")

	w := sendIssueCommentWebhook(t, db, payload)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestIssueCommentEvent_MultiChannelDeduplication(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	defer gock.Off()
	// 2つのチャンネルに1回ずつ通知されることを期待
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true, "ts": "111.222", "channel": "C_CHAN1"})
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true, "ts": "333.444", "channel": "C_CHAN2"})

	// 異なるチャンネルに2つのタスク
	db.Create(&models.ReviewTask{
		ID: "multi-ch-task-1", PRURL: "https://github.com/test/repo/pull/20",
		Repo: "test/repo", PRNumber: 20, Title: "Multi Channel PR",
		SlackTS: "1111.1111", SlackChannel: "C_CHAN1",
		Status: "in_review", LabelName: "needs-review",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	db.Create(&models.ReviewTask{
		ID: "multi-ch-task-2", PRURL: "https://github.com/test/repo/pull/20",
		Repo: "test/repo", PRNumber: 20, Title: "Multi Channel PR",
		SlackTS: "2222.2222", SlackChannel: "C_CHAN2",
		Status: "in_review", LabelName: "review-needed",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	payload := buildIssueCommentPayload("created", 20, "test", "repo", "pr-author", "pr-author",
		"https://github.com/test/repo/pull/20#issuecomment-200")

	w := sendIssueCommentWebhook(t, db, payload)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, gock.IsDone(), "両チャンネルにSlack APIが呼ばれるべき")
}

func TestIssueCommentEvent_SameChannelDeduplication(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	defer gock.Off()
	// 同一チャンネルなので1回だけ通知
	gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true, "ts": "111.222", "channel": "C_SAME"})

	// 同じチャンネルに2つのタスク（異なるラベル設定）
	db.Create(&models.ReviewTask{
		ID: "same-ch-task-1", PRURL: "https://github.com/test/repo/pull/30",
		Repo: "test/repo", PRNumber: 30, Title: "Same Channel PR",
		SlackTS: "3333.3333", SlackChannel: "C_SAME",
		Status: "in_review", LabelName: "needs-review",
		CreatedAt: time.Now().Add(-1 * time.Hour), UpdatedAt: time.Now(),
	})
	db.Create(&models.ReviewTask{
		ID: "same-ch-task-2", PRURL: "https://github.com/test/repo/pull/30",
		Repo: "test/repo", PRNumber: 30, Title: "Same Channel PR",
		SlackTS: "4444.4444", SlackChannel: "C_SAME",
		Status: "in_review", LabelName: "review-needed",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	payload := buildIssueCommentPayload("created", 30, "test", "repo", "pr-author", "pr-author",
		"https://github.com/test/repo/pull/30#issuecomment-300")

	w := sendIssueCommentWebhook(t, db, payload)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, gock.IsDone(), "Slack APIは1回だけ呼ばれるべき")
}

func TestIssueCommentEvent_EmptySlackTSSkipped(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	defer gock.Off()
	// SlackTSが空のタスクはスキップされるので、Slack APIは呼ばれない
	mock := gock.New("https://slack.com").
		Post("/api/chat.postMessage").
		Reply(200).
		JSON(map[string]interface{}{"ok": true})

	// SlackTS空のタスク（まだSlackメッセージ送信前）
	db.Create(&models.ReviewTask{
		ID: "empty-ts-task", PRURL: "https://github.com/test/repo/pull/40",
		Repo: "test/repo", PRNumber: 40, Title: "Empty TS PR",
		SlackTS: "", SlackChannel: "C_EMPTY",
		Status: "pending", LabelName: "needs-review",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})

	payload := buildIssueCommentPayload("created", 40, "test", "repo", "pr-author", "pr-author",
		"https://github.com/test/repo/pull/40#issuecomment-400")

	w := sendIssueCommentWebhook(t, db, payload)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, gock.IsPending(), "SlackTS空のタスクではSlack APIは呼ばれないべき")
	_ = mock
}
