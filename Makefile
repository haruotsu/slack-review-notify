.PHONY: build run test clean all help lint lint-install deps dev slackhog up down

APP_NAME := slack-review-notify
GO := go

help:
	@echo "使用可能なコマンド:"
	@echo "  make build    - アプリケーションをビルドします"
	@echo "  make run      - アプリケーションを実行します"
	@echo "  make test     - テストを実行します"
	@echo "  make clean    - ビルド成果物を削除します"
	@echo "  make all      - cleanとビルドを実行します"
	@echo "  make lint     - lintを実行します"
	@echo "  make lint-install - golangci-lintをインストールします"
	@echo "  make slackhog - SlackHogを起動します（開発用Slackモック）"
	@echo "  make up       - docker composeで開発環境を起動します"
	@echo "  make down     - docker composeで開発環境を停止します"
	@echo "  make help     - このヘルプメッセージを表示します"

run: build
	./$(APP_NAME)

test:
	$(GO) test ./...

test-coverage:
	$(GO) test -coverprofile=coverage.out ./...

clean:
	rm -f $(APP_NAME)
	rm -f *.db

all: clean build

lint:
	golangci-lint run

lint-install:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin latest

deps:
	$(GO) mod download

dev:
	$(GO) run main.go

slackhog:
	docker run --rm -p 4112:4112 ghcr.io/harakeishi/slackhog

up:
	docker compose up --build -d

down:
	docker compose down
