# slack-review-notify
<p align="center">
<img src="images/slack-review-notify-logo.png" width="40%">
</p>

**GitHub PRのレビュー依頼をラベルによってSlackに自動通知し、レビュワーのアサインおよびレビューのリマインドを行うツールです**。

## 機能
- **Slackコマンドによる設定管理**: すべての設定をSlackから変更可能
- **自動通知**: PRにラベルが付けられると、設定されたSlackチャンネルに通知
- **PR作成者へのメンション**: PR作成者もメンションされるため、スレッドの会話通知を受け取れる
- **カスタマイズ可能な営業時間**: チャンネルごとに営業時間を個別設定、深夜営業にも対応
- **タイムゾーン対応**: グローバルチームでの運用を支援（JST、UTC、その他多数対応）
- **営業時間外待機機能**: 営業時間外にPRにラベルが付けられた場合、営業時間内まで待機してから通知
- **ランダムレビュワー選択**: 設定されたレビュワーリストからランダムに選択
- **定期リマインド**: レビューが完了するまで、設定した頻度でリマインド
- **営業時間外リマインド制御**: 営業時間外は1回のみリマインド、2回目以降は翌営業日まで待機
- **リマインダー一時停止**: 事前設定可能な複数の時間間隔でリマインドを一時停止
- **🎉 自動レビュー完了検知 & 感謝メッセージ**: GitHubでレビューが行われると自動的に感謝メッセージ（`感謝！👏`）を投稿
- **レビュワー変更**: 「変わってほしい！」ボタンで簡単にレビュワーを再抽選

## 実際の様子
<p align="center">
<img src="images/slack-review-notify-demo.png" width="100%">
</p>

## 設定方法
### GitHubの設定
GitHubのリポジトリのSettingsのWebhooksから、以下を設定してください:
- Payload URL: https://<あなたのドメイン>/webhook
- Content type: application/json
- Secret: 空やお好きな文字列
- Enable SSL verification: チェックを入れる
- Let me select individual eventsにチェックを入れて、以下のイベントを有効化:
  - **Pull requests**: PRのラベル付け/削除を検知
  - **Pull request reviews**: レビューの承認/変更要求/コメントを検知（自動完了通知機能）

### 環境変数
`.env`ファイルを作成し、以下の値を設定します:
```
SLACK_BOT_TOKEN=xoxb-your-slack-bot-token
SLACK_SIGNING_SECRET=your-slack-signing-secret
GITHUB_WEBHOOK_SECRET=your-github-webhook-secret
DB_PATH=review_tasks.db  # デフォルト: review_tasks.db（省略可能）
```

## 検証例
ローカルサーバーを立てて、検証する方法を以下に記載しました。
[/docs/example_usage.md](./docs/example_usage.md)

上記はngrokを用いた方法で説明していますが、k8sやAWS EC2などお好きな環境にデプロイして、実際に使ってみてください。slack appのmanifest.jsonのsampleもおいてあります。


## 使い方
### Botをチャンネルに追加
```
/invite @review-notify-bot
```

### 通知設定
コマンド一覧 (**この設定は、`/slack-review-notify help`で確認できます。**)

コマンド形式: `/slack-review-notify [ラベル名] サブコマンド [引数]`

- `/slack-review-notify help`: コマンドのヘルプを表示
- `/slack-review-notify show`: そのチャンネルで設定されているラベルの一覧を表示
**[ラベル名]を省略すると「needs-review」というデフォルトのラベルを使用します**

- `/slack-review-notify [ラベル名] show`: 指定したラベルの設定を表示
- `/slack-review-notify [ラベル名] set-mention @user`: メンション先を設定
- `/slack-review-notify [ラベル名] add-reviewer @user1,@user2`: レビュワーを追加
- `/slack-review-notify [ラベル名] show-reviewers`: 登録されたレビュワーリストを表示
- `/slack-review-notify [ラベル名] clear-reviewers`: レビュワーリストをクリア
- `/slack-review-notify [ラベル名] add-repo owner/repo`: 通知対象リポジトリを追加
- `/slack-review-notify [ラベル名] remove-repo owner/repo`: 通知対象リポジトリを削除
- `/slack-review-notify [ラベル名] set-label 新ラベル名`: ラベル名を変更
- `/slack-review-notify [ラベル名] set-reviewer-reminder-interval 30`: レビュワー割り当て後のリマインド頻度を設定（分単位）
- `/slack-review-notify [ラベル名] set-business-hours-start 09:00`: 営業時間の開始時刻を設定（HH:MM形式）
- `/slack-review-notify [ラベル名] set-business-hours-end 18:00`: 営業時間の終了時刻を設定（HH:MM形式）
- `/slack-review-notify [ラベル名] set-timezone Asia/Tokyo`: タイムゾーンを設定（例: `Asia/Tokyo`, `UTC`, `America/New_York`）
- `/slack-review-notify [ラベル名] activate`: このラベルの通知を有効化
- `/slack-review-notify [ラベル名] deactivate`: このラベルの通知を無効化

