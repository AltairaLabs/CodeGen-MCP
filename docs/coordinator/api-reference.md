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
- [Worker Lifecycle](#worker-lifecycle)
- [Worker Registry](#worker-registry)
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

### `NewRealWorkerClient`

Creates a production worker client that routes tasks to registered workers via gRPC.

```go
func NewRealWorkerClient(registry *WorkerRegistry, sessionMgr *SessionManager) *RealWorkerClient
```

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `registry` | `*WorkerRegistry` | Registry of available workers |
| `sessionMgr` | `*SessionManager` | Session manager for session-to-worker affinity |

**Returns:** Configured `*RealWorkerClient` that routes tasks to workers

**Example:**

```go
registry := coordinator.NewWorkerRegistry()
sessionMgr := coordinator.NewSessionManager()
workerClient := coordinator.NewRealWorkerClient(registry, sessionMgr)
```

**Behavior:**

- Routes tasks to the worker assigned to the session
- Establishes gRPC connection to worker's TaskExecution service
- Streams task execution with real-time logs and progress
- Handles connection errors and worker unavailability
- Returns aggregated task results

## Worker Lifecycle

The Coordinator implements the `WorkerLifecycleServer` gRPC service to manage worker registration, heartbeats, and deregistration.

### Service Definition

```protobuf
service WorkerLifecycle {
  rpc RegisterWorker(RegisterRequest) returns (RegisterResponse);
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
  rpc DeregisterWorker(DeregisterRequest) returns (DeregisterResponse);
}
```

### `RegisterWorker`

Registers a new worker with the coordinator.

```go
func (cs *CoordinatorServer) RegisterWorker(ctx context.Context, 
    req *protov1.RegisterRequest) (*protov1.RegisterResponse, error)
```

**Request:**

```protobuf
message RegisterRequest {
  string worker_id = 1;          // Unique worker identifier
  int32 max_sessions = 2;        // Maximum concurrent sessions
  map<string, string> metadata = 3;  // Worker metadata (region, zone, etc.)
  string grpc_address = 6;       // Worker's gRPC address (planned)
}
```

**Response:**

```protobuf
message RegisterResponse {
  bool success = 1;              // Registration succeeded
  string message = 2;            // Status message
  int32 heartbeat_interval_seconds = 3;  // Required heartbeat interval (30s)
}
```

**Example:**

```go
resp, err := client.RegisterWorker(ctx, &protov1.RegisterRequest{
    WorkerId:   "worker-1",
    MaxSessions: 5,
    Metadata: map[string]string{
        "region": "us-west-2",
        "zone":   "us-west-2a",
    },
})
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Registered, heartbeat every %ds\n", resp.HeartbeatIntervalSeconds)
```

**Behavior:**

- Validates worker_id is not empty
- Allows re-registration (updates existing worker)
- Initializes worker with zero active sessions
- Sets last_heartbeat to current time
- Returns 30-second heartbeat interval

### `Heartbeat`

Updates worker's last activity timestamp and active session count.

```go
func (cs *CoordinatorServer) Heartbeat(ctx context.Context, 
    req *protov1.HeartbeatRequest) (*protov1.HeartbeatResponse, error)
```

**Request:**

```protobuf
message HeartbeatRequest {
  string worker_id = 1;              // Worker identifier
  int32 active_sessions = 2;         // Current session count
  repeated string session_ids = 3;   // Active session IDs
}
```

**Response:**

```protobuf
message HeartbeatResponse {
  bool acknowledged = 1;         // Heartbeat received
  string message = 2;            // Status message
}
```

**Example:**

```go
resp, err := client.Heartbeat(ctx, &protov1.HeartbeatRequest{
    WorkerId:       "worker-1",
    ActiveSessions: 3,
    SessionIds:     []string{"sess-1", "sess-2", "sess-3"},
})
if err != nil {
    log.Printf("Heartbeat failed: %v", err)
}
```

**Behavior:**

- Returns error if worker not registered
- Updates worker's last_heartbeat timestamp
- Updates worker's active_sessions count
- Logs invalid session IDs (sessions not in SessionManager)
- Used by cleanup loop to detect stale workers

### `DeregisterWorker`

Removes a worker from the registry.

```go
func (cs *CoordinatorServer) DeregisterWorker(ctx context.Context, 
    req *protov1.DeregisterRequest) (*protov1.DeregisterResponse, error)
```

**Request:**

```protobuf
message DeregisterRequest {
  string worker_id = 1;              // Worker to deregister
  repeated string session_ids = 2;   // Sessions to destroy (optional)
}
```

**Response:**

```protobuf
message DeregisterResponse {
  bool success = 1;              // Deregistration succeeded
  string message = 2;            // Status message
}
```

**Example:**

```go
resp, err := client.DeregisterWorker(ctx, &protov1.DeregisterRequest{
    WorkerId:   "worker-1",
    SessionIds: []string{"sess-1", "sess-2"},
})
if err != nil {
    log.Printf("Deregistration failed: %v", err)
}
```

**Behavior:**

- Returns error if worker not found
- Removes worker from registry
- Destroys specified sessions from SessionManager
- Logs invalid session IDs
- Used during graceful worker shutdown

### `StartCleanupLoop`

Starts a background goroutine that removes stale workers.

```go
func (cs *CoordinatorServer) StartCleanupLoop(ctx context.Context, interval time.Duration)
```

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `ctx` | `context.Context` | Context for cancellation |
| `interval` | `time.Duration` | Cleanup check interval |

**Example:**

```go
// Start cleanup loop that checks every minute
// Removes workers with no heartbeat for 5 minutes
ctx := context.Background()
server.StartCleanupLoop(ctx, 1*time.Minute)
```

**Behavior:**

- Runs in background goroutine
- Wakes up every `interval` duration
- Removes workers with no heartbeat for 5 minutes
- Stops when context is cancelled
- Logs each cleanup operation

## Worker Registry

Manages the pool of available workers and provides capacity-aware worker selection.

### `NewWorkerRegistry`

Creates a new worker registry with empty worker map.

```go
func NewWorkerRegistry() *WorkerRegistry
```

**Returns:** Initialized `*WorkerRegistry`

**Example:**

```go
registry := coordinator.NewWorkerRegistry()
```

### `RegisterWorker`

Adds or updates a worker in the registry.

```go
func (wr *WorkerRegistry) RegisterWorker(workerID string, maxSessions int32, 
    metadata map[string]string) error
```

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `workerID` | `string` | Unique worker identifier |
| `maxSessions` | `int32` | Maximum concurrent sessions |
| `metadata` | `map[string]string` | Custom worker metadata |

**Returns:** Error if workerID is empty

**Example:**

```go
err := registry.RegisterWorker("worker-1", 5, map[string]string{
    "region": "us-west-2",
})
```

### `GetWorker`

Retrieves a worker by ID.

```go
func (wr *WorkerRegistry) GetWorker(workerID string) (*RegisteredWorker, bool)
```

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `workerID` | `string` | Worker identifier to retrieve |

**Returns:**

- `*RegisteredWorker` - The worker if found
- `bool` - true if worker exists, false otherwise

**Example:**

```go
if worker, ok := registry.GetWorker("worker-1"); ok {
    fmt.Printf("Worker has %d active sessions\n", worker.ActiveSessions)
}
```

### `FindWorkerWithCapacity`

Finds a worker with available session capacity.

```go
func (wr *WorkerRegistry) FindWorkerWithCapacity() (*RegisteredWorker, error)
```

**Returns:**

- `*RegisteredWorker` - Worker with available capacity
- `error` - Error if no workers available

**Example:**

```go
worker, err := registry.FindWorkerWithCapacity()
if err != nil {
    log.Printf("No workers available: %v", err)
    return
}
fmt.Printf("Selected worker: %s\n", worker.WorkerID)
```

**Behavior:**

- Returns first worker where `ActiveSessions < MaxSessions`
- Thread-safe with read lock
- Returns error if no workers have capacity
- Used by SessionManager when assigning workers to sessions

### `UpdateHeartbeat`

Updates a worker's last heartbeat and active session count.

```go
func (wr *WorkerRegistry) UpdateHeartbeat(workerID string, 
    activeSessions int32) error
```

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `workerID` | `string` | Worker to update |
| `activeSessions` | `int32` | Current active session count |

**Returns:** Error if worker not found

**Example:**

```go
err := registry.UpdateHeartbeat("worker-1", 3)
if err != nil {
    log.Printf("Heartbeat update failed: %v", err)
}
```

### `DeregisterWorker`

Removes a worker from the registry.

```go
func (wr *WorkerRegistry) DeregisterWorker(workerID string) error
```

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `workerID` | `string` | Worker to remove |

**Returns:** Error if worker not found

**Example:**

```go
err := registry.DeregisterWorker("worker-1")
if err != nil {
    log.Printf("Worker not found: %v", err)
}
```

### `RemoveStaleWorkers`

Removes workers that haven't sent heartbeat within timeout.

```go
func (wr *WorkerRegistry) RemoveStaleWorkers(timeout time.Duration) int
```

**Parameters:**

| Name | Type | Description |
|------|------|-------------|
| `timeout` | `time.Duration` | Maximum time since last heartbeat |

**Returns:** Number of workers removed

**Example:**

```go
// Remove workers with no heartbeat for 5 minutes
removed := registry.RemoveStaleWorkers(5 * time.Minute)
log.Printf("Removed %d stale workers\n", removed)
```

### `ListWorkers`

Returns a list of all registered workers.

```go
func (wr *WorkerRegistry) ListWorkers() []*RegisteredWorker
```

**Returns:** Slice of all registered workers

**Example:**

```go
workers := registry.ListWorkers()
for _, worker := range workers {
    fmt.Printf("Worker %s: %d/%d sessions\n", 
        worker.WorkerID, 
        worker.ActiveSessions, 
        worker.MaxSessions)
}
```

### `WorkerCount`

Returns the number of registered workers.

```go
func (wr *WorkerRegistry) WorkerCount() int
```

**Returns:** Count of registered workers

**Example:**

```go
count := registry.WorkerCount()
fmt.Printf("Total workers: %d\n", count)
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

### `RegisteredWorker`

Represents a registered worker in the WorkerRegistry.

```go
type RegisteredWorker struct {
    WorkerID       string
    MaxSessions    int32
    ActiveSessions int32
    LastHeartbeat  time.Time
    Metadata       map[string]string
}
```

**Field Details:**

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `WorkerID` | `string` | Unique worker identifier | `"worker-1"` |
| `MaxSessions` | `int32` | Maximum concurrent sessions | `5` |
| `ActiveSessions` | `int32` | Current active session count | `3` |
| `LastHeartbeat` | `time.Time` | Last heartbeat timestamp | `2025-11-08T12:05:30Z` |
| `Metadata` | `map[string]string` | Custom worker metadata | `{"region": "us-west-2"}` |

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

## Worker Registration Flow

### Typical Registration Sequence

1. **Worker starts** and initializes gRPC server
2. **Worker calls RegisterWorker** with worker_id, max_sessions, metadata
3. **Coordinator responds** with heartbeat_interval_seconds (30s)
4. **Worker starts heartbeat loop** sending heartbeat every 30s
5. **Coordinator updates** worker's last_heartbeat and active_sessions
6. **On shutdown, worker calls DeregisterWorker** with session_ids to destroy
7. **Coordinator removes** worker from registry and cleans up sessions

### Session Assignment Flow

1. **Client calls CreateSession** on coordinator
2. **SessionManager calls FindWorkerWithCapacity** on WorkerRegistry
3. **Registry returns** worker with available capacity
4. **SessionManager assigns** worker to session
5. **SessionManager calls CreateSession** on worker via gRPC
6. **Worker creates** isolated workspace and Python environment
7. **Session is ready** for task execution

### Task Routing Flow

1. **Client calls tool** via MCP protocol
2. **MCP Server calls ExecuteTask** on WorkerClient
3. **RealWorkerClient looks up** session's assigned worker
4. **RealWorkerClient establishes** gRPC connection to worker
5. **Worker executes task** in session workspace
6. **Worker streams** logs and progress back
7. **RealWorkerClient aggregates** result and returns to MCP Server

## See Also

- [Coordinator Architecture](./README.md) - High-level overview and diagrams
- [Deployment Guide](./deployment.md) - Production deployment patterns
- [Testing Guide](./testing.md) - How to test coordinator components
- [Worker API Reference](../worker/api-reference.md) - Worker-side APIs
