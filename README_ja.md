# slack-review-notify

**GitHub PRのレビュー依頼をラベルによってSlackに自動通知し、レビュワーのアサインおよびレビューのリマインドを行うツールです。**

> **[日本語](./README_ja.md)** | **[English](./README_en.md)**

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
- **自動レビュー完了検知 & 感謝メッセージ**: GitHubでレビューが行われると自動的に感謝メッセージを投稿
- **レビュワー変更**: 「変わってほしい！」ボタンで簡単にレビュワーを再抽選
- **i18n対応**: チャンネルごとの言語設定（日本語 / 英語）

## 実際の様子

### Slackでの通知
<p align="center">
<img src="images/slack-review-notify-demo.png" width="100%">
</p>

### レビュー承認フロー (スレッド表示)
![](docs/screenshots/05-ja-review-fully-approved.png)

### PRマージ通知 (スレッド表示)
![](docs/screenshots/09-ja-pr-merged.png)

<details>
<summary>その他のスクリーンショット</summary>

#### 営業時間外の通知
![](docs/screenshots/03-ja-off-hours.png)

#### ラベル削除によるタスク完了
![](docs/screenshots/11-ja-label-removed.png)

#### リマインダー + 一時停止ドロップダウン
![](docs/screenshots/12-ja-reminder.png)

#### 英語チャンネルの表示例
![](docs/screenshots/08-en-re-review.png)

</details>

## 設定方法
### GitHubの設定
GitHubのリポジトリのSettingsのWebhooksから、以下を設定してください:
- Payload URL: `https://<あなたのドメイン>/webhook`
- Content type: `application/json`
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
- `/slack-review-notify [ラベル名] set-required-approvals N`: 必要なapprove数を設定（1〜10）
- `/slack-review-notify [ラベル名] set-language ja|en`: メッセージの言語を設定
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

### 休暇管理
- `/slack-review-notify set-away @user [until YYYY-MM-DD] [reason 理由]`: ユーザーを休暇に設定
- `/slack-review-notify unset-away @user`: ユーザーの休暇を解除
- `/slack-review-notify show-availability`: 休暇中のユーザー一覧を表示

### レビュー管理
通知メッセージから各種アクションを実行できます:

#### 自動レビュー完了検知
GitHubでレビューが行われると、自動的に感謝メッセージがスレッドに投稿されます：
- 承認時: `✅ reviewerさんがレビューを承認しました！感謝！👏`
- 変更要求時: `🔄 reviewerさんが変更を要求しました 感謝！👏`
- コメント時: `💬 reviewerさんがレビューコメントを残しました 感謝！👏`

#### 営業時間外待機機能
営業時間外にPRにラベルが付けられた場合：
- 即座に通知せず、営業時間内まで待機
- 営業時間になると自動的にレビュワーをアサインして通知
- チームメンバーに迷惑をかけることなく適切な時間にレビュー依頼が可能

**営業時間の設定**
- デフォルト: 平日9:00-18:00（JST）
- チャンネルごとに個別に設定可能
- 深夜営業（日をまたぐ時間）にも対応（例: 22:00-06:00）
- タイムゾーン設定により、グローバルチームでの運用にも対応

#### 手動操作
- 「レビュー完了」ボタン: 手動でレビュー完了として記録
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

#### 言語設定
```bash
# チャンネルの言語を英語に設定
/slack-review-notify set-language en

# チャンネルの言語を日本語に設定
/slack-review-notify set-language ja
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

### SlackHogを使った開発 (Slack APIモック)

[SlackHog](https://github.com/harakeishi/slackhog) はSlack APIのモックサーバーです。実際のSlackワークスペースなしで開発・テストができます。

#### Docker Compose（推奨）

```bash
# app + SlackHog をまとめて起動
make up

# 停止
make down
```

起動されるもの:
- **App**: `http://localhost:8080` — Slack APIの呼び出し先が自動的にSlackHogに設定される
- **SlackHog**: `http://localhost:4112` — メッセージやスレッドを確認できるWeb UI

`docker-compose.yml` により `SLACK_API_BASE_URL=http://slackhog:4112/api` が自動設定されます。

#### SlackHog単体起動（`make dev` と併用）

```bash
# ターミナル1: SlackHogを起動
make slackhog

# ターミナル2: SlackHogに向けてアプリを起動
SLACK_API_BASE_URL=http://localhost:4112/api make dev
```

#### メッセージの確認

ブラウザで `http://localhost:4112` を開きます。サイドバーのチャンネルをクリックするとメッセージを確認できます。返信バッジ（例: 「3件の返信」）をクリックするとスレッドパネルが開きます。

#### E2Eテスト

`e2e/` ディレクトリにPlaywrightベースのE2Eテストがあります:

```bash
cd e2e
npm install
npx playwright install chromium

# 先にDocker環境を起動
make up

# APIテスト（78テスト）
npx playwright test i18n-e2e.test.ts

# スクリーンショット撮影（リマインダー待ちのため約3分）
npx playwright test full-screenshot.test.ts --timeout=300000
```

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
