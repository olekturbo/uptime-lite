BINARY     := uptime-lite
BUILD_DIR  := ./bin
CMD        := ./cmd/server
SWAG       := $(shell go env GOPATH)/bin/swag
COVER_FILE := coverage.out

.DEFAULT_GOAL := help

# ── Build ────────────────────────────────────────────────────────────────────────

.PHONY: build
build: ## Compile the binary into ./bin/
	@mkdir -p $(BUILD_DIR)
	go build -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY) $(CMD)

.PHONY: run
run: ## Run the server directly with go run (requires Postgres + Redis)
	go run $(CMD)

# ── Docker ───────────────────────────────────────────────────────────────────────

.PHONY: up
up: ## Start Postgres and Redis via docker compose
	docker compose up -d

.PHONY: down
down: ## Stop and remove docker compose containers
	docker compose down

.PHONY: up-full
up-full: up ## Start infra + build and run the binary
	$(MAKE) build
	$(BUILD_DIR)/$(BINARY)

# ── Swagger ───────────────────────────────────────────────────────────────────────

.PHONY: swag
swag: ## Regenerate Swagger docs (./docs/)
	$(SWAG) init -g $(CMD)/main.go -o ./docs

# ── Tests ────────────────────────────────────────────────────────────────────────

.PHONY: test
test: ## Run all tests
	go test ./...

.PHONY: test-v
test-v: ## Run all tests with verbose output
	go test -v ./...

.PHONY: coverage
coverage: ## Run tests and open HTML coverage report
	go test -coverprofile=$(COVER_FILE) ./...
	go tool cover -html=$(COVER_FILE)

# ── Code quality ─────────────────────────────────────────────────────────────────

.PHONY: lint
lint: ## Run golangci-lint (must be installed separately)
	golangci-lint run ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: tidy
tidy: ## Tidy and verify go.mod / go.sum
	go mod tidy
	go mod verify

# ── Cleanup ───────────────────────────────────────────────────────────────────────

.PHONY: clean
clean: ## Remove build artefacts and coverage files
	rm -rf $(BUILD_DIR) $(COVER_FILE) coverage.html

# ── Help ─────────────────────────────────────────────────────────────────────────

.PHONY: help
help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'
