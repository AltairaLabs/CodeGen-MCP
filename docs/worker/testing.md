---
title: Worker Testing Guide
layout: default
---

# Worker Testing Guide

Comprehensive testing strategies and practices for the CodeGen-MCP Worker component.

## Test Coverage Overview

The Worker component maintains high test coverage across all critical functionality:

| Package | Coverage | Tests | Status |
|---------|----------|-------|--------|
| `internal/worker` | 83.9% | 66 | ✅ Passing |
| Session Management | 90%+ | 15 | ✅ Passing |
| Task Execution | 85%+ | 18 | ✅ Passing |
| Tool Handlers | 80%+ | 20 | ✅ Passing |
| Checkpointing | 85%+ | 13 | ✅ Passing |

**Coverage Goal:** Maintain ≥80% overall coverage with ≥75% per package.

## Running Tests

### Quick Start

```bash
# Run all tests
make test

# Run worker tests only
go test -v ./internal/worker/...

# Run with coverage
go test -v -cover ./internal/worker/...

# Generate coverage report
go test -v -coverprofile=coverage.out ./internal/worker/...
go tool cover -html=coverage.out -o coverage.html
```

### Test Output

```bash
=== RUN   TestWorkerServerCreateSession
--- PASS: TestWorkerServerCreateSession (0.15s)
=== RUN   TestWorkerServerGetCapacity
--- PASS: TestWorkerServerGetCapacity (0.01s)
=== RUN   TestTaskExecutorExecuteTask
--- PASS: TestTaskExecutorExecuteTask (0.05s)
...
PASS
coverage: 83.9% of statements
ok      github.com/AltairaLabs/codegen-mcp/internal/worker    2.451s
```

## Test Structure

### Unit Tests

Test individual components in isolation:

```go
// internal/worker/session_pool_test.go
func TestSessionPoolCreateSession(t *testing.T) {
    tests := []struct {
        name        string
        workspaceID string
        userID      string
        maxSessions int
        wantErr     bool
    }{
        {
            name:        "successful creation",
            workspaceID: "workspace-1",
            userID:      "user-1",
            maxSessions: 10,
            wantErr:     false,
        },
        {
            name:        "capacity exceeded",
            workspaceID: "workspace-2",
            userID:      "user-2",
            maxSessions: 0,
            wantErr:     true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            pool := &SessionPool{
                maxSessions: tt.maxSessions,
                sessions:    make(map[string]*WorkerSession),
            }
            
            session, err := pool.CreateSession(
                tt.workspaceID, 
                tt.userID,
            )
            
            if (err != nil) != tt.wantErr {
                t.Errorf("CreateSession() error = %v, wantErr %v", 
                    err, tt.wantErr)
                return
            }
            
            if !tt.wantErr && session == nil {
                t.Error("CreateSession() returned nil session")
            }
        })
    }
}
```

### Integration Tests

Test component interactions:

```go
// internal/worker/integration_test.go
func TestWorkerEndToEnd(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }
    
    // Start gRPC server
    server := startTestServer(t)
    defer server.Stop()
    
    // Connect client
    conn, err := grpc.Dial("localhost:50052", 
        grpc.WithInsecure())
    if err != nil {
        t.Fatal(err)
    }
    defer conn.Close()
    
    client := protov1.NewSessionManagementClient(conn)
    
    // Create session
    createResp, err := client.CreateSession(context.Background(),
        &protov1.CreateSessionRequest{
            WorkspaceId: "test-workspace",
        })
    if err != nil {
        t.Fatalf("CreateSession failed: %v", err)
    }
    
    sessionID := createResp.SessionId
    
    // Execute task
    taskClient := protov1.NewTaskExecutionClient(conn)
    stream, err := taskClient.ExecuteTask(context.Background(),
        &protov1.TaskRequest{
            TaskId:    "test-task",
            SessionId: sessionID,
            ToolName:  "echo",
            Arguments: map[string]string{
                "message": "integration test",
            },
        })
    if err != nil {
        t.Fatalf("ExecuteTask failed: %v", err)
    }
    
    // Collect results
    for {
        resp, err := stream.Recv()
        if err == io.EOF {
            break
        }
        if err != nil {
            t.Fatalf("stream.Recv() failed: %v", err)
        }
        
        if result, ok := resp.Payload.(*protov1.TaskResponse_Result); ok {
            if result.Result.Status != protov1.TaskStatus_STATUS_SUCCESS {
                t.Errorf("Task failed: %v", result.Result.Error)
            }
        }
    }
    
    // Clean up
    _, err = client.DestroySession(context.Background(),
        &protov1.DestroySessionRequest{
            SessionId: sessionID,
        })
    if err != nil {
        t.Errorf("DestroySession failed: %v", err)
    }
}
```

