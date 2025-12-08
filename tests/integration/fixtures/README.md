# Test Fixtures

このディレクトリには統合テストで使用するフィクスチャデータが格納されています。

## ファイル構成

### channel_configs.json
チャンネル設定のフィクスチャデータです。以下のフィールドを含みます：

- `id`: 設定ID
- `slack_channel_id`: SlackチャンネルID
- `label_name`: 対象のGitHubラベル名
- `default_mention_id`: デフォルトのメンション先ユーザーID
- `reviewer_list`: レビュワーリスト（カンマ区切り）
- `repository_list`: 対象リポジトリリスト（カンマ区切り）
- `is_active`: 設定が有効かどうか
- `reminder_interval`: リマインド頻度（分単位）
- `reviewer_reminder_interval`: レビュワー割り当て後のリマインド頻度（分単位）
- `business_hours_start`: 営業時間開始（HH:MM形式）
- `business_hours_end`: 営業時間終了（HH:MM形式）
- `timezone`: タイムゾーン

### review_tasks.json
レビュータスクのフィクスチャデータです。以下のフィールドを含みます：

- `id`: タスクID
- `pr_url`: Pull RequestのURL
- `repo`: リポジトリ名
- `pr_number`: PR番号
- `title`: PRタイトル
- `slack_ts`: Slackメッセージのタイムスタンプ
- `slack_channel`: SlackチャンネルID
- `reviewer`: 割り当てられたレビュワー
- `status`: タスクステータス（pending, in_review, paused, archived, done, waiting_business_hours）
- `label_name`: GitHubラベル名
- `watching_until`: 監視終了時刻（ISO8601形式）
- `reminder_paused_until`: リマインダー一時停止終了時刻（ISO8601形式）
- `out_of_hours_reminded`: 営業時間外に自動でリマインドを一時停止したかのフラグ

### webhook_payloads.json
GitHubウェブフックペイロードのフィクスチャデータです。以下のペイロードを含みます：

- `pull_request_labeled`: PRにラベルが付与された時のペイロード
- `pull_request_labeled_urgent`: 緊急レビューラベルが付与された時のペイロード
- `pull_request_labeled_backend`: バックエンドレビューラベルが付与された時のペイロード
- `pull_request_unlabeled`: PRからラベルが削除された時のペイロード
- `pull_request_opened`: PRが作成された時のペイロード

## 使用方法

### 基本的な読み込み

```go
import "slack-review-notify/tests/integration/fixtures"

// チャンネル設定の読み込み
configs, err := fixtures.LoadChannelConfigs()
if err != nil {
    t.Fatal(err)
}

// レビュータスクの読み込み
tasks, err := fixtures.LoadReviewTasks()
if err != nil {
    t.Fatal(err)
}

// ウェブフックペイロードの読み込み
payloads, err := fixtures.LoadWebhookPayloads()
if err != nil {
    t.Fatal(err)
}

// 特定のペイロードを読み込み
payload, err := fixtures.LoadWebhookPayload("pull_request_labeled")
if err != nil {
    t.Fatal(err)
}
```

### ヘルパー関数

フィクスチャデータから特定のアイテムを検索するためのヘルパー関数が提供されています：

```go
// チャンネルIDで設定を検索
config := fixtures.GetChannelConfigByID(configs, "C01TEST001")

// ラベル名で設定を検索
config := fixtures.GetChannelConfigByLabel(configs, "needs-review")

// PR番号とリポジトリでタスクを検索
task := fixtures.GetReviewTaskByPR(tasks, 123, "owner/repo")

// ステータスでタスクをフィルタ
pendingTasks := fixtures.GetReviewTasksByStatus(tasks, "pending")

// チャンネルでタスクをフィルタ
channelTasks := fixtures.GetReviewTasksByChannel(tasks, "C01TEST001")
```

## テスト例

```go
func TestWebhookHandler(t *testing.T) {
    // フィクスチャからペイロードを読み込み
    payload, err := fixtures.LoadWebhookPayload("pull_request_labeled")
    require.NoError(t, err)

    // ペイロードをJSONにエンコード
    body, err := json.Marshal(payload)
    require.NoError(t, err)

    // HTTPリクエストを作成してテスト
    req := httptest.NewRequest("POST", "/webhook", bytes.NewBuffer(body))
    // ... テストコード
}

func TestTaskCreation(t *testing.T) {
    // フィクスチャから設定を読み込み
    configs, err := fixtures.LoadChannelConfigs()
    require.NoError(t, err)

    // テスト用の設定を取得
    config := fixtures.GetChannelConfigByLabel(configs, "needs-review")
    require.NotNil(t, config)

    // 設定を使ってテスト
    // ... テストコード
}
```

## フィクスチャデータの追加・編集

フィクスチャデータを追加または編集する場合は、対応するJSONファイルを直接編集してください。
JSONの構造は各モデルのフィールドに対応しています。

編集後は以下のコマンドでテストを実行し、正しく読み込めることを確認してください：

```bash
go test ./tests/integration/fixtures/
```
