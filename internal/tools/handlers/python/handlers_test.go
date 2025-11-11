package python

import (
	"context"
	"fmt"
	"testing"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/config"
	"github.com/AltairaLabs/codegen-mcp/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

// Mock implementations for testing
type mockSessionManager struct {
	sessions        map[string]*types.Session
	workersAvail    bool
	createSessionFn func(ctx context.Context, sessionID, userID, workspaceID string) *types.Session
}

func (m *mockSessionManager) GetSession(sessionID string) (*types.Session, bool) {
	session, exists := m.sessions[sessionID]
	return session, exists
}

func (m *mockSessionManager) CreateSession(ctx context.Context, sessionID, userID, workspaceID string) *types.Session {
	if m.createSessionFn != nil {
		return m.createSessionFn(ctx, sessionID, userID, workspaceID)
	}
	session := &types.Session{
		ID:          sessionID,
		UserID:      userID,
		WorkspaceID: workspaceID,
		WorkerID:    "worker-1",
	}
	m.sessions[sessionID] = session
	return session
}

func (m *mockSessionManager) HasWorkersAvailable() bool {
	return m.workersAvail
}

type mockTaskQueue struct {
	tasks       map[string]*types.Task
	enqueueErr  error
	getTaskErr  error
	taskCounter int
}

func (m *mockTaskQueue) EnqueueTypedTask(ctx context.Context, sessionID string, request *protov1.ToolRequest) (string, error) {
	if m.enqueueErr != nil {
		return "", m.enqueueErr
	}
	m.taskCounter++
	taskID := fmt.Sprintf("task-%d", m.taskCounter)
	task := &types.Task{ID: taskID, Sequence: m.taskCounter}
	m.tasks[taskID] = task
	return taskID, nil
}

func (m *mockTaskQueue) GetTask(ctx context.Context, taskID string) (*types.Task, error) {
	if m.getTaskErr != nil {
		return nil, m.getTaskErr
	}
	task, exists := m.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task not found")
	}
	return task, nil
}

type mockAuditLogger struct {
	calls   []types.AuditEntry
	results []types.AuditEntry
}

func (m *mockAuditLogger) LogToolCall(ctx context.Context, entry *types.AuditEntry) {
	m.calls = append(m.calls, *entry)
}

func (m *mockAuditLogger) LogToolResult(ctx context.Context, entry *types.AuditEntry) {
	m.results = append(m.results, *entry)
}

type mockResultStreamer struct {
	subscriptions []struct{ taskID, sessionID string }
}

func (m *mockResultStreamer) Subscribe(taskID, sessionID string) {
	m.subscriptions = append(m.subscriptions, struct{ taskID, sessionID string }{taskID, sessionID})
}

// Tests for RunHandler
func TestRunHandler(t *testing.T) {
	sessionMgr := &mockSessionManager{
		sessions:     make(map[string]*types.Session),
		workersAvail: true,
	}
	taskQueue := &mockTaskQueue{
		tasks: make(map[string]*types.Task),
	}
	auditLogger := &mockAuditLogger{}
	resultStreamer := &mockResultStreamer{}

	handler := NewRunHandler(sessionMgr, taskQueue, auditLogger, resultStreamer)

	ctx := context.WithValue(context.Background(), "session_id", "test-session")
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: config.ToolRunPython,
			Arguments: map[string]interface{}{
				"code": "print('hello world')",
			},
		},
	}

	result, err := handler.Handle(ctx, request)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	// Verify audit log
	if len(auditLogger.calls) != 1 {
		t.Errorf("Expected 1 audit call, got: %d", len(auditLogger.calls))
	} else {
		call := auditLogger.calls[0]
		if call.ToolName != config.ToolRunPython {
			t.Errorf("Expected tool name '%s', got: %s", config.ToolRunPython, call.ToolName)
		}
		if call.Arguments["code"] != "print('hello world')" {
			t.Error("Expected code argument to be logged")
		}
	}

	// Verify result streamer subscription
	if len(resultStreamer.subscriptions) != 1 {
		t.Errorf("Expected 1 subscription, got: %d", len(resultStreamer.subscriptions))
	}
}

func TestInstallHandler(t *testing.T) {
	sessionMgr := &mockSessionManager{
		sessions:     make(map[string]*types.Session),
		workersAvail: true,
	}
	taskQueue := &mockTaskQueue{
		tasks: make(map[string]*types.Task),
	}
	auditLogger := &mockAuditLogger{}
	resultStreamer := &mockResultStreamer{}

	handler := NewInstallHandler(sessionMgr, taskQueue, auditLogger, resultStreamer)

	ctx := context.WithValue(context.Background(), "session_id", "test-session")
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: config.ToolPkgInstall,
			Arguments: map[string]interface{}{
				"packages": "numpy pandas matplotlib",
			},
		},
	}

	result, err := handler.Handle(ctx, request)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	// Verify audit log
	if len(auditLogger.calls) != 1 {
		t.Errorf("Expected 1 audit call, got: %d", len(auditLogger.calls))
	} else {
		call := auditLogger.calls[0]
		if call.ToolName != config.ToolPkgInstall {
			t.Errorf("Expected tool name '%s', got: %s", config.ToolPkgInstall, call.ToolName)
		}
		if call.Arguments["packages"] != "numpy pandas matplotlib" {
			t.Error("Expected packages argument to be logged")
		}
	}
}

func TestValidateWorkspacePath(t *testing.T) {
	testCases := []struct {
		name        string
		path        string
		expectError bool
	}{
		{"valid relative path", "scripts/test.py", false},
		{"valid nested path", "src/modules/helper.py", false},
		{"absolute path", "/etc/passwd", true},
		{"path traversal up", "../test.py", true},
		{"path traversal deep", "../../etc/passwd", true},
		{"path traversal with slash", "scripts/../../../etc/passwd", true},
		{"clean relative path", "./scripts/test.py", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateWorkspacePath(tc.path)
			if tc.expectError && err == nil {
				t.Errorf("Expected error for path %s", tc.path)
			}
			if !tc.expectError && err != nil {
				t.Errorf("Expected no error for path %s, got: %v", tc.path, err)
			}
		})
	}
}
