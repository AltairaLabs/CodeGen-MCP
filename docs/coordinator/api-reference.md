---
title: Coordinator API Reference
layout: default
---

# Coordinator API Reference

Complete reference for the Coordinator's internal APIs, types, and interfaces.

## Table of Contents

- [MCP Server](#mcp-server)
- [Session Manager](#session-manager)
- [Worker Client](#worker-client)
- [Audit Logger](#audit-logger)
- [Types](#types)

## MCP Server

### `NewMCPServer`

Creates and initializes a new MCP server instance.

```go
func NewMCPServer(cfg Config, sessionMgr *SessionManager, 
                  worker WorkerClient, audit *AuditLogger) *MCPServer
```

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `cfg` | `Config` | Server configuration (name, version) |
| `sessionMgr` | `*SessionManager` | Session lifecycle manager |
| `worker` | `WorkerClient` | Worker communication interface |
| `audit` | `*AuditLogger` | Audit logging implementation |

**Returns:** Configured `*MCPServer` ready to serve

**Example:**

```go
cfg := coordinator.Config{
    Name:    "codegen-mcp-coordinator",
    Version: "0.1.0",
}
sessionMgr := coordinator.NewSessionManager()
worker := coordinator.NewMockWorkerClient()
audit := coordinator.NewAuditLogger(logger)

server := coordinator.NewMCPServer(cfg, sessionMgr, worker, audit)
```

### `Serve`

Starts the MCP server with stdio transport.

```go
func (ms *MCPServer) Serve() error
```

**Returns:** Error if server fails to start or encounters fatal error

**Example:**

```go
if err := server.Serve(); err != nil {
    log.Fatal("Server failed:", err)
}
```

### `ServeWithLogger`

Starts the MCP server with custom logger.

```go
func (ms *MCPServer) ServeWithLogger(logger *slog.Logger) error
```

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `logger` | `*slog.Logger` | Structured logger instance |

**Returns:** Error if server fails to start

## Session Manager

### `NewSessionManager`

Creates a new session manager with empty session map.

```go
func NewSessionManager() *SessionManager
```

**Returns:** Initialized `*SessionManager`

**Example:**

```go
sessionMgr := coordinator.NewSessionManager()
```

### `CreateSession`

Creates a new session for an MCP client.

```go
func (sm *SessionManager) CreateSession(ctx context.Context, 
                                        sessionID, userID, 
                                        workspaceID string) *Session
```

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `ctx` | `context.Context` | Request context |
| `sessionID` | `string` | Unique session identifier |
| `userID` | `string` | User/client identifier |
| `workspaceID` | `string` | Workspace isolation boundary |

**Returns:** Created `*Session` with timestamps initialized

**Example:**

```go
session := sessionMgr.CreateSession(
    ctx, 
    "sess-abc123", 
    "user-xyz", 
    "workspace-001",
)
```

### `GetSession`

Retrieves an existing session and updates LastActive timestamp.

```go
func (sm *SessionManager) GetSession(sessionID string) (*Session, bool)
```

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `sessionID` | `string` | Session identifier to retrieve |

**Returns:**
- `*Session` - The session if found
- `bool` - true if session exists, false otherwise

**Example:**

```go
if session, ok := sessionMgr.GetSession("sess-abc123"); ok {
    fmt.Printf("Session workspace: %s\n", session.WorkspaceID)
}
```

### `DeleteSession`

Removes a session from the manager.

```go
func (sm *SessionManager) DeleteSession(sessionID string)
```

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `sessionID` | `string` | Session identifier to delete |

**Example:**

```go
sessionMgr.DeleteSession("sess-abc123")
```

### `CleanupStale`

Removes sessions inactive longer than specified duration.

```go
func (sm *SessionManager) CleanupStale(maxAge time.Duration) int
```

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `maxAge` | `time.Duration` | Maximum inactivity before cleanup |

**Returns:** Number of sessions deleted

**Example:**

```go
// Clean up sessions older than 30 minutes
deleted := sessionMgr.CleanupStale(30 * time.Minute)
log.Printf("Cleaned up %d stale sessions\n", deleted)
```

### `SessionCount`

Returns the current number of active sessions.

```go
func (sm *SessionManager) SessionCount() int
```

**Returns:** Active session count

**Example:**

```go
count := sessionMgr.SessionCount()
fmt.Printf("Active sessions: %d\n", count)
```

## Worker Client

### Interface Definition

```go
type WorkerClient interface {
    ExecuteTask(ctx context.Context, workspaceID, toolName string, 
                args TaskArgs) (*TaskResult, error)
}
```

### `ExecuteTask`

Sends a task to a worker and returns the execution result.

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `ctx` | `context.Context` | Request context with timeout/cancellation |
| `workspaceID` | `string` | Target workspace identifier |
| `toolName` | `string` | Tool to execute (e.g., "fs.read") |
| `args` | `TaskArgs` | Tool arguments as map |

**Returns:**
- `*TaskResult` - Execution result with output/error
- `error` - Communication or validation error

**Example:**

```go
result, err := worker.ExecuteTask(
    ctx,
    "workspace-001",
    "fs.read",
    coordinator.TaskArgs{"path": "src/main.py"},
)
if err != nil {
    log.Printf("Task failed: %v", err)
    return
}
fmt.Println(result.Output)
```

### `NewMockWorkerClient`

Creates a mock worker client for testing.

```go
func NewMockWorkerClient() *MockWorkerClient
```

**Returns:** Mock implementation that simulates task execution

**Example:**

```go
mockWorker := coordinator.NewMockWorkerClient()
// Use in tests or local development
```

## Audit Logger

### `NewAuditLogger`

Creates a new audit logger with structured logging backend.

```go
func NewAuditLogger(logger *slog.Logger) *AuditLogger
```

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `logger` | `*slog.Logger` | Structured logger for audit output |

**Returns:** Configured `*AuditLogger`

**Example:**

```go
logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
audit := coordinator.NewAuditLogger(logger)
```

### `LogToolCall`

Logs the initiation of a tool call.

```go
func (al *AuditLogger) LogToolCall(ctx context.Context, entry *AuditEntry)
```

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `ctx` | `context.Context` | Request context |
| `entry` | `*AuditEntry` | Audit entry with call details |

**Example:**

```go
audit.LogToolCall(ctx, &coordinator.AuditEntry{
    SessionID:   "sess-abc123",
    UserID:      "user-xyz",
    ToolName:    "fs.read",
    Arguments:   coordinator.TaskArgs{"path": "src/main.py"},
    WorkspaceID: "workspace-001",
})
```

### `LogToolResult`

Logs the completion of a tool call.

```go
func (al *AuditLogger) LogToolResult(ctx context.Context, entry *AuditEntry)
```

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `ctx` | `context.Context` | Request context |
| `entry` | `*AuditEntry` | Audit entry with result or error |

**Example:**

```go
audit.LogToolResult(ctx, &coordinator.AuditEntry{
    SessionID: "sess-abc123",
    ToolName:  "fs.read",
    Result: &coordinator.TaskResult{
        Success:  true,
        Output:   "file contents here",
        Duration: 150 * time.Millisecond,
    },
})
```

## Types

### `Config`

Server configuration structure.

```go
type Config struct {
    Name    string  // Service identifier (e.g., "codegen-mcp-coordinator")
    Version string  // Semantic version (e.g., "0.1.0")
}
```

### `Session`

Represents an MCP client session with isolation.

```go
type Session struct {
    ID          string              // Unique session identifier
    WorkspaceID string              // Workspace isolation boundary
    UserID      string              // User/client identifier
    CreatedAt   time.Time           // Session creation timestamp
    LastActive  time.Time           // Last activity timestamp
    Metadata    map[string]string   // Custom session metadata
}
```

**Field Details:**

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `ID` | `string` | Session UUID or identifier | `"sess-abc123"` |
| `WorkspaceID` | `string` | Logical workspace boundary | `"workspace-001"` |
| `UserID` | `string` | Client/user identifier | `"user-xyz"` |
| `CreatedAt` | `time.Time` | When session was created | `2025-11-08T12:00:00Z` |
| `LastActive` | `time.Time` | Last tool call timestamp | `2025-11-08T12:05:30Z` |
| `Metadata` | `map[string]string` | Custom key-value pairs | `{"region": "us-west-2"}` |

### `TaskArgs`

Type alias for task argument maps.

```go
type TaskArgs map[string]interface{}
```

**Example:**

```go
args := coordinator.TaskArgs{
    "path":     "src/main.py",
    "contents": "print('hello')",
    "mode":     0644,
}
```

### `TaskResult`

Represents the execution result from a worker.

```go
type TaskResult struct {
    Success  bool          // Whether task completed successfully
    Output   string        // Standard output from task
    Error    string        // Error message if failed
    ExitCode int           // Process exit code (0 = success)
    Duration time.Duration // Execution time
}
```

**Field Details:**

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `Success` | `bool` | Task completion status | `true` |
| `Output` | `string` | stdout or result data | `"Test passed"` |
| `Error` | `string` | Error message if any | `"file not found"` |
| `ExitCode` | `int` | Unix exit code | `0` (success), `1` (error) |
| `Duration` | `time.Duration` | Execution duration | `150ms` |

### `AuditEntry`

Audit log entry for provenance tracking.

```go
type AuditEntry struct {
    Timestamp   time.Time              // When event occurred
    SessionID   string                 // Associated session
    UserID      string                 // User who initiated
    ToolName    string                 // Tool that was called
    Arguments   map[string]interface{} // Tool arguments
    Result      *TaskResult            // Execution result (if completed)
    ErrorMsg    string                 // Error message (if failed)
    TraceID     string                 // Distributed tracing ID
    WorkspaceID string                 // Target workspace
}
```

**Usage in Audit Trail:**

```go
// Log tool call
entry := &coordinator.AuditEntry{
    Timestamp:   time.Now(),
    SessionID:   "sess-abc123",
    UserID:      "user-xyz",
    ToolName:    "fs.write",
    Arguments:   args,
    WorkspaceID: "workspace-001",
}
audit.LogToolCall(ctx, entry)

// Later, log result
entry.Result = result
entry.Timestamp = time.Now()
audit.LogToolResult(ctx, entry)
```

## Error Handling

### Common Errors

**Path Validation Errors:**

```go
// Absolute path
err := fmt.Errorf("path must be relative to workspace root, got absolute path: %s", path)

// Path traversal
err := fmt.Errorf("path traversal not allowed: %s", path)
```

**Session Errors:**

```go
// Session not found
if session, ok := sessionMgr.GetSession(id); !ok {
    // Create default session or return error
}
```

**Worker Errors:**

```go
// Task execution failure
result, err := worker.ExecuteTask(ctx, workspace, tool, args)
if err != nil {
    return mcp.NewToolResultError(err.Error()), nil
}

// Task completed with error
if !result.Success {
    return mcp.NewToolResultError(result.Error), nil
}
```

## Thread Safety

All public APIs are thread-safe:

- **SessionManager:** Uses `sync.RWMutex` for concurrent access
- **AuditLogger:** Thread-safe via slog's internal synchronization
- **MCPServer:** Request handlers are concurrent-safe

## Performance Considerations

### Session Management

- `GetSession()` updates LastActive timestamp - requires write lock
- `SessionCount()` uses read lock - very fast
- `CleanupStale()` iterates all sessions - O(n) operation

### Memory Management

- Sessions stored in-memory - plan for ~1KB per session
- Automatic cleanup prevents unbounded growth
- Consider external session store for high-scale deployments

### Concurrency

- Tool handlers execute concurrently (one per request)
- Worker client should implement connection pooling
- No blocking operations in hot path (logging is async)

## See Also

- [Coordinator Architecture](./README.md) - High-level overview and diagrams
- [Deployment Guide](./deployment.md) - Production deployment patterns
- [Testing Guide](./testing.md) - How to test coordinator components
