package artifact

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
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

func TestGetHandlerNoWorkers(t *testing.T) {
	sessionMgr := &mockSessionManager{
		sessions:     make(map[string]*types.Session),
		workersAvail: false,
	}
	taskQueue := &mockTaskQueue{tasks: make(map[string]*types.Task)}
	auditor := &mockAuditLogger{}
	streamer := &mockResultStreamer{}

	handler := NewGetHandler(sessionMgr, taskQueue, auditor, streamer)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "artifact.get",
			Arguments: map[string]interface{}{
				"artifact_id": "test-artifact",
			},
		},
	}

	result, err := handler.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !result.IsError {
		t.Error("Expected error result when no workers available")
	}
}

func TestGetHandlerMissingArtifactID(t *testing.T) {
	sessionMgr := &mockSessionManager{
		sessions:     make(map[string]*types.Session),
		workersAvail: true,
	}
	taskQueue := &mockTaskQueue{tasks: make(map[string]*types.Task)}
	auditor := &mockAuditLogger{}
	streamer := &mockResultStreamer{}

	handler := NewGetHandler(sessionMgr, taskQueue, auditor, streamer)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "artifact.get",
			Arguments: map[string]interface{}{},
		},
	}

	result, err := handler.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !result.IsError {
		t.Error("Expected error result when artifact_id is missing")
	}
}

func TestGetHandlerSuccess(t *testing.T) {
	sessionMgr := &mockSessionManager{
		sessions:     make(map[string]*types.Session),
		workersAvail: true,
	}
	sessionMgr.sessions["default-session"] = &types.Session{
		ID:          "default-session",
		UserID:      "default-user",
		WorkspaceID: "default-workspace",
		WorkerID:    "worker-1",
	}

	taskQueue := &mockTaskQueue{tasks: make(map[string]*types.Task)}
	auditor := &mockAuditLogger{}
	streamer := &mockResultStreamer{}

	handler := NewGetHandler(sessionMgr, taskQueue, auditor, streamer)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "artifact.get",
			Arguments: map[string]interface{}{
				"artifact_id": "test-artifact-123",
			},
		},
	}

	result, err := handler.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.IsError {
		t.Errorf("Expected success result, got error")
	}

	// Verify response structure
	if len(result.Content) == 0 {
		t.Fatal("Expected response content")
	}

	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("Expected TextContent, got %T", result.Content[0])
	}

	var response types.TaskResponse
	if err := json.Unmarshal([]byte(text.Text), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if response.TaskID != "task-1" {
		t.Errorf("Expected task ID 'task-1', got '%s'", response.TaskID)
	}

	if response.Status != "queued" {
		t.Errorf("Expected status 'queued', got '%s'", response.Status)
	}

	if response.SessionID != "default-session" {
		t.Errorf("Expected session ID 'default-session', got '%s'", response.SessionID)
	}

	// Verify audit logging
	if len(auditor.calls) != 1 {
		t.Errorf("Expected 1 audit call, got %d", len(auditor.calls))
	}

	// Verify result streamer subscription
	if len(streamer.subscriptions) != 1 {
		t.Fatalf("Expected 1 subscription, got %d", len(streamer.subscriptions))
	}
	if streamer.subscriptions[0].sessionID != "default-session" {
		t.Errorf("Expected subscription for session 'default-session', got '%s'", streamer.subscriptions[0].sessionID)
	}
}

func TestGetHandlerCreateSession(t *testing.T) {
	sessionMgr := &mockSessionManager{
		sessions:     make(map[string]*types.Session),
		workersAvail: true,
	}
	taskQueue := &mockTaskQueue{tasks: make(map[string]*types.Task)}
	auditor := &mockAuditLogger{}
	streamer := &mockResultStreamer{}

	handler := NewGetHandler(sessionMgr, taskQueue, auditor, streamer)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "artifact.get",
			Arguments: map[string]interface{}{
				"artifact_id": "test-artifact",
			},
		},
	}

	result, err := handler.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.IsError {
		t.Error("Expected success result")
	}

	// Verify session was created
	if len(sessionMgr.sessions) != 1 {
		t.Errorf("Expected 1 session to be created, got %d", len(sessionMgr.sessions))
	}
}

