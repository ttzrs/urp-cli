# URP CLI Makefile
# Usage: make [target]

BINARY := urp
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.1.0")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

.PHONY: all build test lint clean install smoke release docker help preflight

# Default target
all: build

## Build targets

build: ## Build the binary
	go build $(LDFLAGS) -o $(BINARY) ./cmd/urp

build-static: ## Build static binary (for containers)
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(BINARY) ./cmd/urp

## Preflight checks

preflight: ## Check all prerequisites (Docker + Memgraph)
	@echo "=== URP Preflight Check ==="
	@echo -n "Docker: " && docker info > /dev/null 2>&1 && echo "OK" || (echo "FAIL - Docker not running" && exit 1)
	@echo -n "Network: " && docker network inspect urp-network > /dev/null 2>&1 && echo "OK" || (echo "FAIL - Run 'docker compose up -d'" && exit 1)
	@echo -n "Memgraph: " && docker ps -q --filter "name=urp-memgraph" --filter "status=running" | grep -q . && echo "OK" || (echo "FAIL - Run 'docker compose up -d memgraph'" && exit 1)
	@echo -n "Images: " && docker image inspect urp:latest > /dev/null 2>&1 && echo "OK" || echo "WARN - Run 'make docker-all'"
	@echo "=== All checks passed ==="

## Test targets

test: ## Run unit tests (no infra required)
	go test ./... -v -short

test-integration: preflight ## Run integration tests (requires Docker + Memgraph)
	go test ./... -v -tags=integration

test-all: preflight ## Run all tests including integration
	go test ./... -v -tags=integration

test-e2e: preflight ## Run E2E tests (creates real containers)
	URP_E2E_TEST=1 go test ./internal/container/... -v -run TestE2E -timeout 120s

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

docker-latest: ## Build default Docker image (urp:latest)
	docker build --target full -t urp:latest .

docker-minimal: ## Build minimal Docker image
	docker build --target minimal -t urp:minimal .

docker-worker: ## Build worker Docker image
	docker build --target worker -t urp:worker .

docker-master: ## Build master Docker image
	docker build --target master -t urp:master .

docker-all: docker-latest docker-minimal docker-worker docker-master ## Build all Docker images
	@echo "Built: urp:latest urp:minimal urp:worker urp:master"

## Infrastructure

infra-up: ## Start infrastructure (Memgraph + TEI)
	docker compose up -d memgraph tei
	@echo "Waiting for Memgraph..."
	@sleep 5
	@docker compose exec memgraph mgconsole --execute "RETURN 1;" > /dev/null && echo "Memgraph ready!"

infra-down: ## Stop infrastructure
	docker compose down

infra-clean: ## Remove infrastructure and volumes
	docker compose down -v

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
