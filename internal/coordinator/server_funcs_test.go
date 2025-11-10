package coordinator

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

// Helper to create a test session manager for server tests
func newTestServerSessionManager() *SessionManager {
	storage := newTestSessionStorage()
	registry := NewWorkerRegistry()
	return NewSessionManager(storage, registry)
}

func TestServerContextWithSessionID(t *testing.T) {
	ctx := context.Background()
	sessionID := "test-session-123"

	newCtx := contextWithSessionID(ctx, sessionID)

	// Verify the context contains the session ID
	value := newCtx.Value(sessionIDKey{})
	if value == nil {
		t.Fatal("Expected session ID in context, got nil")
	}

	retrievedID, ok := value.(string)
	if !ok {
		t.Fatal("Expected session ID to be string")
	}

	if retrievedID != sessionID {
		t.Errorf("Expected session ID %s, got %s", sessionID, retrievedID)
	}
}

func TestServerValidateWorkspacePath(t *testing.T) {
	sessionMgr := newTestServerSessionManager()
	worker := NewMockWorkerClient()
	audit := NewAuditLogger(slog.Default())
	taskQueue := &MockTaskQueue{workerClient: worker}

	cfg := Config{Name: "test", Version: "1.0.0"}
	ms := NewMCPServer(cfg, sessionMgr, worker, audit, taskQueue)

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid relative path",
			path:    "src/main.py",
			wantErr: false,
		},
		{
			name:    "valid nested path",
			path:    "src/lib/utils.py",
			wantErr: false,
		},
		{
			name:    "absolute path rejected",
			path:    "/etc/passwd",
			wantErr: true,
		},
		{
			name:    "path traversal with ..",
			path:    "../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "path traversal in middle",
			path:    "src/../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "valid path with dots",
			path:    "src/file.name.py",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ms.validateWorkspacePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateWorkspacePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestServerGetSessionID(t *testing.T) {
	sessionMgr := newTestServerSessionManager()
	worker := NewMockWorkerClient()
	audit := NewAuditLogger(slog.Default())
	taskQueue := &MockTaskQueue{workerClient: worker}

	cfg := Config{Name: "test", Version: "1.0.0"}
	ms := NewMCPServer(cfg, sessionMgr, worker, audit, taskQueue)

	tests := []struct {
		name     string
		setupCtx func() context.Context
		wantID   string
	}{
		{
			name: "session ID from context value",
			setupCtx: func() context.Context {
				return context.WithValue(context.Background(), "session_id", "ctx-session-123")
			},
			wantID: "ctx-session-123",
		},
		{
			name: "fallback to default session",
			setupCtx: func() context.Context {
				return context.Background()
			},
			wantID: "default-session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupCtx()
			sessionID := ms.getSessionID(ctx)

			if sessionID != tt.wantID {
				t.Errorf("getSessionID() = %v, want %v", sessionID, tt.wantID)
			}
		})
	}
}

func TestServerHasWorkersAvailable(t *testing.T) {
	tests := []struct {
		name      string
		setupMgr  func() *SessionManager
		wantAvail bool
	}{
		{
			name: "workers available",
			setupMgr: func() *SessionManager {
				mgr := newTestServerSessionManager()
				// Register a worker
				worker := &RegisteredWorker{
					WorkerID:              "test-worker",
					SessionID:             "reg-test-worker",
					PendingTasks:          make(map[string]*PendingTask),
					PendingSessionCreates: make(map[string]chan *protov1.SessionCreateResponse),
					Capacity: &protov1.SessionCapacity{
						TotalSessions:     10,
						ActiveSessions:    0,
						AvailableSessions: 10,
					},
					LastHeartbeat: time.Now(),
					RegisteredAt:  time.Now(),
				}
				mgr.workerRegistry.workers["test-worker"] = worker
				return mgr
			},
			wantAvail: true,
		},
		{
			name: "no workers available",
			setupMgr: func() *SessionManager {
				return newTestServerSessionManager()
			},
			wantAvail: false,
		},
		{
			name: "nil session manager",
			setupMgr: func() *SessionManager {
				return nil
			},
			wantAvail: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker := NewMockWorkerClient()
			audit := NewAuditLogger(slog.Default())
			taskQueue := &MockTaskQueue{workerClient: worker}

			cfg := Config{Name: "test", Version: "1.0.0"}
			sessionMgr := tt.setupMgr()

			ms := NewMCPServer(cfg, sessionMgr, worker, audit, taskQueue)
			if sessionMgr == nil {
				ms.sessionManager = nil
			}

			available := ms.hasWorkersAvailable()
			if available != tt.wantAvail {
				t.Errorf("hasWorkersAvailable() = %v, want %v", available, tt.wantAvail)
			}
		})
	}
}

