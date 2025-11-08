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
	@echo "Installing protoc plugins..."
	@go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	@go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

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
	@go test -coverprofile=coverage.out -covermode=atomic \
		-coverpkg=$$(go list ./... | grep -v '/api/proto/v1$$' | tr '\n' ',' | sed 's/,$$//') \
		./...
	@echo "Filtering coverage data (excluding generated proto files and main.go)..."
	@grep -v -E '(\.pb\.go|cmd/.*/main\.go):' coverage.out > coverage.filtered.out || true
	@mv coverage.filtered.out coverage.out
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

coverage: ## Generate coverage report
	@echo "Generating coverage report..."
	@go test -coverprofile=coverage.out -covermode=atomic \
		-coverpkg=$$(go list ./... | grep -v '/api/proto/v1$$' | tr '\n' ',' | sed 's/,$$//') \
		./...
	@echo "Filtering coverage data (excluding generated proto files and main.go)..."
	@grep -v -E '(\.pb\.go|cmd/.*/main\.go):' coverage.out > coverage.filtered.out || true
	@mv coverage.filtered.out coverage.out
	@go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"
	@go tool cover -func=coverage.out | grep "^total:"

lint: ## Run linters
	@echo "Running go vet..."
	@go vet ./...
	@echo "Running go fmt..."
	@go fmt ./...
	@echo "Running golangci-lint..."
	@golangci-lint run ./... || echo "Linter found issues (non-blocking)"

lint-ci: ## Run linters for CI (strict mode)
	@echo "Running go vet..."
	@go vet ./...
	@echo "Running go fmt..."
	@test -z "$$(gofmt -l .)" || (echo "Go files need formatting:" && gofmt -l . && exit 1)
	@echo "Running golangci-lint..."
	@golangci-lint run ./...

ci: ## Run full CI pipeline locally (install, build, test, lint)
	@echo "=== Running Full CI Pipeline ==="
	@echo ""
	@echo "Step 1/4: Installing dependencies..."
	@$(MAKE) install
	@echo ""
	@echo "Step 2/4: Building binaries..."
	@$(MAKE) build
	@echo ""
	@echo "Step 3/4: Running tests with coverage..."
	@$(MAKE) test-unit
	@echo ""
	@echo "Step 4/4: Running linters..."
	@$(MAKE) lint-ci || echo "Linting completed with issues"
	@echo ""
	@echo "=== CI Pipeline Complete ==="
	@echo "Build artifacts: bin/coordinator, bin/worker"
	@echo "Coverage report: coverage.out"
	@echo ""
	@echo "To view coverage:"
	@echo "  make coverage"
	@echo ""
	@echo "To run individual steps:"
	@echo "  make install    - Install dependencies"
	@echo "  make build      - Build binaries"
	@echo "  make test-unit  - Run tests with coverage"
	@echo "  make lint-ci    - Run strict linting"

clean: ## Clean build artifacts
	@rm -rf bin/
	@rm -f coverage.out
	@rm -f race-test.log
	@echo "Cleaned build artifacts"
