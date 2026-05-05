.PHONY: help infra infra-down dev watch build run test tidy clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

infra: ## Start dev infra (Redis only) in Docker
	docker compose -f compose.dev.yml up -d

infra-down: ## Stop dev infra
	docker compose -f compose.dev.yml down

dev: ## Run server locally with hot reload (requires air: go install github.com/air-verse/air@latest)
	air

watch: dev ## Alias for dev

build: ## Build production binary
	@mkdir -p bin
	go build -o bin/beacon ./cmd/server

run: ## Run server locally (no hot reload)
	go run ./cmd/server

test: ## Run tests
	go test ./...

tidy: ## go mod tidy
	go mod tidy

clean: ## Remove generated files
	rm -rf bin tmp
