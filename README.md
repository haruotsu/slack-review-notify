# slack-review-notify
GitHub PRのレビュー依頼をSlackに自動通知するツールです。PRにラベルが付けられると、設定されたSlackチャンネルに通知が送信され、レビュアーのアサインや進捗管理を簡単に行えます。

## 機能
- PRにラベルが付けられると、設定されたSlackチャンネルに通知が送信されます。
- レビュー担当者のアサインや進捗管理を簡単に行えます。
- リマインダーを送信することで、レビュー担当者の進捗を管理できます。

## 設定方法
### 環境変数
`.env`ファイルを作成し、以下の値を設定します:
```
SLACK_BOT_TOKEN=xoxb-your-slack-bot-token
GITHUB_WEBHOOK_SECRET=your-github-webhook-secret
SLACK_BOT_USER_ID=UxxxxxxXX
```

### Slackアプリ設定
- Slack APIサイトで新しいアプリを作成
- 以下の権限を追加:
  - chat:write
  - channels:read
  - commands
- スラッシュコマンドを追加: /slack-review-notify
- イベント購読を有効化: member_joined_channel
- アプリをワークスペースにインストール

### GitHub Webhook設定
- リポジトリまたは組織に新しいWebhookを追加:
  - ペイロードURL: https://あなたのサーバーのドメイン/webhook
  - コンテンツタイプ: application/json
  - シークレット: .envのGITHUB_WEBHOOK_SECRETと同じ値
  - イベント: Pull requests

## 使い方
### Botをチャンネルに追加
```
/invite @review-notify-bot
```

### 通知設定
以下のコマンドでチャンネルごとに設定を行います:
```
# 現在の設定を表示
/slack-review-notify show

# メンション先を設定
/slack-review-notify set-mention @username

# 監視するリポジトリを追加
/slack-review-notify add-repo owner/repo

# 監視するリポジトリを削除
/slack-review-notify remove-repo owner/repo

# 通知対象のラベルを設定 (デフォルト: needs-review)
/slack-review-notify set-label needs-review

# 通知を有効化/無効化
/slack-review-notify activate
/slack-review-notify deactivate
```

### レビュー管理
通知メッセージから各種アクションを実行できます:
- 「レビューします！」: レビュー担当者としてアサイン
- 「レビュー完了」: レビュー完了として記録
- リマインダー機能: 未アサインのPRは30分ごと、レビュー中は1時間ごとにリマインダー

## ライセンス
MIT License

## コントリビューション
PR大歓迎です！大きな変更を行う場合は、issueで議論していきましょう！
