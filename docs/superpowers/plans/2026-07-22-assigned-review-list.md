# 自分宛てレビュー依頼一覧コマンド Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `/slack-review-notify reviews` で、全チャンネルからコマンド実行者本人へ割り当てられた未完了レビュー依頼だけを、クリック可能なPRリンク付きのephemeral一覧として表示する。

**Architecture:** 既存の `HandleSlackCommand` に `reviews` 分岐を追加し、`ReviewTask` を既存DBから取得してGo側で担当者IDを完全一致判定する。表示構築は `handlers/command.go` の非公開ヘルパーに閉じ、既存i18nマップを使って日英のSlack mrkdwnを生成する。

**Tech Stack:** Go 1.24.1、Gin、GORM、SQLite、testify、既存i18nパッケージ

## Global Constraints

- コマンドは `/slack-review-notify reviews` とする。
- 検索対象は全Slackチャンネルの `ReviewTask` とし、チャンネルでは絞り込まない。
- 実行者のSlack User IDが `Reviewers` のカンマ区切り要素または旧形式の `Reviewer` に完全一致するタスクだけを表示する。
- 実行者のSlack User IDが `ApprovedBy` に完全一致する承認済みタスクは表示しない。
- 未完了ステータスは `pending`、`in_review`、`waiting_business_hours`、`snoozed` の4種類とする。
- DB側で担当者候補を前絞りし、完全一致はGo側で担保する。
- 応答は成功、該当なし、DBエラーのすべてでHTTP 200かつ `response_type: "ephemeral"` とする。
- `CreatedAt` 昇順、同時刻は `ID` 昇順で並べ、最大50件を表示する。
- 51件以上の場合は全一致件数をヘッダーへ出し、末尾に残件数を表示する。
- 各項目にクリック可能なGitHub PRリンクとSlackチャンネルメンションを含める。
- 表示アイコンはSlackのASCII emoji aliasとし、Unicode絵文字を追加しない。
- PRタイトルは空白を1個へ正規化し、`&`、`<`、`>` をSlack向けにエスケープする。
- 新しい依存関係、DBスキーマ、マイグレーション、公開インターフェース、Slack API呼び出しを追加しない。
- 既存コマンド、通知、リマインド、タスク状態遷移を変更しない。

---

### Task 1: 自分宛てレビュー依頼一覧コマンド

**Files:**
- Create: `handlers/reviews_command_test.go`
- Modify: `handlers/command.go:80-87,151-284` および同ファイル末尾の非公開ヘルパー群
- Modify: `handlers/commands_test.go:60-71`
- Modify: `i18n/messages_ja.go:42-81` およびコマンド別メッセージ定義
- Modify: `i18n/messages_en.go:42-81` およびコマンド別メッセージ定義

**Interfaces:**
- Consumes: `models.ReviewTask`、`i18n.L(lang)`、`setupCommandIntegrationTestDB`、`setupHTTPRequest`、`services.IsTestMode`
- Produces: `showAssignedReviews(c *gin.Context, db *gorm.DB, userID, lang string)`、`isAssignedReview(task models.ReviewTask, userID string) bool`、`slackSafeReviewTitle(title string) string`、`assignedReviewStatusPresentation(status, lang string) (string, string)`、`respondEphemeral(c *gin.Context, text string)`

- [ ] **Step 1: 仕様全体を表す失敗テストを追加する**

`handlers/reviews_command_test.go` を次の内容で作成する。

