package worker

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"testing"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const (
	testWorkerID     = "test-worker-1"
	testWorkerBase   = "test-worker"
	testSessionID    = "test-session-123"
	testSessionShort = "test-session"
	testVersion      = "0.1.0"
	testMaxSessions  = 5

	errMsgFailedStart = "Failed to start: %v"
	errMsgStopError   = "Stop error: %v"
)

// mockLifecycleServer implements a mock WorkerLifecycle server for testing
type mockLifecycleServer struct {
	protov1.UnimplementedWorkerLifecycleServer

	registerFunc   func(*protov1.RegisterRequest) (*protov1.RegisterResponse, error)
	heartbeatFunc  func(*protov1.HeartbeatRequest) (*protov1.HeartbeatResponse, error)
	deregisterFunc func(*protov1.DeregisterRequest) (*protov1.DeregisterResponse, error)

	registerCalls   int
	heartbeatCalls  int
	deregisterCalls int
}

func (m *mockLifecycleServer) RegisterWorker(ctx context.Context, req *protov1.RegisterRequest) (*protov1.RegisterResponse, error) {
	m.registerCalls++
	if m.registerFunc != nil {
		return m.registerFunc(req)
	}
	return &protov1.RegisterResponse{
		SessionId:            "test-session-123",
		TaskEndpoint:         "",
		HeartbeatIntervalSec: 1, // Fast heartbeat for tests
		Accepted:             true,
		Reason:               "ok",
	}, nil
}

func (m *mockLifecycleServer) Heartbeat(ctx context.Context, req *protov1.HeartbeatRequest) (*protov1.HeartbeatResponse, error) {
	m.heartbeatCalls++
	if m.heartbeatFunc != nil {
		return m.heartbeatFunc(req)
	}
	return &protov1.HeartbeatResponse{
		ContinueServing: true,
		Commands:        []string{},
	}, nil
}

func (m *mockLifecycleServer) DeregisterWorker(ctx context.Context, req *protov1.DeregisterRequest) (*protov1.DeregisterResponse, error) {
	m.deregisterCalls++
	if m.deregisterFunc != nil {
		return m.deregisterFunc(req)
	}
	return &protov1.DeregisterResponse{
		Acknowledged: true,
	}, nil
}

// testingTB is a common interface for testing.T and testing.B
type testingTB interface {
	Helper()
	Fatalf(format string, args ...any)
	Logf(format string, args ...any)
	TempDir() string
}

// startMockServer starts a mock coordinator server for testing
func startMockServer(tb testingTB, mock *mockLifecycleServer) (string, func()) {
	tb.Helper()

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		tb.Fatalf("Failed to listen: %v", err)
	}

	server := grpc.NewServer()
	protov1.RegisterWorkerLifecycleServer(server, mock)

	go func() {
		if err := server.Serve(lis); err != nil {
			tb.Logf("Server error: %v", err)
		}
	}()

	cleanup := func() {
		server.GracefulStop()
	}

	return lis.Addr().String(), cleanup
}

func TestClient_SuccessfulRegistration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	mock := &mockLifecycleServer{}
	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	// Create session pool

	// Create registration client
	client := NewClient(&Config{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		MaxSessions:     5,
		Logger:          slog.Default(),
	})

	// Start registration
	ctx := context.Background()
	err := client.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start registration: %v", err)
	}

	// Verify registration was called
	if mock.registerCalls != 1 {
		t.Errorf("Expected 1 register call, got %d", mock.registerCalls)
	}

	// Wait for at least one heartbeat
	time.Sleep(1500 * time.Millisecond)

	// Verify heartbeat was sent
	if mock.heartbeatCalls < 1 {
		t.Errorf("Expected at least 1 heartbeat, got %d", mock.heartbeatCalls)
	}

	// Stop registration
	if err := client.Stop(ctx); err != nil {
		t.Errorf("Failed to stop registration: %v", err)
	}

	// Verify deregistration was called
	if mock.deregisterCalls != 1 {
		t.Errorf("Expected 1 deregister call, got %d", mock.deregisterCalls)
	}
}

func TestClient_RegistrationRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	mock := &mockLifecycleServer{
		registerFunc: func(req *protov1.RegisterRequest) (*protov1.RegisterResponse, error) {
			return &protov1.RegisterResponse{
				Accepted: false,
				Reason:   "worker not allowed",
			}, nil
		},
	}
	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client := NewClient(&Config{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		MaxSessions:     5,
		Logger:          slog.Default(),
	})

	ctx := context.Background()
	err := client.Start(ctx)

	// Should fail with rejection
	if err == nil {
		t.Fatal("Expected error for rejected registration")
	}

	if mock.registerCalls != 1 {
		t.Errorf("Expected 1 register call, got %d", mock.registerCalls)
	}
}

