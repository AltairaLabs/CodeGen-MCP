# Docker Setup for CodeGen-MCP

This directory contains Docker configurations for running the CodeGen-MCP coordinator and worker services in containers.

## Quick Start

### Prerequisites

- Docker Engine 20.10+ or Docker Desktop
- Docker Compose V2+
- Make (for convenience commands)

### Start Services (Development Mode)

```bash
# Build binaries and start containers with mounted binaries
make docker-up-dev

# View logs
make docker-logs

# Stop services
make docker-down-dev
```

Development mode mounts pre-built binaries from `./bin/`, allowing fast iteration:

```bash
# Make code changes...
make build                                           # Rebuild binaries
docker compose -f docker-compose.dev.yml restart     # Restart containers
# Changes are live!
```

## Docker Compose Configurations

### Production Setup (`docker-compose.yml`)

Builds images from Dockerfiles and runs services with proper isolation:

```bash
# Build and start services
make docker-up

# Or manually
docker compose up --build -d
```

**Services:**
- **coordinator**: Runs on port `50051`
- **worker-1**: Connects to coordinator, uses named volumes
- **worker-2**: Additional worker (start with `--profile multi-worker`)

**Features:**
- Multi-stage builds for optimized images
- Named Docker volumes for persistence
- Health checks for coordinator
- Automatic dependency ordering

### Development Setup (`docker-compose.dev.yml`)

Mounts pre-built binaries for rapid development:

```bash
# Build binaries and start dev containers
make docker-up-dev

# With multiple workers
make docker-up-multi
```

**Key Differences from Production:**
- Binaries mounted from `./bin/` directory (hot reload)
- Workspaces mounted to `./tmp/dev/` for inspection
- Debug logging enabled
- Faster iteration cycle

## Available Make Targets

### Building

```bash
make docker-build                  # Build all images
make docker-build-coordinator      # Build coordinator only
make docker-build-worker           # Build worker only
```

### Running

```bash
make docker-up                     # Start production setup
make docker-up-dev                 # Start development setup
make docker-up-multi               # Start with multiple workers
make docker-down                   # Stop production containers
make docker-down-dev               # Stop development containers
```

### Monitoring

```bash
make docker-logs                   # Tail all service logs
make docker-logs-coordinator       # Tail coordinator logs
make docker-logs-worker            # Tail worker logs
make docker-ps                     # Show running containers
```

### Development

```bash
make docker-restart                # Restart all services
make docker-restart-worker         # Restart worker-1 only
make docker-shell-coordinator      # Open shell in coordinator
make docker-shell-worker           # Open shell in worker
```

### Cleanup

```bash
make docker-clean                  # Stop and remove volumes
make docker-clean-all              # Remove images and volumes
```

## Docker Images

### Coordinator Image

**Base**: `debian:bookworm-slim`
**Size**: ~50MB (binary + CA certs)
**Build**: Multi-stage (Go builder → minimal runtime)

```dockerfile
FROM golang:1.23.4-bookworm-slim AS builder
# ... build coordinator binary ...
FROM debian:bookworm-slim
# ... copy binary, minimal runtime ...
```

**Features:**
- Non-root user (`coordinator:coordinator`)
- Minimal dependencies (only CA certificates)
- Optimized for size

### Worker Image (Builder Image)

**Base**: `python:3.11.8-bookworm-slim`
**Size**: ~800MB (Python + Go + build tools)
**Build**: Multi-stage (Go builder → Python runtime)

```dockerfile
FROM golang:1.23.4-bookworm-slim AS go-builder
# ... build worker binary ...
FROM python:3.11.8-bookworm-slim
# ... copy binary, install Python tools ...
```

**Features:**
- Non-root user (`builder:builder`)
- Python 3.11 with venv support
- Python tooling: pip-tools, pytest, mypy, flake8
- Build tools: git, curl, ca-certificates
- Worker binary built and installed
- Isolated workspaces for sessions

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                   Docker Network                     │
│                (codegen-network)                     │
│                                                      │
│  ┌───────────────────┐                              │
│  │   Coordinator     │                              │
│  │   Port: 50051     │◄─────────────┐               │
│  └───────────────────┘               │               │
│                                      │               │
│  ┌───────────────────┐              │               │
│  │    Worker 1       │──────────────┘               │
│  │  Port: 50052      │                              │
│  │  Workspaces: Vol  │                              │
│  └───────────────────┘                              │
│                                                      │
│  ┌───────────────────┐                              │
│  │    Worker 2       │──────────────┐               │
│  │  Port: 50052      │              │               │
│  │  Workspaces: Vol  │              │               │
│  └───────────────────┘              │               │
│                                     │               │
└─────────────────────────────────────┼───────────────┘
                                      │
                                      │ gRPC: 50051
                                      ▼
                               ┌──────────────┐
                               │  MCP Client  │
                               │     (LLM)    │
                               └──────────────┘
