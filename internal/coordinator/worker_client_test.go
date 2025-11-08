package coordinator

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"google.golang.org/grpc"
)

const (
	testWorkerID      = "test-worker-1"
	testSessionID     = "test-session"
	workerSessionID   = "worker-session-1"
	testWorkspaceID   = "workspace1"
	testToolName      = "test-tool"
	unexpectedErrMsg  = "Unexpected error: %v"
	errNotImplemented = "not implemented"
)

// mockTaskExecutionClient implements protov1.TaskExecutionClient for testing
type mockTaskExecutionClient struct {
	protov1.UnimplementedTaskExecutionServer
	executeFunc func(
		ctx context.Context,
		req *protov1.TaskRequest,
		opts ...grpc.CallOption,
	) (protov1.TaskExecution_ExecuteTaskClient, error)
}

func (m *mockTaskExecutionClient) ExecuteTask(
	ctx context.Context,
	req *protov1.TaskRequest,
	opts ...grpc.CallOption,
) (protov1.TaskExecution_ExecuteTaskClient, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, req, opts...)
	}
	return nil, errors.New("not implemented")
}

func (m *mockTaskExecutionClient) CancelTask(
	ctx context.Context,
	req *protov1.CancelRequest,
	opts ...grpc.CallOption,
) (*protov1.CancelResponse, error) {
	return nil, errors.New("not implemented")
}

func (m *mockTaskExecutionClient) GetTaskStatus(
	ctx context.Context,
	req *protov1.StatusRequest,
	opts ...grpc.CallOption,
) (*protov1.StatusResponse, error) {
	return nil, errors.New("not implemented")
}

// mockExecuteTaskStream implements the streaming client
type mockExecuteTaskStream struct {
	grpc.ClientStream
	responses []*protov1.TaskResponse
	index     int
}

func (m *mockExecuteTaskStream) Recv() (*protov1.TaskResponse, error) {
	if m.index >= len(m.responses) {
		return nil, io.EOF
	}
	resp := m.responses[m.index]
	m.index++
	return resp, nil
}

func (m *mockExecuteTaskStream) CloseSend() error {
	return nil
}

// mockSessionMgmtClient implements protov1.SessionManagementClient for testing
type mockSessionMgmtClient struct {
	protov1.UnimplementedSessionManagementServer
	createFunc func(
		ctx context.Context,
		req *protov1.CreateSessionRequest,
		opts ...grpc.CallOption,
	) (*protov1.CreateSessionResponse, error)
}

func (m *mockSessionMgmtClient) CreateSession(
	ctx context.Context,
	req *protov1.CreateSessionRequest,
	opts ...grpc.CallOption,
) (*protov1.CreateSessionResponse, error) {
	if m.createFunc != nil {
		return m.createFunc(ctx, req, opts...)
	}
	return &protov1.CreateSessionResponse{
		SessionId: workerSessionID,
		WorkerId:  req.WorkerId,
	}, nil
}

func (m *mockSessionMgmtClient) DestroySession(
	ctx context.Context,
	req *protov1.DestroySessionRequest,
	opts ...grpc.CallOption,
) (*protov1.DestroySessionResponse, error) {
	return nil, errors.New(errNotImplemented)
}

func (m *mockSessionMgmtClient) CheckpointSession(
	ctx context.Context,
	req *protov1.CheckpointRequest,
	opts ...grpc.CallOption,
) (*protov1.CheckpointResponse, error) {
	return nil, errors.New(errNotImplemented)
}

func (m *mockSessionMgmtClient) RestoreSession(
	ctx context.Context,
	req *protov1.RestoreRequest,
	opts ...grpc.CallOption,
) (*protov1.RestoreResponse, error) {
	return nil, errors.New(errNotImplemented)
}

func (m *mockSessionMgmtClient) GetSessionStatus(
	ctx context.Context,
	req *protov1.SessionStatusRequest,
	opts ...grpc.CallOption,
) (*protov1.SessionStatusResponse, error) {
	return nil, errors.New(errNotImplemented)
}

