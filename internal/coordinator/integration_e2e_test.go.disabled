package coordinator_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/coordinator"
	"github.com/AltairaLabs/codegen-mcp/internal/worker"
	"google.golang.org/grpc"
)

const (
	testTimeout = 30 * time.Second
)

// TestEndToEndIntegration tests the full coordinator-worker flow
func TestEndToEndIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	t.Log("Step 1: Starting coordinator...")
	coordRegistry, coordSessionMgr, coordServer := startCoordinator(t, logger)

	t.Log("Step 2: Starting worker...")
	workerServer, workerSessionPool := startWorker(t, logger)

	t.Log("Step 3: Registering worker with coordinator...")
	registerWorker(t, ctx, coordRegistry, workerSessionPool)

	t.Log("Step 4: Creating session...")
	session := createSession(t, ctx, coordSessionMgr, coordRegistry)

	t.Log("Step 5: Executing echo task...")
	testEchoTask(t, ctx, coordRegistry, coordSessionMgr, session, logger)

	t.Log("Step 6: Executing fs.write task...")
	testFsWriteTask(t, ctx, coordRegistry, coordSessionMgr, session, logger)

	t.Log("Step 7: Executing fs.read task...")
	testFsReadTask(t, ctx, coordRegistry, coordSessionMgr, session, logger)

	t.Log("✅ Integration test completed successfully!")

	// Cleanup
	_ = coordServer
	_ = workerServer
}

func startCoordinator(t *testing.T, logger *slog.Logger) (
	*coordinator.WorkerRegistry,
	*coordinator.SessionManager,
	*coordinator.CoordinatorServer,
) {
	t.Helper()

	registry := coordinator.NewWorkerRegistry()
	sessionMgr := coordinator.NewSessionManager(registry)
	coordServer := coordinator.NewCoordinatorServer(registry, sessionMgr, logger)

	// Start gRPC server for worker connections
	grpcServer := grpc.NewServer()
	coordServer.RegisterWithServer(grpcServer)

	// Note: In a real integration test, you would start the server on a port
	// For this test, we'll just create the components and mock the worker registration

	return registry, sessionMgr, coordServer
}

func startWorker(t *testing.T, logger *slog.Logger) (
	*worker.WorkerServer,
	*worker.SessionPool,
) {
	t.Helper()

	tempDir := t.TempDir()
	workerServer := worker.NewWorkerServer("integration-worker", 5, tempDir)
	sessionPool := worker.NewSessionPool("integration-worker", 5, tempDir)

	return workerServer, sessionPool
}

func registerWorker(
	t *testing.T,
	ctx context.Context,
	registry *coordinator.WorkerRegistry,
	sessionPool *worker.SessionPool,
) {
	t.Helper()

	// Create mock clients
	sessionMgmt := &mockSessionMgmtForIntegration{sessionPool: sessionPool}
	taskExec := &mockTaskExecForIntegration{sessionPool: sessionPool}
	artifacts := &mockArtifactServiceForIntegration{}

	// Create a mock worker registration
	worker := &coordinator.RegisteredWorker{
		WorkerID: "integration-worker",
		Client: coordinator.WorkerServiceClient{
			SessionMgmt: sessionMgmt,
			TaskExec:    taskExec,
			Artifacts:   artifacts,
		},
		Capabilities: &protov1.WorkerCapabilities{
			SupportedTools: []string{"echo", "fs.write", "fs.read", "fs.list"},
			Languages:      []string{"python"},
			MaxSessions:    5,
		},
		Limits: &protov1.ResourceLimits{
			MaxMemoryBytes:      8 * 1024 * 1024 * 1024, // 8GB
			MaxCpuMillicores:    4000,
			MaxDiskBytes:        100 * 1024 * 1024 * 1024, // 100GB
			MaxConcurrentTasks:  10,
			MaxMemoryPerSession: 2 * 1024 * 1024 * 1024,  // 2GB
			MaxDiskPerSession:   10 * 1024 * 1024 * 1024, // 10GB
		},
		Status: &protov1.WorkerStatus{
			State:       protov1.WorkerStatus_STATE_IDLE,
			ActiveTasks: 0,
		},
		Capacity: &protov1.SessionCapacity{
			TotalSessions:     5,
			AvailableSessions: 5,
		},
		RegisteredAt:      time.Now(),
		LastHeartbeat:     time.Now(),
		HeartbeatInterval: 30 * time.Second,
	}

	err := registry.RegisterWorker("integration-worker", worker)
	if err != nil {
		t.Fatalf("Failed to register worker: %v", err)
	}

	t.Log("Worker registered successfully")
}