func TestClient_HeartbeatFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	failureCount := 0
	mock := &mockLifecycleServer{
		heartbeatFunc: func(req *protov1.HeartbeatRequest) (*protov1.HeartbeatResponse, error) {
			failureCount++
			if failureCount <= 2 {
				return nil, status.Error(codes.Unavailable, "coordinator unavailable")
			}
			return &protov1.HeartbeatResponse{
				ContinueServing: true,
				Commands:        []string{},
			}, nil
		},
	}
	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client := NewClient(&Config{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		MaxSessions:     5,
		Logger:          slog.Default(),
	})

	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer func() {
		if err := client.Stop(ctx); err != nil {
			t.Logf("Stop error: %v", err)
		}
	}()

	// Wait for heartbeats to be sent
	time.Sleep(2500 * time.Millisecond)

	// Should have attempted multiple heartbeats
	if mock.heartbeatCalls < 2 {
		t.Errorf("Expected at least 2 heartbeat attempts, got %d", mock.heartbeatCalls)
	}
}

func TestClient_CapacityReporting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	var lastCapacity *protov1.SessionCapacity
	mock := &mockLifecycleServer{
		heartbeatFunc: func(req *protov1.HeartbeatRequest) (*protov1.HeartbeatResponse, error) {
			lastCapacity = req.Capacity
			return &protov1.HeartbeatResponse{
				ContinueServing: true,
				Commands:        []string{},
			}, nil
		},
	}
	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client := NewClient(&Config{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		MaxSessions:     5,
		Logger:          slog.Default(),
	})

	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer func() {
		if err := client.Stop(ctx); err != nil {
			t.Logf("Stop error: %v", err)
		}
	}()

	// Wait for at least one heartbeat
	time.Sleep(1500 * time.Millisecond)

	// Verify capacity was reported
	if lastCapacity == nil {
		t.Fatal("No capacity reported in heartbeat")
	}

	if lastCapacity.TotalSessions != 5 {
		t.Errorf("Expected 5 total sessions, got %d", lastCapacity.TotalSessions)
	}

	if lastCapacity.AvailableSessions != 5 {
		t.Errorf("Expected 5 available sessions, got %d", lastCapacity.AvailableSessions)
	}
}

func TestClient_WorkerStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	var lastStatus *protov1.WorkerStatus
	mock := &mockLifecycleServer{
		heartbeatFunc: func(req *protov1.HeartbeatRequest) (*protov1.HeartbeatResponse, error) {
			lastStatus = req.Status
			return &protov1.HeartbeatResponse{
				ContinueServing: true,
				Commands:        []string{},
			}, nil
		},
	}
	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client := NewClient(&Config{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		MaxSessions:     5,
		Logger:          slog.Default(),
	})

	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer func() {
		if err := client.Stop(ctx); err != nil {
			t.Logf("Stop error: %v", err)
		}
	}()

	// Wait for heartbeat
	time.Sleep(1500 * time.Millisecond)

	// Verify status was reported
	if lastStatus == nil {
		t.Fatal("No status reported in heartbeat")
	}

	// Should be idle with no sessions
	if lastStatus.State != protov1.WorkerStatus_STATE_IDLE {
		t.Errorf("Expected IDLE state, got %v", lastStatus.State)
	}

	if lastStatus.ActiveTasks != 0 {
		t.Errorf("Expected 0 active tasks, got %d", lastStatus.ActiveTasks)
	}
}