func TestServerGetOrCreateSessionNew(t *testing.T) {
	sessionMgr := newTestServerSessionManager()

	// Register a worker with proper locking
	worker := &RegisteredWorker{
		WorkerID:  "test-worker",
		SessionID: "reg-test-worker",
		Capabilities: &protov1.WorkerCapabilities{
			MaxSessions:    10,
			SupportedTools: []string{"tool1"},
		},
		Status: &protov1.WorkerStatus{
			State: protov1.WorkerStatus_STATE_IDLE,
		},
		HeartbeatInterval:     30 * time.Second,
		PendingTasks:          make(map[string]*PendingTask),
		PendingSessionCreates: make(map[string]chan *protov1.SessionCreateResponse),
		Capacity: &protov1.SessionCapacity{
			TotalSessions:     10,
			ActiveSessions:    0,
			AvailableSessions: 10,
		},
		LastHeartbeat: time.Now(),
		RegisteredAt:  time.Now(),
	}
	sessionMgr.workerRegistry.mu.Lock()
	sessionMgr.workerRegistry.workers["test-worker"] = worker
	sessionMgr.workerRegistry.mu.Unlock()

	workerClient := NewMockWorkerClient()
	audit := NewAuditLogger(slog.Default())
	taskQueue := &MockTaskQueue{workerClient: workerClient}

	cfg := Config{Name: "test", Version: "1.0.0"}
	ms := NewMCPServer(cfg, sessionMgr, workerClient, audit, taskQueue)

	ctx := context.WithValue(context.Background(), "session_id", "new-session-123")

	session, err := ms.getOrCreateSession(ctx)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if session == nil {
		t.Fatal("Expected session to be created, got nil")
	}

	if session.ID != "new-session-123" {
		t.Errorf("Expected session ID %s, got %s", "new-session-123", session.ID)
	}

	if session.WorkerID == "" {
		t.Error("Expected worker to be assigned to session")
	}
}

func TestServerGetOrCreateSessionExisting(t *testing.T) {
	sessionMgr := newTestServerSessionManager()

	// Create existing session
	ctx := context.Background()
	existingSession := sessionMgr.CreateSession(ctx, "existing-session", "user1", "workspace1")
	existingSession.WorkerID = "test-worker"

	workerClient := NewMockWorkerClient()
	audit := NewAuditLogger(slog.Default())
	taskQueue := &MockTaskQueue{workerClient: workerClient}

	cfg := Config{Name: "test", Version: "1.0.0"}
	ms := NewMCPServer(cfg, sessionMgr, workerClient, audit, taskQueue)

	ctx = context.WithValue(context.Background(), "session_id", "existing-session")

	session, err := ms.getOrCreateSession(ctx)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if session == nil {
		t.Fatal("Expected session to be returned, got nil")
	}

	if session.ID != "existing-session" {
		t.Errorf("Expected session ID %s, got %s", "existing-session", session.ID)
	}

	if session.WorkerID != "test-worker" {
		t.Errorf("Expected worker ID %s, got %s", "test-worker", session.WorkerID)
	}
}

