.PHONY: build test lint format generate-sql

VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo unknown)
LDFLAGS  = -X main.version=$(VERSION)

generate-sql:
	@go generate ./internal/repository

build: generate-sql
	@go build -ldflags "$(LDFLAGS)" -o bin/app ./cmd/app

test:
	@go test ./...

lint:
	@golangci-lint run ./...

format:
	@find . -name "*.go" -not -path "./vendor/*" | xargs grep -rL "DO NOT EDIT" | xargs goimports -w
