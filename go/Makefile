# URP CLI Makefile
# Usage: make [target]

BINARY := urp
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.1.0")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

.PHONY: all build test lint clean install smoke release docker help

# Default target
all: build

## Build targets

build: ## Build the binary
	go build $(LDFLAGS) -o $(BINARY) ./cmd/urp

build-static: ## Build static binary (for containers)
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BINARY) ./cmd/urp

## Test targets

test: ## Run all tests
	go test ./... -v

test-short: ## Run tests (skip slow ones)
	go test ./... -short

test-cover: ## Run tests with coverage
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

smoke: build ## Run smoke tests
	./scripts/smoke-test.sh --quick

smoke-full: build ## Run full smoke tests
	./scripts/smoke-test.sh

## Code quality

lint: ## Run linters
	@command -v golangci-lint >/dev/null 2>&1 || { echo "Install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; exit 1; }
	golangci-lint run ./...

fmt: ## Format code
	go fmt ./...
	goimports -w .

vet: ## Run go vet
	go vet ./...

## Cleanup

clean: ## Remove build artifacts
	rm -f $(BINARY) coverage.out coverage.html
	go clean -cache -testcache

## Installation

install: build ## Install to GOPATH/bin
	go install ./cmd/urp

## Docker targets

docker-minimal: ## Build minimal Docker image
	docker build --target minimal -t urp:minimal .

docker-worker: ## Build worker Docker image
	docker build --target worker -t urp:worker .

docker-master: ## Build master Docker image
	docker build --target master -t urp:master .

docker-all: docker-minimal docker-worker docker-master ## Build all Docker images

## Release

release: clean test lint build-static ## Prepare for release
	@echo "Binary: $(BINARY)"
	@echo "Version: $(VERSION)"
	@ls -lh $(BINARY)

## Development

dev: build ## Build and show version
	./$(BINARY) version

watch: ## Watch for changes and rebuild (requires entr)
	@command -v entr >/dev/null 2>&1 || { echo "Install entr: sudo dnf install entr"; exit 1; }
	find . -name '*.go' | entr -c make build

## Utilities

deps: ## Download dependencies
	go mod download
	go mod tidy

update-deps: ## Update dependencies
	go get -u ./...
	go mod tidy

## Help

help: ## Show this help
	@echo "URP CLI Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