func createSession(
	t *testing.T,
	ctx context.Context,
	sessionMgr *coordinator.SessionManager,
	registry *coordinator.WorkerRegistry,
) *coordinator.Session {
	t.Helper()

	session := sessionMgr.CreateSession(ctx, "integration-session", "integration-user", "integration-workspace")
	if session == nil {
		t.Fatal("Failed to create session")
	}

	if session.WorkerID == "" {
		t.Fatal("Session should have worker assigned")
	}

	if session.WorkerSessionID == "" {
		t.Fatal("Session should have worker session ID")
	}

	t.Logf("Session created: ID=%s, WorkerID=%s, WorkerSessionID=%s",
		session.ID, session.WorkerID, session.WorkerSessionID)

	return session
}

func testEchoTask(
	t *testing.T,
	ctx context.Context,
	registry *coordinator.WorkerRegistry,
	sessionMgr *coordinator.SessionManager,
	session *coordinator.Session,
	logger *slog.Logger,
) {
	t.Helper()

	client := coordinator.NewRealWorkerClient(registry, sessionMgr, logger)

	ctx = context.WithValue(ctx, "session_id", session.ID)
	result, err := client.ExecuteTask(ctx, session.WorkspaceID, "echo", coordinator.TaskArgs{
		"message": "Hello from integration test!",
	})

	if err != nil {
		t.Fatalf("Echo task failed: %v", err)
	}

	if !result.Success {
		t.Error("Echo task should succeed")
	}

	if result.Output != "Hello from integration test!" {
		t.Errorf("Expected echo output 'Hello from integration test!', got %q", result.Output)
	}

	t.Log("✓ Echo task completed successfully")
}

func testFsWriteTask(
	t *testing.T,
	ctx context.Context,
	registry *coordinator.WorkerRegistry,
	sessionMgr *coordinator.SessionManager,
	session *coordinator.Session,
	logger *slog.Logger,
) {
	t.Helper()

	client := coordinator.NewRealWorkerClient(registry, sessionMgr, logger)

	ctx = context.WithValue(ctx, "session_id", session.ID)
	result, err := client.ExecuteTask(ctx, session.WorkspaceID, "fs.write", coordinator.TaskArgs{
		"path":     "test.txt",
		"contents": "Integration test content",
	})

	if err != nil {
		t.Fatalf("fs.write task failed: %v", err)
	}

	if !result.Success {
		t.Error("fs.write task should succeed")
	}

	t.Log("✓ fs.write task completed successfully")
}

func testFsReadTask(
	t *testing.T,
	ctx context.Context,
	registry *coordinator.WorkerRegistry,
	sessionMgr *coordinator.SessionManager,
	session *coordinator.Session,
	logger *slog.Logger,
) {
	t.Helper()

	client := coordinator.NewRealWorkerClient(registry, sessionMgr, logger)

	ctx = context.WithValue(ctx, "session_id", session.ID)
	result, err := client.ExecuteTask(ctx, session.WorkspaceID, "fs.read", coordinator.TaskArgs{
		"path": "test.txt",
	})

	if err != nil {
		t.Fatalf("fs.read task failed: %v", err)
	}

	if !result.Success {
		t.Error("fs.read task should succeed")
	}

	if result.Output != "Integration test content" {
		t.Errorf("Expected file content 'Integration test content', got %q", result.Output)
	}

	t.Log("✓ fs.read task completed successfully")
}

// Mock implementations for integration testing

type mockSessionMgmtForIntegration struct {
	protov1.UnimplementedSessionManagementServer
	sessionPool *worker.SessionPool
}

func (m *mockSessionMgmtForIntegration) CreateSession(
	ctx context.Context,
	req *protov1.CreateSessionRequest,
	opts ...grpc.CallOption,
) (*protov1.CreateSessionResponse, error) {
	return m.sessionPool.CreateSession(ctx, req)
}

func (m *mockSessionMgmtForIntegration) DestroySession(
	ctx context.Context,
	req *protov1.DestroySessionRequest,
	opts ...grpc.CallOption,
) (*protov1.DestroySessionResponse, error) {
	return &protov1.DestroySessionResponse{Destroyed: true}, nil
}

func (m *mockSessionMgmtForIntegration) CheckpointSession(
	ctx context.Context,
	req *protov1.CheckpointRequest,
	opts ...grpc.CallOption,
) (*protov1.CheckpointResponse, error) {
	return &protov1.CheckpointResponse{}, nil
}

func (m *mockSessionMgmtForIntegration) RestoreSession(
	ctx context.Context,
	req *protov1.RestoreRequest,
	opts ...grpc.CallOption,
) (*protov1.RestoreResponse, error) {
	return &protov1.RestoreResponse{}, nil
}

func (m *mockSessionMgmtForIntegration) GetSessionStatus(
	ctx context.Context,
	req *protov1.SessionStatusRequest,
	opts ...grpc.CallOption,
) (*protov1.SessionStatusResponse, error) {
	return &protov1.SessionStatusResponse{}, nil
}

type mockTaskExecForIntegration struct {
	protov1.UnimplementedTaskExecutionServer
	sessionPool   *worker.SessionPool
	fileStorage   map[string]string // Shared file storage across tasks
	fileStorageMu sync.Mutex
}

