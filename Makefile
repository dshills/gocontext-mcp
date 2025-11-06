# GoContext MCP Server Makefile
.PHONY: all build build-purego test test-race lint bench clean help

# Build variables
BINARY_NAME=gocontext
BUILD_DIR=bin
VERSION?=1.0.0
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.buildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)"

# Default target
all: lint test build

## help: Display this help message
help:
	@echo "GoContext MCP Server - Available targets:"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

## build: Build binary with CGO (includes sqlite-vec extension)
build:
	@echo "Building with CGO (sqlite-vec + FTS5)..."
	CGO_ENABLED=1 go build $(LDFLAGS) -tags "sqlite_vec sqlite_fts5" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/gocontext
	@echo "Binary created at $(BUILD_DIR)/$(BINARY_NAME)"

## build-purego: Build pure Go binary (no CGO, slower vector ops)
build-purego:
	@echo "Building pure Go binary (no CGO)..."
	CGO_ENABLED=0 go build $(LDFLAGS) -tags "purego" -o $(BUILD_DIR)/$(BINARY_NAME)-purego ./cmd/gocontext
	@echo "Binary created at $(BUILD_DIR)/$(BINARY_NAME)-purego"

## test: Run all tests
test:
	@echo "Running tests..."
	go test -v -cover ./...

## test-race: Run tests with race detector
test-race:
	@echo "Running tests with race detector..."
	go test -race -v ./...

## test-coverage: Run tests with coverage report
test-coverage:
	@echo "Running tests with coverage..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## lint: Run golangci-lint
lint:
	@echo "Running linters..."
	golangci-lint run

## lint-fix: Run golangci-lint with auto-fix
lint-fix:
	@echo "Running linters with auto-fix..."
	golangci-lint run --fix

## bench: Run benchmarks
bench:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

## bench-cpu: Run benchmarks with CPU profiling
bench-cpu:
	@echo "Running benchmarks with CPU profiling..."
	go test -bench=. -benchmem -cpuprofile=cpu.prof ./...
	@echo "CPU profile: cpu.prof (analyze with: go tool pprof cpu.prof)"

## bench-mem: Run benchmarks with memory profiling
bench-mem:
	@echo "Running benchmarks with memory profiling..."
	go test -bench=. -benchmem -memprofile=mem.prof ./...
	@echo "Memory profile: mem.prof (analyze with: go tool pprof mem.prof)"

## clean: Remove build artifacts and test outputs
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html
	rm -f cpu.prof mem.prof
	rm -f *.test
	@echo "Clean complete"

## deps: Download and verify dependencies
deps:
	@echo "Downloading dependencies..."
	go mod download
	go mod verify

## tidy: Clean up go.mod and go.sum
tidy:
	@echo "Tidying dependencies..."
	go mod tidy

## fmt: Format all Go source files
fmt:
	@echo "Formatting code..."
	go fmt ./...

## vet: Run go vet
vet:
	@echo "Running go vet..."
	go vet ./...

## ci: Run full CI pipeline (format, lint, test, build)
ci: fmt lint test test-race build build-purego
	@echo "CI pipeline complete"

## install: Install binary to GOPATH/bin
install:
	@echo "Installing binary..."
	CGO_ENABLED=1 go install $(LDFLAGS) -tags "sqlite_vec sqlite_fts5" ./cmd/gocontext

## dev: Quick development build and test
dev: fmt lint test build
	@echo "Development build complete"