### Table-Driven Tests

Test multiple scenarios efficiently:

```go
// internal/worker/tool_handlers_test.go
func TestHandleToolExecution(t *testing.T) {
    tests := []struct {
        name      string
        toolName  string
        args      map[string]string
        wantErr   bool
        wantCode  int32
        validate  func(*testing.T, map[string]string)
    }{
        {
            name:     "echo success",
            toolName: "echo",
            args:     map[string]string{"message": "hello"},
            wantErr:  false,
            validate: func(t *testing.T, outputs map[string]string) {
                if outputs["message"] != "hello" {
                    t.Errorf("got %v, want hello", outputs["message"])
                }
            },
        },
        {
            name:     "fs.write success",
            toolName: "fs.write",
            args: map[string]string{
                "path":     "test.txt",
                "contents": "test data",
            },
            wantErr: false,
            validate: func(t *testing.T, outputs map[string]string) {
                if outputs["bytes_written"] != "9" {
                    t.Errorf("got %v bytes, want 9", 
                        outputs["bytes_written"])
                }
            },
        },
        {
            name:     "run.python success",
            toolName: "run.python",
            args:     map[string]string{"code": "print('test')"},
            wantErr:  false,
            wantCode: 0,
            validate: func(t *testing.T, outputs map[string]string) {
                if !strings.Contains(outputs["stdout"], "test") {
                    t.Errorf("stdout missing 'test': %v", 
                        outputs["stdout"])
                }
            },
        },
        {
            name:     "invalid tool",
            toolName: "invalid.tool",
            args:     map[string]string{},
            wantErr:  true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            handler := NewToolHandler(createTestSession(t))
            
            outputs, err := handler.Execute(
                tt.toolName,
                tt.args,
            )
            
            if (err != nil) != tt.wantErr {
                t.Errorf("Execute() error = %v, wantErr %v", 
                    err, tt.wantErr)
                return
            }
            
            if tt.validate != nil {
                tt.validate(t, outputs)
            }
        })
    }
}
```

## Test Categories

### Session Management Tests

Test session lifecycle and isolation:

```go
func TestSessionPoolCapacity(t *testing.T) {
    pool := NewSessionPool(2) // Max 2 sessions
    
    // Create first session
    s1, err := pool.CreateSession("ws-1", "user-1")
    assert.NoError(t, err)
    assert.Equal(t, 1, pool.GetActiveCount())
    
    // Create second session
    s2, err := pool.CreateSession("ws-2", "user-2")
    assert.NoError(t, err)
    assert.Equal(t, 2, pool.GetActiveCount())
    
    // Third session should fail
    _, err = pool.CreateSession("ws-3", "user-3")
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "capacity exceeded")
    
    // Destroy one session
    err = pool.DestroySession(s1.SessionID)
    assert.NoError(t, err)
    assert.Equal(t, 1, pool.GetActiveCount())
    
    // Now third session should succeed
    s3, err := pool.CreateSession("ws-3", "user-3")
    assert.NoError(t, err)
    assert.Equal(t, 2, pool.GetActiveCount())
}

func TestSessionIsolation(t *testing.T) {
    pool := NewSessionPool(5)
    
    // Create two sessions
    s1, _ := pool.CreateSession("ws-1", "user-1")
    s2, _ := pool.CreateSession("ws-2", "user-2")
    
    // Write file in session 1
    handler1 := NewToolHandler(s1)
    _, err := handler1.Execute("fs.write", map[string]string{
        "path":     "test.txt",
        "contents": "session 1 data",
    })
    assert.NoError(t, err)
    
    // Verify file doesn't exist in session 2
    handler2 := NewToolHandler(s2)
    _, err = handler2.Execute("fs.read", map[string]string{
        "path": "test.txt",
    })
    assert.Error(t, err) // File should not exist
}
```