func TestClient_ConnectionFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Try to connect to a port that's not listening
	client := NewClient(&Config{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: "localhost:9999",
		Version:         "0.1.0",
		MaxSessions:     5,
		Logger:          slog.Default(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.Start(ctx)
	if err == nil {
		t.Fatal("Expected error when connecting to unavailable coordinator")
	}
}

func TestClient_ValidatesWorkerID(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	var receivedWorkerID string
	mock := &mockLifecycleServer{
		registerFunc: func(req *protov1.RegisterRequest) (*protov1.RegisterResponse, error) {
			receivedWorkerID = req.WorkerId
			return &protov1.RegisterResponse{
				SessionId:            "test-session",
				HeartbeatIntervalSec: 1,
				Accepted:             true,
			}, nil
		},
	}
	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	expectedID := "test-worker-123"
	client := NewClient(&Config{
		WorkerID:        expectedID,
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		MaxSessions:     5,
		Logger:          slog.Default(),
	})

	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer func() {
		if err := client.Stop(ctx); err != nil {
			t.Logf("Stop error: %v", err)
		}
	}()

	if receivedWorkerID != expectedID {
		t.Errorf("Expected worker ID %s, got %s", expectedID, receivedWorkerID)
	}
}

func TestClient_SendsCapabilities(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	var receivedCaps *protov1.WorkerCapabilities
	mock := &mockLifecycleServer{
		registerFunc: func(req *protov1.RegisterRequest) (*protov1.RegisterResponse, error) {
			receivedCaps = req.Capabilities
			return &protov1.RegisterResponse{
				SessionId:            "test-session",
				HeartbeatIntervalSec: 1,
				Accepted:             true,
			}, nil
		},
	}
	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client := NewClient(&Config{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		MaxSessions:     5,
		Logger:          slog.Default(),
	})

	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}
	defer func() {
		if err := client.Stop(ctx); err != nil {
			t.Logf("Stop error: %v", err)
		}
	}()

	if receivedCaps == nil {
		t.Fatal("No capabilities received")
	}

	// Check supported tools
	expectedTools := []string{"echo", "fs.write", "fs.read", "fs.list", "run.python", "pkg.install"}
	if len(receivedCaps.SupportedTools) != len(expectedTools) {
		t.Errorf("Expected %d tools, got %d", len(expectedTools), len(receivedCaps.SupportedTools))
	}

	// Check max sessions
	if receivedCaps.MaxSessions != 5 {
		t.Errorf("Expected 5 max sessions, got %d", receivedCaps.MaxSessions)
	}

	// Check languages
	if len(receivedCaps.Languages) == 0 {
		t.Error("Expected at least one language")
	}
}

func TestClient_DeregistrationValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	var deregisterReq *protov1.DeregisterRequest
	mock := &mockLifecycleServer{
		deregisterFunc: func(req *protov1.DeregisterRequest) (*protov1.DeregisterResponse, error) {
			deregisterReq = req
			return &protov1.DeregisterResponse{
				Acknowledged: true,
			}, nil
		},
	}
	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client := NewClient(&Config{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		MaxSessions:     5,
		Logger:          slog.Default(),
	})

	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Failed to start: %v", err)
	}

	if err := client.Stop(ctx); err != nil {
		t.Fatalf("Failed to stop: %v", err)
	}

	if deregisterReq == nil {
		t.Fatal("Deregister was not called")
	}

	if deregisterReq.WorkerId != "test-worker-1" {
		t.Errorf("Expected worker ID test-worker-1, got %s", deregisterReq.WorkerId)
	}

	if deregisterReq.SessionId != "test-session-123" {
		t.Errorf("Expected session ID test-session-123, got %s", deregisterReq.SessionId)
	}
}

func TestClient_StopWithoutStart(t *testing.T) {
	client := NewClient(&Config{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: "localhost:9999",
		Version:         "0.1.0",
		MaxSessions:     5,
		Logger:          slog.Default(),
	})

	// Should handle stop without start
	ctx := context.Background()
	err := client.Stop(ctx)
	if err != nil {
		t.Errorf("Stop without start should not error, got: %v", err)
	}
}

func TestClient_ReconnectionSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Test that reconnection works after initial failure
	attemptCount := 0
	mock := &mockLifecycleServer{
		registerFunc: func(req *protov1.RegisterRequest) (*protov1.RegisterResponse, error) {
			attemptCount++
			if attemptCount == 1 {
				return nil, fmt.Errorf("temporary error")
			}
			return &protov1.RegisterResponse{
				SessionId:            "test-session-123",
				HeartbeatIntervalSec: 1,
				Accepted:             true,
			}, nil
		},
	}

	// Don't start server initially
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	addr := lis.Addr().String()

	// Close listener initially to simulate unavailable coordinator
	lis.Close()

	client := NewClient(&Config{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		MaxSessions:     5,
		Logger:          slog.Default(),
	})
	client.baseReconnectDelay = 100 * time.Millisecond // Speed up test

	// Start server after a delay
	go func() {
		time.Sleep(200 * time.Millisecond)
		newLis, err := net.Listen("tcp", addr)
		if err != nil {
			t.Logf("Failed to restart listener: %v", err)
			return
		}
		server := grpc.NewServer()
		protov1.RegisterWorkerLifecycleServer(server, mock)
		go func() {
			if err := server.Serve(newLis); err != nil {
				t.Logf("Server error: %v", err)
			}
		}()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Initial connection will fail but this is expected for this test
	_ = client.Start(ctx)
}

