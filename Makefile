.PHONY: setup fmt vet test lint build

## First-time setup: install git hooks
setup:
	git config core.hooksPath .githooks
	@echo "Git hooks installed (.githooks/pre-commit)"

## Format all Go files
fmt:
	gofmt -w .

## Run go vet
vet:
	go vet ./...

## Run tests
test:
	go test ./...

## Run tests with coverage
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

## Run golangci-lint
lint:
	golangci-lint run

## Build binary
build:
	go build -o composed .