### Task Execution Tests

Test task lifecycle and cancellation:

```go
func TestTaskExecutorCancellation(t *testing.T) {
    executor := NewTaskExecutor()
    session := createTestSession(t)
    
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    // Start long-running task
    resultCh := make(chan *TaskResult)
    go func() {
        result, _ := executor.ExecuteTask(ctx, &TaskRequest{
            TaskID:    "long-task",
            SessionID: session.SessionID,
            ToolName:  "run.python",
            Arguments: map[string]string{
                "code": "import time; time.sleep(60)",
            },
        })
        resultCh <- result
    }()
    
    // Wait for task to start
    time.Sleep(100 * time.Millisecond)
    
    // Cancel task
    cancel()
    
    // Verify task was cancelled
    result := <-resultCh
    assert.Equal(t, TaskStatus_STATUS_CANCELLED, result.Status)
}

func TestTaskTimeout(t *testing.T) {
    executor := NewTaskExecutor()
    session := createTestSession(t)
    
    ctx, cancel := context.WithTimeout(
        context.Background(),
        1*time.Second,
    )
    defer cancel()
    
    result, err := executor.ExecuteTask(ctx, &TaskRequest{
        TaskID:    "timeout-task",
        SessionID: session.SessionID,
        ToolName:  "run.python",
        Arguments: map[string]string{
            "code": "import time; time.sleep(10)",
        },
        Constraints: &TaskConstraints{
            TimeoutSeconds: 1,
        },
    })
    
    assert.NoError(t, err)
    assert.Equal(t, TaskStatus_STATUS_TIMEOUT, result.Status)
}
```

### Tool Handler Tests

Test each tool implementation:

```go
func TestToolHandlerFileSystem(t *testing.T) {
    session := createTestSession(t)
    handler := NewToolHandler(session)
    
    // Test fs.write
    outputs, err := handler.Execute("fs.write", map[string]string{
        "path":     "data/test.txt",
        "contents": "test content",
    })
    assert.NoError(t, err)
    assert.Equal(t, "12", outputs["bytes_written"])
    
    // Test fs.read
    outputs, err = handler.Execute("fs.read", map[string]string{
        "path": "data/test.txt",
    })
    assert.NoError(t, err)
    assert.Equal(t, "test content", outputs["contents"])
    
    // Test fs.list
    outputs, err = handler.Execute("fs.list", map[string]string{
        "path": "data",
    })
    assert.NoError(t, err)
    assert.Contains(t, outputs["entries"], "test.txt")
}

func TestToolHandlerPython(t *testing.T) {
    session := createTestSession(t)
    handler := NewToolHandler(session)
    
    tests := []struct {
        name     string
        code     string
        wantOut  string
        wantCode int32
    }{
        {
            name:     "simple print",
            code:     "print('hello')",
            wantOut:  "hello",
            wantCode: 0,
        },
        {
            name:     "math operations",
            code:     "print(2 + 2)",
            wantOut:  "4",
            wantCode: 0,
        },
        {
            name:     "syntax error",
            code:     "print('unclosed",
            wantCode: 1,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            outputs, err := handler.Execute("run.python", 
                map[string]string{"code": tt.code})
            
            if tt.wantCode != 0 {
                assert.Error(t, err)
                return
            }
            
            assert.NoError(t, err)
            assert.Contains(t, outputs["stdout"], tt.wantOut)
            assert.Equal(t, fmt.Sprint(tt.wantCode), 
                outputs["exit_code"])
        })
    }
}

func TestToolHandlerPackageInstall(t *testing.T) {
    session := createTestSession(t)
    handler := NewToolHandler(session)
    
    // Install packages
    outputs, err := handler.Execute("pkg.install", map[string]string{
        "requirements": "requests\nflask==2.3.0",
    })
    assert.NoError(t, err)
    assert.Contains(t, outputs["installed"], "requests")
    assert.Contains(t, outputs["installed"], "flask")
    
    // Verify installation
    outputs, err = handler.Execute("run.python", map[string]string{
        "code": "import requests; print(requests.__version__)",
    })
    assert.NoError(t, err)
    assert.NotEmpty(t, outputs["stdout"])
}
```