```go
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"slack-review-notify/models"
	"slack-review-notify/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type reviewsCommandResponse struct {
	ResponseType string `json:"response_type"`
	Text         string `json:"text"`
}

func runReviewsCommand(t *testing.T, db *gorm.DB, channelID string) reviewsCommandResponse {
	t.Helper()
	services.IsTestMode = true
	t.Cleanup(func() { services.IsTestMode = false })

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/slack/command", HandleSlackCommand(db))

	req := setupHTTPRequest(t, "reviews", channelID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	var response reviewsCommandResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
	return response
}

func TestReviewsCommandShowsAssignedActiveTasksAcrossChannels(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)
	base := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	tasks := []models.ReviewTask{
		{ID: "01", PRURL: "https://github.com/example/api/pull/101", Repo: "example/api", PRNumber: 101, Title: "  Fix  <auth> &\n login ", SlackChannel: "C_BACKEND", Reviewers: "U999, U12345", Status: "in_review", CreatedAt: base},
		{ID: "02", PRURL: "https://github.com/example/web/pull/102", Repo: "example/web", PRNumber: 102, Title: "Web fix", SlackChannel: "C_FRONTEND", Reviewer: " U12345 ", Status: "waiting_business_hours", CreatedAt: base},
		{ID: "03", PRURL: "https://github.com/example/app/pull/103", Repo: "example/app", PRNumber: 103, Title: "App fix", SlackChannel: "C_APP", Reviewer: "U12345", Reviewers: "U12345", Status: "paused", CreatedAt: base.Add(2 * time.Minute)},
		{ID: "04", PRURL: "https://github.com/example/job/pull/104", Repo: "example/job", PRNumber: 104, Title: "Job fix", SlackChannel: "C_JOB", Reviewers: "U12345", Status: "pending", CreatedAt: base.Add(3 * time.Minute)},
		{ID: "05", PRURL: "https://github.com/example/legacy/pull/105", Repo: "example/legacy", PRNumber: 105, Title: "Legacy fix", SlackChannel: "C_LEGACY", Reviewers: "U12345", Status: "snoozed", CreatedAt: base.Add(4 * time.Minute)},
		{ID: "06", PRURL: "https://github.com/example/other/pull/106", Repo: "example/other", PRNumber: 106, Title: "Other user", SlackChannel: "C_OTHER", Reviewers: "U123450", Status: "in_review", CreatedAt: base.Add(5 * time.Minute)},
		{ID: "07", PRURL: "https://github.com/example/done/pull/107", Repo: "example/done", PRNumber: 107, Title: "Completed", SlackChannel: "C_DONE", Reviewers: "U12345", Status: "completed", CreatedAt: base.Add(6 * time.Minute)},
		{ID: "08", PRURL: "https://github.com/example/manual/pull/108", Repo: "example/manual", PRNumber: 108, Title: "Done", SlackChannel: "C_DONE", Reviewers: "U12345", Status: "done", CreatedAt: base.Add(7 * time.Minute)},
		{ID: "09", PRURL: "https://github.com/example/archive/pull/109", Repo: "example/archive", PRNumber: 109, Title: "Archived", SlackChannel: "C_ARCHIVE", Reviewers: "U12345", Status: "archived", CreatedAt: base.Add(8 * time.Minute)},
	}
	for i := range tasks {
		require.NoError(t, db.Create(&tasks[i]).Error)
	}
	deleted := models.ReviewTask{ID: "10", PRURL: "https://github.com/example/deleted/pull/110", Repo: "example/deleted", PRNumber: 110, Title: "Deleted", SlackChannel: "C_DELETED", Reviewers: "U12345", Status: "in_review", CreatedAt: base.Add(9 * time.Minute)}
	require.NoError(t, db.Create(&deleted).Error)
	require.NoError(t, db.Delete(&deleted).Error)

	response := runReviewsCommand(t, db, "C_COMMAND")
	assert.Equal(t, "ephemeral", response.ResponseType)
	assert.Contains(t, response.Text, ":clipboard: *あなたへの未完了レビュー依頼 (4件)*")
	assert.Contains(t, response.Text, ":large_blue_circle: <https://github.com/example/api/pull/101|example/api #101>")
	assert.Contains(t, response.Text, "Fix &lt;auth&gt; &amp; login")
	assert.Contains(t, response.Text, "<#C_BACKEND> | レビュー中")
	assert.Contains(t, response.Text, ":crescent_moon:")
	assert.Contains(t, response.Text, ":hourglass_flowing_sand:")
	assert.Contains(t, response.Text, ":zzz:")

	expectedOrder := []string{"pull/101", "pull/102", "pull/104", "pull/105"}
	lastIndex := -1
	for _, fragment := range expectedOrder {
		index := strings.Index(response.Text, fragment)
		assert.Greater(t, index, lastIndex, "expected %s after the previous PR", fragment)
		lastIndex = index
	}
	assert.NotContains(t, response.Text, "pull/103")
	assert.NotContains(t, response.Text, "pull/106")
	assert.NotContains(t, response.Text, "pull/107")
	assert.NotContains(t, response.Text, "pull/108")
	assert.NotContains(t, response.Text, "pull/109")
	assert.NotContains(t, response.Text, "pull/110")
}

func TestReviewsCommandReturnsLocalizedEmptyState(t *testing.T) {
	tests := []struct {
		name      string
		language  string
		expected  string
		channelID string
	}{
		{name: "Japanese default", expected: ":white_check_mark: あなたへの未完了レビュー依頼はありません。", channelID: "C_JA"},
		{name: "English config", language: "en", expected: ":white_check_mark: You have no unfinished review requests.", channelID: "C_EN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := setupCommandIntegrationTestDB(t)
			if tt.language != "" {
				config := models.ChannelConfig{ID: "config-" + tt.language, SlackChannelID: tt.channelID, LabelName: "needs-review", Language: tt.language}
				require.NoError(t, db.Create(&config).Error)
			}
			response := runReviewsCommand(t, db, tt.channelID)
			assert.Equal(t, "ephemeral", response.ResponseType)
			assert.Equal(t, tt.expected, response.Text)
		})
	}
}

func TestReviewsCommandUsesEnglishPresentation(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)
	config := models.ChannelConfig{ID: "config-en", SlackChannelID: "C_EN", LabelName: "needs-review", Language: "en"}
	task := models.ReviewTask{ID: "english-task", PRURL: "https://github.com/example/en/pull/201", Repo: "example/en", PRNumber: 201, Title: "English title", SlackChannel: "C_EN_REVIEW", Reviewers: "U12345", Status: "waiting_business_hours", CreatedAt: time.Now()}
	require.NoError(t, db.Create(&config).Error)
	require.NoError(t, db.Create(&task).Error)

	response := runReviewsCommand(t, db, "C_EN")
	assert.Contains(t, response.Text, ":clipboard: *Your unfinished review requests (1)*")
	assert.Contains(t, response.Text, ":crescent_moon: <https://github.com/example/en/pull/201|example/en #201>")
	assert.Contains(t, response.Text, "<#C_EN_REVIEW> | Waiting for business hours")
}

func TestReviewsCommandLimitsResultsToOldest50(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)
	base := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	for i := 0; i < 51; i++ {
		task := models.ReviewTask{
			ID: fmt.Sprintf("task-%03d", i), PRURL: fmt.Sprintf("https://github.com/example/limit/pull/%03d", i),
			Repo: "example/limit", PRNumber: i, Title: fmt.Sprintf("Task %03d", i), SlackChannel: "C_LIMIT",
			Reviewers: "U12345", Status: "in_review", CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}
		require.NoError(t, db.Create(&task).Error)
	}

	response := runReviewsCommand(t, db, "C_COMMAND")
	assert.Contains(t, response.Text, "未完了レビュー依頼 (51件)")
	assert.Equal(t, 50, strings.Count(response.Text, ":large_blue_circle:"))
	assert.Contains(t, response.Text, "pull/000|example/limit #0")
	assert.Contains(t, response.Text, "pull/049|example/limit #49")
	assert.NotContains(t, response.Text, "pull/050|example/limit #50")
	assert.Contains(t, response.Text, ":information_source: ほか1件あります。")
}

func TestReviewsCommandReturnsEphemeralDatabaseError(t *testing.T) {
	db := setupCommandIntegrationTestDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	response := runReviewsCommand(t, db, "C_COMMAND")
	assert.Equal(t, "ephemeral", response.ResponseType)
	assert.Equal(t, ":warning: レビュー依頼の取得に失敗しました。", response.Text)
}
```

