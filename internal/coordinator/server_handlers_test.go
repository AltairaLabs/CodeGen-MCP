package coordinator

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/mark3labs/mcp-go/mcp"
)

// Test handleEcho directly by calling it as a method
func TestHandleEcho(t *testing.T) {
	storage := newTestSessionStorage()
	sm := NewSessionManager(storage, nil)
	worker := NewMockWorkerClient()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := NewAuditLogger(logger)

	cfg := Config{
		Name:    "TestServer",
		Version: "1.0.0",
	}

	server := NewMCPServer(cfg, sm, worker, audit)

	// Create a session for context
	session := sm.CreateSession(context.Background(), "test-echo", "user1", "workspace1")
	ctx := context.WithValue(context.Background(), "session_id", session.ID)

	// Create a CallToolRequest with arguments
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "echo",
			Arguments: map[string]interface{}{
				"message": "Hello from test",
			},
		},
	}

	// Call the handler directly
	result, err := server.handleEcho(ctx, request)

	if err != nil {
		t.Fatalf("handleEcho returned error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Check that result contains text
	if len(result.Content) == 0 {
		t.Error("Expected result to have content")
	}
}

// Test handleEcho with missing message argument
func TestHandleEcho_MissingMessage(t *testing.T) {
	storage := newTestSessionStorage()
	sm := NewSessionManager(storage, nil)
	worker := NewMockWorkerClient()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := NewAuditLogger(logger)

	cfg := Config{
		Name:    "TestServer",
		Version: "1.0.0",
	}

	server := NewMCPServer(cfg, sm, worker, audit)
	ctx := context.Background()

	// Create request without message
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "echo",
			Arguments: map[string]interface{}{},
		},
	}

	result, err := server.handleEcho(ctx, request)

	// Should return error result, not an error
	if err != nil {
		t.Fatalf("handleEcho should not return error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if !result.IsError {
		t.Error("Expected error result for missing message")
	}
}

// Test handleFsRead directly
func TestHandleFsRead(t *testing.T) {
	storage := newTestSessionStorage()
	sm := NewSessionManager(storage, nil)
	worker := NewMockWorkerClient()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := NewAuditLogger(logger)

	cfg := Config{
		Name:    "TestServer",
		Version: "1.0.0",
	}

	server := NewMCPServer(cfg, sm, worker, audit)

	session := sm.CreateSession(context.Background(), "test-read", "user1", "workspace1")
	ctx := context.WithValue(context.Background(), "session_id", session.ID)

	// Test valid path
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "fs.read",
			Arguments: map[string]interface{}{
				"path": "test.txt",
			},
		},
	}

	result, err := server.handleFsRead(ctx, request)

	if err != nil {
		t.Fatalf("handleFsRead returned error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.IsError {
		t.Errorf("Expected success, got error: %v", result.Content)
	}
}

// Test handleFsRead with absolute path (should fail)
func TestHandleFsRead_AbsolutePath(t *testing.T) {
	storage := newTestSessionStorage()
	sm := NewSessionManager(storage, nil)
	worker := NewMockWorkerClient()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := NewAuditLogger(logger)

	cfg := Config{
		Name:    "TestServer",
		Version: "1.0.0",
	}

	server := NewMCPServer(cfg, sm, worker, audit)

	session := sm.CreateSession(context.Background(), "test-read-abs", "user1", "workspace1")
	ctx := context.WithValue(context.Background(), "session_id", session.ID)

	// Test absolute path (should be rejected)
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "fs.read",
			Arguments: map[string]interface{}{
				"path": "/etc/passwd",
			},
		},
	}

	result, err := server.handleFsRead(ctx, request)

	if err != nil {
		t.Fatalf("handleFsRead should not return error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if !result.IsError {
		t.Error("Expected error result for absolute path")
	}
}

// Test handleFsRead with path traversal (should fail)
func TestHandleFsRead_PathTraversal(t *testing.T) {
	storage := newTestSessionStorage()
	sm := NewSessionManager(storage, nil)
	worker := NewMockWorkerClient()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := NewAuditLogger(logger)

	cfg := Config{
		Name:    "TestServer",
		Version: "1.0.0",
	}

	server := NewMCPServer(cfg, sm, worker, audit)

	session := sm.CreateSession(context.Background(), "test-read-traversal", "user1", "workspace1")
	ctx := context.WithValue(context.Background(), "session_id", session.ID)

	// Test path traversal
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "fs.read",
			Arguments: map[string]interface{}{
				"path": "../etc/passwd",
			},
		},
	}

	result, err := server.handleFsRead(ctx, request)

	if err != nil {
		t.Fatalf("handleFsRead should not return error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if !result.IsError {
		t.Error("Expected error result for path traversal")
	}
}

