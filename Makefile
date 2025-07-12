# Makefile for gcauto Go application

# Variables
BINARY_NAME=gcauto
GO_FILES=$(wildcard *.go)
BUILD_DIR=build
INSTALL_DIR=/usr/local/bin

# Get OS and architecture
GOOS?=$(shell go env GOOS)
GOARCH?=$(shell go env GOARCH)

# Build output name
ifeq ($(GOOS),windows)
	BINARY_EXT=.exe
else
	BINARY_EXT=
endif
OUTPUT_NAME=$(BINARY_NAME)-$(GOOS)-$(GOARCH)$(BINARY_EXT)

# Default target
all: fmt install

# Build the application
build: $(BUILD_DIR)/$(OUTPUT_NAME)

$(BUILD_DIR)/$(OUTPUT_NAME): $(GO_FILES) go.mod
	@mkdir -p $(BUILD_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) go build -o $(BUILD_DIR)/$(OUTPUT_NAME) .

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)

# Install the binary to system PATH (only for native builds)
install: build-native
	sudo cp $(BUILD_DIR)/$(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
	sudo chmod +x $(INSTALL_DIR)/$(BINARY_NAME)

# Build for native OS/architecture
build-native:
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) .

# Uninstall the binary from system PATH
uninstall:
	sudo rm -f $(INSTALL_DIR)/$(BINARY_NAME)

# Test the application
test:
	go test -v ./...

# Run the application
run: build-native
	$(BUILD_DIR)/$(BINARY_NAME)

# Format Go code
fmt:
	go fmt ./...


.PHONY: lint
lint:
	@echo "🔍 Running linters..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run *.go --config .golangci.yaml; \
	else \
		echo "⚠️  golangci-lint not found, running go vet instead"; \
		go vet ./...; \
	fi

# Check for Go modules updates
mod-update:
	go mod tidy
	go mod download

# Development build (with race detector)
dev-build:
	@mkdir -p $(BUILD_DIR)
	go build -race -o $(BUILD_DIR)/$(BINARY_NAME) .

# Build for all platforms
build-all: build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 build-windows-amd64

# Build for specific platforms
build-linux-amd64:
	GOOS=linux GOARCH=amd64 $(MAKE) build

build-linux-arm64:
	GOOS=linux GOARCH=arm64 $(MAKE) build

build-darwin-amd64:
	GOOS=darwin GOARCH=amd64 $(MAKE) build

build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 $(MAKE) build

build-windows-amd64:
	GOOS=windows GOARCH=amd64 $(MAKE) build

# Show help
help:
	@echo "Available targets:"
	@echo "  build             - Build for specified GOOS/GOARCH (default: current system)"
	@echo "  build-native      - Build for current system"
	@echo "  build-all         - Build for all supported platforms"
	@echo "  build-linux-amd64 - Build for Linux AMD64"
	@echo "  build-linux-arm64 - Build for Linux ARM64"
	@echo "  build-darwin-amd64- Build for macOS AMD64"
	@echo "  build-darwin-arm64- Build for macOS ARM64 (Apple Silicon)"
	@echo "  build-windows-amd64 - Build for Windows AMD64"
	@echo "  clean             - Clean build artifacts"
	@echo "  install           - Install binary to $(INSTALL_DIR)"
	@echo "  uninstall         - Remove binary from $(INSTALL_DIR)"
	@echo "  test              - Run tests"
	@echo "  run               - Build and run the application"
	@echo "  fmt               - Format Go code"
	@echo "  lint              - Run linters"
	@echo "  mod-update        - Update Go modules"
	@echo "  dev-build         - Development build with race detector"
	@echo "  help              - Show this help message"
	@echo ""
	@echo "Environment variables:"
	@echo "  GOOS              - Target operating system (default: $(GOOS))"
	@echo "  GOARCH            - Target architecture (default: $(GOARCH))"
	@echo ""
	@echo "Examples:"
	@echo "  make build                    - Build for current system"
	@echo "  GOOS=linux GOARCH=arm64 make build - Build for Linux ARM64"
	@echo "  make build-all                - Build for all platforms"

.PHONY: all build build-native build-all build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64 build-windows-amd64 clean install uninstall test run fmt lint mod-update dev-build help