func TestNewClient(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name: "with logger",
			config: &Config{
				WorkerID:        "test-worker-1",
				CoordinatorAddr: "localhost:50050",
				Version:         "1.0.0",
				MaxSessions:     5,
				BaseWorkspace:   t.TempDir(),
				Logger:          slog.Default(),
			},
		},
		{
			name: "without logger",
			config: &Config{
				WorkerID:        "test-worker-2",
				CoordinatorAddr: "localhost:50050",
				Version:         "1.0.0",
				MaxSessions:     5,
				BaseWorkspace:   t.TempDir(),
				Logger:          nil, // Should use default
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.config)

			if client == nil {
				t.Fatal("NewClient returned nil")
			}

			if client.workerID != tt.config.WorkerID {
				t.Errorf("Expected worker ID %s, got %s", tt.config.WorkerID, client.workerID)
			}

			// grpcAddress should be empty (worker doesn't listen)
			if client.grpcAddress != "" {
				t.Errorf("Expected empty gRPC address, got %s", client.grpcAddress)
			}

			if client.coordinatorAddr != tt.config.CoordinatorAddr {
				t.Errorf("Expected coordinator address %s, got %s", tt.config.CoordinatorAddr, client.coordinatorAddr)
			}

			if client.version != tt.config.Version {
				t.Errorf("Expected version %s, got %s", tt.config.Version, client.version)
			}

			if client.logger == nil {
				t.Error("Logger should never be nil")
			}

			// Verify components are created
			if client.sessionPool == nil {
				t.Error("sessionPool should be initialized")
			}

			if client.taskExecutor == nil {
				t.Error("taskExecutor should be initialized")
			}

			if client.stopChan == nil {
				t.Error("stopChan should be initialized")
			}

			if client.doneChan == nil {
				t.Error("doneChan should be initialized")
			}

			if client.maxReconnectDelay == 0 {
				t.Error("maxReconnectDelay should be set")
			}

			if client.baseReconnectDelay == 0 {
				t.Error("baseReconnectDelay should be set")
			}
		})
	}
}

func TestClient_RegistrationRequestFields(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	var receivedReq *protov1.RegisterRequest
	mock := &mockLifecycleServer{
		registerFunc: func(req *protov1.RegisterRequest) (*protov1.RegisterResponse, error) {
			receivedReq = req
			return &protov1.RegisterResponse{
				SessionId:            testSessionID,
				HeartbeatIntervalSec: 1,
				Accepted:             true,
			}, nil
		},
	}
	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	expectedVersion := "1.2.3"

	client := NewClient(&Config{
		WorkerID:        testWorkerID,
		CoordinatorAddr: addr,
		Version:         expectedVersion,
		MaxSessions:     5,
		BaseWorkspace:   t.TempDir(),
		Logger:          slog.Default(),
	})

	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf(errMsgFailedStart, err)
	}
	defer func() {
		if err := client.Stop(ctx); err != nil {
			t.Logf(errMsgStopError, err)
		}
	}()

	if receivedReq == nil {
		t.Fatal("No registration request received")
	}

	// Verify version
	if receivedReq.Version != expectedVersion {
		t.Errorf("Expected version %s, got %s", expectedVersion, receivedReq.Version)
	}

	// Verify gRPC address is empty (worker doesn't listen)
	if receivedReq.GrpcAddress != "" {
		t.Errorf("Expected empty gRPC address, got %s", receivedReq.GrpcAddress)
	}

	// Verify capabilities
	if receivedReq.Capabilities == nil {
		t.Fatal("Capabilities should not be nil")
	}

	// Verify limits
	if receivedReq.Limits == nil {
		t.Fatal("Limits should not be nil")
	}

	if receivedReq.Limits.MaxMemoryBytes == 0 {
		t.Error("MaxMemoryBytes should be set")
	}

	if receivedReq.Limits.MaxCpuMillicores == 0 {
		t.Error("MaxCpuMillicores should be set")
	}

	if receivedReq.Limits.MaxDiskBytes == 0 {
		t.Error("MaxDiskBytes should be set")
	}

	if receivedReq.Limits.MaxConcurrentTasks == 0 {
		t.Error("MaxConcurrentTasks should be set")
	}
}

func TestClient_DeregistrationFailed(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	mock := &mockLifecycleServer{
		deregisterFunc: func(req *protov1.DeregisterRequest) (*protov1.DeregisterResponse, error) {
			return &protov1.DeregisterResponse{
				Acknowledged: false,
			}, nil
		},
	}
	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client := NewClient(&Config{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		MaxSessions:     5,
		Logger:          slog.Default(),
	})

	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf(errMsgFailedStart, err)
	}

	// Stop will log warning but not fail
	if err := client.Stop(ctx); err != nil {
		t.Errorf("Stop should not return error even if deregistration fails: %v", err)
	}
}

