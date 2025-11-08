---
title: Worker API Reference
layout: default
---

# Worker API Reference

Complete API documentation for the CodeGen-MCP Worker component.

## Table of Contents

- [gRPC Services](#grpc-services)
  - [SessionManagement](#sessionmanagement-service)
  - [TaskExecution](#taskexecution-service)
  - [ArtifactService](#artifactservice-service)
- [Core Types](#core-types)
- [Tool Handlers](#tool-handlers)
- [Error Codes](#error-codes)
- [Usage Examples](#usage-examples)

## gRPC Services

The Worker exposes three main gRPC services defined in `api/proto/v1/*.proto`.

### WorkerLifecycle Client

Workers implement a **client** for the `WorkerLifecycle` gRPC service to register with the coordinator.

#### Registration Flow

Workers register with the coordinator at startup and maintain the connection through heartbeats.

**Registration Client Initialization:**

```go
import (
    protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
    "google.golang.org/grpc"
)

// Connect to coordinator
conn, err := grpc.Dial(coordinatorAddr, grpc.WithInsecure())
if err != nil {
    log.Fatal(err)
}
defer conn.Close()

client := protov1.NewWorkerLifecycleClient(conn)

// Create registration client
regClient := worker.NewRegistrationClient(
    client,
    workerID,
    maxSessions,
    sessionPool,
    logger,
)

// Start registration and heartbeat loop
ctx := context.Background()
if err := regClient.Register(ctx); err != nil {
    log.Fatal(err)
}
```

#### RegisterWorker

Registers the worker with the coordinator at startup.

**Request:**

```protobuf
message RegisterRequest {
  string worker_id = 1;          // Unique worker identifier
  int32 max_sessions = 2;        // Maximum concurrent sessions
  map<string, string> metadata = 3;  // Worker metadata
  string grpc_address = 6;       // Worker's gRPC address (planned)
}
```

**Response:**

```protobuf
message RegisterResponse {
  bool success = 1;              // Registration succeeded
  string message = 2;            // Status message
  int32 heartbeat_interval_seconds = 3;  // Required heartbeat interval
}
```

**Example:**

```go
func (rc *RegistrationClient) Register(ctx context.Context) error {
    resp, err := rc.client.RegisterWorker(ctx, &protov1.RegisterRequest{
        WorkerId:    rc.workerID,
        MaxSessions: rc.maxSessions,
        Metadata: map[string]string{
            "version": "0.1.0",
            "region":  "us-west-2",
        },
    })
    if err != nil {
        return fmt.Errorf("registration failed: %w", err)
    }
    
    rc.logger.Info("Registered with coordinator",
        "heartbeat_interval", resp.HeartbeatIntervalSeconds)
    
    // Start heartbeat loop
    go rc.heartbeatLoop(ctx, resp.HeartbeatIntervalSeconds)
    
    return nil
}
```

#### Heartbeat Loop

Workers send periodic heartbeats to maintain registration.

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
func (rc *RegistrationClient) heartbeatLoop(ctx context.Context, intervalSeconds int32) {
    ticker := time.NewTicker(time.Duration(intervalSeconds) * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            // Get current session count from pool
            activeSessions, sessionIDs := rc.sessionPool.GetActiveSessions()
            
            resp, err := rc.client.Heartbeat(ctx, &protov1.HeartbeatRequest{
                WorkerId:       rc.workerID,
                ActiveSessions: int32(activeSessions),
                SessionIds:     sessionIDs,
            })
            if err != nil {
                rc.logger.Error("Heartbeat failed", "error", err)
                continue
            }
            
            rc.logger.Debug("Heartbeat sent",
                "active_sessions", activeSessions,
                "acknowledged", resp.Acknowledged)
        }
    }
}
```

#### DeregisterWorker

Workers deregister gracefully on shutdown.

**Request:**

```protobuf
message DeregisterRequest {
  string worker_id = 1;              // Worker to deregister
  repeated string session_ids = 2;   // Sessions to destroy
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
func (rc *RegistrationClient) Deregister(ctx context.Context) error {
    // Get all active session IDs
    _, sessionIDs := rc.sessionPool.GetActiveSessions()
    
    resp, err := rc.client.DeregisterWorker(ctx, &protov1.DeregisterRequest{
        WorkerId:   rc.workerID,
        SessionIds: sessionIDs,
    })
    if err != nil {
        return fmt.Errorf("deregistration failed: %w", err)
    }
    
    rc.logger.Info("Deregistered from coordinator",
        "success", resp.Success,
        "message", resp.Message)
    
    return nil
}
```

#### Graceful Shutdown

Workers deregister and clean up resources on shutdown signals.

**Example:**

```go
func main() {
    // ... initialize worker, session pool, registration client ...
    
    // Setup signal handling
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    
    // Wait for shutdown signal
    <-sigChan
    logger.Info("Shutdown signal received, deregistering...")
    
    // Deregister from coordinator
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    if err := regClient.Deregister(ctx); err != nil {
        logger.Error("Failed to deregister", "error", err)
    }
    
    // Stop gRPC server
    grpcServer.GracefulStop()
    
    logger.Info("Worker shutdown complete")
}
```

### SessionManagement Service

Manages session lifecycle, capacity, and checkpointing.

#### CreateSession

Create a new isolated session with workspace and Python environment.

**Request:**
```protobuf
message CreateSessionRequest {
  string workspace_id = 1;   // Workspace identifier
  string user_id = 2;        // User identifier (optional)
  SessionConfig config = 3;  // Session configuration (optional)
}

message SessionConfig {
  string language = 1;              // "python" | "node" | "go"
  ResourceLimits limits = 2;        // Resource constraints
  map<string, string> env = 3;      // Environment variables
}

message ResourceLimits {
  int32 max_memory_mb = 1;     // Memory limit in MB
  int32 max_cpu_percent = 2;   // CPU percentage limit
  int32 max_disk_mb = 3;       // Disk space limit in MB
  int32 timeout_seconds = 4;   // Task timeout in seconds
}
```

**Response:**
```protobuf
message CreateSessionResponse {
  string session_id = 1;       // Generated session ID
  string worker_id = 2;        // Worker identifier
  SessionState state = 3;      // Current state (READY)
  int64 created_at_ms = 4;     // Creation timestamp
}
```

**Example:**
```go
client := protov1.NewSessionManagementClient(conn)

resp, err := client.CreateSession(ctx, &protov1.CreateSessionRequest{
    WorkspaceId: "project-123",
    UserId:      "user-456",
    Config: &protov1.SessionConfig{
        Language: "python",
        Limits: &protov1.ResourceLimits{
            MaxMemoryMb:    512,
            MaxCpuPercent:  80,
            TimeoutSeconds: 300,
        },
    },
})
```

#### DestroySession

Destroy a session and clean up its workspace.

**Request:**
```protobuf
message DestroySessionRequest {
  string session_id = 1;          // Session to destroy
  bool create_checkpoint = 2;     // Create checkpoint before destroy
}
```

**Response:**
```protobuf
message DestroySessionResponse {
  bool success = 1;               // Whether destruction succeeded
  string checkpoint_id = 2;       // Created checkpoint ID (if requested)
}
```

**Example:**
```go
resp, err := client.DestroySession(ctx, &protov1.DestroySessionRequest{
    SessionId:        "session-abc123",
    CreateCheckpoint: true,
})
```

#### GetSessionStatus

Get current status of a session.

**Request:**
```protobuf
message SessionStatusRequest {
  string session_id = 1;    // Session to query
}
```

**Response:**
```protobuf
message SessionStatusResponse {
  string session_id = 1;
  string workspace_id = 2;
  string user_id = 3;
  SessionState state = 4;           // CREATING, READY, ACTIVE, etc.
  int64 created_at_ms = 5;
  int64 last_activity_ms = 6;
  ResourceMetrics usage = 7;        // Current resource usage
  int32 active_tasks = 8;
  repeated string recent_tasks = 9;
  string last_checkpoint_id = 10;
  repeated string artifacts = 11;
}

message ResourceMetrics {
  int64 memory_bytes = 1;     // Current memory usage
  float cpu_percent = 2;      // Current CPU usage
  int64 disk_bytes = 3;       // Current disk usage
}

enum SessionState {
  SESSION_STATE_UNSPECIFIED = 0;
  SESSION_STATE_CREATING = 1;
  SESSION_STATE_READY = 2;
  SESSION_STATE_ACTIVE = 3;
  SESSION_STATE_CHECKPOINTING = 4;
  SESSION_STATE_TERMINATING = 5;
  SESSION_STATE_FAILED = 6;
}
```

**Example:**
```go
resp, err := client.GetSessionStatus(ctx, &protov1.SessionStatusRequest{
    SessionId: "session-abc123",
})

fmt.Printf("State: %v, Active Tasks: %d\n", resp.State, resp.ActiveTasks)
```

#### CheckpointSession

Create a checkpoint of a session's workspace.

**Request:**
```protobuf
message CheckpointRequest {
  string session_id = 1;      // Session to checkpoint
  bool incremental = 2;       // Incremental checkpoint (future)
}
```

**Response:**
```protobuf
message CheckpointResponse {
  string checkpoint_id = 1;         // Checkpoint identifier
  int64 size_bytes = 2;             // Archive size
  string storage_url = 3;           // Storage location
  CheckpointMetadata metadata = 4;  // Checkpoint metadata
}

message CheckpointMetadata {
  string session_id = 1;
  int64 created_at_ms = 2;
  string checksum_sha256 = 3;
  repeated string installed_packages = 4;
  repeated string modified_files = 5;
}
```

**Example:**
```go
resp, err := client.CheckpointSession(ctx, &protov1.CheckpointRequest{
    SessionId:   "session-abc123",
    Incremental: false,
})

fmt.Printf("Checkpoint: %s, Size: %d bytes\n", 
    resp.CheckpointId, resp.SizeBytes)
```

#### RestoreSession

Restore a session from a checkpoint.

**Request:**
```protobuf
message RestoreRequest {
  string checkpoint_id = 1;   // Checkpoint to restore
}
```

**Response:**
```protobuf
message RestoreResponse {
  string session_id = 1;              // New session ID
  string worker_id = 2;               // Worker identifier
  SessionState state = 3;             // Current state
  CheckpointMetadata checkpoint_metadata = 4;
}
```

**Example:**
```go
resp, err := client.RestoreSession(ctx, &protov1.RestoreRequest{
    CheckpointId: "checkpoint-abc123-1699564800",
})

fmt.Printf("Restored to session: %s\n", resp.SessionId)
```

#### GetCapacity

Get current worker capacity information.

**Request:**
```protobuf
message CapacityRequest {
  // Empty - returns current capacity
}
```

**Response:**
```protobuf
message SessionCapacity {
  int32 total_sessions = 1;       // Maximum sessions
  int32 active_sessions = 2;      // Current sessions
  int32 available_sessions = 3;   // Remaining capacity
  repeated SessionInfo sessions = 4;
}

message SessionInfo {
  string session_id = 1;
  string workspace_id = 2;
  SessionState state = 3;
  int64 last_activity_ms = 4;
  ResourceMetrics usage = 5;
  int32 active_tasks = 6;
}
```

**Example:**
```go
resp, err := client.GetCapacity(ctx, &protov1.CapacityRequest{})

fmt.Printf("Capacity: %d/%d sessions\n", 
    resp.ActiveSessions, resp.TotalSessions)
```

### TaskExecution Service

Executes tasks with streaming support.

#### ExecuteTask

Execute a task in a session with real-time streaming.

**Request:**
```protobuf
message TaskRequest {
  string task_id = 1;                 // Task identifier
  string session_id = 2;              // Target session
  string tool_name = 3;               // Tool to execute
  map<string, string> arguments = 4;  // Tool arguments
  TaskConstraints constraints = 5;    // Execution constraints
}

message TaskConstraints {
  int32 timeout_seconds = 1;    // Task timeout
  int32 max_memory_mb = 2;      // Memory limit
  int32 max_cpu_percent = 3;    // CPU limit
}
```

**Response (Stream):**
```protobuf
message TaskResponse {
  string task_id = 1;
  
  oneof payload {
    ProgressUpdate progress = 2;
    LogEntry log = 3;
    TaskResult result = 4;
  }
}

message ProgressUpdate {
  int32 percent_complete = 1;
  string stage = 2;
  string message = 3;
}

message LogEntry {
  int64 timestamp_ms = 1;
  LogLevel level = 2;
  string message = 3;
  string source = 4;
}

message TaskResult {
  TaskStatus status = 1;
  map<string, string> outputs = 2;
  string error = 3;
  int32 exit_code = 4;
}

enum TaskStatus {
  STATUS_UNSPECIFIED = 0;
  STATUS_SUCCESS = 1;
  STATUS_FAILURE = 2;
  STATUS_TIMEOUT = 3;
  STATUS_CANCELLED = 4;
}
```

**Example:**
```go
client := protov1.NewTaskExecutionClient(conn)

stream, err := client.ExecuteTask(ctx, &protov1.TaskRequest{
    TaskId:    "task-123",
    SessionId: "session-abc123",
    ToolName:  "run.python",
    Arguments: map[string]string{
        "code": "print('Hello, World!')",
    },
    Constraints: &protov1.TaskConstraints{
        TimeoutSeconds: 30,
    },
})

for {
    resp, err := stream.Recv()
    if err == io.EOF {
        break
    }
    
    switch payload := resp.Payload.(type) {
    case *protov1.TaskResponse_Progress:
        fmt.Printf("Progress: %d%% - %s\n", 
            payload.Progress.PercentComplete, 
            payload.Progress.Message)
    case *protov1.TaskResponse_Log:
        fmt.Printf("Log: %s\n", payload.Log.Message)
    case *protov1.TaskResponse_Result:
        fmt.Printf("Result: %v\n", payload.Result.Outputs)
    }
}
```

#### CancelTask

Cancel a running task.

**Request:**
```protobuf
message CancelRequest {
  string task_id = 1;       // Task to cancel
  string session_id = 2;    // Session containing task
}
```

**Response:**
```protobuf
message CancelResponse {
  bool cancelled = 1;       // Whether task was cancelled
  string state = 2;         // Current state ("canceled" | "not_found")
}
```

**Example:**
```go
resp, err := client.CancelTask(ctx, &protov1.CancelRequest{
    TaskId:    "task-123",
    SessionId: "session-abc123",
})

if resp.Cancelled {
    fmt.Println("Task cancelled successfully")
}
```

#### GetTaskStatus

Get status of a task.

**Request:**
```protobuf
message StatusRequest {
  string task_id = 1;       // Task to query
  string session_id = 2;    // Session containing task
}
```

**Response:**
```protobuf
message StatusResponse {
  string task_id = 1;
  TaskStatus status = 2;      // Current status
  int64 start_time_ms = 3;
  int64 end_time_ms = 4;
}
```

**Example:**
```go
resp, err := client.GetTaskStatus(ctx, &protov1.StatusRequest{
    TaskId:    "task-123",
    SessionId: "session-abc123",
})

fmt.Printf("Task status: %v\n", resp.Status)
```

### ArtifactService Service

Manages build artifacts and outputs.

#### GetUploadURL

Get a pre-signed URL for artifact upload.

**Request:**
```protobuf
message UploadURLRequest {
  string session_id = 1;
  string artifact_name = 2;
  string content_type = 3;
  int64 size_bytes = 4;
}
```

**Response:**
```protobuf
message UploadURLResponse {
  string upload_url = 1;        // Pre-signed upload URL
  string artifact_id = 2;       // Artifact identifier
  int64 expires_at_ms = 3;      // URL expiration
}
```

**Example:**
```go
client := protov1.NewArtifactServiceClient(conn)

resp, err := client.GetUploadURL(ctx, &protov1.UploadURLRequest{
    SessionId:    "session-abc123",
    ArtifactName: "build-output.tar.gz",
    ContentType:  "application/gzip",
    SizeBytes:    1024000,
})

// Use upload_url to PUT artifact
```

#### RecordArtifact

Record artifact metadata.

**Request:**
```protobuf
message ArtifactMetadata {
  string session_id = 1;
  string artifact_id = 2;
  string artifact_name = 3;
  string artifact_type = 4;
  int64 size_bytes = 5;
  string checksum_sha256 = 6;
  map<string, string> metadata = 7;
}
```

**Response:**
```protobuf
message ArtifactReceipt {
  string artifact_id = 1;
  string storage_url = 2;
  int64 recorded_at_ms = 3;
}
```

**Example:**
```go
resp, err := client.RecordArtifact(ctx, &protov1.ArtifactMetadata{
    SessionId:      "session-abc123",
    ArtifactId:     "artifact-456",
    ArtifactName:   "build-output.tar.gz",
    ArtifactType:   "build_archive",
    SizeBytes:      1024000,
    ChecksumSha256: "abc123...",
    Metadata: map[string]string{
        "build_id": "build-789",
        "commit":   "abc123def",
    },
})
```

## Core Types

### WorkerSession

Internal session representation (not exposed via gRPC).

```go
type WorkerSession struct {
    SessionID        string
    WorkspaceID      string
    UserID           string
    WorkspacePath    string
    State            SessionState
    Config           *SessionConfig
    CreatedAt        time.Time
    LastActivity     time.Time
    ResourceUsage    *ResourceMetrics
    ActiveTasks      int32
    RecentTasks      []string
    LastCheckpointID string
    Artifacts        []string
    InstalledPkgs    []string
    mu               sync.RWMutex
}
```

### ActiveTask

Internal task tracking (not exposed via gRPC).

```go
type ActiveTask struct {
    TaskID    string
    SessionID string
    ToolName  string
    StartTime time.Time
    Status    TaskStatus
    Cancel    context.CancelFunc
    mu        sync.RWMutex
}
```

## Tool Handlers

### Available Tools

| Tool | Description | Required Args | Optional Args | Returns |
|------|-------------|---------------|---------------|---------|
| `echo` | Echo message | `message` | - | `message` |
| `fs.write` | Write file | `path`, `contents` | - | `path`, `bytes_written` |
| `fs.read` | Read file | `path` | - | `path`, `contents`, `size` |
| `fs.list` | List directory | `path` | - | `entries` |
| `run.python` | Execute Python | `code` | - | `stdout`, `stderr`, `exit_code` |
| `pkg.install` | Install packages | `requirements` | - | `installed`, `stdout` |

### Tool: echo

Test tool for connectivity.

**Arguments:**
- `message` (string, required): Message to echo back

**Returns:**
- `message` (string): The echoed message

**Example:**
```json
{
  "tool_name": "echo",
  "arguments": {
    "message": "Hello, World!"
  }
}
```

**Response:**
```json
{
  "status": "STATUS_SUCCESS",
  "outputs": {
    "message": "Hello, World!"
  }
}
```

### Tool: fs.write

Write content to a file in the workspace.

**Arguments:**
- `path` (string, required): Relative path within workspace
- `contents` (string, required): File contents

**Returns:**
- `path` (string): Written file path
- `bytes_written` (string): Number of bytes written

**Example:**
```json
{
  "tool_name": "fs.write",
  "arguments": {
    "path": "src/main.py",
    "contents": "print('Hello')"
  }
}
```

**Response:**
```json
{
  "status": "STATUS_SUCCESS",
  "outputs": {
    "path": "src/main.py",
    "bytes_written": "15"
  }
}
```

### Tool: fs.read

Read a file from the workspace.

**Arguments:**
- `path` (string, required): Relative path within workspace

**Returns:**
- `path` (string): Read file path
- `contents` (string): File contents
- `size` (string): File size in bytes

**Example:**
```json
{
  "tool_name": "fs.read",
  "arguments": {
    "path": "src/main.py"
  }
}
```

**Response:**
```json
{
  "status": "STATUS_SUCCESS",
  "outputs": {
    "path": "src/main.py",
    "contents": "print('Hello')",
    "size": "15"
  }
}
```

### Tool: fs.list

List directory contents.

**Arguments:**
- `path` (string, required): Relative directory path

**Returns:**
- `entries` (string): Comma-separated list of entries

**Example:**
```json
{
  "tool_name": "fs.list",
  "arguments": {
    "path": "src"
  }
}
```

**Response:**
```json
{
  "status": "STATUS_SUCCESS",
  "outputs": {
    "entries": "main.py,utils.py,__pycache__/"
  }
}
```

### Tool: run.python

Execute Python code in the session's environment.

**Arguments:**
- `code` (string, required): Python code to execute

**Returns:**
- `stdout` (string): Standard output
- `stderr` (string): Standard error
- `exit_code` (string): Process exit code

**Streaming:**
- Sends progress updates during execution
- Streams stdout/stderr as log entries

**Example:**
```json
{
  "tool_name": "run.python",
  "arguments": {
    "code": "import sys\nprint('Python version:', sys.version)"
  }
}
```

**Response:**
```json
{
  "status": "STATUS_SUCCESS",
  "outputs": {
    "stdout": "Python version: 3.11.0 ...",
    "stderr": "",
    "exit_code": "0"
  }
}
```

### Tool: pkg.install

Install Python packages via pip.

**Arguments:**
- `requirements` (string, required): Newline-separated package list

**Returns:**
- `installed` (string): Comma-separated installed packages
- `stdout` (string): Pip output

**Streaming:**
- Sends progress updates during installation
- Streams pip output as log entries

**Example:**
```json
{
  "tool_name": "pkg.install",
  "arguments": {
    "requirements": "requests\nflask==2.3.0"
  }
}
```

**Response:**
```json
{
  "status": "STATUS_SUCCESS",
  "outputs": {
    "installed": "requests,flask",
    "stdout": "Successfully installed requests-2.31.0 flask-2.3.0"
  }
}
```

## Error Codes

### Session Errors

| Error | gRPC Code | Description |
|-------|-----------|-------------|
| Session not found | `NOT_FOUND` | Session ID doesn't exist |
| Capacity exceeded | `RESOURCE_EXHAUSTED` | Maximum sessions reached |
| Invalid session state | `FAILED_PRECONDITION` | Operation not allowed in current state |
| Initialization failed | `INTERNAL` | Failed to create workspace or venv |

### Task Errors

| Error | gRPC Code | Description |
|-------|-----------|-------------|
| Task not found | `NOT_FOUND` | Task ID doesn't exist |
| Task timeout | `DEADLINE_EXCEEDED` | Task exceeded timeout limit |
| Task cancelled | `CANCELLED` | Task was cancelled by client |
| Invalid arguments | `INVALID_ARGUMENT` | Missing or invalid tool arguments |

### Tool Errors

| Error | gRPC Code | Description |
|-------|-----------|-------------|
| Invalid path | `INVALID_ARGUMENT` | Path validation failed |
| File not found | `NOT_FOUND` | Requested file doesn't exist |
| Permission denied | `PERMISSION_DENIED` | Operation not allowed |
| Python not found | `FAILED_PRECONDITION` | Python interpreter not available |
| Package install failed | `INTERNAL` | Pip installation failed |

### Checkpoint Errors

| Error | gRPC Code | Description |
|-------|-----------|-------------|
| Checkpoint not found | `NOT_FOUND` | Checkpoint ID doesn't exist |
| Archive failed | `INTERNAL` | Failed to create/extract archive |
| Disk full | `RESOURCE_EXHAUSTED` | Insufficient disk space |

## Usage Examples

### Complete Workflow Example

```go
package main

import (
    "context"
    "fmt"
    "io"
    "log"
    
    protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
    "google.golang.org/grpc"
)

func main() {
    // Connect to worker
    conn, err := grpc.Dial("localhost:50052", grpc.WithInsecure())
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()
    
    sessionClient := protov1.NewSessionManagementClient(conn)
    taskClient := protov1.NewTaskExecutionClient(conn)
    
    ctx := context.Background()
    
    // 1. Create session
    sessionResp, err := sessionClient.CreateSession(ctx, 
        &protov1.CreateSessionRequest{
            WorkspaceId: "project-123",
            UserId:      "user-456",
        })
    if err != nil {
        log.Fatal(err)
    }
    sessionID := sessionResp.SessionId
    fmt.Printf("Created session: %s\n", sessionID)
    
    // 2. Write Python file
    _, err = executeTask(taskClient, sessionID, "fs.write", map[string]string{
        "path":     "main.py",
        "contents": "print('Hello from Worker!')",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // 3. Install packages
    _, err = executeTask(taskClient, sessionID, "pkg.install", map[string]string{
        "requirements": "requests",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // 4. Execute Python code
    result, err := executeTask(taskClient, sessionID, "run.python", map[string]string{
        "code": "import requests\nprint(requests.__version__)",
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Python output: %s\n", result.Outputs["stdout"])
    
    // 5. Create checkpoint
    checkpointResp, err := sessionClient.CheckpointSession(ctx,
        &protov1.CheckpointRequest{
            SessionId: sessionID,
        })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Checkpoint created: %s\n", checkpointResp.CheckpointId)
    
    // 6. Destroy session
    _, err = sessionClient.DestroySession(ctx,
        &protov1.DestroySessionRequest{
            SessionId: sessionID,
        })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println("Session destroyed")
}

func executeTask(client protov1.TaskExecutionClient, sessionID, toolName string, args map[string]string) (*protov1.TaskResult, error) {
    ctx := context.Background()
    
    stream, err := client.ExecuteTask(ctx, &protov1.TaskRequest{
        TaskId:    fmt.Sprintf("task-%d", time.Now().UnixNano()),
        SessionId: sessionID,
        ToolName:  toolName,
        Arguments: args,
    })
    if err != nil {
        return nil, err
    }
    
    var result *protov1.TaskResult
    for {
        resp, err := stream.Recv()
        if err == io.EOF {
            break
        }
        if err != nil {
            return nil, err
        }
        
        if r, ok := resp.Payload.(*protov1.TaskResponse_Result); ok {
            result = r.Result
        }
    }
    
    return result, nil
}
```

## See Also

- [Architecture Overview](./README.md) - System design and components
- [Deployment Guide](./deployment.md) - Production deployment
- [Testing Guide](./testing.md) - Testing strategies
- [Protocol Buffers](https://protobuf.dev/) - Protobuf documentation
- [gRPC Go](https://grpc.io/docs/languages/go/) - gRPC documentation