### Checkpoint Tests

Test checkpoint creation and restoration:

```go
func TestCheckpointerCreateRestore(t *testing.T) {
    session := createTestSession(t)
    checkpointer := NewCheckpointer(session)
    
    // Create workspace content
    writeFile(t, session.WorkspacePath, "data.txt", "test data")
    writeFile(t, session.WorkspacePath, "script.py", "print('test')")
    
    // Create checkpoint
    checkpointID, err := checkpointer.Checkpoint()
    assert.NoError(t, err)
    assert.NotEmpty(t, checkpointID)
    
    // Verify checkpoint file exists
    checkpointPath := filepath.Join(
        session.CheckpointDir,
        checkpointID+".tar.gz",
    )
    assert.FileExists(t, checkpointPath)
    
    // Modify workspace
    writeFile(t, session.WorkspacePath, "data.txt", "modified")
    
    // Restore checkpoint
    restoredSession, err := checkpointer.Restore(checkpointID)
    assert.NoError(t, err)
    
    // Verify restoration
    content := readFile(t, restoredSession.WorkspacePath, "data.txt")
    assert.Equal(t, "test data", content)
    
    scriptContent := readFile(t, 
        restoredSession.WorkspacePath, "script.py")
    assert.Equal(t, "print('test')", scriptContent)
}

func TestCheckpointerLargeFiles(t *testing.T) {
    session := createTestSession(t)
    checkpointer := NewCheckpointer(session)
    
    // Create large file (10MB)
    largeData := make([]byte, 10*1024*1024)
    for i := range largeData {
        largeData[i] = byte(i % 256)
    }
    
    err := os.WriteFile(
        filepath.Join(session.WorkspacePath, "large.bin"),
        largeData,
        0644,
    )
    assert.NoError(t, err)
    
    // Checkpoint should succeed
    checkpointID, err := checkpointer.Checkpoint()
    assert.NoError(t, err)
    
    // Verify checkpoint size
    info, err := os.Stat(filepath.Join(
        session.CheckpointDir,
        checkpointID+".tar.gz",
    ))
    assert.NoError(t, err)
    assert.Greater(t, info.Size(), int64(0))
    
    // Restore and verify
    restored, err := checkpointer.Restore(checkpointID)
    assert.NoError(t, err)
    
    restoredData, err := os.ReadFile(
        filepath.Join(restored.WorkspacePath, "large.bin"),
    )
    assert.NoError(t, err)
    assert.Equal(t, largeData, restoredData)
}
```

## Test Helpers

### Common Test Utilities