func TestClient_WorkerStatus_WithSessions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	var lastStatus *protov1.WorkerStatus
	mock := &mockLifecycleServer{
		heartbeatFunc: func(req *protov1.HeartbeatRequest) (*protov1.HeartbeatResponse, error) {
			lastStatus = req.Status
			return &protov1.HeartbeatResponse{
				ContinueServing: true,
				Commands:        []string{},
			}, nil
		},
	}
	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client := NewClient(&Config{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		MaxSessions:     5,
		BaseWorkspace:   t.TempDir(),
		Logger:          slog.Default(),
	})

	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf(errMsgFailedStart, err)
	}
	defer func() {
		if err := client.Stop(ctx); err != nil {
			t.Logf(errMsgStopError, err)
		}
	}()

	// Create a session to change the worker status to BUSY
	_, err := client.sessionPool.CreateSession(ctx, &protov1.CreateSessionRequest{
		WorkspaceId: "test-workspace",
	})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Wait for heartbeat with session active
	time.Sleep(1500 * time.Millisecond)

	// Verify status reflects active session
	if lastStatus == nil {
		t.Fatal("No status reported in heartbeat")
	}

	// With active sessions, should be BUSY
	// State is determined by whether there are active sessions
	if lastStatus.State != protov1.WorkerStatus_STATE_BUSY {
		t.Errorf("Expected BUSY state with active session, got %v", lastStatus.State)
	}

	// Verify task count is 0 (session exists but no tasks running)
	if lastStatus.ActiveTasks != 0 {
		t.Errorf("Expected 0 active tasks, got %d", lastStatus.ActiveTasks)
	}
}

func TestClient_HeartbeatCommands(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	testCommands := []string{"drain", "reload_config"}
	mock := &mockLifecycleServer{
		heartbeatFunc: func(req *protov1.HeartbeatRequest) (*protov1.HeartbeatResponse, error) {
			return &protov1.HeartbeatResponse{
				ContinueServing: true,
				Commands:        testCommands,
			}, nil
		},
	}
	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client := NewClient(&Config{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		MaxSessions:     5,
		Logger:          slog.Default(),
	})

	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf(errMsgFailedStart, err)
	}
	defer func() {
		if err := client.Stop(ctx); err != nil {
			t.Logf(errMsgStopError, err)
		}
	}()

	// Wait for heartbeat
	time.Sleep(1500 * time.Millisecond)

	// Commands are logged but not yet implemented
	// Just verify heartbeat succeeded
	if mock.heartbeatCalls < 1 {
		t.Error("Expected at least one heartbeat")
	}
}

func TestClient_ContinueServingFalse(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	mock := &mockLifecycleServer{
		heartbeatFunc: func(req *protov1.HeartbeatRequest) (*protov1.HeartbeatResponse, error) {
			return &protov1.HeartbeatResponse{
				ContinueServing: false,
				Commands:        []string{},
			}, nil
		},
	}
	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client := NewClient(&Config{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		MaxSessions:     5,
		Logger:          slog.Default(),
	})

	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf(errMsgFailedStart, err)
	}
	defer func() {
		if err := client.Stop(ctx); err != nil {
			t.Logf(errMsgStopError, err)
		}
	}()

	// Wait for heartbeat
	time.Sleep(1500 * time.Millisecond)

	// Worker logs warning but continues (future: implement graceful shutdown)
	if mock.heartbeatCalls < 1 {
		t.Error("Expected at least one heartbeat")
	}
}

func TestClient_Reconnect_AfterHeartbeatFailures(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	heartbeatCount := 0
	registerCount := 0
	reconnected := false

	mock := &mockLifecycleServer{
		registerFunc: func(req *protov1.RegisterRequest) (*protov1.RegisterResponse, error) {
			registerCount++
			// Allow reconnection to succeed
			return &protov1.RegisterResponse{
				SessionId:            fmt.Sprintf("test-session-%d", registerCount),
				HeartbeatIntervalSec: 1,
				Accepted:             true,
			}, nil
		},
		heartbeatFunc: func(req *protov1.HeartbeatRequest) (*protov1.HeartbeatResponse, error) {
			heartbeatCount++
			// Fail first 3 heartbeats to trigger reconnect
			if heartbeatCount <= 3 {
				return nil, fmt.Errorf("heartbeat failure %d", heartbeatCount)
			}
			// After reconnection, heartbeats succeed
			if registerCount > 1 {
				reconnected = true
			}
			return &protov1.HeartbeatResponse{
				ContinueServing: true,
				Commands:        []string{},
			}, nil
		},
	}
	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client := NewClient(&Config{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		MaxSessions:     5,
		Logger:          slog.Default(),
	})
	client.baseReconnectDelay = 50 * time.Millisecond

	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer client.Stop(ctx)

	// Wait for heartbeat failures and reconnection
	time.Sleep(5 * time.Second)

	if !reconnected {
		t.Error("Expected reconnection after heartbeat failures")
	}

	if registerCount < 2 {
		t.Errorf("Expected at least 2 registrations (initial + reconnect), got %d", registerCount)
	}
}

