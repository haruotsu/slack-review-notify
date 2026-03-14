.PHONY: build run test clean all help lint lint-install deps dev slackhog up down

APP_NAME := slack-review-notify
GO := go

help:
	@echo "Available commands:"
	@echo "  make build    - Build the application"
	@echo "  make run      - Run the application"
	@echo "  make test     - Run tests"
	@echo "  make clean    - Remove build artifacts"
	@echo "  make all      - Run clean and build"
	@echo "  make lint     - Run lint"
	@echo "  make lint-install - Install golangci-lint"
	@echo "  make slackhog - Start SlackHog (Slack mock for development)"
	@echo "  make up       - Start development environment with docker compose"
	@echo "  make down     - Stop development environment with docker compose"
	@echo "  make help     - Show this help message"

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
	docker run --rm -p 4112:4112 -v $(CURDIR)/slackhog.yaml:/etc/slackhog/slackhog.yaml:ro ghcr.io/harakeishi/slackhog -config /etc/slackhog/slackhog.yaml

up:
	docker compose up --build -d

down:
	docker compose down
