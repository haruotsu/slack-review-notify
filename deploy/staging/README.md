# ステージング環境デプロイ手順

このディレクトリには、slack-review-notifyアプリケーションをステージング環境にデプロイするための設定ファイルが含まれています。

## 前提条件

- Docker および Docker Compose がインストールされていること
- Slack App が作成され、以下の設定が完了していること：
  - Bot Token Scopes の設定
  - Event Subscriptions の有効化
  - Slash Commands の設定
  - Interactive Components の設定
- GitHub Webhook の設定（オプション）
- SSL証明書（HTTPS使用時）

## 構成

### ファイル一覧

- `docker-compose.yml`: Docker Compose設定ファイル
- `.env.example`: 環境変数のテンプレート
- `nginx.conf`: Nginxリバースプロキシ設定
- `README.md`: このファイル

### サービス構成

1. **app**: slack-review-notifyアプリケーション本体
   - ポート: 8080
   - データベース: SQLite（永続化ボリューム）
   - ヘルスチェック: `/health` エンドポイント

2. **nginx**: リバースプロキシ
   - ポート: 80 (HTTP), 443 (HTTPS)
   - SSL終端
   - キャッシュ機能

3. **backup**: データベースバックアップ
   - 1日1回自動バックアップ
   - 7日以上古いバックアップを自動削除

## デプロイ手順

### 1. 初回セットアップ

```bash
# ステージング環境用ディレクトリに移動
cd deploy/staging

# 環境変数ファイルを作成
cp .env.example .env

# .env ファイルを編集して実際の値を設定
vi .env
```

### 2. 環境変数の設定

`.env` ファイルに以下の必須項目を設定してください：

```bash
# Slack設定（必須）
SLACK_BOT_TOKEN=xoxb-xxxxxxxxxxxx-xxxxxxxxxxxx-xxxxxxxxxxxxxxxxxxxxxxxx
SLACK_SIGNING_SECRET=xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

# GitHub設定（オプション、Webhook使用時）
GITHUB_WEBHOOK_SECRET=your-webhook-secret

# ログレベル（推奨: info または debug）
LOG_LEVEL=info
```

### 3. SSL証明書の設置（HTTPS使用時）

```bash
# SSL証明書ディレクトリを作成
mkdir -p ssl

# 証明書ファイルを配置
# - ssl/cert.pem: SSL証明書
# - ssl/key.pem: 秘密鍵
```

Let's Encryptを使用する場合:

```bash
# certbotで証明書を取得
sudo certbot certonly --standalone -d staging.example.com

# 証明書をコピー
sudo cp /etc/letsencrypt/live/staging.example.com/fullchain.pem ssl/cert.pem
sudo cp /etc/letsencrypt/live/staging.example.com/privkey.pem ssl/key.pem
sudo chmod 644 ssl/cert.pem
sudo chmod 600 ssl/key.pem
```

### 4. Nginx設定の調整

`nginx.conf` ファイルでドメイン名を編集:

```nginx
server_name staging.example.com;
```

### 5. アプリケーションの起動

```bash
# バックグラウンドで起動
docker-compose up -d

# ログを確認
docker-compose logs -f app

# ステータス確認
docker-compose ps
```

### 6. ヘルスチェック

```bash
# ローカルからヘルスチェック
curl http://localhost:8080/health

# 外部からのアクセス確認（HTTPSの場合）
curl https://staging.example.com/health
```

### 7. Slack/GitHub Webhook URLの設定

アプリケーションが起動したら、以下のURLを各サービスに設定してください：

- **Slack Event Subscriptions**: `https://staging.example.com/slack/events`
- **Slack Slash Commands**: `https://staging.example.com/slack/command`
- **Slack Interactive Components**: `https://staging.example.com/slack/interactive`
- **GitHub Webhooks**: `https://staging.example.com/webhook`

## 運用

### アプリケーションの更新

```bash
# 最新イメージを取得
docker-compose pull

# コンテナを再作成
docker-compose up -d

# ログを確認
docker-compose logs -f app
```

### ログの確認

```bash
# 全サービスのログ
docker-compose logs -f

# アプリケーションのログのみ
docker-compose logs -f app

# Nginxのログのみ
docker-compose logs -f nginx

# 最新100行のみ表示
docker-compose logs --tail=100 app
```