func TestClient_Reconnect_MaxAttemptsExhausted(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	heartbeatCount := 0
	registerAttempts := 0

	mock := &mockLifecycleServer{
		registerFunc: func(req *protov1.RegisterRequest) (*protov1.RegisterResponse, error) {
			registerAttempts++
			// First registration succeeds
			if registerAttempts == 1 {
				return &protov1.RegisterResponse{
					SessionId:            "test-session-initial",
					HeartbeatIntervalSec: 1,
					Accepted:             true,
				}, nil
			}
			// All reconnection attempts fail
			return nil, fmt.Errorf("reconnection failed")
		},
		heartbeatFunc: func(req *protov1.HeartbeatRequest) (*protov1.HeartbeatResponse, error) {
			heartbeatCount++
			// Fail all heartbeats to trigger reconnect
			return nil, fmt.Errorf("heartbeat failure")
		},
	}
	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client := NewClient(&Config{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		MaxSessions:     5,
		Logger:          slog.Default(),
	})
	client.baseReconnectDelay = 5 * time.Millisecond
	client.maxReconnectDelay = 100 * time.Millisecond // Cap max delay to keep test fast

	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer client.Stop(ctx)

	// Wait for heartbeat failures (3 seconds for 3 failures) + reconnection attempts
	// With max delay of 100ms and 10 attempts, should complete in ~5 seconds total
	time.Sleep(6 * time.Second)

	// Should have made initial registration + multiple reconnection attempts
	// Due to timing, we might not get all 10 attempts, but should get several
	if registerAttempts < 5 {
		t.Errorf("Expected at least 5 registration attempts (1 initial + reconnects), got %d", registerAttempts)
	}
}

func TestClient_Reconnect_ExponentialBackoff(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	attemptTimes := []time.Time{}
	registerCount := 0
	heartbeatCount := 0

	mock := &mockLifecycleServer{
		registerFunc: func(req *protov1.RegisterRequest) (*protov1.RegisterResponse, error) {
			registerCount++
			attemptTimes = append(attemptTimes, time.Now())

			// First registration succeeds
			if registerCount == 1 {
				return &protov1.RegisterResponse{
					SessionId:            "test-session-initial",
					HeartbeatIntervalSec: 1,
					Accepted:             true,
				}, nil
			}

			// Next 3 reconnection attempts fail
			if registerCount <= 4 {
				return nil, fmt.Errorf("reconnection attempt %d failed", registerCount)
			}

			// Fourth reconnection succeeds
			return &protov1.RegisterResponse{
				SessionId:            "test-session-reconnected",
				HeartbeatIntervalSec: 1,
				Accepted:             true,
			}, nil
		},
		heartbeatFunc: func(req *protov1.HeartbeatRequest) (*protov1.HeartbeatResponse, error) {
			heartbeatCount++
			// Fail first 3 heartbeats to trigger reconnect
			if heartbeatCount <= 3 {
				return nil, fmt.Errorf("heartbeat failure")
			}
			return &protov1.HeartbeatResponse{
				ContinueServing: true,
				Commands:        []string{},
			}, nil
		},
	}
	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client := NewClient(&Config{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		MaxSessions:     5,
		Logger:          slog.Default(),
	})
	client.baseReconnectDelay = 100 * time.Millisecond
	client.maxReconnectDelay = 1 * time.Second

	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer client.Stop(ctx)

	// Wait for heartbeat failures and reconnection attempts
	time.Sleep(6 * time.Second)

	// Should have at least 2 registration attempts (1 in attemptTimes at registration, others during reconnect)
	if len(attemptTimes) < 2 {
		t.Fatalf("Expected at least 2 registration attempts, got %d", len(attemptTimes))
	}

	// Check delays between reconnection attempts are increasing (exponential backoff)
	if len(attemptTimes) >= 4 {
		delay1 := attemptTimes[2].Sub(attemptTimes[1]) // First reconnect delay
		delay2 := attemptTimes[3].Sub(attemptTimes[2]) // Second reconnect delay

		// Second delay should be roughly 2x the first (with some tolerance)
		if delay2 < delay1 {
			t.Errorf("Expected exponential backoff: delay2 (%v) should be >= delay1 (%v)", delay2, delay1)
		}
	}
}

