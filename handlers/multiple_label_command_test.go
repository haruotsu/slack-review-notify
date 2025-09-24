package handlers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"slack-review-notify/models"
	"slack-review-notify/services"

	"github.com/gin-gonic/gin"
	"github.com/h2non/gock"
	"github.com/stretchr/testify/assert"
)

func TestMultipleLabelCommandHandler(t *testing.T) {
	tests := []struct {
		name           string
		command        string
		text           string
		expectedStatus int
		expectedBody   string
		setupConfig    *models.ChannelConfig
	}{
		{
			name:           "複数ラベルでset-mentionコマンド",
			command:        "/slack-review-notify",
			text:           `"hoge-project,needs-review" set-mention @team`,
			expectedStatus: 200,
			expectedBody:   "ラベル「hoge-project,needs-review」のメンション先を <@team> に設定しました。",
		},
		{
			name:           "複数ラベルでadd-repoコマンド",
			command:        "/slack-review-notify",
			text:           `"frontend,urgent,needs-review" add-repo owner/webapp`,
			expectedStatus: 200,
			expectedBody:   "ラベル「frontend,urgent,needs-review」の通知対象リポジトリに `owner/webapp` を追加しました。",
		},
		{
			name:           "複数ラベル設定の表示",
			command:        "/slack-review-notify",
			text:           `"project-a,needs-review" show`,
			expectedStatus: 200,
			expectedBody:   "*このチャンネルのラベル「project-a,needs-review」のレビュー通知設定*",
			setupConfig: &models.ChannelConfig{
				SlackChannelID:   "C1234567890",
				LabelName:        "project-a,needs-review",
				DefaultMentionID: "@team",
				RepositoryList:   "owner/repo",
				IsActive:         true,
				CreatedAt:        time.Now(),
				UpdatedAt:        time.Now(),
			},
		},
		{
			name:           "スペース付き複数ラベル設定",
			command:        "/slack-review-notify",
			text:           `"project a, needs review, urgent" set-mention @team`,
			expectedStatus: 200,
			expectedBody:   "ラベル「project a, needs review, urgent」のメンション先を <@team> に設定しました。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// セットアップ
			db := setupTestDB(t)
			gin.SetMode(gin.TestMode)
			services.IsTestMode = true

			// 既存設定があれば作成
			if tt.setupConfig != nil {
				tt.setupConfig.ID = "test-config-id"
				db.Create(tt.setupConfig)
			}

			// リクエストボディを作成
			data := url.Values{}
			data.Set("command", tt.command)
			data.Set("text", tt.text)
			data.Set("channel_id", "C1234567890")
			data.Set("user_id", "U1234567890")

			// リクエスト作成
			req, _ := http.NewRequest("POST", "/slack/command", strings.NewReader(data.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

			// Slack署名検証をモック（テストモードでスキップ）
			req.Header.Set("X-Slack-Signature", "test")
			req.Header.Set("X-Slack-Request-Timestamp", "1234567890")

			w := httptest.NewRecorder()

			// Ginルーターを作成してリクエスト実行
			router := gin.Default()
			router.POST("/slack/command", HandleSlackCommand(db))
			router.ServeHTTP(w, req)

			// HTTPステータス確認
			assert.Equal(t, tt.expectedStatus, w.Code)

			// レスポンス内容確認
			assert.Contains(t, w.Body.String(), tt.expectedBody)

			// データベースに保存されたか確認
			if strings.Contains(tt.text, "set-mention") || strings.Contains(tt.text, "add-repo") {
				var config models.ChannelConfig
				labelName := extractLabelFromText(tt.text)
				result := db.Where("slack_channel_id = ? AND label_name = ?", "C1234567890", labelName).First(&config)
				assert.NoError(t, result.Error, "設定がデータベースに保存されていません")
			}

			// クリーンアップ
			gock.Off()
		})
	}
}

// テキストからラベル名を抽出するヘルパー関数
func extractLabelFromText(text string) string {
	parts := parseCommand(text)
	if len(parts) > 0 {
		return parts[0]
	}
	return "needs-review"
}

func TestMultipleLabelHelp(t *testing.T) {
	// セットアップ
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	services.IsTestMode = true

	// helpコマンドのリクエスト
	data := url.Values{}
	data.Set("command", "/slack-review-notify")
	data.Set("text", "help")
	data.Set("channel_id", "C1234567890")
	data.Set("user_id", "U1234567890")

	req, _ := http.NewRequest("POST", "/slack/command", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Slack-Signature", "test")
	req.Header.Set("X-Slack-Request-Timestamp", "1234567890")

	w := httptest.NewRecorder()

	router := gin.Default()
	router.POST("/slack/command", HandleSlackCommand(db))
	router.ServeHTTP(w, req)

	// HTTPステータス確認
	assert.Equal(t, 200, w.Code)

	// ヘルプメッセージに複数ラベルの説明が含まれているか確認
	body := w.Body.String()
	assert.Contains(t, body, "*複数ラベルAND条件の設定*")
	assert.Contains(t, body, "カンマ区切りで複数のラベルを指定")
	assert.Contains(t, body, "hoge-project,needs-review")
	assert.Contains(t, body, "全てのラベルが付いている場合のみ通知")
}