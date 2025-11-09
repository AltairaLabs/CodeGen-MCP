# End-to-End MCP Tests

This directory contains end-to-end tests for the CodeGen-MCP system using Python and the MCP SDK.

## Quick Start

```bash
# Setup the e2e test environment (creates venv, installs dependencies)
make setup-e2e-tests

# Run the e2e tests (starts Docker services automatically)
make test-e2e
```

The test now uses **HTTP/SSE transport** to connect to the coordinator running in Docker, enabling full integration testing with workers.

## What Gets Tested

The e2e test validates the complete MCP workflow using HTTP/SSE transport:

1. **Connection**: Establishes MCP connection to coordinator via HTTP/SSE (http://localhost:8080/mcp)
2. **Tool Discovery**: Lists all available tools (fs.read, fs.write, fs.list, run.python, pkg.install, echo)
3. **File Operations**: 
   - Write a Python file using `fs.write`
   - Read it back using `fs.read`
   - List directory contents using `fs.list`
4. **Python Execution**: 
   - Execute Python code from a file using `run.python`
   - Execute inline Python code using `run.python`
5. **Package Management**:
   - Install a package using `pkg.install` (e.g., requests)
   - Import and use the installed package
6. **Echo Test**: Verify basic tool calling works
7. **Worker Integration**: Tests execute on real workers via Docker

## Architecture

### Transport Modes

The coordinator supports two MCP transport modes:

1. **stdio transport** (local, single-process):
   - Used when running coordinator directly: `./bin/coordinator`
   - Communicates via stdin/stdout
   - Cannot connect to separate worker processes
   - Useful for tool development and protocol testing

2. **HTTP/SSE transport** (network, multi-process):
   - Used in Docker: `./bin/coordinator --http`
   - Communicates via HTTP with SSE for notifications
   - Enables network-based MCP connections
   - Full worker integration via gRPC (separate channel)
   - Production-ready for real integrations

### E2E Test Flow

```
┌─────────────┐     HTTP/SSE      ┌─────────────┐     gRPC       ┌────────┐
│ test_mcp.py │ ────────────────> │ Coordinator │ ────────────> │ Worker │
│  (Python)   │    :8080/mcp      │  (Docker)   │    :50051      │  (Go)  │
└─────────────┘                   └─────────────┘                └────────┘
```

## Manual Testing

### With Docker (Full Integration)

```bash
# Start services
docker compose up -d

# Wait for workers to register
sleep 5

# Set coordinator URL and run test
COORDINATOR_URL="http://localhost:8080/mcp" python tests/e2e/test_mcp.py

# Clean up
docker compose down
```

### Local stdio Mode (No Workers)

```bash
# Build coordinator
make build-coordinator

# Run test (will show "no workers available" - expected)
./bin/coordinator &
COORDINATOR_PID=$!

# Test will fail on Python execution (no workers)
python tests/e2e/test_mcp.py

kill $COORDINATOR_PID
```

## Requirements

- Python 3.9+
- MCP Python SDK (installed automatically by `make setup-e2e-tests`)
- Docker and Docker Compose (for full integration tests)
- Go 1.25+ (for building binaries)

## Troubleshooting

**Connection refused errors:**
```bash
# Check if coordinator is running
docker compose ps
curl http://localhost:8080/mcp/sse  # Should return SSE stream
```

**No workers available:**
```bash
# Check worker logs
docker compose logs worker-1
docker compose logs coordinator | grep "Worker registered"
```

**Python import errors:**
```bash
# Reinstall test dependencies
make clean-e2e
make setup-e2e-tests
```