func (m *mockTaskExecForIntegration) ExecuteTask(
	ctx context.Context,
	req *protov1.TaskRequest,
	opts ...grpc.CallOption,
) (protov1.TaskExecution_ExecuteTaskClient, error) {
	// Create a stream wrapper with shared file storage
	m.fileStorageMu.Lock()
	if m.fileStorage == nil {
		m.fileStorage = make(map[string]string)
	}
	m.fileStorageMu.Unlock()

	stream := &mockExecuteTaskStreamForIntegration{
		ctx:         ctx,
		req:         req,
		sessionPool: m.sessionPool,
		fileStorage: m.fileStorage,
		storageMu:   &m.fileStorageMu,
	}

	// Execute task and get responses
	stream.execute()

	return stream, nil
}

func (m *mockTaskExecForIntegration) CancelTask(
	ctx context.Context,
	req *protov1.CancelRequest,
	opts ...grpc.CallOption,
) (*protov1.CancelResponse, error) {
	return &protov1.CancelResponse{}, nil
}

func (m *mockTaskExecForIntegration) GetTaskStatus(
	ctx context.Context,
	req *protov1.StatusRequest,
	opts ...grpc.CallOption,
) (*protov1.StatusResponse, error) {
	return &protov1.StatusResponse{}, nil
}

type mockExecuteTaskStreamForIntegration struct {
	grpc.ClientStream
	ctx         context.Context
	req         *protov1.TaskRequest
	sessionPool *worker.SessionPool
	responses   []*protov1.TaskResponse
	index       int
	fileStorage map[string]string // Shared reference to parent's file storage
	storageMu   *sync.Mutex       // Shared lock for file storage
}

func (m *mockExecuteTaskStreamForIntegration) execute() {
	// Verify session exists
	_, err := m.sessionPool.GetSession(m.req.SessionId)
	if err != nil {
		m.responses = []*protov1.TaskResponse{
			{
				Payload: &protov1.TaskResponse_Error{
					Error: &protov1.TaskError{
						Code:    "SESSION_NOT_FOUND",
						Message: fmt.Sprintf("session not found: %v", err),
					},
				},
			},
		}
		return
	}

	// Execute the task based on tool name
	var result *protov1.TaskResult
	switch m.req.ToolName {
	case "echo":
		result = &protov1.TaskResult{
			Status: protov1.TaskResult_STATUS_SUCCESS,
			Outputs: map[string]string{
				"output": m.req.Arguments["message"],
			},
		}
	case "fs.write":
		// Simulate file write
		m.storageMu.Lock()
		if m.fileStorage == nil {
			m.fileStorage = make(map[string]string)
		}
		m.fileStorage[m.req.Arguments["path"]] = m.req.Arguments["contents"]
		m.storageMu.Unlock()

		result = &protov1.TaskResult{
			Status: protov1.TaskResult_STATUS_SUCCESS,
			Outputs: map[string]string{
				"output": fmt.Sprintf("Wrote file %s", m.req.Arguments["path"]),
			},
		}
	case "fs.read":
		// Simulate file read
		path := m.req.Arguments["path"]
		m.storageMu.Lock()
		content, exists := m.fileStorage[path]
		m.storageMu.Unlock()

		if !exists {
			result = &protov1.TaskResult{
				Status: protov1.TaskResult_STATUS_FAILURE,
				Outputs: map[string]string{
					"error": "file not found",
				},
			}
		} else {
			result = &protov1.TaskResult{
				Status: protov1.TaskResult_STATUS_SUCCESS,
				Outputs: map[string]string{
					"output": content,
				},
			}
		}
	default:
		result = &protov1.TaskResult{
			Status: protov1.TaskResult_STATUS_FAILURE,
			Outputs: map[string]string{
				"error": "unknown tool: " + m.req.ToolName,
			},
		}
	}

	m.responses = []*protov1.TaskResponse{
		{
			Payload: &protov1.TaskResponse_Result{
				Result: result,
			},
		},
	}
}

func (m *mockExecuteTaskStreamForIntegration) Recv() (*protov1.TaskResponse, error) {
	if m.index >= len(m.responses) {
		return nil, io.EOF
	}
	resp := m.responses[m.index]
	m.index++
	return resp, nil
}

type mockArtifactServiceForIntegration struct {
	protov1.UnimplementedArtifactServiceServer
}

func (m *mockArtifactServiceForIntegration) GetUploadURL(
	ctx context.Context,
	req *protov1.UploadRequest,
	opts ...grpc.CallOption,
) (*protov1.UploadResponse, error) {
	return &protov1.UploadResponse{}, nil
}

func (m *mockArtifactServiceForIntegration) RecordArtifact(
	ctx context.Context,
	req *protov1.ArtifactMetadata,
	opts ...grpc.CallOption,
) (*protov1.ArtifactResponse, error) {
	return &protov1.ArtifactResponse{}, nil
}
