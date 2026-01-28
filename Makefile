.PHONY: build test lint clean install run deps docs
# Binary name
BINARY_NAME=mcp-filepuff
# Build directory
BUILD_DIR=bin
# Main package
MAIN_PKG=./cmd/mcp-filepuff

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt

# Build flags
LDFLAGS=-ldflags "-s -w" -buildvcs=false

# Default target
all: deps test build

# Install dependencies
deps:
	$(GOMOD) download
	$(GOMOD) tidy

# Build the binary
build:
	mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_PKG)

# Build for all platforms
build-all:
	mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_PKG)
	GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_PKG)
	GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_PKG)
	GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_PKG)
	GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_PKG)

# Run tests
test:
	$(GOTEST) -v -coverprofile=coverage.out ./...

# Run tests with race detector on critical packages
test-race:
	@echo "Running race detector tests on critical packages..."
	$(GOTEST) -v -race -timeout=5m ./internal/edit/...
	$(GOTEST) -v -race -timeout=5m ./internal/lsp/...
	$(GOTEST) -v -race -timeout=5m ./internal/parser/...
	$(GOTEST) -v -race -timeout=5m ./internal/server/...
	@echo "Race detector tests completed successfully"

# Run tests with short flag
test-short:
	$(GOTEST) -v -short ./...

# Run all tests including race detector
test-all: test test-race

# Run linters
lint:
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out

# Install binary to GOPATH/bin
install: build
	cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/

# Run the server (for development)
run: build
	./$(BUILD_DIR)/$(BINARY_NAME) -log-level debug

# Run with specific workspace
run-workspace: build
	./$(BUILD_DIR)/$(BINARY_NAME) -workspace $(WORKSPACE) -log-level debug

# Show help
help:
	@echo "Available targets:"
	@echo "  deps          - Download and tidy dependencies"
	@echo "  build         - Build the binary"
	@echo "  build-all     - Build for all platforms"
	@echo "  test          - Run tests with coverage"
	@echo "  test-race     - Run tests with race detector on critical packages"
	@echo "  test-all      - Run all tests including race detector"
	@echo "  test-short    - Run short tests"
	@echo "  lint          - Run linters"
	@echo "  clean         - Clean build artifacts"
	@echo "  install       - Install binary to GOPATH/bin"
	@echo "  run           - Build and run the server"
	@echo "  run-workspace - Run with specific workspace (WORKSPACE=/path)"


# Generate API documentation
docs:
	@echo "Generating API documentation..."
	$(GOCMD) run ./cmd/docgen
	@echo "Documentation generated in docs/API.md"