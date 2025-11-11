package filesystem

import (
	"context"
	"testing"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

// Mock implementations for testing
type mockSessionManager struct {
	sessions         map[string]*types.Session
	workersAvailable bool
}

func (m *mockSessionManager) GetSession(sessionID string) (*types.Session, bool) {
	session, exists := m.sessions[sessionID]
	return session, exists
}

func (m *mockSessionManager) CreateSession(ctx context.Context, sessionID, userID, workspaceID string) *types.Session {
	session := &types.Session{
		ID:          sessionID,
		UserID:      userID,
		WorkspaceID: workspaceID,
		WorkerID:    "test-worker-1",
	}
	if m.sessions == nil {
		m.sessions = make(map[string]*types.Session)
	}
	m.sessions[sessionID] = session
	return session
}

func (m *mockSessionManager) HasWorkersAvailable() bool {
	return m.workersAvailable
}

type mockTaskQueue struct {
	tasks map[string]*types.Task
}

func (m *mockTaskQueue) EnqueueTypedTask(ctx context.Context, sessionID string, request *protov1.ToolRequest) (string, error) {
	taskID := "test-task-456"
	if m.tasks == nil {
		m.tasks = make(map[string]*types.Task)
	}
	m.tasks[taskID] = &types.Task{ID: taskID, Sequence: 2}
	return taskID, nil
}

func (m *mockTaskQueue) GetTask(ctx context.Context, taskID string) (*types.Task, error) {
	if task, exists := m.tasks[taskID]; exists {
		return task, nil
	}
	return &types.Task{ID: taskID, Sequence: 2}, nil
}

type mockAuditLogger struct {
	calls   []*types.AuditEntry
	results []*types.AuditEntry
}

func (m *mockAuditLogger) LogToolCall(ctx context.Context, entry *types.AuditEntry) {
	if m.calls == nil {
		m.calls = make([]*types.AuditEntry, 0)
	}
	m.calls = append(m.calls, entry)
}

func (m *mockAuditLogger) LogToolResult(ctx context.Context, entry *types.AuditEntry) {
	if m.results == nil {
		m.results = make([]*types.AuditEntry, 0)
	}
	m.results = append(m.results, entry)
}

type mockResultStreamer struct {
	subscriptions map[string]string
}

func (m *mockResultStreamer) Subscribe(taskID, sessionID string) {
	if m.subscriptions == nil {
		m.subscriptions = make(map[string]string)
	}
	m.subscriptions[taskID] = sessionID
}

func TestReadHandler(t *testing.T) {
	// Setup mocks
	sessionMgr := &mockSessionManager{workersAvailable: true}
	taskQueue := &mockTaskQueue{}
	auditLogger := &mockAuditLogger{}
	resultStreamer := &mockResultStreamer{}

	handler := NewReadHandler(sessionMgr, taskQueue, auditLogger, resultStreamer)

	// Create a mock request
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "fs.read",
			Arguments: map[string]interface{}{
				"path": "test/file.txt",
			},
		},
	}

	// Test the handler
	ctx := context.WithValue(context.Background(), "session_id", "test-session")
	result, err := handler.Handle(ctx, request)

	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	if result == nil {
		t.Fatalf("Handler returned nil result")
	}

	// Verify audit logging occurred
	if len(auditLogger.calls) != 1 {
		t.Fatalf("Expected 1 audit call, got %d", len(auditLogger.calls))
	}

	call := auditLogger.calls[0]
	if call.ToolName != "fs.read" {
		t.Fatalf("Expected tool name 'fs.read', got %s", call.ToolName)
	}

	if call.Arguments["path"] != "test/file.txt" {
		t.Fatalf("Expected path 'test/file.txt', got %v", call.Arguments["path"])
	}
}

func TestReadHandlerNoWorkers(t *testing.T) {
	sessionMgr := &mockSessionManager{workersAvailable: false}
	handler := NewReadHandler(sessionMgr, &mockTaskQueue{}, &mockAuditLogger{}, &mockResultStreamer{})

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "fs.read",
			Arguments: map[string]interface{}{
				"path": "test/file.txt",
			},
		},
	}

	ctx := context.Background()
	result, err := handler.Handle(ctx, request)

	if err != nil {
		t.Fatalf("Handler returned error: %v", err)
	}

	if result == nil {
		t.Fatalf("Handler returned nil result")
	}

	// Should return an error result due to no workers
	if result.Content == nil {
		t.Fatalf("Result should contain error content")
	}
}

func TestValidateWorkspacePath(t *testing.T) {
	tests := []struct {
		path      string
		shouldErr bool
	}{
		{"test/file.txt", false},
		{"file.txt", false},
		{"/absolute/path", true},
		{"../outside", true},
		{"test/../../outside", true}, // This will be normalized to "../outside"
		{"test/./file.txt", false},
	}

	for _, test := range tests {
		err := validateWorkspacePath(test.path)
		if test.shouldErr && err == nil {
			t.Fatalf("Expected error for path %s, got nil", test.path)
		}
		if !test.shouldErr && err != nil {
			t.Fatalf("Expected no error for path %s, got %v", test.path, err)
		}
	}
}
