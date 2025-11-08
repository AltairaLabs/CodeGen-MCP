---
title: CodeGen-MCP Documentation
layout: default
---

# CodeGen-MCP Documentation

Welcome to the CodeGen-MCP documentation. This guide covers architecture, deployment, API reference, and testing for all components.

## 📚 Documentation Structure

### [Coordinator](./coordinator/)

The Coordinator is the central MCP server that orchestrates code generation workflows.

- **[Architecture Overview](./coordinator/README.md)** - Component design, security model, and extensibility
- **[API Reference](./coordinator/api-reference.md)** - Complete API documentation with examples
- **[Deployment Guide](./coordinator/deployment.md)** - Docker, Kubernetes, and production deployment
- **[Testing Guide](./coordinator/testing.md)** - Test strategies and coverage

**Status:** ✅ Implemented (v0.1.0, 33.2% overall / 80%+ core logic coverage, 51 tests)

### Worker (Coming Soon)

The Worker executes code generation tasks inside secure Python environments.

- Architecture Overview - Component design and communication protocols
- API Reference - gRPC/HTTP API documentation
- Deployment Guide - Container configuration and Kubernetes manifests
- Testing Guide - Unit and integration tests

**Status:** ⏳ Planned

### Builder Container (Coming Soon)

The Builder Container provides reproducible Python execution environments.

- Dockerfile Reference - Multi-stage build and dependencies
- Environment Configuration - Python packages and tools
- Security Model - Isolation and sandboxing
- Customization Guide - Extending the base image

**Status:** ⏳ Planned

## 🚀 Quick Start

### Local Development

```bash
# Clone repository
git clone https://github.com/AltairaLabs/codegen-mcp.git
cd codegen-mcp

# Build all components
make build

# Run coordinator
./bin/coordinator --debug

# Run tests
make test

# Check code quality
make lint
```

### Docker Deployment

```bash
# Build images
docker build -f docker/coordinator/Dockerfile -t codegen-mcp/coordinator:latest .
docker build -f docker/builder/Dockerfile -t codegen-mcp/worker:latest .

# Run with docker-compose
docker-compose up -d
```

### Kubernetes Deployment

```bash
# Create namespace
kubectl create namespace codegen-mcp

# Deploy coordinator
kubectl apply -f k8s/coordinator/

# Verify deployment
kubectl get pods -n codegen-mcp
```

## 🏗️ System Architecture

```mermaid
graph TB
    subgraph "LLM Client Layer"
        LLM[LLM Client<br/>Claude, GPT-4, etc.]
    end
    
    subgraph "Coordinator Layer"
        COORD[Coordinator<br/>MCP Server]
        SESSION[Session Manager]
        AUDIT[Audit Logger]
    end
    
    subgraph "Worker Layer"
        W1[Worker Container 1<br/>Go + Python]
        W2[Worker Container 2<br/>Go + Python]
        WN[Worker Container N<br/>Go + Python]
    end
    
    subgraph "Storage Layer"
        WORKSPACE[Workspace Storage<br/>PVC / S3]
        LOGS[Audit Logs<br/>Database / S3]
    end
    
    LLM -->|stdio/SSE/HTTP| COORD
    COORD --> SESSION
    COORD --> AUDIT
    COORD -->|gRPC/HTTP| W1
    COORD -->|gRPC/HTTP| W2
    COORD -->|gRPC/HTTP| WN
    
    W1 -.->|Workspace| WORKSPACE
    W2 -.->|Workspace| WORKSPACE
    WN -.->|Workspace| WORKSPACE
    
    AUDIT -.->|Persist| LOGS
    
    style COORD fill:#4A90E2
    style SESSION fill:#F5A623
    style W1 fill:#7ED321
    style W2 fill:#7ED321
    style WN fill:#7ED321
```

### Component Flow

1. **LLM Client** sends MCP tool call to Coordinator via stdio/HTTP
2. **Coordinator** validates request, creates/retrieves session
3. **Session Manager** provides workspace isolation context
4. **Coordinator** routes task to available Worker container
5. **Worker** executes tool in isolated Python environment
6. **Worker** returns result to Coordinator
7. **Coordinator** logs to Audit Logger for provenance
8. **Coordinator** returns result to LLM Client via MCP protocol

## 🔧 Core Concepts

### Model Context Protocol (MCP)