// Helper to create a mock worker with task execution client
func setupMockWorker(
	t *testing.T,
	responses []*protov1.TaskResponse,
) (*WorkerRegistry, *SessionManager) {
	t.Helper()

	registry := NewWorkerRegistry()
	sessionManager := NewSessionManager(registry)

	mockTaskClient := &mockTaskExecutionClient{
		executeFunc: func(
			ctx context.Context,
			req *protov1.TaskRequest,
			opts ...grpc.CallOption,
		) (protov1.TaskExecution_ExecuteTaskClient, error) {
			return &mockExecuteTaskStream{
				responses: responses,
			}, nil
		},
	}

	mockSessionClient := &mockSessionMgmtClient{}

	worker := &RegisteredWorker{
		WorkerID: testWorkerID,
		Client: WorkerServiceClient{
			TaskExec:    mockTaskClient,
			SessionMgmt: mockSessionClient,
		},
		Status: &protov1.WorkerStatus{
			State: protov1.WorkerStatus_STATE_IDLE,
		},
		Capacity: &protov1.SessionCapacity{
			TotalSessions:     10,
			ActiveSessions:    1,
			AvailableSessions: 9,
		},
		LastHeartbeat:     time.Now(),
		HeartbeatInterval: 30 * time.Second,
	}

	if err := registry.RegisterWorker(testWorkerID, worker); err != nil {
		t.Fatalf("Failed to register worker: %v", err)
	}

	return registry, sessionManager
}

func TestRealWorkerClient_ExecuteTask_Success(t *testing.T) {
	responses := []*protov1.TaskResponse{
		{
			Payload: &protov1.TaskResponse_Result{
				Result: &protov1.TaskResult{
					Status: protov1.TaskResult_STATUS_SUCCESS,
					Outputs: map[string]string{
						"output": "test output",
					},
					Metadata: &protov1.ExecutionMetadata{
						ExitCode: 0,
					},
				},
			},
		},
	}

	registry, sessionManager := setupMockWorker(t, responses)
	logger := slog.Default()
	client := NewRealWorkerClient(registry, sessionManager, logger)

	// Create a session with worker assignment
	ctx := context.WithValue(context.Background(), "session_id", "test-session")
	session := sessionManager.CreateSession(ctx, "test-session", "user1", "workspace1")
	session.WorkerID = "test-worker-1"
	session.WorkerSessionID = "worker-session-1"

	result, err := client.ExecuteTask(ctx, "workspace1", "test-tool", TaskArgs{
		"arg1": "value1",
	})

	if err != nil {
		t.Fatalf("ExecuteTask failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success=true")
	}

	if result.Output != "test output" {
		t.Errorf("Expected output='test output', got '%s'", result.Output)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}
}

