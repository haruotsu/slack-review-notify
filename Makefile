.PHONY: build run test clean all help lint lint-install deps dev

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