### ユーザーマッピング（PR作成者の通知用）
GitHubユーザーとSlackユーザーを紐付けることで、PR作成者にもメンションが届き、スレッド通知を受け取れます。

- `/slack-review-notify map-user <github-username> @slack-user`: GitHubユーザーとSlackユーザーを紐付け
- `/slack-review-notify show-user-mappings`: 登録済みのユーザーマッピング一覧を表示
- `/slack-review-notify remove-user-mapping <github-username>`: ユーザーマッピングを削除

**例:**
```bash
# 自分のGitHubユーザー名とSlackアカウントを紐付け
/slack-review-notify map-user octocat @john

# マッピング一覧を確認
/slack-review-notify show-user-mappings

# マッピングを削除
/slack-review-notify remove-user-mapping octocat
```

マッピングを設定すると、PRにラベルが付けられた際に以下のように表示されます：
```
@team @john からのレビュー依頼があります

*PRタイトル*: hogehoge API の実装
*URL*: https://github.com/owner/repo/pull/123
```

### レビュー管理
通知メッセージから各種アクションを実行できます:

#### 🎉 自動レビュー完了検知
GitHubでレビューが行われると、自動的に感謝メッセージがスレッドに投稿されます：
- 承認時: `✅ reviewerさんがレビューを承認しました！感謝！👏`
- 変更要求時: `🔄 reviewerさんが変更を要求しました 感謝！👏`
- コメント時: `💬 reviewerさんがレビューコメントを残しました 感謝！👏`

#### 📅 営業時間外待機機能
営業時間外にPRにラベルが付けられた場合：
- 即座に通知せず、営業時間内まで待機
- 営業時間になると自動的にレビュワーをアサインして通知
- チームメンバーに迷惑をかけることなく適切な時間にレビュー依頼が可能

**営業時間の設定**
- デフォルト: 平日9:00-18:00（JST）
- チャンネルごとに個別に設定可能
- 深夜営業（日をまたぐ時間）にも対応（例: 22:00-06:00）
- タイムゾーン設定により、グローバルチームでの運用にも対応

#### 📱 手動操作
- 「レビュー完了」ボタン: 手動でレビュー完了として記録（`✅ <@user> さんがレビューを完了しました！感謝！👏`）
- 「変わってほしい！」ボタン: レビュー担当者を再抽選
- 初回リマインダー一時停止: レビュワー割り当て時に事前にリマインダーを一時停止可能
- リマインダー一時停止: 1時間, 2時間, 4時間, 今日は通知しない (翌営業日の朝まで停止), 完全停止のパターンで変更

### 設定例
#### ユーザーマッピングの設定（推奨）
PR作成者にもスレッド通知が届くようにするため、最初にユーザーマッピングを設定することをおすすめします。

```bash
# チームメンバーのGitHubとSlackアカウントを紐付け
/slack-review-notify map-user alice @alice
/slack-review-notify map-user bob @bob
/slack-review-notify map-user charlie @charlie

# マッピングの確認
/slack-review-notify show-user-mappings
```

#### 営業時間とタイムゾーンの設定
```bash
# 営業時間を9:00-18:00に設定
/slack-review-notify set-business-hours-start 09:00
/slack-review-notify set-business-hours-end 18:00

# タイムゾーンを日本に設定
/slack-review-notify set-timezone Asia/Tokyo

# 深夜営業チーム（22:00-06:00）の場合
/slack-review-notify night-shift set-business-hours-start 22:00
/slack-review-notify night-shift set-business-hours-end 06:00
/slack-review-notify night-shift set-timezone Asia/Tokyo

# アメリカチームの場合
/slack-review-notify us-team set-business-hours-start 09:00
/slack-review-notify us-team set-business-hours-end 17:00
/slack-review-notify us-team set-timezone America/New_York

# 設定確認
/slack-review-notify show
```

#### 基本的な通知設定
```bash
# needs-reviewラベル用の設定
/slack-review-notify add-repo owner/repository
/slack-review-notify set-mention @team-lead
/slack-review-notify add-reviewer @reviewer1,@reviewer2,@reviewer3

# securityラベル用の設定
/slack-review-notify security add-repo owner/repository
/slack-review-notify security set-mention @security-team
/slack-review-notify security add-reviewer @security-expert1,@security-expert2
```

