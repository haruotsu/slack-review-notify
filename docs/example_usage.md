## Slack Review Notifyを試してみる
ngrokという、ローカルサーバーを公開するツールを使って、Slack Review Notifyを試す方法を残します。

### ngrokの設定
#### 1. ngrokに登録してインストール
https://dashboard.ngrok.com/signup

登録すると、各種OSに応じたインストール方法が表示されるので手順にしたがってインストールします。

#### 2. ngrokを起動
今回、slack-review-notifyは8080番ポートで起動するので、ngrokも8080番ポートに向けて起動します。
```bash
ngrok http 8080
```

すると、以下のような画面になるので、`https://<ngrokのID>.ngrok-free.app`をコピーしておきます。
```bash
ngrok                                       (Ctrl+C to quit)
                                                            
👋 Goodbye tunnels, hello Agent Endpoints: https://ngrok.com
                                                            
Session Status                online                        
Account                       hoge-piyo@example.com
Version                       3.22.1                        
Region                        Japan (jp)                    
Latency                       6ms                           
Web Interface                 http://127.0.0.1:4040         
Forwarding                    https://<ngrokのID>.ngrok-free.app -> http://localhost:8080     
```

### Slack Botの設定
#### 1. Slack Appの作成
以下のリンクから、Slack Appを作成します。

https://api.slack.com/apps

作成の際は、From scratchから作成します。 App Nameは適当に入力してください。
![image](images/slack-app1.png)

#### 2. Manifest.jsonの設定
左のメニューから、Features -> AppManifestを選択して、以下のManifestをJSONにコピペしてSave Changesします。
```json
{
    "display_information": {
        "name": "slack-review-notify"
    },
    "features": {
        "bot_user": {
            "display_name": "slack-review-notify",
            "always_online": false
        },
        "slash_commands": [
            {
                "command": "/slack-review-notify",
                "url": "https://<ngrokのID>.ngrok-free.app/slack/command",
                "description": "PRレビュー通知ボットの設定",
                "usage_hint": "help",
                "should_escape": false
            }
        ]
    },
    "oauth_config": {
        "scopes": {
            "bot": [
                "channels:manage",
                "channels:read",
                "chat:write",
                "commands",
                "groups:read",
                "groups:write",
                "mpim:read",
                "channels:history",
                "groups:history"
            ]
        }
    },
    "settings": {
        "interactivity": {
            "is_enabled": true,
            "request_url": "https://<ngrokのID>.ngrok-free.app/slack/actions"
        },
        "org_deploy_enabled": false,
        "socket_mode_enabled": false,
        "token_rotation_enabled": false
    }
}
```

#### 4. Slack Botのインストール
左のメニューから、Features -> OAuth & Permissionsを選択して、Install to slack-notify-testを選択してインストールまで行います。
![image](images/slack-app2.png)

インストールが完了すると、OAuthTokensが発行されるので、それをコピーしておきます。
```
xoxb-hoge-piyo-1234567890
```


### 通知対象とするリポジトリのWebhook設定
これから通知対象とするリポジトリの設定をします。GitHubのリポジトリの設定画面から、Settings -> Webhooks -> Add webhookを選択して、以下を入力してください。
- Payload URL: `https://<ngrokのID>.ngrok-free.app/github/webhook`
- Content type: application/json
- Secret: 任意の文字列 (入力しなくてもよい)
- Enable SSL verification: チェックを入れる
- Let me select individual eventsにチェックを入れる: Pull requestsにチェックを入れる




### ローカルサーバーの設定
ngrokの画面を立てた状態で、以下のようにGitHubからリポジトリをクローンしてきます
```bash
git clone https://github.com/haruotsu/slack-review-notify.git
```

対象のリポジトリに移動して、.env.exampleをコピーして.envファイルを作成して以下の内容を記載してください。
```
SLACK_BOT_TOKEN=xoxb-hoge-piyo-1234567890 (Slack BotのOAuthTokenを記載してください)
GITHUB_WEBHOOK_SECRET=fugafuga (もしWebhookのSecretを設定している場合は、その値を記載してください)
```


その後、以下のコマンドを実行してローカルサーバーを起動します。
```bash
make dev
```

これで、アプリが使えるようになりました。

### ボットを招待
対象のチャンネルで以下のコマンドを実行するなり、@メンションで呼ぶなりで招待をします。
```
/invite @slack-review-notify
```

これにて、Slack Appの設定は完了です。

### テスト
はじめにコマンドを入力して初期設定をみてみましょう。
```
/slack-review-notify show
```

![image](images/slack-app3.png)

初期設定では、通知対象リポジトリを設定していないので、通知対象リポジトリを設定します。
```
/slack-review-notify add-repo yourname/yourrepo
```

今回はテストなので、リマインド間隔を1分にしてみましょう。
```
/slack-review-notify set-reviewer-reminder-interval 1
```

レビュワーも設定しましょうか。今回は仮置きでなんでも大丈夫です
```
/slack-review-notify add-reviewer @user1, @user2
```



なお、コマンドがわからなくなったら、`/slack-review-notify help`で確認できるので積極的に使っていってください。他にもいろいろなコマンドを準備しています。

```
/slack-review-notify help
```

![image](images/slack-app4.png)

それでは、先ほどWebhookを設定したリポジトリにPRを作成して、needs-reviewラベルを付けてみましょう! これで通知がきたら成功です。
ぜひ、いろいろな使い方を試してみてください！
![image](images/slack-review-notify-demo.png)


## まとめ

いかがだったでしょうか！AIの登場でコード生成が加速する現代のチームに特に役立つツールをつくってみました。今後もしかすると、AIツールによる自動mergeやレビューの定量指標を具体化することで、こういった悩みは解決されるかもしれませんが、まずは簡単なところから、という気持ちで実装してみました。

botが自動でリマインドする**Slack Review Notify**というツール、ぜひ使ってみて感想を教えてください。こうしたらもっとよくなる、などじゃんじゃんお待ちしています。

リポジトリのスターも押してくれると嬉しいです！
https://github.com/haruotsu/slack-review-notify
