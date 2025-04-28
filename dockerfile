FROM golang:1.24-alpine AS builder

WORKDIR /app

# 必要なビルドツールとSQLite開発ライブラリをインストール
RUN apk add --no-cache gcc musl-dev sqlite-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO_ENABLED=1 に変更
RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o slack-review-notify

# 実行用イメージ
FROM alpine:latest

# 必要なライブラリをインストール（SQLite実行に必要）
RUN apk --no-cache add ca-certificates sqlite

WORKDIR /app

COPY --from=builder /app/slack-review-notify .

EXPOSE 8080

CMD ["./slack-review-notify"]