## 開発

### ローカルでの開発方法
```bash
# 依存関係のインストール
make deps

# 開発サーバーの実行（ホットリロード）
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

ポートは8080です。

## 統合テスト

### 統合テストの概要
本プロジェクトでは、実際のコンポーネント間の連携を検証するための統合テストを提供しています。統合テストは以下をカバーします：

- **Webhookハンドリング**: GitHubからのWebhookを受信してSlackに通知
- **Slackコマンド処理**: `/slack-review-notify`コマンドによる設定管理
- **Slackインタラクション**: ボタンクリックなどのインタラクティブ要素
- **リマインダー機能**: 定期的なレビューリマインドの動作
- **エンドツーエンドシナリオ**: 実際の使用フローを模したシナリオテスト

### テスト環境のセットアップ

統合テストは隔離されたDocker Compose環境で実行されます。以下のコンポーネントが自動的にセットアップされます：

- **アプリケーションコンテナ**: テスト対象のslack-review-notifyアプリ（ポート8080）
- **モックサーバー**: Slack/GitHub APIのモック用（ポート1080）
- **テスト用データベース**: 永続化ボリュームを使用したSQLiteデータベース

#### 必要な環境変数

統合テストに必要な環境変数は以下の通りです（すべてオプション、未設定時はデフォルト値を使用）：

```bash
# Slack設定（デフォルト: xoxb-test-token）
export SLACK_BOT_TOKEN=xoxb-test-token

# Slack署名検証用シークレット（デフォルト: test-signing-secret）
export SLACK_SIGNING_SECRET=test-signing-secret

# GitHub Webhook署名検証（テストモードでは空にして署名検証をスキップ）
export GITHUB_WEBHOOK_SECRET=

# テスト用データベースパス（デフォルト: test_integration.db）
export DB_PATH=test_integration.db

# テストモード有効化（デフォルト: true）
export TEST_MODE=true
```

**重要**: テストモードでは`GITHUB_WEBHOOK_SECRET`を空にすることで、GitHub Webhookの署名検証をスキップします。これにより、実際のGitHubシークレットなしでテストを実行できます。

### テスト実行方法

#### 統合テストスクリプトを使用する（推奨）

最も簡単な方法は、専用スクリプトを使用することです：

```bash
# 統合テストを自動実行
./scripts/run-integration-tests.sh
```

このスクリプトは以下を自動的に実行します：

1. **環境変数チェック**: 必要な環境変数の確認とデフォルト値の設定
2. **テスト用DBセットアップ**: 既存のテストDBを削除して新規作成
3. **Docker Composeサービス起動**: アプリケーションとモックサーバーを起動
4. **ヘルスチェック**: アプリケーションが正常に起動するまで待機
5. **ユニットテスト実行**: `go test ./...`でユニットテストを実行
6. **統合テスト実行**: `tests/integration`ディレクトリの統合テストを実行
7. **カバレッジレポート生成**: テストカバレッジを集計して表示
8. **自動クリーンアップ**: テスト後にDocker Composeサービスを停止・削除

#### 手動でテスト環境を起動する

Docker Compose環境を手動で管理したい場合：

```bash
# テスト環境の起動
docker-compose -f docker-compose.test.yml --env-file .env.test up -d

# ログの確認
docker-compose -f docker-compose.test.yml logs -f app

# ヘルスチェックの確認
docker-compose -f docker-compose.test.yml ps

# 統合テストの実行
go test -v ./tests/integration/...

# テスト環境の停止
docker-compose -f docker-compose.test.yml down

# テスト環境の完全削除（ボリュームも削除）
docker-compose -f docker-compose.test.yml down -v
```

#### 個別のテストケースを実行する

特定のテストケースのみを実行する場合：

```bash
# 特定のパッケージのテスト
go test -v ./tests/integration/

# 特定のテスト関数を実行
go test -v ./tests/integration/ -run TestWebhookHandler

# レースコンディションの検出を有効化
go test -v -race ./tests/integration/

