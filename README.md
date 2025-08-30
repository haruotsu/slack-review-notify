# slack-review-notify
<p align="center">
<img src="images/slack-review-notify-logo.png" width="40%">
</p>

**GitHub PRのレビュー依頼をラベルによってSlackに自動通知し、レビュワーのアサインおよびレビューのリマインドを行うツールです**。

## 機能
- **Slackコマンドによる設定管理**: すべての設定をSlackから変更可能
- **自動通知**: PRにラベルが付けられると、設定されたSlackチャンネルに通知
- **営業時間外待機機能**: 営業時間外（平日10:00-18:59以外）にPRにラベルが付けられた場合、営業時間内まで待機してから通知
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
- `/slack-review-notify [ラベル名] activate`: このラベルの通知を有効化
- `/slack-review-notify [ラベル名] deactivate`: このラベルの通知を無効化

### レビュー管理
通知メッセージから各種アクションを実行できます:

#### 🎉 自動レビュー完了検知
GitHubでレビューが行われると、自動的に感謝メッセージがスレッドに投稿されます：
- 承認時: `✅ reviewerさんがレビューを承認しました！感謝！👏`
- 変更要求時: `🔄 reviewerさんが変更を要求しました 感謝！👏`
- コメント時: `💬 reviewerさんがレビューコメントを残しました 感謝！👏`

#### 📅 営業時間外待機機能
営業時間外（平日10:00-18:59以外の土日・夜間）にPRにラベルが付けられた場合：
- 即座に通知せず、営業時間内まで待機
- 営業時間になると自動的にレビュワーをアサインして通知
- チームメンバーに迷惑をかけることなく適切な時間にレビュー依頼が可能

#### 📱 手動操作
- 「レビュー完了」ボタン: 手動でレビュー完了として記録（`✅ <@user> さんがレビューを完了しました！感謝！👏`）
- 「変わってほしい！」ボタン: レビュー担当者を再抽選
- 初回リマインダー一時停止: レビュワー割り当て時に事前にリマインダーを一時停止可能
- リマインダー一時停止: 1時間, 2時間, 4時間, 今日は通知しない (翌営業日の朝まで停止), 完全停止のパターンで変更


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
