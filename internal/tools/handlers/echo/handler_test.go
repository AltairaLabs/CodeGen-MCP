// Package echo provides the echo tool handler implementation
package echo

import (
	"context"
	"testing"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

// Mock implementations for testing
type mockSessionManager struct {
	sessions map[string]*types.Session
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

type mockTaskQueue struct {
	tasks map[string]*types.Task
}

func (m *mockTaskQueue) EnqueueTypedTask(ctx context.Context, sessionID string, request *protov1.ToolRequest) (string, error) {
	taskID := "test-task-123"
	if m.tasks == nil {
		m.tasks = make(map[string]*types.Task)
	}
	m.tasks[taskID] = &types.Task{ID: taskID, Sequence: 1}
	return taskID, nil
}

func (m *mockTaskQueue) GetTask(ctx context.Context, taskID string) (*types.Task, error) {
	if task, exists := m.tasks[taskID]; exists {
		return task, nil
	}
	return &types.Task{ID: taskID, Sequence: 1}, nil
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

func TestEchoHandler(t *testing.T) {
	// Setup mocks
	sessionMgr := &mockSessionManager{}
	taskQueue := &mockTaskQueue{}
	auditLogger := &mockAuditLogger{}
	resultStreamer := &mockResultStreamer{}

	handler := NewHandler(sessionMgr, taskQueue, auditLogger, resultStreamer)

	// Create a mock request
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "echo",
			Arguments: map[string]interface{}{
				"message": "Hello, World!",
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
	if call.ToolName != "echo" {
		t.Fatalf("Expected tool name 'echo', got %s", call.ToolName)
	}

	if call.Arguments["message"] != "Hello, World!" {
		t.Fatalf("Expected message 'Hello, World!', got %v", call.Arguments["message"])
	}

	// Verify session was created
	if len(sessionMgr.sessions) != 1 {
		t.Fatalf("Expected 1 session, got %d", len(sessionMgr.sessions))
	}

	// Verify task was enqueued
	if len(taskQueue.tasks) != 1 {
		t.Fatalf("Expected 1 task, got %d", len(taskQueue.tasks))
	}

	// Verify result streamer subscription
	if len(resultStreamer.subscriptions) != 1 {
		t.Fatalf("Expected 1 subscription, got %d", len(resultStreamer.subscriptions))
	}
}

func TestEchoHandlerMissingMessage(t *testing.T) {
	handler := NewHandler(&mockSessionManager{}, &mockTaskQueue{}, &mockAuditLogger{}, &mockResultStreamer{})

	// Create request without required message parameter
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "echo",
			Arguments: map[string]interface{}{},
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

	// Should return an error result due to missing message
	if result.Content == nil {
		t.Fatalf("Result should contain error content")
	}
}

func TestNewHandler(t *testing.T) {
	sessionMgr := &mockSessionManager{}
	taskQueue := &mockTaskQueue{}
	auditLogger := &mockAuditLogger{}
	resultStreamer := &mockResultStreamer{}

	handler := NewHandler(sessionMgr, taskQueue, auditLogger, resultStreamer)

	if handler == nil {
		t.Fatalf("NewHandler returned nil")
	}

	if handler.sessionManager != sessionMgr {
		t.Fatalf("Session manager not set correctly")
	}

	if handler.taskQueue != taskQueue {
		t.Fatalf("Task queue not set correctly")
	}

	if handler.auditLogger != auditLogger {
		t.Fatalf("Audit logger not set correctly")
	}

	if handler.resultStreamer != resultStreamer {
		t.Fatalf("Result streamer not set correctly")
	}
}