```

## Environment Variables

### Coordinator

| Variable    | Default | Description                     |
|-------------|---------|----------------------------------|
| GRPC_PORT   | 50051   | gRPC server port                |
| LOG_LEVEL   | info    | Logging level (debug, info, etc)|

### Worker

| Variable          | Default                     | Description                        |
|-------------------|-----------------------------|------------------------------------|
| WORKER_ID         | worker-1                    | Unique worker identifier           |
| GRPC_PORT         | 50052                       | gRPC server port                   |
| MAX_SESSIONS      | 5                           | Maximum concurrent sessions        |
| BASE_WORKSPACE    | /home/builder/workspaces    | Base directory for session work    |
| COORDINATOR_ADDR  | coordinator:50051           | Coordinator gRPC address           |

## Volumes

### Production (Named Volumes)

- `codegen-worker-1-workspaces`: Persistent storage for worker 1 session files
- `codegen-worker-1-checkpoints`: Checkpoint storage for worker 1
- `codegen-worker-2-workspaces`: Persistent storage for worker 2 session files
- `codegen-worker-2-checkpoints`: Checkpoint storage for worker 2

### Development (Bind Mounts)

- `./bin/coordinator` → `/app/coordinator` (coordinator binary)
- `./bin/worker` → `/usr/local/bin/worker` (worker binary)
- `./tmp/dev/worker-1/workspaces` → `/home/builder/workspaces` (session files)
- `./tmp/dev/worker-1/checkpoints` → `/home/builder/.checkpoints` (checkpoints)

## Development Workflow

### Typical Development Cycle

1. **Start services in dev mode:**
   ```bash
   make docker-up-dev
   ```

2. **Make code changes**

3. **Rebuild and restart:**
   ```bash
   make build                                      # Rebuild binaries
   docker compose -f docker-compose.dev.yml restart # Restart containers
   ```

4. **View logs:**
   ```bash
   make docker-logs
   ```

5. **Test changes:**
   ```bash
   # Connect your MCP client to localhost:50051
   ```

### Debugging

**Open shell in worker container:**
```bash
make docker-shell-worker
# Now inside container
ls -la /home/builder/workspaces
cat /home/builder/workspaces/session-*/somefile.py
```

**Inspect workspace files:**
```bash
# Workspaces are mounted to local filesystem
ls -la tmp/dev/worker-1/workspaces/
cat tmp/dev/worker-1/workspaces/session-abc123/code.py
```

**View live logs:**
```bash
make docker-logs
# or
docker compose -f docker-compose.dev.yml logs -f worker-1
```

## Networking

All services run on a custom bridge network (`codegen-network`/`codegen-network-dev`):

- Coordinator accessible at: `coordinator:50051` (internal) or `localhost:50051` (host)
- Workers connect to coordinator via hostname: `coordinator:50051`
- Service discovery via Docker DNS

## Health Checks

**Coordinator:**
- Check: `nc -z localhost 50051`
- Interval: 10s
- Timeout: 5s
- Retries: 3

Workers depend on coordinator health before starting.

## Troubleshooting

### Container won't start

```bash
# Check logs
make docker-logs

# Check container status
make docker-ps

# Restart specific service
docker compose -f docker-compose.dev.yml restart worker-1
```

### Can't connect to coordinator

```bash
# Verify coordinator is healthy
docker compose ps coordinator

# Check if port is accessible
nc -zv localhost 50051

# Check coordinator logs
make docker-logs-coordinator
```

### Worker can't register

```bash
# Check worker logs
make docker-logs-worker

# Verify network connectivity
docker compose exec worker-1 nc -zv coordinator 50051

# Check coordinator logs for registration attempts
make docker-logs-coordinator
```

### Permission issues in container

```bash
# Check user/group
docker compose exec worker-1 id

# Check workspace permissions
docker compose exec worker-1 ls -la /home/builder/workspaces
```

### Binary not updating in dev mode

```bash
# Rebuild binary
make build

# Force restart containers
docker compose -f docker-compose.dev.yml restart

# Verify binary was updated
ls -lh bin/worker
docker compose exec worker-1 ls -lh /usr/local/bin/worker
```

## Testing

### Manual Testing

1. Start services:
   ```bash
   make docker-up-dev
   ```

2. Connect MCP client to `localhost:50051`

3. Execute MCP tools (fs.write, run.python, etc.)

4. Verify in worker container:
   ```bash
   make docker-shell-worker
   ls /home/builder/workspaces/
   ```

### Integration Testing

```bash
# TODO: Implement
make docker-test
```

## Production Deployment

For production deployment:

1. Use `docker-compose.yml` (not dev version)
2. Configure environment variables appropriately
3. Set up persistent volumes or external storage
4. Consider Kubernetes for orchestration (see k8s docs)
5. Set up monitoring and logging
6. Configure resource limits

## Next Steps

- [ ] Implement Python venv initialization in worker sessions
- [ ] Add integration tests that run against containers
- [ ] Set up artifact storage (S3/MinIO)
- [ ] Add Prometheus metrics export
- [ ] Create Kubernetes deployment manifests
- [ ] Add CI/CD pipeline for image building

## References

- [Worker-Coordinator Protocol](../docs/local-backlog/worker-coordinator-protocol.md)
- [Implementation Status](../docs/local-backlog/implementation-status.md)
- [Coordinator Documentation](../docs/coordinator/)
- [Worker Documentation](../docs/worker/)