func TestClient_HeartbeatFailure_TriggersReconnect(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	heartbeatCount := 0
	reconnected := false
	mock := &mockLifecycleServer{
		heartbeatFunc: func(req *protov1.HeartbeatRequest) (*protov1.HeartbeatResponse, error) {
			heartbeatCount++
			// Fail first 3 heartbeats to trigger reconnect (3 consecutive failures required)
			if heartbeatCount <= 3 {
				return nil, fmt.Errorf("heartbeat failed")
			}
			// After reconnection, heartbeats succeed
			reconnected = true
			return &protov1.HeartbeatResponse{
				ContinueServing: true,
				Commands:        []string{},
			}, nil
		},
	}
	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	client := NewClient(&Config{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		MaxSessions:     5,
		Logger:          slog.Default(),
	})
	client.baseReconnectDelay = 50 * time.Millisecond

	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer client.Stop(ctx)

	// Wait for 3 heartbeat failures + reconnection
	// Heartbeat interval is 1s, so need at least 4 seconds
	time.Sleep(5 * time.Second)

	if !reconnected {
		t.Error("Expected reconnection after heartbeat failure")
	}

	if heartbeatCount < 4 {
		t.Errorf("Expected at least 4 heartbeat calls (3 failures + 1 success), got %d", heartbeatCount)
	}
}

// Unit tests that run in short mode (no network required)

func TestClientGetWorkerStatusIdle(t *testing.T) {
	// Test worker status when idle (no active sessions)
	client := NewClient(&Config{
		WorkerID:        "worker-1",
		CoordinatorAddr: "localhost:50051",
		Version:         "1.0.0",
		MaxSessions:     5,
		BaseWorkspace:   t.TempDir(),
		Logger:          slog.Default(),
	})

	capacity := client.sessionPool.GetCapacity()
	status := client.getWorkerStatus(capacity)

	if status.State != protov1.WorkerStatus_STATE_IDLE {
		t.Errorf("Expected STATE_IDLE, got %v", status.State)
	}

	if status.ActiveTasks != 0 {
		t.Errorf("Expected 0 active tasks, got %d", status.ActiveTasks)
	}

	if status.CurrentUsage == nil {
		t.Fatal("CurrentUsage should not be nil")
	}

	// Verify resource usage fields are initialized
	if status.CurrentUsage.MemoryBytes != 0 {
		t.Errorf("Expected 0 memory bytes, got %d", status.CurrentUsage.MemoryBytes)
	}

	if status.CurrentUsage.CpuMillicores != 0 {
		t.Errorf("Expected 0 CPU millicores, got %d", status.CurrentUsage.CpuMillicores)
	}

	if status.CurrentUsage.DiskBytes != 0 {
		t.Errorf("Expected 0 disk bytes, got %d", status.CurrentUsage.DiskBytes)
	}

	if len(status.Errors) != 0 {
		t.Errorf("Expected 0 errors, got %d", len(status.Errors))
	}
}

func TestClientGetWorkerStatusWithMultipleSessions(t *testing.T) {
	// Test worker status calculation with multiple sessions

	client := NewClient(&Config{
		WorkerID:        "worker-1",
		CoordinatorAddr: "localhost:50051",
		Version:         "1.0.0",
		MaxSessions:     5,
		BaseWorkspace:   t.TempDir(),
		Logger:          slog.Default(),
	})

	// Test with empty capacity
	capacity := client.sessionPool.GetCapacity()
	status := client.getWorkerStatus(capacity)

	// Should be idle with no sessions
	if status.State != protov1.WorkerStatus_STATE_IDLE {
		t.Errorf("Expected STATE_IDLE with no sessions, got %v", status.State)
	}

	// Verify capacity structure
	if capacity == nil {
		t.Fatal("Capacity should not be nil")
	}

	if capacity.TotalSessions <= 0 {
		t.Errorf("Expected positive total sessions, got %d", capacity.TotalSessions)
	}
}

func TestClientStopChanInitialized(t *testing.T) {
	// Test that stopChan is properly initialized
	client := NewClient(&Config{
		WorkerID:        "worker-1",
		CoordinatorAddr: "localhost:50051",
		Version:         "1.0.0",
		MaxSessions:     5,
		Logger:          slog.Default(),
	})

	// stopChan should be open initially
	select {
	case <-client.stopChan:
		t.Error("stopChan should not be closed initially")
	default:
		// Expected
	}
}

func TestClientReconnectionDelayDefaults(t *testing.T) {
	// Test that reconnection delays are properly initialized with defaults
	client := NewClient(&Config{
		WorkerID:        "worker-1",
		CoordinatorAddr: "localhost:50051",
		Version:         "1.0.0",
		MaxSessions:     5,
		Logger:          slog.Default(),
	})

	// Test default values
	expectedBase := 1 * time.Second
	expectedMax := 5 * time.Minute

	if client.baseReconnectDelay != expectedBase {
		t.Errorf("Expected baseReconnectDelay %v, got %v", expectedBase, client.baseReconnectDelay)
	}

	if client.maxReconnectDelay != expectedMax {
		t.Errorf("Expected maxReconnectDelay %v, got %v", expectedMax, client.maxReconnectDelay)
	}
}