CodeGen-MCP implements the [Model Context Protocol](https://spec.modelcontextprotocol.io/), enabling LLMs to:

- Execute code generation workflows
- Read and write files in isolated workspaces
- Install dependencies and run tests
- Maintain session state across interactions

### Workspace Isolation

Each LLM session gets an isolated workspace:

- **Unique WorkspaceID** - Logical boundary for files and operations
- **Path Validation** - Prevents directory traversal and escapes
- **Resource Limits** - CPU, memory, and disk quotas
- **Ephemeral Storage** - Cleaned up after session expires

### Security Model

Multi-layered security approach:

1. **Path Validation** - All file operations validated before execution
2. **Container Isolation** - Workers run in isolated containers
3. **Non-Root Execution** - All processes run as unprivileged users
4. **Audit Logging** - Complete provenance trail of all operations
5. **Network Policies** - Kubernetes network segmentation
6. **Resource Limits** - Prevents resource exhaustion attacks

## 📖 MCP Tools

### Current Tools (v0.1.0)

| Tool | Description | Arguments |
|------|-------------|-----------|
| `echo` | Echo message (test tool) | `message: string` |
| `fs.read` | Read file from workspace | `path: string` |
| `fs.write` | Write file to workspace | `path: string, contents: string` |

### Planned Tools

| Tool | Description | Status |
|------|-------------|--------|
| `run.python` | Execute Python code | ⏳ Planned |
| `pkg.install` | Install Python packages | ⏳ Planned |
| `test.run` | Run test suites | ⏳ Planned |
| `lint.check` | Run linters/type checkers | ⏳ Planned |
| `git.clone` | Clone repository | ⏳ Planned |
| `git.commit` | Commit changes | ⏳ Planned |
| `artifact.create` | Build distributable packages | ⏳ Planned |

## 🔍 Component Status

| Component | Version | Coverage | Tests | Status |
|-----------|---------|----------|-------|--------|
| Coordinator | v0.1.0 | 33.2% (80%+ core) | 51 | ✅ Stable |
| Worker | v0.1.0 | 45.7% | 11 | ✅ Stable |
| Builder Container | - | - | - | ⏳ Planned |
| CI/CD Pipeline | - | - | - | ✅ Active |

## 📊 Quality Metrics

### Test Coverage

- **Target:** 80%+ coverage for core business logic
- **Current:** 
  - Coordinator: 33.2% overall (80%+ core logic, 51 tests)
  - Worker: 45.7% (11 tests)
- **Integration Tests:** ✅ Full end-to-end test suite
- **CI/CD:** Automated testing on every commit

### Linting

- **golangci-lint:** 24+ enabled linters
- **Current Status:** 6 non-blocking gosec warnings (down from 15)
- **Configuration:** `.golangci.yml`

## 🛠️ Development

### Prerequisites

- Go 1.25+
- Docker 20.10+
- Kubernetes 1.28+ (for production)
- make

### Building

```bash
# Build all binaries
make build

# Build specific component
go build -o bin/coordinator ./cmd/coordinator

# Build Docker images
make docker-build
```

### Testing

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Run linting
make lint

# Run specific component tests
go test -v ./internal/coordinator/...
```

### Code Quality

```bash
# Format code
make fmt

# Run all quality checks
make ci
```

## 📝 Contributing

We welcome contributions! Please see:

- [CONTRIBUTING.md](../CONTRIBUTING.md) - Contribution guidelines
- [CODE_OF_CONDUCT.md](../CODE_OF_CONDUCT.md) - Community standards
- [.github/pull_request_template.md](../.github/pull_request_template.md) - PR checklist

### Development Workflow

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Write tests for your changes
4. Ensure all tests pass (`make test`)
5. Lint your code (`make lint`)
6. Commit changes (`git commit -m 'Add amazing feature'`)
7. Push to branch (`git push origin feature/amazing-feature`)
8. Open a Pull Request

## 🔗 Resources

### External Documentation

- [Model Context Protocol Specification](https://spec.modelcontextprotocol.io/)
- [mcp-go Library](https://github.com/mark3labs/mcp-go)
- [Kubernetes Documentation](https://kubernetes.io/docs/)
- [Docker Documentation](https://docs.docker.com/)

### Related Projects

- [PromptKit](https://github.com/AltairaLabs/promptkit-public) - Prompt engineering framework
- [Arena](https://github.com/AltairaLabs/arena) - LLM evaluation platform

## 📧 Support

- **GitHub Issues:** [Report bugs and request features](https://github.com/AltairaLabs/codegen-mcp/issues)
- **GitHub Discussions:** [Ask questions and share ideas](https://github.com/AltairaLabs/codegen-mcp/discussions)
- **Documentation:** You're reading it!

## 📄 License

Apache 2.0 - See [LICENSE](../LICENSE) for details

Copyright © 2025 AltairaLabs.ai

---

**Built with ❤️ by the AltairaLabs team**
