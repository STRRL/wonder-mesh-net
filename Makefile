.PHONY: help build build-all clean test check image generate web web-deps web-clean

# Build variables
BINARY_NAME := wonder
BUILD_DIR := bin
GO := go
GOFLAGS := -v

# Web UI variables
WEB_DIR := web
WEB_DIST := $(WEB_DIR)/dist
UI_STATIC := internal/app/coordinator/ui/static

# Version info: tag if tagged, "untagged" otherwise; sha with -dirty suffix if dirty
GIT_SHA := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_DIRTY := $(shell git diff-index --quiet HEAD -- 2>/dev/null || echo "dirty")
GIT_TAG := $(shell git describe --tags --exact-match 2>/dev/null)

ifdef GIT_TAG
    VERSION := $(GIT_TAG)
else
    VERSION := untagged
endif

ifdef GIT_DIRTY
    GIT_SHA := $(GIT_SHA)-dirty
endif

VERSION_PKG := github.com/strrl/wonder-mesh-net/cmd/wonder/commands
LDFLAGS := -s -w -X $(VERSION_PKG).version=$(VERSION) -X $(VERSION_PKG).gitSHA=$(GIT_SHA)

help: ## Show this help message
	@echo "Wonder Mesh Net - Build System"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ { printf "  %-15s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

web-deps: ## Install web dependencies
	cd $(WEB_DIR) && npm ci

web: web-deps ## Build web UI
	cd $(WEB_DIR) && npm run build
	rm -rf $(UI_STATIC)
	cp -r $(WEB_DIST) $(UI_STATIC)

web-clean: ## Clean web build artifacts
	rm -rf $(WEB_DIR)/node_modules $(WEB_DIST) $(UI_STATIC)
	mkdir -p $(UI_STATIC)
	touch $(UI_STATIC)/.gitkeep

build: web ## Build the wonder binary (includes web UI)
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/wonder

build-all: ## Build for all platforms (linux/darwin, amd64/arm64)
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/wonder
	GOOS=linux GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/wonder
	GOOS=darwin GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/wonder
	GOOS=darwin GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/wonder

clean: web-clean ## Remove build artifacts
	rm -rf $(BUILD_DIR)
	rm -f coverage.out

test: ## Run tests
	$(GO) test -race -coverprofile=coverage.out ./...

check: ## Run all code checks (fmt, vet, lint)
	@echo "Running gofmt..."
	@gofmt -w .
	@echo "Running go vet..."
	$(GO) vet ./...
	@echo "Running golangci-lint..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed, skipping"; \
	fi
	@echo "All checks passed"

image: ## Build and push multi-arch Docker image
	./hack/build-image.sh

generate: ## Generate code (sqlc)
	@echo "Running sqlc generate..."
	@if command -v sqlc >/dev/null 2>&1; then \
		sqlc generate; \
	else \
		echo "sqlc not installed. Install with: go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest"; \
		exit 1; \
	fi
