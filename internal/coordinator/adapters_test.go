package coordinator

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/storage"
	"github.com/AltairaLabs/codegen-mcp/internal/types"
)

// Test sessionManagerAdapter
func TestSessionManagerAdapter_GetSession(t *testing.T) {
	storage := newTestSessionStorage()
	wr := NewWorkerRegistry()
	sm := NewSessionManager(storage, wr)
	adapter := newSessionManagerAdapter(sm, wr)

	// Test non-existent session
	_, ok := adapter.GetSession("nonexistent")
	if ok {
		t.Error("Expected session not found, but got one")
	}

	// Create a session
	ctx := context.Background()
	createdSession := sm.CreateSession(ctx, "test-session", "user-1", "workspace-1")
	if createdSession == nil {
		t.Fatal("Failed to create session")
	}

	// Test retrieving existing session
	session, ok := adapter.GetSession("test-session")
	if !ok {
		t.Fatal("Expected to find session")
	}
	if session.ID != "test-session" {
		t.Errorf("Expected session ID 'test-session', got %s", session.ID)
	}
	if session.UserID != "user-1" {
		t.Errorf("Expected user ID 'user-1', got %s", session.UserID)
	}
	if session.WorkspaceID != "workspace-1" {
		t.Errorf("Expected workspace ID 'workspace-1', got %s", session.WorkspaceID)
	}
}

func TestSessionManagerAdapter_CreateSession(t *testing.T) {
	storage := newTestSessionStorage()
	wr := NewWorkerRegistry()
	sm := NewSessionManager(storage, wr)
	adapter := newSessionManagerAdapter(sm, wr)

	ctx := context.Background()
	session := adapter.CreateSession(ctx, "new-session", "user-2", "workspace-2")

	if session == nil {
		t.Fatal("Expected session to be created")
	}
	if session.ID != "new-session" {
		t.Errorf("Expected session ID 'new-session', got %s", session.ID)
	}
	if session.UserID != "user-2" {
		t.Errorf("Expected user ID 'user-2', got %s", session.UserID)
	}
	if session.WorkspaceID != "workspace-2" {
		t.Errorf("Expected workspace ID 'workspace-2', got %s", session.WorkspaceID)
	}
}

func TestSessionManagerAdapter_HasWorkersAvailable(t *testing.T) {
	storage := newTestSessionStorage()
	wr := NewWorkerRegistry()
	sm := NewSessionManager(storage, wr)
	adapter := newSessionManagerAdapter(sm, wr)

	// Initially no workers
	if adapter.HasWorkersAvailable() {
		t.Error("Expected no workers available initially")
	}

	// Test with nil registry
	adapterNil := newSessionManagerAdapter(sm, nil)
	if adapterNil.HasWorkersAvailable() {
		t.Error("Expected no workers when registry is nil")
	}
}

// Mock task queue for testing
type mockTaskQueueForAdapterTest struct {
	tasks map[string]*storage.QueuedTask
}

func (m *mockTaskQueueForAdapterTest) EnqueueTypedTask(ctx context.Context, sessionID string, request *protov1.ToolRequest) (string, error) {
	taskID := "task-123"
	m.tasks[taskID] = &storage.QueuedTask{
		ID:        taskID,
		SessionID: sessionID,
		Sequence:  1,
	}
	return taskID, nil
}

func (m *mockTaskQueueForAdapterTest) GetTask(ctx context.Context, taskID string) (*storage.QueuedTask, error) {
	task, ok := m.tasks[taskID]
	if !ok {
		return nil, errors.New("task not found")
	}
	return task, nil
}

func TestTaskQueueAdapter_EnqueueTypedTask(t *testing.T) {
	mockTQ := &mockTaskQueueForAdapterTest{
		tasks: make(map[string]*storage.QueuedTask),
	}
	adapter := newTaskQueueAdapter(mockTQ)

	ctx := context.Background()
	request := &protov1.ToolRequest{}
	taskID, err := adapter.EnqueueTypedTask(ctx, "session-1", request)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if taskID != "task-123" {
		t.Errorf("Expected task ID 'task-123', got %s", taskID)
	}
}

func TestTaskQueueAdapter_GetTask(t *testing.T) {
	mockTQ := &mockTaskQueueForAdapterTest{
		tasks: map[string]*storage.QueuedTask{
			"task-456": {
				ID:       "task-456",
				Sequence: 42,
			},
		},
	}
	adapter := newTaskQueueAdapter(mockTQ)

	ctx := context.Background()
	task, err := adapter.GetTask(ctx, "task-456")

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if task.ID != "task-456" {
		t.Errorf("Expected task ID 'task-456', got %s", task.ID)
	}
	if task.Sequence != 42 {
		t.Errorf("Expected sequence 42, got %d", task.Sequence)
	}

	// Test non-existent task
	_, err = adapter.GetTask(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent task")
	}
}

func TestTaskQueueAdapter_GetTask_SequenceOverflow(t *testing.T) {
	const maxInt32 uint64 = 1 << 31
	mockTQ := &mockTaskQueueForAdapterTest{
		tasks: map[string]*storage.QueuedTask{
			"overflow-task": {
				ID:       "overflow-task",
				Sequence: maxInt32 + 100, // Overflow scenario
			},
		},
	}
	adapter := newTaskQueueAdapter(mockTQ)

	ctx := context.Background()
	task, err := adapter.GetTask(ctx, "overflow-task")

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	// Sequence should be wrapped
	if task.Sequence < 0 || task.Sequence > int(maxInt32) {
		t.Errorf("Expected sequence to be wrapped, got %d", task.Sequence)
	}
}

func TestAuditLoggerAdapter_LogToolCall(t *testing.T) {
	al := NewAuditLogger(slog.Default())
	adapter := newAuditLoggerAdapter(al)

	ctx := context.Background()
	entry := &types.AuditEntry{
		SessionID:   "session-1",
		UserID:      "user-1",
		ToolName:    "test-tool",
		Arguments:   map[string]interface{}{"key": "value"},
		WorkspaceID: "workspace-1",
	}

	// Just ensure no panic occurs
	adapter.LogToolCall(ctx, entry)
}

func TestAuditLoggerAdapter_LogToolResult(t *testing.T) {
	al := NewAuditLogger(slog.Default())
	adapter := newAuditLoggerAdapter(al)

	ctx := context.Background()
	entry := &types.AuditEntry{
		SessionID:   "session-2",
		UserID:      "user-2",
		ToolName:    "result-tool",
		WorkspaceID: "workspace-2",
		ErrorMsg:    "test error",
	}

	// Just ensure no panic occurs
	adapter.LogToolResult(ctx, entry)
}

func TestResultStreamerAdapter_Subscribe(t *testing.T) {
	rs := NewResultStreamer(NewSSESessionManager(), nil, slog.Default())
	adapter := newResultStreamerAdapter(rs)

	// Just ensure no panic occurs
	adapter.Subscribe("task-789", "session-3")
}