# カバレッジ付きで実行
go test -v -coverprofile=coverage.out ./tests/integration/
```

### テストデータとフィクスチャ

統合テストでは、`tests/integration/fixtures`ディレクトリにテストデータ（フィクスチャ）を格納しています：

- **channel_configs.json**: チャンネル設定のサンプルデータ
- **review_tasks.json**: レビュータスクのサンプルデータ
- **webhook_payloads.json**: GitHub Webhookペイロードのサンプル

フィクスチャの詳細については、[tests/integration/fixtures/README.md](./tests/integration/fixtures/README.md)を参照してください。

### CI/CDでの統合テスト

GitHub Actionsを使用した自動統合テストが設定されています。

**トリガー条件:**
- `staging` ブランチへのプッシュ
- `staging` ブランチへのPull Request
- 手動実行（workflow_dispatch）

**実行内容:**
1. **統合テスト**: Docker Compose環境での自動テスト実行
2. **Lint**: golangci-lintによるコード品質チェック
3. **ビルド**: バイナリのビルド確認
4. **Dockerビルド**: Dockerイメージのビルド確認

**必要なシークレット設定（GitHub Settings > Secrets）:**
- `SLACK_BOT_TOKEN_TEST`: テスト用Slack Bot Token（オプション、未設定時はモック値を使用）
- `SLACK_SIGNING_SECRET_TEST`: テスト用Slack Signing Secret（オプション）
- `GITHUB_WEBHOOK_SECRET_TEST`: テスト用GitHub Webhook Secret（オプション）

詳細は `.github/workflows/integration-test.yml` を参照してください。

### トラブルシューティング

#### アプリケーションが起動しない

**症状**: `docker-compose up`後にアプリケーションが起動しない

**確認事項**:
```bash
# コンテナのログを確認
docker-compose -f docker-compose.test.yml logs app

# コンテナのステータスを確認
docker-compose -f docker-compose.test.yml ps

# ヘルスチェックが失敗していないか確認
docker inspect slack-review-notify-test | grep -A 10 Health
```

**対処法**:
- 環境変数が正しく設定されているか確認
- ポート8080が他のプロセスで使用されていないか確認
- データベースファイルのパーミッションを確認

#### テストが失敗する

**症状**: 統合テストが一部失敗する

**確認事項**:
```bash
# 詳細ログ付きでテストを実行
go test -v -race ./tests/integration/... 2>&1 | tee test.log

# テストDBの状態を確認
sqlite3 test_integration.db ".tables"
sqlite3 test_integration.db "SELECT * FROM channel_configs;"
```

**対処法**:
- テスト用DBが古い状態の場合は削除して再実行: `rm test_integration.db`
- Docker Composeサービスを再起動: `docker-compose -f docker-compose.test.yml restart`
- キャッシュをクリア: `go clean -testcache`

#### ポート競合エラー

**症状**: `bind: address already in use`エラーが発生

**確認事項**:
```bash
# 8080ポートを使用しているプロセスを確認
lsof -i :8080

# 1080ポートを使用しているプロセスを確認
lsof -i :1080
```

**対処法**:
- 既存のDocker Composeサービスを停止: `docker-compose -f docker-compose.test.yml down`
- 競合しているプロセスを停止
- 別のポートを使用するようにdocker-compose.test.ymlを編集

#### データベースロックエラー

**症状**: `database is locked`エラーが発生

**対処法**:
```bash
# すべてのテストプロセスを終了
pkill -f "go test"

# テストDBファイルを削除
rm -f test_integration.db*

# Docker Composeを完全にクリーンアップ
docker-compose -f docker-compose.test.yml down -v

# 再度テストを実行
./scripts/run-integration-tests.sh
```

#### モックサーバーが起動しない

**症状**: MockServerコンテナが起動しない

**確認事項**:
```bash
# モックサーバーのログを確認
docker-compose -f docker-compose.test.yml logs mock-server

# 初期化ファイルが存在するか確認
ls -la test/mock/initializer.json
```

**対処法**:
- `test/mock`ディレクトリが存在するか確認
- `test/mock/initializer.json`を作成: `mkdir -p test/mock && echo '{}' > test/mock/initializer.json`
- Docker Composeサービスを再起動

#### CI/CDでテストが失敗する

**症状**: ローカルでは成功するがCI/CDで失敗する

**確認事項**:
- GitHub Secretsが正しく設定されているか確認
- CI/CDログで具体的なエラーメッセージを確認
- タイムアウト設定が十分か確認

**対処法**:
- ローカルでCI/CDと同じ環境変数を使用してテスト
- テストのタイムアウト時間を延長
- GitHub Actionsのジョブログを詳細に確認


### デプロイ
アプリをk8sやAWS EC2などお好きな環境で実行してください。

#### リリースワークフロー (タグ作成時に実行)
`v*` の形式でタグを作成すると、自動的に以下のプラットフォーム向けのバイナリがビルドされリリースされます:
- Linux (amd64)
- macOS (amd64, arm64)
- Windows (amd64)

## コントリビューション
スター & PR大歓迎。大きな変更を行う場合は、issueで議論していきましょう！

## ライセンス
Apache License Version 2.0, January 2004
