package coordinator

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/cache"
	"github.com/AltairaLabs/codegen-mcp/internal/storage"
	"github.com/AltairaLabs/codegen-mcp/internal/types"
)

const testOutput = "test output"

// Helper to create a test MCPServer
func createTestMCPServer(t *testing.T) *MCPServer {
	t.Helper()

	registry := NewWorkerRegistry()
	sessionStorage := newTestSessionStorage()
	sessionManager := NewSessionManager(sessionStorage, registry)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	auditLogger := NewAuditLogger(logger)
	taskQueue := NewMockTaskQueue()

	config := Config{
		Name:    "test-server",
		Version: "1.0.0",
	}

	// Create a mock worker client
	workerClient := &MockWorkerClient{}

	server := NewMCPServer(config, sessionManager, workerClient, auditLogger, taskQueue)
	return server
}

func TestNewMCPServer(t *testing.T) {
	server := createTestMCPServer(t)

	if server == nil {
		t.Fatal("Expected non-nil MCPServer")
	}

	// Verify all components are properly initialized
	if server.server == nil {
		t.Error("Expected non-nil underlying server")
	}

	if server.sessionManager == nil {
		t.Error("Expected non-nil session manager")
	}

	if server.taskQueue == nil {
		t.Error("Expected non-nil task queue")
	}

	if server.auditLogger == nil {
		t.Error("Expected non-nil audit logger")
	}

	if server.resultStreamer == nil {
		t.Error("Expected non-nil result streamer")
	}

	if server.resultCache == nil {
		t.Error("Expected non-nil result cache")
	}

	if server.sseManager == nil {
		t.Error("Expected non-nil SSE manager")
	}

	if server.toolRegistry == nil {
		t.Error("Expected non-nil tool registry")
	}
}

// TestContextWithSessionID removed - contextWithSessionID is now in taskqueue package
// TestHasWorkersAvailable* tests removed - hasWorkersAvailable function removed as unused

func TestServer(t *testing.T) {
	server := createTestMCPServer(t)

	underlyingServer := server.Server()
	if underlyingServer == nil {
		t.Error("Expected non-nil underlying server")
	}

	if underlyingServer != server.server {
		t.Error("Expected returned server to match internal server")
	}
}

func TestTaskResultAdapterGetOutput(t *testing.T) {
	mockResult := &mockTaskResult{output: testOutput, duration: 5 * time.Second}
	adapter := &taskResultAdapter{result: mockResult}

	output := adapter.GetOutput()
	if output != testOutput {
		t.Errorf("Expected output %s, got %s", testOutput, output)
	}
}

func TestResultCacheTaskAdapterGet(t *testing.T) {
	resultCache := cache.NewResultCache(5 * time.Minute)
	defer resultCache.Close()

	adapter := &resultCacheTaskAdapter{rc: resultCache}

	// Store a result first
	ctx := context.Background()
	mockResult := &mockTaskResult{output: testOutput, duration: 2 * time.Second}
	err := resultCache.Store(ctx, "task1", mockResult)
	if err != nil {
		t.Fatalf("Failed to store result: %v", err)
	}

	// Get the result
	result, err := adapter.Get(ctx, "task1")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	output := result.GetOutput()
	if output != testOutput {
		t.Errorf("Expected output %s, got %s", testOutput, output)
	}
}

func TestResultCacheTaskAdapterGetNonExistent(t *testing.T) {
	resultCache := cache.NewResultCache(5 * time.Minute)
	defer resultCache.Close()

	adapter := &resultCacheTaskAdapter{rc: resultCache}

	ctx := context.Background()
	_, err := adapter.Get(ctx, "non-existent")
	if err == nil {
		t.Error("Expected error when getting non-existent task")
	}
}

func TestTaskQueueTaskAdapterGetTask(t *testing.T) {
	taskQueue := NewMockTaskQueue()
	adapter := &taskQueueTaskAdapter{tq: taskQueue}

	ctx := context.Background()
	task, err := adapter.GetTask(ctx, "task1")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if task == nil {
		t.Fatal("Expected non-nil task")
	}
	if task.ID != "task1" {
		t.Errorf("Expected task ID task1, got %s", task.ID)
	}
}

func TestSessionManagerAdapterBasic(t *testing.T) {
	registry := NewWorkerRegistry()
	sessionStorage := newTestSessionStorage()
	sessionManager := NewSessionManager(sessionStorage, registry)
	adapter := &sessionManagerAdapter{sm: sessionManager}

	if adapter.sm != sessionManager {
		t.Error("Expected session manager to be set correctly")
	}
}