```go
// internal/worker/testing.go
package worker

import (
    "os"
    "path/filepath"
    "testing"
)

func createTestSession(t *testing.T) *WorkerSession {
    t.Helper()
    
    tmpDir, err := os.MkdirTemp("", "worker-test-*")
    if err != nil {
        t.Fatal(err)
    }
    
    t.Cleanup(func() {
        os.RemoveAll(tmpDir)
    })
    
    workspacePath := filepath.Join(tmpDir, "workspace")
    if err := os.MkdirAll(workspacePath, 0755); err != nil {
        t.Fatal(err)
    }
    
    return &WorkerSession{
        SessionID:     "test-session-" + t.Name(),
        WorkspaceID:   "test-workspace",
        UserID:        "test-user",
        WorkspacePath: workspacePath,
        State:         SessionStateReady,
    }
}

func writeFile(t *testing.T, dir, name, content string) {
    t.Helper()
    
    path := filepath.Join(dir, name)
    if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
        t.Fatal(err)
    }
    
    if err := os.WriteFile(path, []byte(content), 0644); err != nil {
        t.Fatal(err)
    }
}

func readFile(t *testing.T, dir, name string) string {
    t.Helper()
    
    content, err := os.ReadFile(filepath.Join(dir, name))
    if err != nil {
        t.Fatal(err)
    }
    
    return string(content)
}

func startTestServer(t *testing.T) *grpc.Server {
    t.Helper()
    
    lis, err := net.Listen("tcp", "localhost:0")
    if err != nil {
        t.Fatal(err)
    }
    
    server := grpc.NewServer()
    workerServer := NewWorkerServer(10) // Max 10 sessions
    
    protov1.RegisterSessionManagementServer(server, workerServer)
    protov1.RegisterTaskExecutionServer(server, workerServer)
    
    go server.Serve(lis)
    
    t.Cleanup(func() {
        server.Stop()
    })
    
    return server
}
```

## Continuous Integration

### GitHub Actions Workflow

```yaml
# .github/workflows/worker-tests.yml
name: Worker Tests

on:
  push:
    branches: [ main, develop ]
    paths:
      - 'internal/worker/**'
      - 'cmd/worker/**'
  pull_request:
    branches: [ main ]

jobs:
  test:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        go-version: ['1.22', '1.23']
    
    steps:
    - uses: actions/checkout@v4
    
    - name: Set up Go
      uses: actions/setup-go@v5
      with:
        go-version: ${{ matrix.go-version }}
    
    - name: Install Python
      uses: actions/setup-python@v5
      with:
        python-version: '3.11'
    
    - name: Run Tests
      run: |
        go test -v -race -coverprofile=coverage.out \
          ./internal/worker/...
    
    - name: Coverage Check
      run: |
        coverage=$(go tool cover -func=coverage.out | \
          grep total | awk '{print $3}' | sed 's/%//')
        echo "Coverage: ${coverage}%"
        if (( $(echo "$coverage < 80" | bc -l) )); then
          echo "❌ Coverage below 80%"
          exit 1
        fi
        echo "✅ Coverage above 80%"
    
    - name: Upload Coverage
      uses: codecov/codecov-action@v4
      with:
        files: ./coverage.out
        flags: worker
```

## Test Maintenance

### Coverage Goals

- **Overall:** ≥80%
- **Session Management:** ≥85%
- **Task Execution:** ≥85%
- **Tool Handlers:** ≥75%
- **Checkpointing:** ≥80%

### Testing Checklist

- [ ] All public functions have tests
- [ ] Error paths are tested
- [ ] Edge cases are covered
- [ ] Integration tests exist
- [ ] Cleanup in t.Cleanup()
- [ ] No test dependencies on external services
- [ ] Tests are deterministic
- [ ] Tests run in parallel where possible

### Adding New Tests

When adding new functionality:

1. Write tests first (TDD)
2. Ensure ≥80% coverage for new code
3. Add table-driven tests for multiple scenarios
4. Include error case tests
5. Add integration tests if needed
6. Update CI if new dependencies required

## See Also

- [Architecture Overview](./README.md) - System design
- [API Reference](./api-reference.md) - API documentation
- [Deployment Guide](./deployment.md) - Production deployment
- [Go Testing](https://go.dev/doc/tutorial/add-a-test) - Go testing documentation