`handlers/commands_test.go` の `TestHandleSlackCommand_Help` に次のassertionを追加する。

```go
assert.Contains(t, w.Body.String(), "/slack-review-notify reviews")
```

- [ ] **Step 2: テストが機能未実装を理由に失敗することを確認する**

Run:

```bash
go test ./handlers -run 'TestReviewsCommand|TestHandleSlackCommand_Help' -v
```

Expected: `reviews` が既知のサブコマンドではなく、レスポンスがephemeral JSONにならないため `TestReviewsCommand...` がFAILし、ヘルプに `/slack-review-notify reviews` がないため `TestHandleSlackCommand_Help` もFAILする。

- [ ] **Step 3: 失敗テストを通す最小実装を追加する**

`handlers/command.go` の `potentialSubCommands` に `reviews` を追加する。

```go
potentialSubCommands := []string{"show", "help", "reviews", "set-mention", "add-reviewer",
```

同ファイルの `switch subCommand` に次の分岐を追加する。

```go
case "reviews":
	showAssignedReviews(c, db, userID, lang)
```

同ファイルの非公開ヘルパー群へ次を追加する。

```go
const maxAssignedReviews = 50

func showAssignedReviews(c *gin.Context, db *gorm.DB, userID, lang string) {
	t := i18n.L(lang)
	userID = strings.TrimSpace(userID)
	var tasks []models.ReviewTask
	result := db.Where("status IN ?", []string{
		"pending", "in_review", "waiting_business_hours", "snoozed",
	}).Where("(reviewers LIKE ? OR TRIM(reviewer) = ?)", "%"+userID+"%", userID).
		Order("created_at ASC").Order("id ASC").Find(&tasks)
	if result.Error != nil {
		log.Printf("assigned review list query error: %v", result.Error)
		respondEphemeral(c, t("cmd.reviews.error"))
		return
	}

	assigned := make([]models.ReviewTask, 0, len(tasks))
	for _, task := range tasks {
		if isAssignedReview(task, userID) {
			assigned = append(assigned, task)
		}
	}

	if len(assigned) == 0 {
		respondEphemeral(c, t("cmd.reviews.empty"))
		return
	}

	var response strings.Builder
	response.WriteString(t("cmd.reviews.header", len(assigned)))
	displayCount := len(assigned)
	if displayCount > maxAssignedReviews {
		displayCount = maxAssignedReviews
	}
	for _, task := range assigned[:displayCount] {
		emoji, statusLabel := assignedReviewStatusPresentation(task.Status, lang)
		fmt.Fprintf(&response, "\n\n%s <%s|%s #%d>\n%s\n<#%s> | %s",
			emoji, task.PRURL, task.Repo, task.PRNumber,
			slackSafeReviewTitle(task.Title), task.SlackChannel, statusLabel)
	}
	if len(assigned) > displayCount {
		response.WriteString("\n\n")
		response.WriteString(t("cmd.reviews.truncated", len(assigned)-displayCount))
	}

	respondEphemeral(c, response.String())
}

func isAssignedReview(task models.ReviewTask, userID string) bool {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false
	}
	for _, reviewer := range strings.Split(task.ApprovedBy, ",") {
		if strings.TrimSpace(reviewer) == userID {
			return false
		}
	}
	if strings.TrimSpace(task.Reviewer) == userID {
		return true
	}
	for _, reviewer := range strings.Split(task.Reviewers, ",") {
		if strings.TrimSpace(reviewer) == userID {
			return true
		}
	}
	return false
}

func slackSafeReviewTitle(title string) string {
	title = strings.Join(strings.Fields(title), " ")
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
	).Replace(title)
}

func assignedReviewStatusPresentation(status, lang string) (string, string) {
	t := i18n.L(lang)
	switch status {
	case "pending":
		return ":hourglass_flowing_sand:", t("cmd.reviews.status.pending")
	case "in_review":
		return ":large_blue_circle:", t("cmd.reviews.status.in_review")
	case "waiting_business_hours":
		return ":crescent_moon:", t("cmd.reviews.status.waiting_business_hours")
	case "snoozed":
		return ":zzz:", t("cmd.reviews.status.snoozed")
	default:
		return ":grey_question:", status
	}
}

func respondEphemeral(c *gin.Context, text string) {
	c.JSON(http.StatusOK, gin.H{
		"response_type": "ephemeral",
		"text":          text,
	})
}
```

