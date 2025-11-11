# Contributing to CodeGen MCP

Thank you for your interest in contributing to CodeGen MCP! This document provides comprehensive guidelines and instructions for contributing to our open source project.

## Code of Conduct

This project adheres to the [Contributor Covenant Code of Conduct](./CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code. Please report unacceptable behavior to [conduct@altairalabs.ai](mailto:conduct@altairalabs.ai).

## Developer Certificate of Origin (DCO)

This project uses the Developer Certificate of Origin (DCO) to ensure that contributors have the right to submit their contributions. By making a contribution to this project, you certify that:

1. The contribution was created in whole or in part by you and you have the right to submit it under the open source license indicated in the file; or
2. The contribution is based upon previous work that, to the best of your knowledge, is covered under an appropriate open source license and you have the right under that license to submit that work with modifications, whether created in whole or in part by you, under the same open source license (unless you are permitted to submit under a different license), as indicated in the file; or
3. The contribution was provided directly to you by some other person who certified (1), (2) or (3) and you have not modified it.

### Signing Your Commits

To sign off on your commits, add the `-s` flag to your git commit command:

```bash
git commit -s -m "Your commit message"
```

This adds a "Signed-off-by" line to your commit message:

```
Signed-off-by: Your Name <your.email@example.com>
```

## How to Contribute

### Reporting Bugs

- Check existing issues first
- Provide clear reproduction steps
- Include version information
- Share relevant configuration/code samples

### Suggesting Features

- Open an issue describing the feature
- Explain the use case and benefits
- Discuss implementation approach

### Submitting Changes

1. **Fork the repository**
2. **Create a feature branch**: `git checkout -b feature/your-feature-name`
3. **Make your changes**
4. **Write/update tests**
5. **Run tests**: `make test`
6. **Run linter**: `make lint`
7. **Commit your changes**: Use clear, descriptive commit messages
8. **Push to your fork**: `git push origin feature/your-feature-name`
9. **Open a Pull Request**

## Development Setup

### Prerequisites

- Go 1.25 or later
- Make (for build automation)
- Docker (for worker sandboxes)

### Setup

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/CodeGen-MCP.git
cd CodeGen-MCP

# Install dependencies
make install

# Run tests
make test

# Run linter
make lint

# Build all components
make build
```

### Project Structure

```
CodeGen-MCP/
├── cmd/
│   ├── coordinator/     # Coordinator service entry point
│   └── worker/          # Worker agent entry point
├── internal/
│   ├── coordinator/     # Coordinator implementation
│   │   ├── server.go    # MCP server
│   │   ├── session.go   # Session management
│   │   ├── worker.go    # Worker client
│   │   └── audit.go     # Audit logging
│   ├── worker/          # Worker implementation
│   ├── mcp/             # MCP protocol schemas
│   └── sandbox/         # Sandbox runtime
├── docker/
│   └── builder/         # Docker builder for workers
├── docs/                # Documentation
│   ├── coordinator/     # Coordinator architecture docs
│   └── README.md        # Documentation index
├── bin/                 # Compiled binaries
├── RFCs/                # Architecture RFCs
├── tools.go             # Go tools dependencies
└── Makefile             # Build automation
```

## Component-Specific Contribution Guidelines

### Coordinator (`internal/coordinator/`)

**Focus**: Control plane for managing sessions and workers

**Key Areas for Contribution:**
- MCP server and protocol improvements
- Session management and isolation
- Worker pool orchestration and health monitoring
- Audit logging and observability
- Security and access control
- Performance optimizations for concurrent sessions

**Testing Coordinator Changes:**
```bash
# Build Coordinator
make build

# Run Coordinator tests
cd internal/coordinator && go test ./...

# Run integration tests
cd internal/coordinator && go test -run Integration ./...

# Start Coordinator locally
./bin/coordinator
```

### Worker (`internal/worker/`)

**Focus**: Data plane for executing code in sandboxes

**Key Areas for Contribution:**
- Sandbox runtime improvements
- Docker integration and optimization
- Tool execution framework (file operations, code execution)
- Resource management and cleanup
- Security and isolation enhancements

**Testing Worker Changes:**
```bash
# Build Worker
make build

# Run Worker tests
cd internal/worker && go test ./...

# Test with Docker
docker build -f docker/builder/Dockerfile -t codegen-worker .
docker run codegen-worker
```

### MCP Protocol (`internal/mcp/`)

**Focus**: Model Context Protocol schemas and tooling

**Key Areas for Contribution:**
- MCP tool definitions and schemas
- Protocol validation
- Documentation and examples
- New tool types and capabilities

**Testing MCP Changes:**
```bash
# Validate schemas
cd internal/mcp/schema && make validate

# Run protocol tests
cd internal/mcp && go test ./...
```

### Sandbox (`internal/sandbox/`)

**Focus**: Isolated execution environments

**Key Areas for Contribution:**
- Language runtime support (Python, Node.js, Go, etc.)
- Filesystem isolation improvements
- Resource limits and monitoring
- Security hardening
- Performance optimizations

**Testing Sandbox Changes:**
```bash
# Run sandbox tests
cd internal/sandbox && go test ./...

# Test with various runtimes
./bin/worker --runtime python
./bin/worker --runtime nodejs
```

## Coding Guidelines

### Go Code Style

- Follow standard Go conventions
- Use `gofmt` for formatting (included in `make fmt`)
- Write clear, descriptive variable/function names
- Add package-level documentation comments
- Keep functions focused and testable

### Testing

- Write unit tests for new functionality
- Maintain test coverage above 80%
- Use table-driven tests where appropriate
- Mock external dependencies (workers, storage)
- Write integration tests for complex workflows
- Test concurrent scenarios and race conditions

#### Code Coverage Requirements

This project maintains code coverage requirements to ensure code quality:

- **Minimum Coverage**: 80% for all production code
- **Coverage Generation**: Use `make coverage` to generate reports
- **Coverage Exclusions**: The following files are excluded from coverage requirements:
  - Generated protobuf files: `**/*.pb.go`, `**/*_grpc.pb.go`
  - Application entry points: `cmd/*/main.go`
  - Infrastructure coordination code:
    - `**/*_serve.go`: HTTP/gRPC server startup code
    - `**/*_streams.go`: Bidirectional gRPC stream handling
    - `**/*_loops.go`: Infinite event loops
    - `**/*_dispatch.go`: Goroutine/channel orchestration
    - `**/*_integration.go`: Integration test helpers
  - Example and test files: `**/examples/**`, `**/*_test.go`

These exclusions are defined in `sonar-project.properties` under `sonar.coverage.exclusions`.

#### Running Coverage Tests

```bash
# Generate coverage report for unit tests only
make coverage

# View coverage report in browser
open coverage.html

# Run unit tests with coverage output
make test-unit

# Check coverage percentage
go tool cover -func=coverage.out | grep "^total:"
```

**Note**: Coverage excludes integration tests and infrastructure code that's difficult to unit test meaningfully.

### Documentation

- Update README.md if adding features
- Add inline comments for complex logic
- Update component-specific docs in `docs/`
- Add package documentation for new packages
- Include Mermaid diagrams for architecture changes
- Update RFCs for significant design decisions

## Pull Request Process

1. **Ensure CI passes** - All tests and linter checks must pass
2. **Update documentation** - README, examples, inline docs
3. **Add changelog entry** - Describe your changes
4. **Request review** - Tag maintainers (see `.github/CODEOWNERS`)
5. **Address feedback** - Respond to review comments
6. **Resolve all conversations** - All review comments must be marked as resolved
7. **Sign commits** - Use `git commit -s` for DCO compliance
8. **Keep branch updated** - Rebase or merge with latest `main`
9. **Squash merge** - Maintains clean commit history (preferred)

**Note**: The `main` branch is protected. See [Branch Protection Guide](docs/devops/branch-protection.md) and [Quick Reference](docs/devops/branch-protection-quickref.md) for details.

## Release Process

Maintainers handle releases:

1. Update version numbers
2. Update CHANGELOG.md
3. Create git tag
4. Build and test release artifacts
5. Publish to GitHub releases

## Questions?

- Open a GitHub issue for questions
- Check existing documentation
- Review closed issues and PRs

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.
