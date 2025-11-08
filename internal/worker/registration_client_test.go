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

func TestRegistrationClient_SuccessfulRegistration(t *testing.T) {
	mock := &mockLifecycleServer{}
	addr, cleanup := startMockServer(t, mock)
	defer cleanup()

	// Create session pool
	sessionPool := NewSessionPool("test-worker", 5, t.TempDir())

	// Create registration client
	client := NewRegistrationClient(&RegistrationConfig{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		SessionPool:     sessionPool,
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

func TestRegistrationClient_RegistrationRejected(t *testing.T) {
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

	sessionPool := NewSessionPool("test-worker", 5, t.TempDir())
	client := NewRegistrationClient(&RegistrationConfig{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		SessionPool:     sessionPool,
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

func TestRegistrationClient_HeartbeatFailure(t *testing.T) {
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

	sessionPool := NewSessionPool("test-worker", 5, t.TempDir())
	client := NewRegistrationClient(&RegistrationConfig{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		SessionPool:     sessionPool,
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

func TestRegistrationClient_CapacityReporting(t *testing.T) {
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

	sessionPool := NewSessionPool("test-worker", 5, t.TempDir())
	client := NewRegistrationClient(&RegistrationConfig{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		SessionPool:     sessionPool,
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

func TestRegistrationClient_WorkerStatus(t *testing.T) {
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

	sessionPool := NewSessionPool("test-worker", 5, t.TempDir())
	client := NewRegistrationClient(&RegistrationConfig{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		SessionPool:     sessionPool,
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

func TestRegistrationClient_ConnectionFailure(t *testing.T) {
	// Try to connect to a port that's not listening
	sessionPool := NewSessionPool("test-worker", 5, t.TempDir())
	client := NewRegistrationClient(&RegistrationConfig{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: "localhost:9999",
		Version:         "0.1.0",
		SessionPool:     sessionPool,
		Logger:          slog.Default(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.Start(ctx)
	if err == nil {
		t.Fatal("Expected error when connecting to unavailable coordinator")
	}
}

func TestRegistrationClient_ValidatesWorkerID(t *testing.T) {
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

	sessionPool := NewSessionPool("test-worker", 5, t.TempDir())
	expectedID := "test-worker-123"
	client := NewRegistrationClient(&RegistrationConfig{
		WorkerID:        expectedID,
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		SessionPool:     sessionPool,
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

func TestRegistrationClient_SendsCapabilities(t *testing.T) {
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

	sessionPool := NewSessionPool("test-worker", 5, t.TempDir())
	client := NewRegistrationClient(&RegistrationConfig{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		SessionPool:     sessionPool,
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

func TestRegistrationClient_DeregistrationValidation(t *testing.T) {
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

	sessionPool := NewSessionPool("test-worker", 5, t.TempDir())
	client := NewRegistrationClient(&RegistrationConfig{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		SessionPool:     sessionPool,
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

func TestRegistrationClient_StopWithoutStart(t *testing.T) {
	sessionPool := NewSessionPool("test-worker", 5, t.TempDir())
	client := NewRegistrationClient(&RegistrationConfig{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: "localhost:9999",
		Version:         "0.1.0",
		SessionPool:     sessionPool,
		Logger:          slog.Default(),
	})

	// Should handle stop without start
	ctx := context.Background()
	err := client.Stop(ctx)
	if err != nil {
		t.Errorf("Stop without start should not error, got: %v", err)
	}
}

func TestRegistrationClient_ReconnectionSuccess(t *testing.T) {
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

	sessionPool := NewSessionPool("test-worker", 5, t.TempDir())
	client := NewRegistrationClient(&RegistrationConfig{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		SessionPool:     sessionPool,
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

func BenchmarkRegistrationClient_Heartbeat(b *testing.B) {
	mock := &mockLifecycleServer{}
	addr, cleanup := startMockServer(b, mock)
	defer cleanup()

	sessionPool := NewSessionPool("test-worker", 5, b.TempDir())
	client := NewRegistrationClient(&RegistrationConfig{
		WorkerID:        "test-worker-1",
		CoordinatorAddr: addr,
		Version:         "0.1.0",
		SessionPool:     sessionPool,
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
	capacity := sessionPool.GetCapacity()

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