// Test handleFsWrite directly
func TestHandleFsWrite(t *testing.T) {
	storage := newTestSessionStorage()
	sm := NewSessionManager(storage, nil)
	worker := NewMockWorkerClient()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := NewAuditLogger(logger)

	cfg := Config{
		Name:    "TestServer",
		Version: "1.0.0",
	}

	server := NewMCPServer(cfg, sm, worker, audit)

	session := sm.CreateSession(context.Background(), "test-write", "user1", "workspace1")
	ctx := context.WithValue(context.Background(), "session_id", session.ID)

	// Test valid write
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "fs.write",
			Arguments: map[string]interface{}{
				"path":     "output.txt",
				"contents": "test content",
			},
		},
	}

	result, err := server.handleFsWrite(ctx, request)

	if err != nil {
		t.Fatalf("handleFsWrite returned error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.IsError {
		t.Errorf("Expected success, got error: %v", result.Content)
	}
}

// Test handleFsWrite with path traversal (should fail)
func TestHandleFsWrite_PathTraversal(t *testing.T) {
	storage := newTestSessionStorage()
	sm := NewSessionManager(storage, nil)
	worker := NewMockWorkerClient()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := NewAuditLogger(logger)

	cfg := Config{
		Name:    "TestServer",
		Version: "1.0.0",
	}

	server := NewMCPServer(cfg, sm, worker, audit)

	session := sm.CreateSession(context.Background(), "test-write-traversal", "user1", "workspace1")
	ctx := context.WithValue(context.Background(), "session_id", session.ID)

	// Test path traversal
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "fs.write",
			Arguments: map[string]interface{}{
				"path":     "../etc/passwd",
				"contents": "bad content",
			},
		},
	}

	result, err := server.handleFsWrite(ctx, request)

	if err != nil {
		t.Fatalf("handleFsWrite should not return error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if !result.IsError {
		t.Error("Expected error result for path traversal")
	}
}

// Test handleFsWrite with missing contents (should fail)
func TestHandleFsWrite_MissingContents(t *testing.T) {
	storage := newTestSessionStorage()
	sm := NewSessionManager(storage, nil)
	worker := NewMockWorkerClient()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := NewAuditLogger(logger)

	cfg := Config{
		Name:    "TestServer",
		Version: "1.0.0",
	}

	server := NewMCPServer(cfg, sm, worker, audit)

	session := sm.CreateSession(context.Background(), "test-write-missing", "user1", "workspace1")
	ctx := context.WithValue(context.Background(), "session_id", session.ID)

	// Missing contents argument
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "fs.write",
			Arguments: map[string]interface{}{
				"path": "output.txt",
			},
		},
	}

	result, err := server.handleFsWrite(ctx, request)

	if err != nil {
		t.Fatalf("handleFsWrite should not return error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if !result.IsError {
		t.Error("Expected error result for missing contents")
	}
}

// Test validateWorkspacePath directly
func TestValidateWorkspacePath(t *testing.T) {
	storage := newTestSessionStorage()
	sm := NewSessionManager(storage, nil)
	worker := NewMockWorkerClient()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := NewAuditLogger(logger)

	cfg := Config{
		Name:    "TestServer",
		Version: "1.0.0",
	}

	server := NewMCPServer(cfg, sm, worker, audit)

	tests := []struct {
		name      string
		path      string
		expectErr bool
	}{
		{"valid relative path", "file.txt", false},
		{"valid nested path", "dir/subdir/file.txt", false},
		{"absolute path rejected", "/etc/passwd", true},
		{"parent traversal rejected", "../file.txt", true},
		{"nested traversal rejected", "dir/../../etc/passwd", true},
		{"current dir allowed", "./file.txt", false},
		{"hidden file allowed", ".gitignore", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := server.validateWorkspacePath(tt.path)

			if tt.expectErr && err == nil {
				t.Errorf("Expected error for path %s, got nil", tt.path)
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Expected no error for path %s, got %v", tt.path, err)
			}
		})
	}
}

// Test getSessionID directly
func TestGetSessionID(t *testing.T) {
	storage := newTestSessionStorage()
	sm := NewSessionManager(storage, nil)
	worker := NewMockWorkerClient()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := NewAuditLogger(logger)

	cfg := Config{
		Name:    "TestServer",
		Version: "1.0.0",
	}

	server := NewMCPServer(cfg, sm, worker, audit)

	tests := []struct {
		name       string
		ctx        context.Context
		expectedID string
	}{
		{
			"with session_id in context",
			context.WithValue(context.Background(), "session_id", "test-session-123"),
			"test-session-123",
		},
		{
			"without session_id in context",
			context.Background(),
			"default-session",
		},
		{
			"with wrong type in context",
			context.WithValue(context.Background(), "session_id", 12345),
			"default-session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessionID := server.getSessionID(tt.ctx)
			if sessionID != tt.expectedID {
				t.Errorf("Expected session ID %s, got %s", tt.expectedID, sessionID)
			}
		})
	}
}

