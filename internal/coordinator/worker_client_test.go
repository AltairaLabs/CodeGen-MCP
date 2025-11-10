package coordinator

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

// Helper to create a RealWorkerClient for testing
func newTestRealWorkerClient(t *testing.T) (*RealWorkerClient, *SessionManager, *WorkerRegistry) {
	t.Helper()

	registry := NewWorkerRegistry()
	sessionStorage := newTestSessionStorage()
	sm := NewSessionManager(sessionStorage, registry)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	client := NewRealWorkerClient(registry, sm, logger)
	return client, sm, registry
}

func TestNewRealWorkerClient(t *testing.T) {
	registry := NewWorkerRegistry()
	sessionStorage := newTestSessionStorage()
	sm := NewSessionManager(sessionStorage, registry)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	client := NewRealWorkerClient(registry, sm, logger)

	if client == nil {
		t.Fatal("Expected non-nil RealWorkerClient")
	}
	if client.registry != registry {
		t.Error("Expected registry to be set")
	}
	if client.sessionManager != sm {
		t.Error("Expected session manager to be set")
	}
	if client.logger != logger {
		t.Error("Expected logger to be set")
	}
}

func TestRealWorkerClient_CheckSessionState(t *testing.T) {
	client, _, _ := newTestRealWorkerClient(t)

	tests := []struct {
		name        string
		state       SessionState
		stateMsg    string
		expectError bool
	}{
		{
			name:        "ready state",
			state:       SessionStateReady,
			expectError: false,
		},
		{
			name:        "creating state",
			state:       SessionStateCreating,
			expectError: false,
		},
		{
			name:        "failed state",
			state:       SessionStateFailed,
			stateMsg:    "test failure",
			expectError: true,
		},
		{
			name:        "terminating state",
			state:       SessionStateTerminating,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := &Session{
				ID:           "test-session",
				State:        tt.state,
				StateMessage: tt.stateMsg,
			}

			err := client.checkSessionState(session)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestRealWorkerClient_GetSessionAndWorker_NoSession(t *testing.T) {
	client, _, _ := newTestRealWorkerClient(t)
	ctx := context.Background()

	// Try to get session and worker when no session exists
	_, _, err := client.getSessionAndWorker(ctx)
	if err == nil {
		t.Error("Expected error when session doesn't exist")
	}
}

func TestRealWorkerClient_GetSessionAndWorker_NoWorkerAssigned(t *testing.T) {
	client, sm, _ := newTestRealWorkerClient(t)
	ctx := context.Background()

	// Create a session without worker assignment
	session := sm.CreateSession(ctx, "test-session", "user1", "workspace1")
	session.WorkerID = "" // Explicitly clear worker ID
	_ = sm.storage.CreateSession(ctx, session)

	// Try with session in context
	ctx = context.WithValue(ctx, sessionIDKey{}, session.ID)
	_, _, err := client.getSessionAndWorker(ctx)
	if err == nil {
		t.Error("Expected error when no worker assigned")
	}
}

func TestRealWorkerClient_GetSessionAndWorker_WorkerNotFound(t *testing.T) {
	client, sm, _ := newTestRealWorkerClient(t)
	ctx := context.Background()

	// Create a session with non-existent worker ID
	session := sm.CreateSession(ctx, "test-session", "user1", "workspace1")
	session.WorkerID = "nonexistent-worker"
	session.State = SessionStateReady
	_ = sm.storage.CreateSession(ctx, session)

	// Try with session in context
	ctx = context.WithValue(ctx, sessionIDKey{}, session.ID)
	_, _, err := client.getSessionAndWorker(ctx)
	if err == nil {
		t.Error("Expected error when worker not found")
	}
}

func TestRealWorkerClient_GetSessionAndWorker_Success(t *testing.T) {
	client, sm, registry := newTestRealWorkerClient(t)
	ctx := context.Background()

	// Register a worker
	worker := &RegisteredWorker{
		WorkerID: "test-worker",
		Status: &protov1.WorkerStatus{
			State:       protov1.WorkerStatus_STATE_IDLE,
			ActiveTasks: 0,
		},
		Capacity: &protov1.SessionCapacity{
			TotalSessions:     5,
			AvailableSessions: 5,
		},
		LastHeartbeat:     time.Now(),
		HeartbeatInterval: 30 * time.Second,
	}
	registry.RegisterWorker("test-worker", worker)

	// Create a session with worker assignment
	session := sm.CreateSession(ctx, "test-session", "user1", "workspace1")
	session.WorkerID = "test-worker"
	session.State = SessionStateReady
	_ = sm.storage.CreateSession(ctx, session)

	// Get session and worker
	ctx = context.WithValue(ctx, sessionIDKey{}, session.ID)
	retrievedSession, retrievedWorker, err := client.getSessionAndWorker(ctx)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if retrievedSession.ID != session.ID {
		t.Errorf("Expected session ID %s, got %s", session.ID, retrievedSession.ID)
	}
	if retrievedWorker.WorkerID != "test-worker" {
		t.Errorf("Expected worker ID test-worker, got %s", retrievedWorker.WorkerID)
	}
}

func TestRealWorkerClient_ExecuteTask_NoSession(t *testing.T) {
	client, _, _ := newTestRealWorkerClient(t)
	ctx := context.Background()

	// Try to execute task without session
	_, err := client.ExecuteTask(ctx, "workspace1", "echo", TaskArgs{"message": "test"})
	if err == nil {
		t.Error("Expected error when no session exists")
	}
}

func TestRealWorkerClient_ExecuteTask_FailedSession(t *testing.T) {
	client, sm, registry := newTestRealWorkerClient(t)
	ctx := context.Background()

	// Register a worker
	worker := &RegisteredWorker{
		WorkerID: "test-worker",
		Status: &protov1.WorkerStatus{
			State:       protov1.WorkerStatus_STATE_IDLE,
			ActiveTasks: 0,
		},
		Capacity: &protov1.SessionCapacity{
			TotalSessions:     5,
			AvailableSessions: 5,
		},
		LastHeartbeat:     time.Now(),
		HeartbeatInterval: 30 * time.Second,
	}
	registry.RegisterWorker("test-worker", worker)

	// Create a failed session
	session := sm.CreateSession(ctx, "test-session", "user1", "workspace1")
	session.WorkerID = "test-worker"
	session.State = SessionStateFailed
	session.StateMessage = "test failure"
	_ = sm.storage.CreateSession(ctx, session)

	// Try to execute task
	ctx = context.WithValue(ctx, sessionIDKey{}, session.ID)
	_, err := client.ExecuteTask(ctx, "workspace1", "echo", TaskArgs{"message": "test"})
	if err == nil {
		t.Error("Expected error when session is failed")
	}
}

func TestRealWorkerClient_ExecuteTask_NoTaskStream(t *testing.T) {
	client, sm, registry := newTestRealWorkerClient(t)
	ctx := context.Background()

	// Register a worker without task stream
	worker := &RegisteredWorker{
		WorkerID: "test-worker",
		Status: &protov1.WorkerStatus{
			State:       protov1.WorkerStatus_STATE_IDLE,
			ActiveTasks: 0,
		},
		Capacity: &protov1.SessionCapacity{
			TotalSessions:     5,
			AvailableSessions: 5,
		},
		LastHeartbeat:     time.Now(),
		HeartbeatInterval: 30 * time.Second,
		TaskStream:        nil, // No stream
		PendingTasks:      make(map[string]*PendingTask),
	}
	registry.RegisterWorker("test-worker", worker)

	// Create a ready session
	session := sm.CreateSession(ctx, "test-session", "user1", "workspace1")
	session.WorkerID = "test-worker"
	session.State = SessionStateReady
	_ = sm.storage.CreateSession(ctx, session)

	// Try to execute task
	ctx = context.WithValue(ctx, sessionIDKey{}, session.ID)
	_, err := client.ExecuteTask(ctx, "workspace1", "echo", TaskArgs{"message": "test"})
	if err == nil {
		t.Error("Expected error when worker has no task stream")
	}
}

func TestRealWorkerClient_ExecuteTypedTask_NoSession(t *testing.T) {
	client, _, _ := newTestRealWorkerClient(t)
	ctx := context.Background()

	request := &protov1.ToolRequest{
		Request: &protov1.ToolRequest_Echo{
			Echo: &protov1.EchoRequest{Message: "test"},
		},
	}

	// Try to execute typed task without session
	_, err := client.ExecuteTypedTask(ctx, "workspace1", request)
	if err == nil {
		t.Error("Expected error when no session exists")
	}
}

func TestRealWorkerClient_ExecuteTypedTask_FailedSession(t *testing.T) {
	client, sm, registry := newTestRealWorkerClient(t)
	ctx := context.Background()

	// Register a worker
	worker := &RegisteredWorker{
		WorkerID: "test-worker",
		Status: &protov1.WorkerStatus{
			State:       protov1.WorkerStatus_STATE_IDLE,
			ActiveTasks: 0,
		},
		Capacity: &protov1.SessionCapacity{
			TotalSessions:     5,
			AvailableSessions: 5,
		},
		LastHeartbeat:     time.Now(),
		HeartbeatInterval: 30 * time.Second,
	}
	registry.RegisterWorker("test-worker", worker)

	// Create a failed session
	session := sm.CreateSession(ctx, "test-session", "user1", "workspace1")
	session.WorkerID = "test-worker"
	session.State = SessionStateFailed
	session.StateMessage = "test failure"
	_ = sm.storage.CreateSession(ctx, session)

	request := &protov1.ToolRequest{
		Request: &protov1.ToolRequest_Echo{
			Echo: &protov1.EchoRequest{Message: "test"},
		},
	}

	// Try to execute typed task
	ctx = context.WithValue(ctx, sessionIDKey{}, session.ID)
	_, err := client.ExecuteTypedTask(ctx, "workspace1", request)
	if err == nil {
		t.Error("Expected error when session is failed")
	}
}

func TestRealWorkerClient_SyncSessionMetadata(t *testing.T) {
	client, sm, _ := newTestRealWorkerClient(t)
	ctx := context.Background()

	// Create a session
	session := sm.CreateSession(ctx, "test-session", "user1", "workspace1")

	// Sync metadata
	metadata := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	client.syncSessionMetadata(ctx, session.ID, metadata)

	// Verify metadata was synced
	retrievedMetadata, err := sm.storage.GetSessionMetadata(ctx, session.ID)
	if err != nil {
		t.Fatalf("Failed to get session metadata: %v", err)
	}

	if retrievedMetadata["key1"] != "value1" {
		t.Error("Expected key1=value1 in metadata")
	}
	if retrievedMetadata["key2"] != "value2" {
		t.Error("Expected key2=value2 in metadata")
	}
}
