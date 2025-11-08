# AltairaLabs CodeGen MCP Makefile

.PHONY: help build build-coordinator build-worker test test-race lint coverage clean install run-coordinator run-worker proto

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

install: ## Install dependencies
	@echo "Installing Go dependencies..."
	@go mod download

proto: ## Generate Go code from protobuf definitions
	@./scripts/gen/generate-proto.sh

build: proto build-coordinator build-worker ## Build all components

build-coordinator: ## Build coordinator binary
	@echo "Building coordinator..."
	@go build -o bin/coordinator ./cmd/coordinator
	@echo "coordinator built successfully -> bin/coordinator"

build-worker: ## Build worker binary
	@echo "Building worker..."
	@go build -o bin/worker ./cmd/worker
	@echo "worker built successfully -> bin/worker"

run-coordinator: ## Run coordinator locally
	@go run ./cmd/coordinator

run-worker: ## Run worker locally
	@go run ./cmd/worker

test: ## Run all tests
	@echo "Running tests..."
	@go test -v ./...

test-unit: ## Run unit tests with coverage for SonarQube
	@echo "Running unit tests with coverage..."
	@go test -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -func=coverage.out | grep "^total:" || echo "No coverage data"
	@echo "Coverage report generated: coverage.out"

test-race: ## Run tests with race detector
	@echo "Testing with race detector..."
	@go test -race -v ./... 2>&1 | tee race-test.log; \
	if grep -q "^FAIL" race-test.log; then \
		echo "Tests failed"; \
		rm race-test.log; \
		exit 1; \
	else \
		echo "All tests passed (race detector completed)"; \
		rm race-test.log; \
		exit 0; \
	fi

coverage: ## Generate test coverage report
	@echo "Generating coverage report..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | grep "^total:" || echo "No coverage data"
	@echo "Coverage report generated: coverage.out"

lint: ## Run linters
	@echo "Running go vet..."
	@go vet ./...
	@echo "Running go fmt..."
	@go fmt ./...
	@echo "Running golangci-lint..."
	@golangci-lint run ./...

clean: ## Clean build artifacts
	@rm -rf bin/
	@rm -f coverage.out
	@rm -f race-test.log
	@echo "Cleaned build artifacts"