func TestTaskQueueAdapterBasic(t *testing.T) {
	taskQueue := NewMockTaskQueue()
	adapter := &taskQueueAdapter{tq: taskQueue}

	if adapter.tq != taskQueue {
		t.Error("Expected task queue to be set correctly")
	}
}

func TestResultStreamerAdapterBasic(t *testing.T) {
	resultCache := cache.NewResultCache(5 * time.Minute)
	defer resultCache.Close()

	sseManager := NewSSESessionManager()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	resultStreamer := NewResultStreamer(sseManager, resultCache, logger)
	adapter := &resultStreamerAdapter{rs: resultStreamer}

	if adapter.rs != resultStreamer {
		t.Error("Expected result streamer to be set correctly")
	}

	// Test subscribe doesn't panic
	adapter.Subscribe("task1", "session1")
}

// Test all the adapter methods for better coverage

func TestSessionManagerAdapter(t *testing.T) {
	registry := NewWorkerRegistry()
	sessionStorage := newTestSessionStorage()
	sessionManager := NewSessionManager(sessionStorage, registry)
	adapter := &sessionManagerAdapter{sm: sessionManager}

	ctx := context.Background()

	// Test GetSession with non-existent session
	_, ok := adapter.GetSession("non-existent")
	if ok {
		t.Error("Expected false for non-existent session")
	}

	// Test CreateSession
	session := adapter.CreateSession(ctx, "test-session", "user1", "workspace1")
	if session == nil {
		t.Fatal("Expected non-nil session")
	}

	if session.ID != "test-session" {
		t.Errorf("Expected session ID test-session, got %s", session.ID)
	}

	if session.UserID != "user1" {
		t.Errorf("Expected user ID user1, got %s", session.UserID)
	}

	// Test GetSession with existing session
	retrievedSession, ok := adapter.GetSession("test-session")
	if !ok {
		t.Error("Expected true for existing session")
	}
	if retrievedSession.ID != "test-session" {
		t.Errorf("Expected session ID test-session, got %s", retrievedSession.ID)
	}
}

func TestSessionManagerAdapterGetSetSession(t *testing.T) {
	registry := NewWorkerRegistry()
	sessionStorage := newTestSessionStorage()
	sessionManager := NewSessionManager(sessionStorage, registry)
	adapter := newSessionManagerAdapter(sessionManager, registry)

	ctx := context.Background()

	// Test GetSession with non-existent session
	_, ok := adapter.GetSession("non-existent")
	if ok {
		t.Error("Expected false for non-existent session")
	}

	// Test CreateSession
	session := adapter.CreateSession(ctx, "test-session", "user1", "workspace1")
	if session == nil {
		t.Fatal("Expected non-nil session")
	}

	if session.ID != "test-session" {
		t.Errorf("Expected session ID test-session, got %s", session.ID)
	}

	// Test HasWorkersAvailable
	available := adapter.HasWorkersAvailable()
	if available {
		t.Error("Expected no workers available")
	}
}

func TestSessionManagerAdapterAvailability(t *testing.T) {
	registry := NewWorkerRegistry()
	sessionStorage := newTestSessionStorage()
	sessionManager := NewSessionManager(sessionStorage, registry)
	adapter := newSessionManagerAdapter(sessionManager, registry)

	ctx := context.Background()

	// Test GetSession with non-existent session
	_, ok := adapter.GetSession("non-existent")
	if ok {
		t.Error("Expected false for non-existent session")
	}

	// Test CreateSession
	session := adapter.CreateSession(ctx, "test-session", "user1", "workspace1")
	if session == nil {
		t.Fatal("Expected non-nil session")
	}

	if session.ID != "test-session" {
		t.Errorf("Expected session ID test-session, got %s", session.ID)
	}

	// Test HasWorkersAvailable
	available := adapter.HasWorkersAvailable()
	if available {
		t.Error("Expected no workers available")
	}
}

func TestTaskQueueAdapter(t *testing.T) {
	taskQueue := NewMockTaskQueue()
	adapter := &taskQueueAdapter{tq: taskQueue}

	ctx := context.Background()

	// Test EnqueueTypedTask
	request := &protov1.ToolRequest{
		Request: &protov1.ToolRequest_Echo{
			Echo: &protov1.EchoRequest{
				Message: "test message",
			},
		},
	}
	taskID, err := adapter.EnqueueTypedTask(ctx, "session1", request)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if taskID == "" {
		t.Error("Expected non-empty task ID")
	}

	// Test GetTask
	task, err := adapter.GetTask(ctx, "task1")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if task == nil {
		t.Fatal("Expected non-nil task")
	}
	if task.ID != "task1" {
		t.Errorf("Expected task ID task1, got %s", task.ID)
	}
}