func TestRealWorkerClient_ExecuteTask_SessionNotFound(t *testing.T) {
	registry := NewWorkerRegistry()
	sessionManager := NewSessionManager(registry)
	logger := slog.Default()
	client := NewRealWorkerClient(registry, sessionManager, logger)

	ctx := context.WithValue(context.Background(), "session_id", "nonexistent")

	_, err := client.ExecuteTask(ctx, "workspace1", "test-tool", TaskArgs{})

	if err == nil {
		t.Fatal("Expected error for nonexistent session")
	}

	if err.Error() != "session not found: nonexistent" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestRealWorkerClient_ExecuteTask_NoWorkerAssigned(t *testing.T) {
	registry := NewWorkerRegistry()
	sessionManager := NewSessionManager(registry)
	logger := slog.Default()
	client := NewRealWorkerClient(registry, sessionManager, logger)

	ctx := context.WithValue(context.Background(), "session_id", "test-session")
	sessionManager.CreateSession(ctx, "test-session", "user1", "workspace1")

	_, err := client.ExecuteTask(ctx, "workspace1", "test-tool", TaskArgs{})

	if err == nil {
		t.Fatal("Expected error for session without worker")
	}
}

func TestRealWorkerClient_ExecuteTask_WorkerNotFound(t *testing.T) {
	registry := NewWorkerRegistry()
	sessionManager := NewSessionManager(registry)
	logger := slog.Default()
	client := NewRealWorkerClient(registry, sessionManager, logger)

	ctx := context.WithValue(context.Background(), "session_id", "test-session")
	session := sessionManager.CreateSession(ctx, "test-session", "user1", "workspace1")
	session.WorkerID = "nonexistent-worker"

	_, err := client.ExecuteTask(ctx, "workspace1", "test-tool", TaskArgs{})

	if err == nil {
		t.Fatal("Expected error for nonexistent worker")
	}

	if err.Error() != "worker not found: nonexistent-worker" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestRealWorkerClient_ExecuteTask_TaskError(t *testing.T) {
	responses := []*protov1.TaskResponse{
		{
			Payload: &protov1.TaskResponse_Error{
				Error: &protov1.TaskError{
					Code:    "EXECUTION_FAILED",
					Message: "Task execution failed",
				},
			},
		},
	}

	registry, sessionManager := setupMockWorker(t, responses)
	logger := slog.Default()
	client := NewRealWorkerClient(registry, sessionManager, logger)

	ctx := context.WithValue(context.Background(), "session_id", "test-session")
	session := sessionManager.CreateSession(ctx, "test-session", "user1", "workspace1")
	session.WorkerID = "test-worker-1"
	session.WorkerSessionID = "worker-session-1"

	_, err := client.ExecuteTask(ctx, "workspace1", "test-tool", TaskArgs{})

	if err == nil {
		t.Fatal("Expected error for task failure")
	}

	expectedErr := "task failed: EXECUTION_FAILED - Task execution failed"
	if err.Error() != expectedErr {
		t.Errorf("Expected error '%s', got '%s'", expectedErr, err.Error())
	}
}

func TestRealWorkerClient_ExecuteTask_NoResult(t *testing.T) {
	responses := []*protov1.TaskResponse{
		{
			Payload: &protov1.TaskResponse_Log{
				Log: &protov1.LogEntry{
					Level:   protov1.LogEntry_LEVEL_INFO,
					Message: "Processing...",
				},
			},
		},
	}

	registry, sessionManager := setupMockWorker(t, responses)
	logger := slog.Default()
	client := NewRealWorkerClient(registry, sessionManager, logger)

	ctx := context.WithValue(context.Background(), "session_id", "test-session")
	session := sessionManager.CreateSession(ctx, "test-session", "user1", "workspace1")
	session.WorkerID = "test-worker-1"
	session.WorkerSessionID = "worker-session-1"

	_, err := client.ExecuteTask(ctx, "workspace1", "test-tool", TaskArgs{})

	if err == nil {
		t.Fatal("Expected error for missing result")
	}

	if err.Error() != "no result received from worker" {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestRealWorkerClient_ExecuteTask_WithLogs(t *testing.T) {
	responses := []*protov1.TaskResponse{
		{
			Payload: &protov1.TaskResponse_Log{
				Log: &protov1.LogEntry{
					Level:   protov1.LogEntry_LEVEL_INFO,
					Message: "Starting task",
				},
			},
		},
		{
			Payload: &protov1.TaskResponse_Progress{
				Progress: &protov1.ProgressUpdate{
					PercentComplete: 50,
					Stage:           "processing",
				},
			},
		},
		{
			Payload: &protov1.TaskResponse_Result{
				Result: &protov1.TaskResult{
					Status: protov1.TaskResult_STATUS_SUCCESS,
					Outputs: map[string]string{
						"output": "completed",
					},
				},
			},
		},
	}

	registry, sessionManager := setupMockWorker(t, responses)
	logger := slog.Default()
	client := NewRealWorkerClient(registry, sessionManager, logger)

	ctx := context.WithValue(context.Background(), "session_id", "test-session")
	session := sessionManager.CreateSession(ctx, "test-session", "user1", "workspace1")
	session.WorkerID = "test-worker-1"
	session.WorkerSessionID = "worker-session-1"

	result, err := client.ExecuteTask(ctx, "workspace1", "test-tool", TaskArgs{})

	if err != nil {
		t.Fatalf("ExecuteTask failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success=true")
	}

	if result.Output != "completed" {
		t.Errorf("Expected output='completed', got '%s'", result.Output)
	}
}

func TestRealWorkerClient_ExecuteTask_ContextCancellation(t *testing.T) {
	// Mock client that returns context canceled error
	registry := NewWorkerRegistry()
	sessionManager := NewSessionManager(registry)
	logger := slog.Default()

	mockClient := &mockTaskExecutionClient{
		executeFunc: func(
			ctx context.Context,
			req *protov1.TaskRequest,
			opts ...grpc.CallOption,
		) (protov1.TaskExecution_ExecuteTaskClient, error) {
			return nil, context.Canceled
		},
	}

	worker := &RegisteredWorker{
		WorkerID: "test-worker-1",
		Client: WorkerServiceClient{
			TaskExec: mockClient,
		},
		Status: &protov1.WorkerStatus{
			State: protov1.WorkerStatus_STATE_IDLE,
		},
		LastHeartbeat:     time.Now(),
		HeartbeatInterval: 30 * time.Second,
	}

	if err := registry.RegisterWorker("test-worker-1", worker); err != nil {
		t.Fatalf("Failed to register worker: %v", err)
	}

	client := NewRealWorkerClient(registry, sessionManager, logger)

	ctx, cancel := context.WithCancel(context.Background())
	ctx = context.WithValue(ctx, "session_id", "test-session")
	session := sessionManager.CreateSession(ctx, "test-session", "user1", "workspace1")
	session.WorkerID = "test-worker-1"
	session.WorkerSessionID = "worker-session-1"

	cancel() // Cancel immediately

	_, err := client.ExecuteTask(ctx, "workspace1", "test-tool", TaskArgs{})

	if err == nil {
		t.Fatal("Expected error for canceled context")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled error, got: %v", err)
	}
}