func TestGetHandlerSessionCreationFails(t *testing.T) {
	sessionMgr := &mockSessionManager{
		sessions:     make(map[string]*types.Session),
		workersAvail: true,
		createSessionFn: func(ctx context.Context, sessionID, userID, workspaceID string) *types.Session {
			return nil
		},
	}
	taskQueue := &mockTaskQueue{tasks: make(map[string]*types.Task)}
	auditor := &mockAuditLogger{}
	streamer := &mockResultStreamer{}

	handler := NewGetHandler(sessionMgr, taskQueue, auditor, streamer)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "artifact.get",
			Arguments: map[string]interface{}{
				"artifact_id": "test-artifact",
			},
		},
	}

	result, err := handler.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !result.IsError {
		t.Error("Expected error result when session creation fails")
	}
}

func TestGetHandlerEnqueueTaskFails(t *testing.T) {
	sessionMgr := &mockSessionManager{
		sessions:     make(map[string]*types.Session),
		workersAvail: true,
	}
	sessionMgr.sessions["default-session"] = &types.Session{
		ID:          "default-session",
		UserID:      "default-user",
		WorkspaceID: "default-workspace",
		WorkerID:    "worker-1",
	}

	taskQueue := &mockTaskQueue{
		tasks:      make(map[string]*types.Task),
		enqueueErr: fmt.Errorf("queue full"),
	}
	auditor := &mockAuditLogger{}
	streamer := &mockResultStreamer{}

	handler := NewGetHandler(sessionMgr, taskQueue, auditor, streamer)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "artifact.get",
			Arguments: map[string]interface{}{
				"artifact_id": "test-artifact",
			},
		},
	}

	result, err := handler.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !result.IsError {
		t.Error("Expected error result when task enqueue fails")
	}

	// Verify audit logging for failure
	if len(auditor.results) != 1 {
		t.Errorf("Expected 1 audit result, got %d", len(auditor.results))
	}
}

func TestGetHandlerGetTaskFails(t *testing.T) {
	sessionMgr := &mockSessionManager{
		sessions:     make(map[string]*types.Session),
		workersAvail: true,
	}
	sessionMgr.sessions["default-session"] = &types.Session{
		ID:          "default-session",
		UserID:      "default-user",
		WorkspaceID: "default-workspace",
		WorkerID:    "worker-1",
	}

	taskQueue := &mockTaskQueue{
		tasks:      make(map[string]*types.Task),
		getTaskErr: fmt.Errorf("task not found"),
	}
	auditor := &mockAuditLogger{}
	streamer := &mockResultStreamer{}

	handler := NewGetHandler(sessionMgr, taskQueue, auditor, streamer)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "artifact.get",
			Arguments: map[string]interface{}{
				"artifact_id": "test-artifact",
			},
		},
	}

	result, err := handler.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !result.IsError {
		t.Error("Expected error result when GetTask fails")
	}
}

func TestGetHandlerSessionIDFromContext(t *testing.T) {
	sessionMgr := &mockSessionManager{
		sessions:     make(map[string]*types.Session),
		workersAvail: true,
	}
	sessionMgr.sessions["custom-session"] = &types.Session{
		ID:          "custom-session",
		UserID:      "custom-user",
		WorkspaceID: "custom-workspace",
		WorkerID:    "worker-1",
	}

	taskQueue := &mockTaskQueue{tasks: make(map[string]*types.Task)}
	auditor := &mockAuditLogger{}
	streamer := &mockResultStreamer{}

	handler := NewGetHandler(sessionMgr, taskQueue, auditor, streamer)

	// Create context with custom session ID
	ctx := context.WithValue(context.Background(), "session_id", "custom-session")

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "artifact.get",
			Arguments: map[string]interface{}{
				"artifact_id": "test-artifact",
			},
		},
	}

	result, err := handler.Handle(ctx, request)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if result.IsError {
		t.Error("Expected success result")
	}

	// Verify the custom session was used
	text, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("Expected TextContent, got %T", result.Content[0])
	}

	var response types.TaskResponse
	json.Unmarshal([]byte(text.Text), &response)
	if response.SessionID != "custom-session" {
		t.Errorf("Expected session ID 'custom-session', got '%s'", response.SessionID)
	}
}