func TestTaskQueueAdapterFS(t *testing.T) {
	taskQueue := NewMockTaskQueue()
	adapter := newTaskQueueAdapter(taskQueue)

	ctx := context.Background()

	// Test EnqueueTypedTask
	request := &protov1.ToolRequest{
		Request: &protov1.ToolRequest_FsRead{
			FsRead: &protov1.FsReadRequest{
				Path: "/test.txt",
			},
		},
	}
	taskID, err := adapter.EnqueueTypedTask(ctx, "session1", request)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if taskID == "" {
		t.Error("Expected non-empty task ID")
	}

	// Test GetTask
	task, err := adapter.GetTask(ctx, "task1")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if task == nil {
		t.Fatal("Expected non-nil task")
	}
	if task.ID != "task1" {
		t.Errorf("Expected task ID task1, got %s", task.ID)
	}
}

func TestTaskQueueAdapterPython(t *testing.T) {
	taskQueue := NewMockTaskQueue()
	adapter := newTaskQueueAdapter(taskQueue)

	ctx := context.Background()

	// Test EnqueueTypedTask
	request := &protov1.ToolRequest{
		Request: &protov1.ToolRequest_RunPython{
			RunPython: &protov1.RunPythonRequest{
				Source: &protov1.RunPythonRequest_Code{
					Code: "print('hello')",
				},
			},
		},
	}
	taskID, err := adapter.EnqueueTypedTask(ctx, "session1", request)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if taskID == "" {
		t.Error("Expected non-empty task ID")
	}

	// Test GetTask
	task, err := adapter.GetTask(ctx, "task1")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if task == nil {
		t.Fatal("Expected non-nil task")
	}
	if task.ID != "task1" {
		t.Errorf("Expected task ID task1, got %s", task.ID)
	}
}

func TestAuditLoggerAdapter(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	auditLogger := NewAuditLogger(logger)
	adapter := &auditLoggerAdapter{al: auditLogger}

	ctx := context.Background()

	// Test LogToolCall
	entry := &types.AuditEntry{
		SessionID:   "session1",
		UserID:      "user1",
		ToolName:    "echo",
		Arguments:   map[string]interface{}{"message": "test"},
		WorkspaceID: "workspace1",
	}
	// Should not panic
	adapter.LogToolCall(ctx, entry)

	// Test LogToolResult
	resultEntry := &types.AuditEntry{
		SessionID: "session1",
		ToolName:  "echo",
		ErrorMsg:  "",
	}
	// Should not panic
	adapter.LogToolResult(ctx, resultEntry)
}

func TestAuditLoggerAdapterFS(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	auditLogger := NewAuditLogger(logger)
	adapter := newAuditLoggerAdapter(auditLogger)

	ctx := context.Background()

	// Test LogToolCall
	entry := &types.AuditEntry{
		SessionID:   "session1",
		UserID:      "user1",
		ToolName:    "fs.read",
		Arguments:   map[string]interface{}{"path": "/test.txt"},
		WorkspaceID: "workspace1",
	}
	// Should not panic
	adapter.LogToolCall(ctx, entry)

	// Test LogToolResult
	resultEntry := &types.AuditEntry{
		SessionID:   "session1",
		UserID:      "user1",
		ToolName:    "fs.read",
		Arguments:   map[string]interface{}{"path": "/test.txt"},
		WorkspaceID: "workspace1",
		ErrorMsg:    "",
	}
	// Should not panic
	adapter.LogToolResult(ctx, resultEntry)
}

func TestAuditLoggerAdapterPython(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	auditLogger := NewAuditLogger(logger)
	adapter := newAuditLoggerAdapter(auditLogger)

	ctx := context.Background()

	// Test LogToolCall
	entry := &types.AuditEntry{
		SessionID:   "session1",
		UserID:      "user1",
		ToolName:    "run.python",
		Arguments:   map[string]interface{}{"code": "print('hello')"},
		WorkspaceID: "workspace1",
	}
	// Should not panic
	adapter.LogToolCall(ctx, entry)

	// Test LogToolResult
	resultEntry := &types.AuditEntry{
		SessionID:   "session1",
		UserID:      "user1",
		ToolName:    "run.python",
		Arguments:   map[string]interface{}{"code": "print('hello')"},
		WorkspaceID: "workspace1",
		ErrorMsg:    "",
	}
	// Should not panic
	adapter.LogToolResult(ctx, resultEntry)
}