`i18n/messages_ja.go` の基本操作へ次のヘルプ行を追加する。

```text
• /slack-review-notify reviews - 全チャンネルから自分宛ての未完了レビュー依頼を表示
```

同ファイルのメッセージマップへ次を追加する。

```go
// ==================== Command: reviews ====================
"cmd.reviews.header":                        ":clipboard: *あなたへの未完了レビュー依頼 (%d件)*\n_古い依頼から表示しています_",
"cmd.reviews.empty":                         ":white_check_mark: あなたへの未完了レビュー依頼はありません。",
"cmd.reviews.truncated":                     ":information_source: ほか%d件あります。",
"cmd.reviews.error":                         ":warning: レビュー依頼の取得に失敗しました。",
"cmd.reviews.status.pending":                "登録処理中",
"cmd.reviews.status.in_review":              "レビュー中",
"cmd.reviews.status.waiting_business_hours": "営業時間待ち",
"cmd.reviews.status.snoozed":                "スヌーズ中",
```

`i18n/messages_en.go` の基本操作へ次のヘルプ行を追加する。

```text
• /slack-review-notify reviews - Show your unfinished review requests across all channels
```

同ファイルのメッセージマップへ次を追加する。

```go
// ==================== Command: reviews ====================
"cmd.reviews.header":                        ":clipboard: *Your unfinished review requests (%d)*\n_Oldest requests are shown first_",
"cmd.reviews.empty":                         ":white_check_mark: You have no unfinished review requests.",
"cmd.reviews.truncated":                     ":information_source: %d more review request(s) not shown.",
"cmd.reviews.error":                         ":warning: Failed to retrieve review requests.",
"cmd.reviews.status.pending":                "Processing",
"cmd.reviews.status.in_review":              "In review",
"cmd.reviews.status.waiting_business_hours": "Waiting for business hours",
"cmd.reviews.status.snoozed":                "Snoozed",
```

- [ ] **Step 4: フォーマット後、対象テストが通ることを確認する**

Run:

```bash
gofmt -w handlers/command.go handlers/commands_test.go handlers/reviews_command_test.go i18n/messages_ja.go i18n/messages_en.go
go test ./handlers -run 'TestReviewsCommand|TestHandleSlackCommand_Help' -v
```

Expected: 対象テストがすべてPASSし、終了コード0になる。

- [ ] **Step 5: 全体検証を実行する**

Run:

```bash
go test ./...
make lint
make build
git diff --check
```

Expected: 全コマンドが終了コード0。`go test ./...` は全パッケージPASS、`make lint` はissueなし、`make build` はバイナリ生成成功、`git diff --check` は出力なし。

- [ ] **Step 6: 実装をcommitする**

```bash
git add handlers/command.go handlers/commands_test.go handlers/reviews_command_test.go i18n/messages_ja.go i18n/messages_en.go
git commit -m "feat: add assigned review list command"
```

Expected: テスト先行で追加した一覧コマンド実装と日英UI文言が1 commitにまとまり、仕様書・計画書以外の変更は上記5ファイルだけになる。
