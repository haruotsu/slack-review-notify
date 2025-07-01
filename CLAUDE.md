# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Development Commands

### 基本的な開発コマンド
```bash
# 依存関係のインストール
make deps

# 開発サーバーの起動（ホットリロード）
make dev

# バイナリのビルド
make build

# アプリケーションの実行（ビルド後）
make run

# テストの実行
make test

# カバレッジ付きテスト
make test-coverage

# Lintの実行
make lint

# golangci-lintのインストール（初回のみ）
make lint-install

# クリーンアップ（ビルド成果物とDBファイルを削除）
make clean
```

### 個別テストの実行
```bash
# 特定のパッケージのテスト
go test ./handlers/
go test ./services/
go test ./models/

# 特定のテストケースの実行
go test -run TestFunctionName ./services/

# Verbose モードでテスト実行
go test -v ./...
```

## Architecture Overview

### アプリケーション構造
このプロジェクトはGitHub PRのレビュー依頼をSlackで管理するWebアプリケーションで、以下の主要コンポーネントで構成されています：

1. **Webフレームワーク**: Gin (HTTPルーティング)
2. **データベース**: SQLite + GORM (ローカルストレージ)
3. **外部連携**: GitHub Webhooks, Slack API (コマンド、イベント、インタラクティブコンポーネント)

### エンドポイントとイベントフロー

#### GitHub → Slack 通知フロー
1. GitHub PRにラベルが付けられる
2. `/webhook` エンドポイントでWebhookを受信 (main.go:54-223)
3. ChannelConfigテーブルから該当する設定を検索
4. 条件に合致するチャンネルにSlackメッセージを送信
5. ReviewTaskテーブルにタスクを記録
6. レビュワーをランダムに割り当て、スレッドに通知

#### Slackコマンドによる設定管理
1. `/slack-review-notify` コマンドを受信 (handlers/command.go)
2. コマンドをパース（ラベル名、サブコマンド、引数）
3. ChannelConfigテーブルを更新
4. 結果をユーザーに返信

#### バックグラウンドプロセス
1. **タスクチェッカー** (main.go:237-258)
   - 1分ごとに実行
   - レビュー中のタスクをチェックしてリマインド送信
   - 1時間ごとに古いタスクをクリーンアップ

2. **チャンネルチェッカー** (main.go:261-269)
   - 1時間ごとに実行
   - アーカイブされたチャンネルを検出して設定を無効化

### データモデル

#### ChannelConfig (models/channel_config.go)
- チャンネルごとの通知設定を管理
- `slack_channel_id` と `label_name` でユニーク制約
- 複数のラベルに対して異なる設定が可能

#### ReviewTask (models/review_tasks.go)
- PR レビュータスクの状態を管理
- ステータス: pending, in_review, completed, snoozed
- Slackメッセージとの紐付け情報を保持

### 環境変数
```bash
SLACK_BOT_TOKEN=xoxb-...        # Slack Bot Token
SLACK_SIGNING_SECRET=...        # Slack署名検証用
GITHUB_WEBHOOK_SECRET=...       # GitHub Webhook検証用（オプション）
DB_PATH=review_tasks.db         # DBファイルパス（デフォルト: review_tasks.db）
```

### 重要な実装パターン

#### Slack API通信 (services/slack.go)
- 環境変数 `SLACK_BOT_TOKEN` を使用
- メッセージ送信、更新、スレッド投稿の実装
- テストモード対応（IsTestMode フラグ）

#### GitHub Webhook検証 (main.go:55-57)
- `github.ValidatePayload` で署名検証
- Pull Request の labeled イベントのみ処理

#### コマンドパース (handlers/command.go)
- 形式: `/slack-review-notify [ラベル名] サブコマンド [引数]`
- ラベル名省略時は "needs-review" をデフォルト使用
- 正規表現でメンションやリポジトリ名を抽出

#### エラーハンドリング
- 各サービス関数はエラーを返す
- ログ出力後、適切なHTTPステータスで応答
- データベーストランザクションは使用していない（単純な操作のみ）

### デプロイ
- バイナリビルド: `make build` または GitHub Releases
- Docker: `ghcr.io/haruotsu/slack-review-notify:latest`
- ポート: 8080（ハードコード）