func TestResultStreamerAdapter(t *testing.T) {
	resultCache := cache.NewResultCache(5 * time.Minute)
	defer resultCache.Close()

	sseManager := NewSSESessionManager()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	resultStreamer := NewResultStreamer(sseManager, resultCache, logger)
	adapter := newResultStreamerAdapter(resultStreamer)

	// Test subscribe doesn't panic
	adapter.Subscribe("task1", "session1")
}

func TestResultStreamerAdapterPython(t *testing.T) {
	resultCache := cache.NewResultCache(5 * time.Minute)
	defer resultCache.Close()

	sseManager := NewSSESessionManager()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	resultStreamer := NewResultStreamer(sseManager, resultCache, logger)
	adapter := newResultStreamerAdapter(resultStreamer)

	// Test subscribe doesn't panic
	adapter.Subscribe("task1", "session1")
}

func TestRegisterTools(t *testing.T) {
	server := createTestMCPServer(t)

	// Test that the server was created with tools registered
	// This indirectly tests registerTools since it's called in NewMCPServer
	if server.toolRegistry == nil {
		t.Fatal("Expected non-nil tool registry")
	}

	// Check that tools are registered in the registry
	expectedTools := []string{
		"echo",
		"fs.read",
		"fs.write",
		"fs.list",
		"run.python",
		"pkg.install",
		"task.get_result",
		"task.get_status",
	}

	for _, toolName := range expectedTools {
		_, err := server.toolRegistry.GetHandler(toolName)
		if err != nil {
			t.Errorf("Expected tool %s to be registered, but got error: %v", toolName, err)
		}
	}
}

// Helper types for testing

type mockTaskResult struct {
	output   string
	duration time.Duration
	success  bool
	error    string
	exitCode int
}

func (m *mockTaskResult) GetOutput() string {
	return m.output
}

func (m *mockTaskResult) GetDuration() time.Duration {
	return m.duration
}

func (m *mockTaskResult) GetSuccess() bool {
	return m.success
}

func (m *mockTaskResult) GetError() string {
	return m.error
}

func (m *mockTaskResult) GetExitCode() int {
	return m.exitCode
}

func TestSessionManagerAdapterWithNilRegistry(t *testing.T) {
	sessionStorage := newTestSessionStorage()
	sessionManager := NewSessionManager(sessionStorage, NewWorkerRegistry())
	// Create adapter with nil worker registry to test edge case
	adapter := newSessionManagerAdapter(sessionManager, nil)

	// Test HasWorkersAvailable with nil registry
	available := adapter.HasWorkersAvailable()
	if available {
		t.Error("Expected no workers available when registry is nil")
	}
}

func TestTaskQueueAdapterWithLargeSequence(t *testing.T) {
	// Create a mock task queue
	mockTQ := &mockTaskQueue{
		tasks: make(map[string]*storage.QueuedTask),
	}

	// Add a task with a very large sequence number (beyond int32 range)
	largeSeq := uint64(1<<32) + 12345
	taskID := "large-seq-task"
	mockTQ.tasks[taskID] = &storage.QueuedTask{
		ID:       taskID,
		Sequence: largeSeq,
	}

	adapter := newTaskQueueAdapter(mockTQ)
	ctx := context.Background()

	task, err := adapter.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}

	// Verify sequence was safely converted
	if task.Sequence < 0 {
		t.Error("Sequence should not be negative after conversion")
	}

	// The sequence should be modulo'd to fit in int32 range
	const maxInt32 uint64 = 1 << 31
	expectedSeq := int(largeSeq % maxInt32)
	if task.Sequence != expectedSeq {
		t.Errorf("Expected sequence %d, got %d", expectedSeq, task.Sequence)
	}
}

// Mock task queue for testing
type mockTaskQueue struct {
	tasks map[string]*storage.QueuedTask
}

func (m *mockTaskQueue) EnqueueTypedTask(ctx context.Context, sessionID string, request *protov1.ToolRequest) (string, error) {
	return "test-task-id", nil
}

func (m *mockTaskQueue) GetTask(ctx context.Context, taskID string) (*storage.QueuedTask, error) {
	if task, ok := m.tasks[taskID]; ok {
		return task, nil
	}
	return nil, fmt.Errorf("task not found")
}