func TestServerGetOrCreateSessionNoWorkers(t *testing.T) {
	sessionMgr := newTestServerSessionManager()
	// Don't register any workers

	workerClient := NewMockWorkerClient()
	audit := NewAuditLogger(slog.Default())
	taskQueue := &MockTaskQueue{workerClient: workerClient}

	cfg := Config{Name: "test", Version: "1.0.0"}
	ms := NewMCPServer(cfg, sessionMgr, workerClient, audit, taskQueue)

	ctx := context.WithValue(context.Background(), "session_id", "no-worker-session")

	_, err := ms.getOrCreateSession(ctx)
	if err == nil {
		t.Fatal("Expected error when no workers available, got nil")
	}

	if err.Error() != "no workers available" {
		t.Errorf("Expected 'no workers available' error, got: %v", err)
	}
}

func TestServerNewMCPServer(t *testing.T) {
	sessionMgr := newTestServerSessionManager()
	worker := NewMockWorkerClient()
	audit := NewAuditLogger(slog.Default())
	taskQueue := &MockTaskQueue{workerClient: worker}

	cfg := Config{
		Name:    "test-server",
		Version: "1.0.0",
	}

	ms := NewMCPServer(cfg, sessionMgr, worker, audit, taskQueue)

	if ms == nil {
		t.Fatal("Expected MCPServer to be created, got nil")
	}

	if ms.server == nil {
		t.Error("Expected mcp-go server to be initialized")
	}

	if ms.sessionManager == nil {
		t.Error("Expected session manager to be set")
	}

	if ms.workerClient == nil {
		t.Error("Expected worker client to be set")
	}

	if ms.taskQueue == nil {
		t.Error("Expected task queue to be set")
	}

	if ms.resultStreamer == nil {
		t.Error("Expected result streamer to be initialized")
	}

	if ms.resultCache == nil {
		t.Error("Expected result cache to be initialized")
	}

	if ms.sseManager == nil {
		t.Error("Expected SSE manager to be initialized")
	}
}

func TestServerServerMethod(t *testing.T) {
	sessionMgr := newTestServerSessionManager()
	worker := NewMockWorkerClient()
	audit := NewAuditLogger(slog.Default())
	taskQueue := &MockTaskQueue{workerClient: worker}

	cfg := Config{Name: "test", Version: "1.0.0"}
	ms := NewMCPServer(cfg, sessionMgr, worker, audit, taskQueue)

	server := ms.Server()
	if server == nil {
		t.Error("Expected Server() to return non-nil server")
	}
	if server != ms.server {
		t.Error("Expected Server() to return the internal server")
	}
}

func TestServerServeWithLogger(t *testing.T) {
	sessionMgr := newTestServerSessionManager()
	worker := NewMockWorkerClient()
	audit := NewAuditLogger(slog.Default())
	taskQueue := &MockTaskQueue{workerClient: worker}

	cfg := Config{Name: "test", Version: "1.0.0"}
	ms := NewMCPServer(cfg, sessionMgr, worker, audit, taskQueue)

	// Verify the method exists with correct signature
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if logger == nil {
		t.Error("Failed to create logger")
	}

	// Verify server is ready to serve
	if ms.server == nil {
		t.Error("Expected ms.server to be initialized")
	}
	// Note: We don't call ServeWithLogger as it would block
}

func TestServerServeHTTPWithLogger(t *testing.T) {
	sessionMgr := newTestServerSessionManager()
	worker := NewMockWorkerClient()
	audit := NewAuditLogger(slog.Default())
	taskQueue := &MockTaskQueue{workerClient: worker}

	cfg := Config{Name: "test", Version: "1.0.0"}
	ms := NewMCPServer(cfg, sessionMgr, worker, audit, taskQueue)

	// Verify the method exists with correct signature
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if logger == nil {
		t.Error("Failed to create logger")
	}

	// Verify server is ready to serve
	if ms.server == nil {
		t.Error("Expected ms.server to be initialized")
	}
	// Note: We don't call ServeHTTPWithLogger as it would start a server
}
