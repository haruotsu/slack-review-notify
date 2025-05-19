# slack-review-notify
<p align="center">
<img src="images/slack-review-notify-logo.png" width="40%">
</p>

**GitHub PRのレビュー依頼をラベルによってSlackに自動通知し、レビュワーのアサインおよびレビューのリマインドを行うツールです**。

## 機能
- slackコマンドによって、すべての設定を変更できます。
- PRにラベルが付けられると、設定されたSlackチャンネルに通知が送信されます。
- レビュワーをランダムに選択します。
- レビューが完了ボタンが押されるまで、定期的にリマインドします。
- リマインダの一時停止が可能です。

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
- Let me select individual eventsにチェックを入れる: Pull requestsにチェックを入れる

### 環境変数
`.env`ファイルを作成し、以下の値を設定します:
```
SLACK_BOT_TOKEN=xoxb-your-slack-bot-token
GITHUB_WEBHOOK_SECRET=your-github-webhook-secret
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

- `/slack-review-notify show`: 現在の設定を表示
- `/slack-review-notify set-mention @user`: メンション先を設定
- `/slack-review-notify add-reviewer @user1, @user2`: レビュワーを追加
- `/slack-review-notify show-reviewers`: 登録済みのレビュワーリストを表示
- `/slack-review-notify clear-reviewers`: レビュワーリストをクリア
- `/slack-review-notify add-repo owner/repo`: 通知対象リポジトリを追加
- `/slack-review-notify remove-repo owner/repo`: 通知対象リポジトリを削除
- `/slack-review-notify set-label label-name`: 通知対象ラベルを設定
- `/slack-review-notify set-reviewer-reminder-interval 30`: レビュー中のリマインド間隔（分）
- `/slack-review-notify activate`: 通知を有効化
- `/slack-review-notify deactivate`: 通知を無効化
- `/slack-review-notify help`: ヘルプを表示

### レビュー管理
通知メッセージから各種アクションを実行できます:
- 「レビュー完了」: レビュー完了として記録
- 「変わってほしい！」: レビュー担当者を再抽選
- リマインド頻度変更: リマインド頻度を1時間, 2時間, 4時間, 今日は通知しない (翌営業日の朝まで停止), 通知しないのパターンで変更


## 開発

### ローカルでの開発方法
```bash
# 依存関係のインストール
make deps

# 開発サーバーの実行
make dev

# テストの実行
make test

# lintの実行
make lint
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
