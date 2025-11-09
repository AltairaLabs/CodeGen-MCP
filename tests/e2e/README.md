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

### Common Issues

#### 1. Test Timeout / ReadTimeout Error

**Symptoms:**
```
httpx.ReadTimeout: The read operation timed out
```

**Cause:** Coordinator is running but no workers are registered to handle tasks.

**Solution:**
```bash
# Check if workers are running
docker compose ps

# Should show coordinator, worker-1, worker-2 all "Up"
# If workers are missing, start them:
docker compose up -d

# Verify workers registered (wait ~5 seconds after start)
curl http://localhost:8080/health

# Check worker logs for registration messages
docker compose logs worker-1 | grep -i "register"
docker compose logs coordinator | grep -i "worker registered"
```

The e2e test now includes a pre-flight check (Phase 2.5) that detects missing workers immediately instead of timing out.

#### 2. Connection Refused Errors

**Symptoms:**
```
ConnectionRefusedError: [Errno 61] Connection refused
```

**Cause:** Coordinator is not running or running on different port.

**Solution:**
```bash
# Check if coordinator is running
docker compose ps
curl http://localhost:8080/mcp/sse  # Should return SSE stream

# If not running, start services
docker compose up -d

# Check coordinator logs
docker compose logs coordinator
```

#### 3. Port Already In Use

**Symptoms:**
```
Error starting userland proxy: listen tcp4 0.0.0.0:8080: bind: address already in use
```

**Cause:** Another process (possibly a local coordinator) is using port 8080.

**Solution:**
```bash
# Find process using port 8080
lsof -i :8080

# Option 1: Stop the conflicting process
kill <PID>

# Option 2: Change Docker Compose port
# Edit docker-compose.yml, change "8080:8080" to "8081:8080"
```

#### 4. No Workers Available

**Symptoms:**
```
❌ NO WORKERS AVAILABLE
⚠️  The coordinator is running but no workers are registered.
```

**Cause:** Worker processes haven't started or failed to register.

**Diagnosis:**
```bash
# 1. Check worker container status
docker compose ps worker-1 worker-2

# 2. Check worker logs for errors
docker compose logs worker-1 --tail=50

# 3. Verify network connectivity (workers should reach coordinator)
docker compose exec worker-1 ping coordinator

# 4. Check if workers are healthy
curl http://localhost:8080/health
# Should show registered workers with capacity

# 5. Verify gRPC port is accessible (workers use this)
docker compose logs coordinator | grep "Lifecycle gRPC server"
```

**Common Worker Issues:**
- **Build failed:** Run `docker compose build worker-1`
- **Registration timeout:** Increase `COORDINATOR_TIMEOUT` in docker-compose.yml
- **Port conflict:** Check if port 50051 (gRPC) is available
- **Network issues:** Run `docker network inspect codegen-mcp_default`

#### 5. Python Import Errors

**Symptoms:**
```
❌ Error: MCP SDK not installed
ImportError: cannot import name 'ClientSession'
```

**Solution:**
```bash
# Reinstall test dependencies
make clean-e2e
make setup-e2e-tests

# Or manually:
cd tests/e2e
python -m venv venv
source venv/bin/activate
pip install mcp
```

### Debugging Tips

**Enable Verbose Logging:**
```bash
# Set log level for coordinator
docker compose up -d
docker compose logs -f coordinator

# For more detail, edit docker-compose.yml:
# Add: LOG_LEVEL=debug
```

**Manual Tool Testing:**
```bash
# Test echo tool (doesn't require workers)
curl -X POST http://localhost:8080/mcp/tools/call \
  -H "Content-Type: application/json" \
  -d '{"name": "echo", "arguments": {"message": "test"}}'

# Test fs.list (requires workers)
curl -X POST http://localhost:8080/mcp/tools/call \
  -H "Content-Type: application/json" \
  -d '{"name": "fs.list", "arguments": {"path": ""}}'
  
# Should return error if no workers:
# {"error": "no workers available to handle request"}
```

**Check Worker Capacity:**
```bash
# Workers should report capacity in health endpoint
curl http://localhost:8080/health | jq '.workers'

# Expected output:
# {
#   "total": 2,
#   "healthy": 2,
#   "capacity": {
#     "total": 10,
#     "available": 10
#   }
# }
```