func TestGetOrCreateSession_NewSession(t *testing.T) {
	registry := NewWorkerRegistry()
	storage := newTestSessionStorage()
	sm := NewSessionManager(storage, registry)
	worker := NewMockWorkerClient()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := NewAuditLogger(logger)

	cfg := Config{
		Name:    "TestServer",
		Version: "1.0.0",
	}

	server := NewMCPServer(cfg, sm, worker, audit)

	// Register a mock worker with capacity
	mockWorker := &RegisteredWorker{
		WorkerID: "test-worker",
		Status: &protov1.WorkerStatus{
			State:       protov1.WorkerStatus_STATE_IDLE,
			ActiveTasks: 0,
		},
		Capacity: &protov1.SessionCapacity{
			TotalSessions:     5,
			AvailableSessions: 5,
		},
		TaskStream:            nil, // No stream for this test
		PendingTasks:          make(map[string]chan *protov1.TaskStreamResponse),
		PendingSessionCreates: make(map[string]chan *protov1.SessionCreateResponse),
		LastHeartbeat:         time.Now(),
		HeartbeatInterval:     30 * time.Second,
	}
	_ = registry.RegisterWorker("test-worker", mockWorker)

	ctx := context.WithValue(context.Background(), "session_id", "new-session")

	// Should create a new session
	session, err := server.getOrCreateSession(ctx)

	if err != nil {
		t.Fatalf("getOrCreateSession returned error: %v", err)
	}

	if session == nil {
		t.Fatal("Expected non-nil session")
	}

	if session.ID != "new-session" {
		t.Errorf("Expected session ID 'new-session', got %s", session.ID)
	}

	// Verify worker was assigned
	if session.WorkerID == "" {
		t.Error("Expected worker to be assigned to session")
	}

	// Verify session was added to manager
	retrievedSession, ok := sm.GetSession("new-session")
	if !ok {
		t.Error("Session should be in session manager")
	}

	if retrievedSession.ID != session.ID {
		t.Error("Retrieved session should match created session")
	}
}

func TestGetOrCreateSession_ExistingSession(t *testing.T) {
	registry := NewWorkerRegistry()
	storage := newTestSessionStorage()
	sm := NewSessionManager(storage, registry)
	worker := NewMockWorkerClient()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := NewAuditLogger(logger)

	cfg := Config{
		Name:    "TestServer",
		Version: "1.0.0",
	}

	server := NewMCPServer(cfg, sm, worker, audit)

	// Register a mock worker with capacity
	mockWorker := &RegisteredWorker{
		WorkerID: "test-worker",
		Status: &protov1.WorkerStatus{
			State:       protov1.WorkerStatus_STATE_IDLE,
			ActiveTasks: 0,
		},
		Capacity: &protov1.SessionCapacity{
			TotalSessions:     5,
			AvailableSessions: 5,
		},
		TaskStream:            nil, // No stream for this test
		PendingTasks:          make(map[string]chan *protov1.TaskStreamResponse),
		PendingSessionCreates: make(map[string]chan *protov1.SessionCreateResponse),
		LastHeartbeat:         time.Now(),
		HeartbeatInterval:     30 * time.Second,
	}
	_ = registry.RegisterWorker("test-worker", mockWorker)

	// Pre-create a session
	ctx := context.Background()
	existingSession := sm.CreateSession(ctx, "existing-session", "user1", "workspace1")

	// Now try to get it
	ctx = context.WithValue(ctx, "session_id", "existing-session")
	session, err := server.getOrCreateSession(ctx)

	if err != nil {
		t.Fatalf("getOrCreateSession returned error: %v", err)
	}

	if session.ID != existingSession.ID {
		t.Error("Should return existing session")
	}

	// Verify session count didn't increase
	if sm.SessionCount() != 1 {
		t.Errorf("Expected 1 session, got %d", sm.SessionCount())
	}
}

func TestGetOrCreateSession_NoWorkersAvailable(t *testing.T) {
	// Create registry with NO workers
	registry := NewWorkerRegistry()
	storage := newTestSessionStorage()
	sm := NewSessionManager(storage, registry)
	worker := NewMockWorkerClient()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := NewAuditLogger(logger)

	cfg := Config{
		Name:    "TestServer",
		Version: "1.0.0",
	}

	server := NewMCPServer(cfg, sm, worker, audit)

	ctx := context.WithValue(context.Background(), "session_id", "test-session")

	// Should fail because no workers are registered
	_, err := server.getOrCreateSession(ctx)

	if err == nil {
		t.Fatal("Expected error when no workers available")
	}

	expectedErr := "no workers available"
	if err.Error() != expectedErr {
		t.Errorf("Expected error %q, got %q", expectedErr, err.Error())
	}
}