### データベースのバックアップ

自動バックアップは1日1回実行されますが、手動でもバックアップ可能です：

```bash
# バックアップディレクトリを確認
ls -lh backups/

# 手動バックアップ
docker-compose exec app cp /app/data/staging_review_tasks.db /app/data/backup_$(date +%Y%m%d_%H%M%S).db

# バックアップファイルをホストにコピー
docker cp slack-review-notify-staging:/app/data/backup_*.db ./backups/
```

### データベースのリストア

```bash
# アプリケーションを停止
docker-compose stop app

# バックアップからリストア
docker-compose run --rm -v $(pwd)/backups:/backups app sh -c \
  "cp /backups/staging_review_tasks_YYYYMMDD_HHMMSS.db /app/data/staging_review_tasks.db"

# アプリケーションを再起動
docker-compose start app
```

### アプリケーションの停止

```bash
# 停止（コンテナは削除されない）
docker-compose stop

# 停止してコンテナを削除（データは保持）
docker-compose down

# 停止してコンテナとボリュームを削除（データも削除）
docker-compose down -v
```

### リソース使用状況の確認

```bash
# コンテナのリソース使用状況
docker stats slack-review-notify-staging

# ディスク使用量
docker system df

# ボリュームの使用量
docker volume ls
docker volume inspect staging_staging-db-data
```

## トラブルシューティング

### アプリケーションが起動しない

```bash
# コンテナの状態を確認
docker-compose ps

# ログを確認
docker-compose logs app

# ヘルスチェックの状態を確認
docker inspect slack-review-notify-staging | grep -A 10 Health
```

### Slack/GitHubからのWebhookが届かない

1. ファイアウォール設定を確認
2. Nginx設定を確認（`nginx.conf`）
3. Slackイベントサブスクリプションの検証が完了しているか確認
4. GitHub Webhookの配信履歴を確認

```bash
# Nginxのアクセスログを確認
docker-compose logs nginx | grep POST

# アプリケーションのログを確認
docker-compose logs app | grep webhook
```

### データベースが破損した場合

```bash
# 最新のバックアップからリストア
docker-compose stop app
docker-compose run --rm -v $(pwd)/backups:/backups app sh -c \
  "cp /backups/staging_review_tasks_LATEST.db /app/data/staging_review_tasks.db"
docker-compose start app
```

### SSL証明書の更新

```bash
# Let's Encrypt証明書の更新
sudo certbot renew

# 証明書を再コピー
sudo cp /etc/letsencrypt/live/staging.example.com/fullchain.pem ssl/cert.pem
sudo cp /etc/letsencrypt/live/staging.example.com/privkey.pem ssl/key.pem

# Nginxをリロード
docker-compose exec nginx nginx -s reload
```

## モニタリング

### ヘルスチェック

```bash
# 定期的にヘルスチェックを実行
watch -n 10 'curl -s http://localhost:8080/health | jq'
```

### パフォーマンスモニタリング

```bash
# CPU/メモリ使用率を監視
docker stats --no-stream slack-review-notify-staging

# ネットワークトラフィック
docker stats --no-stream --format "table {{.Name}}\t{{.NetIO}}" slack-review-notify-staging
```

## セキュリティ

### 推奨事項

1. `.env` ファイルをバージョン管理に含めない（`.gitignore`に追加済み）
2. SSL/TLSを必ず使用する（HTTPS）
3. ファイアウォールで不要なポートを閉じる
4. 定期的にDockerイメージを更新する
5. ログを定期的に確認し、異常なアクセスをチェック

### アクセス制限

Nginxで特定のIPアドレスのみ許可する場合:

```nginx
# nginx.conf に追加
location / {
    allow 192.168.1.0/24;
    allow 10.0.0.0/8;
    deny all;
    proxy_pass http://app:8080;
}
```

## 参考情報

- [Dockerドキュメント](https://docs.docker.com/)
- [Docker Composeドキュメント](https://docs.docker.com/compose/)
- [Nginxドキュメント](https://nginx.org/en/docs/)
- [Let's Encryptドキュメント](https://letsencrypt.org/docs/)
- [slack-review-notify リポジトリ](https://github.com/haruotsu/slack-review-notify)

## サポート

問題が発生した場合は、GitHubリポジトリのIssueを作成してください。
