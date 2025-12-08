package handlers

import (
	"net/http/httptest"
	"reflect"
	"slack-review-notify/models"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "通常のスペースなしラベル名",
			input:    "needs-review set-mention @user",
			expected: []string{"needs-review", "set-mention", "@user"},
		},
		{
			name:     "ダブルクォートで囲まれたスペース付きラベル名",
			input:    "\"needs review\" set-mention @user",
			expected: []string{"needs review", "set-mention", "@user"},
		},
		{
			name:     "シングルクォートで囲まれたスペース付きラベル名",
			input:    "'security review' add-reviewer @security",
			expected: []string{"security review", "add-reviewer", "@security"},
		},
		{
			name:     "複数のスペース付きパラメータ",
			input:    "\"needs review\" set-mention \"@team lead\"",
			expected: []string{"needs review", "set-mention", "@team lead"},
		},
		{
			name:     "異なるクォート文字の混在",
			input:    "'label name' \"param value\" test",
			expected: []string{"label name", "param value", "test"},
		},
		{
			name:     "クォート内にクォート文字",
			input:    "\"label's name\" 'param \"value\"' test",
			expected: []string{"label's name", "param \"value\"", "test"},
		},
		{
			name:     "空文字列",
			input:    "",
			expected: nil,
		},
		{
			name:     "スペースのみ",
			input:    "   ",
			expected: nil,
		},
		{
			name:     "単一の要素",
			input:    "show",
			expected: []string{"show"},
		},
		{
			name:     "クォートが閉じられていない場合（ダブルクォート）",
			input:    "\"needs review set-mention @user",
			expected: []string{"needs review set-mention @user"},
		},
		{
			name:     "クォートが閉じられていない場合（シングルクォート）",
			input:    "'security review add-reviewer @security",
			expected: []string{"security review add-reviewer @security"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCommand(tt.input)

			// nilと空のスライスを同等として扱う
			if tt.expected == nil && len(result) == 0 {
				return
			}
			if len(tt.expected) == 0 && len(result) == 0 {
				return
			}

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("parseCommand(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSetReviewerCount(t *testing.T) {
	tests := []struct {
		name           string
		setupConfig    *models.ChannelConfig
		channelID      string
		labelName      string
		countStr       string
		expectedCount  int
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "正常系: 新規設定でレビュワー数を3に設定",
			setupConfig: nil,
			channelID:   "C12345",
			labelName:   "needs-review",
			countStr:    "3",
			expectedCount: 3,
			expectedStatus: 200,
			expectedBody: "ラベル「needs-review」のレビュワー割り当て人数を 3人 に設定しました。",
		},
		{
			name: "正常系: 既存設定のレビュワー数を5に更新",
			setupConfig: &models.ChannelConfig{
				ID:               "test-id",
				SlackChannelID:   "C12345",
				LabelName:        "needs-review",
				DefaultMentionID: "U12345",
				ReviewerCount:    1,
				IsActive:         true,
				CreatedAt:        time.Now(),
				UpdatedAt:        time.Now(),
			},
			channelID:      "C12345",
			labelName:      "needs-review",
			countStr:       "5",
			expectedCount:  5,
			expectedStatus: 200,
			expectedBody:   "ラベル「needs-review」のレビュワー割り当て人数を 5人 に更新しました。",
		},
		{
			name: "正常系: レビュワー数を1に設定（最小値）",
			setupConfig: nil,
			channelID:   "C12345",
			labelName:   "needs-review",
			countStr:    "1",
			expectedCount: 1,
			expectedStatus: 200,
			expectedBody: "ラベル「needs-review」のレビュワー割り当て人数を 1人 に設定しました。",
		},
		{
			name: "異常系: レビュワー数が0",
			setupConfig: nil,
			channelID:   "C12345",
			labelName:   "needs-review",
			countStr:    "0",
			expectedCount:  0,
			expectedStatus: 200,
			expectedBody:   "レビュワー数は1以上の整数で指定してください。",
		},
		{
			name: "異常系: レビュワー数が負の値",
			setupConfig: nil,
			channelID:   "C12345",
			labelName:   "needs-review",
			countStr:    "-1",
			expectedCount:  0,
			expectedStatus: 200,
			expectedBody:   "レビュワー数は1以上の整数で指定してください。",
		},
		{
			name: "異常系: レビュワー数が数値でない",
			setupConfig: nil,
			channelID:   "C12345",
			labelName:   "needs-review",
			countStr:    "abc",
			expectedCount:  0,
			expectedStatus: 200,
			expectedBody:   "レビュワー数は1以上の整数で指定してください。",
		},
		{
			name: "異常系: レビュワー数が空文字列",
			setupConfig: nil,
			channelID:   "C12345",
			labelName:   "needs-review",
			countStr:    "",
			expectedCount:  0,
			expectedStatus: 200,
			expectedBody:   "レビュワー数は1以上の整数で指定してください。",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			if err != nil {
				t.Fatalf("failed to open test db: %v", err)
			}

			// マイグレーション
			if err := db.AutoMigrate(&models.ChannelConfig{}); err != nil {
				t.Fatalf("failed to migrate: %v", err)
			}

			// 既存設定がある場合は作成
			if tt.setupConfig != nil {
				db.Create(tt.setupConfig)
			}

			// Ginのテストコンテキストを作成
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			// setReviewerCount関数を呼び出し
			setReviewerCount(c, db, tt.channelID, tt.labelName, tt.countStr)

			// HTTPレスポンスの検証
			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Contains(t, w.Body.String(), tt.expectedBody)

			// 正常系の場合、DBの値を検証
			if tt.expectedCount > 0 {
				var config models.ChannelConfig
				result := db.Where("slack_channel_id = ? AND label_name = ?", tt.channelID, tt.labelName).First(&config)

				assert.NoError(t, result.Error, "設定がDBに保存されているべき")
				assert.Equal(t, tt.expectedCount, config.ReviewerCount, "ReviewerCountが期待値と一致するべき")
				assert.True(t, config.IsActive, "新規作成時はアクティブであるべき")
			}
		})
	}
}