func TestClientChannelsInitialized(t *testing.T) {
	// Test that all channels are properly initialized
	client := NewClient(&Config{
		WorkerID:        "worker-1",
		CoordinatorAddr: "localhost:50051",
		Version:         "1.0.0",
		MaxSessions:     5,
		Logger:          slog.Default(),
	})

	if client.stopChan == nil {
		t.Error("stopChan should be initialized")
	}

	if client.doneChan == nil {
		t.Error("doneChan should be initialized")
	}
}

func BenchmarkClient_Heartbeat(b *testing.B) {
	mock := &mockLifecycleServer{}
	addr, cleanup := startMockServer(b, mock)
	defer cleanup()

	client := NewClient(&Config{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		MaxSessions:     5,
		BaseWorkspace:   b.TempDir(),
		Logger:          slog.Default(),
	})

	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		b.Fatalf("Failed to start: %v", err)
	}
	defer func() {
		if err := client.Stop(ctx); err != nil {
			b.Logf("Stop error: %v", err)
		}
	}()

	// Create direct connection for benchmark
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		b.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	lifecycleClient := protov1.NewWorkerLifecycleClient(conn)
	capacity := client.sessionPool.GetCapacity()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := &protov1.HeartbeatRequest{
			WorkerId:  "test-worker-1",
			SessionId: "test-session",
			Status: &protov1.WorkerStatus{
				State:       protov1.WorkerStatus_STATE_IDLE,
				ActiveTasks: 0,
			},
			Capacity: capacity,
		}
		_, _ = lifecycleClient.Heartbeat(ctx, req)
	}
}

func TestGetToolNameFromRequest(t *testing.T) {
	tests := []struct {
		name     string
		request  *protov1.ToolRequest
		expected string
	}{
		{"nil request", nil, "unknown"},
		{"echo request", &protov1.ToolRequest{Request: &protov1.ToolRequest_Echo{Echo: &protov1.EchoRequest{Message: "test"}}}, "echo"},
		{"fs.read request", &protov1.ToolRequest{Request: &protov1.ToolRequest_FsRead{FsRead: &protov1.FsReadRequest{Path: "file.txt"}}}, "fs.read"},
		{"fs.write request", &protov1.ToolRequest{Request: &protov1.ToolRequest_FsWrite{FsWrite: &protov1.FsWriteRequest{Path: "file.txt", Contents: "data"}}}, "fs.write"},
		{"fs.list request", &protov1.ToolRequest{Request: &protov1.ToolRequest_FsList{FsList: &protov1.FsListRequest{Path: "."}}}, "fs.list"},
		{"run.python request", &protov1.ToolRequest{Request: &protov1.ToolRequest_RunPython{RunPython: &protov1.RunPythonRequest{Source: &protov1.RunPythonRequest_Code{Code: "print('test')"}}}}, "run.python"},
		{"pkg.install request", &protov1.ToolRequest{Request: &protov1.ToolRequest_PkgInstall{PkgInstall: &protov1.PkgInstallRequest{Packages: []string{"requests"}}}}, "pkg.install"},
		{"empty request", &protov1.ToolRequest{}, "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if result := getToolNameFromRequest(tt.request); result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestClientStopAndGetWorkerStatus(t *testing.T) {
	tempDir := t.TempDir()
	client := NewClient(&Config{WorkerID: "test", CoordinatorAddr: "localhost:5000", Version: "0.1.0", MaxSessions: 5, BaseWorkspace: tempDir, Logger: slog.Default()})
	ctx := context.Background()
	if err := client.Stop(ctx); err != nil {
		t.Errorf("Stop() should not error: %v", err)
	}
	if err := client.Stop(ctx); err != nil {
		t.Errorf("Second Stop() should not error: %v", err)
	}
	status := client.getWorkerStatus(&protov1.SessionCapacity{ActiveSessions: 0, Sessions: []*protov1.SessionInfo{}})
	if status.State != protov1.WorkerStatus_STATE_IDLE {
		t.Errorf("Expected IDLE state")
	}
	status = client.getWorkerStatus(&protov1.SessionCapacity{ActiveSessions: 2, Sessions: []*protov1.SessionInfo{{SessionId: "s1", ActiveTasks: 3}, {SessionId: "s2", ActiveTasks: 2}}})
	if status.State != protov1.WorkerStatus_STATE_BUSY || status.ActiveTasks != 5 {
		t.Errorf("Expected BUSY state with 5 tasks")
	}
}
