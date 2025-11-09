# AltairaLabs CodeGen MCP Makefile

.PHONY: help build build-coordinator build-worker test test-race lint coverage clean install run-coordinator run-worker proto \
	docker-build docker-build-coordinator docker-build-worker docker-up docker-up-dev docker-down docker-down-dev \
	docker-logs docker-ps docker-clean docker-shell-coordinator docker-shell-worker docker-test

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

test-unit: ## Run fast unit tests (skips network integration tests)
	@echo "Running unit tests with coverage (excluding network integration tests)..."
	@go test -short -coverprofile=coverage.out -covermode=atomic \
		-coverpkg=$$(go list ./... | grep -v '/api/proto/v1$$' | tr '\n' ',' | sed 's/,$$//') \
		$$(go list ./... | grep -v '/api/proto/v1$$')
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

coverage: ## Generate full coverage report (includes all tests)
	@echo "Generating coverage report..."
	@go test -count=1 -coverprofile=coverage.out -covermode=atomic \
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

###########################################
# Docker Targets
###########################################

docker-build: ## Build Docker images for coordinator and worker
	@echo "Building Docker images..."
	docker compose build

docker-build-coordinator: ## Build coordinator Docker image only
	@echo "Building coordinator image..."
	docker compose build coordinator

docker-build-worker: ## Build worker Docker image only
	@echo "Building worker image..."
	docker compose build worker-1

docker-up: ## Start services with production compose (builds images)
	@echo "Starting services with docker-compose.yml..."
	@mkdir -p tmp/dev/worker-1 tmp/dev/worker-2
	docker compose up --build -d
	@echo ""
	@echo "Services started!"
	@echo "Coordinator: localhost:50051"
	@echo "View logs: make docker-logs"
	@echo "Stop services: make docker-down"

docker-up-dev: build ## Start services with dev compose (mounts binaries)
	@echo "Starting services with docker-compose.dev.yml..."
	@mkdir -p tmp/dev/worker-1/{workspaces,checkpoints}
	@mkdir -p tmp/dev/worker-2/{workspaces,checkpoints}
	docker compose -f docker-compose.dev.yml up --build -d
	@echo ""
	@echo "Services started in DEV mode!"
	@echo "Coordinator: localhost:50051"
	@echo "Binaries mounted from ./bin/"
	@echo "To rebuild: make build && docker compose -f docker-compose.dev.yml restart"
	@echo "View logs: make docker-logs"
	@echo "Stop services: make docker-down-dev"

docker-up-multi: build ## Start with multiple workers (dev mode)
	@echo "Starting services with multiple workers..."
	@mkdir -p tmp/dev/worker-1/{workspaces,checkpoints}
	@mkdir -p tmp/dev/worker-2/{workspaces,checkpoints}
	docker compose -f docker-compose.dev.yml --profile multi-worker up --build -d
	@echo ""
	@echo "Services started with 2 workers!"
	@echo "View logs: make docker-logs"

docker-down: ## Stop and remove production containers
	@echo "Stopping services..."
	docker compose down
	@echo "Services stopped"

docker-down-dev: ## Stop and remove dev containers
	@echo "Stopping dev services..."
	docker compose -f docker-compose.dev.yml down
	@echo "Dev services stopped"

docker-logs: ## Tail logs from all services
	docker compose -f docker-compose.dev.yml logs -f || docker compose logs -f

docker-logs-coordinator: ## Tail coordinator logs only
	docker compose -f docker-compose.dev.yml logs -f coordinator || docker compose logs -f coordinator

docker-logs-worker: ## Tail worker-1 logs only
	docker compose -f docker-compose.dev.yml logs -f worker-1 || docker compose logs -f worker-1

docker-ps: ## Show running containers
	@docker compose -f docker-compose.dev.yml ps || docker compose ps

docker-restart: ## Restart all services
	@echo "Restarting services..."
	docker compose -f docker-compose.dev.yml restart || docker compose restart
	@echo "Services restarted"

docker-restart-worker: ## Restart worker-1 only
	@echo "Restarting worker-1..."
	docker compose -f docker-compose.dev.yml restart worker-1 || docker compose restart worker-1
	@echo "Worker restarted"

docker-shell-coordinator: ## Open shell in coordinator container
	docker exec -it codegen-coordinator-dev /bin/bash || docker exec -it codegen-coordinator /bin/bash

docker-shell-worker: ## Open shell in worker-1 container
	docker exec -it codegen-worker-1-dev /bin/bash || docker exec -it codegen-worker-1 /bin/bash

docker-clean: docker-down docker-down-dev ## Stop containers and clean up volumes
	@echo "Cleaning up Docker resources..."
	docker volume rm codegen-worker-1-workspaces codegen-worker-1-checkpoints \
		codegen-worker-2-workspaces codegen-worker-2-checkpoints 2>/dev/null || true
	@rm -rf tmp/dev
	@echo "Docker cleanup complete"

docker-clean-all: docker-clean ## Remove all Docker images and volumes
	@echo "Removing Docker images..."
	docker rmi codegen-mcp-coordinator:latest codegen-mcp-coordinator:dev \
		codegen-mcp-worker:latest codegen-mcp-worker:dev 2>/dev/null || true
	@echo "All Docker resources cleaned"

docker-test: ## Run integration tests against Docker containers
	@echo "Running Docker integration tests..."
	@./scripts/test-docker-integration.sh

docker-test-multi: ## Run multi-worker and multi-session tests
	@echo "Running multi-worker tests..."
	@./scripts/test-multi-session.sh
