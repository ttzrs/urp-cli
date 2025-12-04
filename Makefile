# URP Project Master Makefile
# Wrapper for Go project and Infrastructure

.PHONY: all build test clean up down logs ps shell

# Default: Build the CLI
all: build

# --- Development ---

build: ## Build the URP CLI (Go)
	$(MAKE) -C go build

test: ## Run unit tests
	$(MAKE) -C go test

lint: ## Run linters
	$(MAKE) -C go lint

clean: ## Clean build artifacts
	$(MAKE) -C go clean

# --- Infrastructure (Docker) ---

up: ## Start the infrastructure (Memgraph)
	docker compose up -d memgraph

up-all: ## Start full stack (Master + Worker + Memgraph)
	docker compose up -d

down: ## Stop the infrastructure
	docker compose down

restart: down up ## Restart infrastructure

logs: ## View logs
	docker compose logs -f

ps: ## View running containers
	docker compose ps

# --- Access ---

shell-master: ## Shell into Master container
	docker compose exec master bash

shell-worker: ## Shell into Worker container
	docker compose exec worker bash

# --- Helpers ---

help: ## Show this help
	@echo "URP Project Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%%-15s\033[0m %s\n", $$1, $$2}'
