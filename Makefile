.PHONY: help build build-all clean test check image

# Build variables
BINARY_NAME := wonder
BUILD_DIR := bin
GO := go
GOFLAGS := -v
LDFLAGS := -s -w

# Version info
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

help: ## Show this help message
	@echo "Wonder Mesh Net - Build System"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ { printf "  %-15s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

build: ## Build the wonder binary
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/wonder

build-all: ## Build for all platforms (linux/darwin, amd64/arm64)
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/wonder
	GOOS=linux GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/wonder
	GOOS=darwin GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/wonder
	GOOS=darwin GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/wonder

clean: ## Remove build artifacts
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
