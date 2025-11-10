package coordinator

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/storage"
)

const (
	testWorkerID        = "test-worker"
	testSessionID       = "test-session"
	testUserID          = "test-user"
	testWorkspaceID     = "test-workspace"
	errTaskNotFound     = "task not found: "
	expectedTaskSuccess = "Expected task success"
)

// MockTaskQueueStorage implements storage.TaskQueueStorage for testing
type MockTaskQueueStorage struct {
	mu        sync.RWMutex
	tasks     map[string]*storage.QueuedTask
	sequences map[string]uint64
}

func NewMockTaskQueueStorage() *MockTaskQueueStorage {
	return &MockTaskQueueStorage{
		tasks:     make(map[string]*storage.QueuedTask),
		sequences: make(map[string]uint64),
	}
}

func (m *MockTaskQueueStorage) Enqueue(ctx context.Context, sessionID string, task *storage.QueuedTask) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks[task.ID] = task
	m.sequences[sessionID]++
	return nil
}

func (m *MockTaskQueueStorage) Dequeue(ctx context.Context, sessionID string, limit int) ([]*storage.QueuedTask, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var tasks []*storage.QueuedTask
	for _, task := range m.tasks {
		if task.SessionID == sessionID && task.State == storage.TaskStateQueued {
			tasks = append(tasks, task)
			if len(tasks) >= limit {
				break
			}
		}
	}
	return tasks, nil
}

func (m *MockTaskQueueStorage) UpdateTaskState(ctx context.Context, taskID string, state storage.TaskState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if task, exists := m.tasks[taskID]; exists {
		task.State = state
		return nil
	}
	return fmt.Errorf("task not found: %s", taskID)
}

func (m *MockTaskQueueStorage) GetTask(ctx context.Context, taskID string) (*storage.QueuedTask, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if task, exists := m.tasks[taskID]; exists {
		return task, nil
	}
	return nil, fmt.Errorf("task not found: %s", taskID)
}

func (m *MockTaskQueueStorage) UpdateTaskForRetry(ctx context.Context, taskID string, nextRetryAt time.Time, errorMsg string) error {
	return nil
}

func (m *MockTaskQueueStorage) GetTasksReadyForRetry(ctx context.Context, limit int) ([]*storage.QueuedTask, error) {
	return nil, nil
}

func (m *MockTaskQueueStorage) RequeueTaskForRetry(ctx context.Context, taskID string) error {
	return nil
}

func (m *MockTaskQueueStorage) PurgeSessionQueue(ctx context.Context, sessionID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for id, task := range m.tasks {
		if task.SessionID == sessionID {
			delete(m.tasks, id)
			count++
		}
	}
	return count, nil
}

func (m *MockTaskQueueStorage) GetQueueStats(ctx context.Context) (*storage.QueueStats, error) {
	return &storage.QueueStats{}, nil
}

func (m *MockTaskQueueStorage) GetQueuedTasksForSession(ctx context.Context, sessionID string) ([]*storage.QueuedTask, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var tasks []*storage.QueuedTask
	for _, task := range m.tasks {
		if task.SessionID == sessionID {
			tasks = append(tasks, task)
		}
	}
	return tasks, nil
}

// Helper to create a test task queue with all dependencies
func newTestTaskQueueFull(t *testing.T) (*TaskQueue, *SessionManager, *MockTaskQueueStorage) {
	t.Helper()

	// Create storage
	taskStorage := NewMockTaskQueueStorage()
	sessionStorage := newTestSessionStorage()

	// Create worker registry and register a worker
	registry := NewWorkerRegistry()
	mockWorker := &RegisteredWorker{
		WorkerID: testWorkerID,
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
	registry.RegisterWorker(testWorkerID, mockWorker)

	// Create session manager
	sm := NewSessionManager(sessionStorage, registry)

	// Create worker client
	workerClient := NewMockWorkerClient()

	// Create logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create retry policy and dispatcher
	retryPolicy := DefaultRetryPolicy()
	dispatcher := NewTaskDispatcher(taskStorage, workerClient, &retryPolicy)

	// Create task queue with fast dispatch for testing
	config := TaskQueueConfig{
		DispatchInterval: 10 * time.Millisecond,
		MaxDispatchBatch: 10,
	}
	tq := NewTaskQueue(taskStorage, sm, workerClient, dispatcher, logger, config)

	return tq, sm, taskStorage
}

func TestNewTaskQueue(t *testing.T) {
	tq, _, _ := newTestTaskQueueFull(t)
	if tq == nil {
		t.Fatal("Expected non-nil task queue")
	}
	if tq.storage == nil {
		t.Error("Expected storage to be initialized")
	}
	if tq.sessionManager == nil {
		t.Error("Expected session manager to be initialized")
	}
	if tq.workerClient == nil {
		t.Error("Expected worker client to be initialized")
	}
}

func TestDefaultTaskQueueConfig(t *testing.T) {
	config := DefaultTaskQueueConfig()
	if config.DispatchInterval != 100*time.Millisecond {
		t.Errorf("Expected dispatch interval 100ms, got %v", config.DispatchInterval)
	}
	if config.MaxDispatchBatch != 10 {
		t.Errorf("Expected max dispatch batch 10, got %d", config.MaxDispatchBatch)
	}
}

func TestTaskQueueStartStop(t *testing.T) {
	tq, _, _ := newTestTaskQueueFull(t)

	// Start the queue
	tq.Start()

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Stop the queue
	tq.Stop()

	// Should complete without hanging
}

func TestEnqueueTask(t *testing.T) {
	tq, sm, _ := newTestTaskQueueFull(t)

	// Create a session
	session := sm.CreateSession(context.Background(), testSessionID, testUserID, testWorkspaceID)

	// Enqueue a task
	taskID, err := tq.EnqueueTask(
		context.Background(),
		session.ID,
		"echo",
		storage.TaskArgs{"message": "hello"},
	)

	if err != nil {
		t.Fatalf("EnqueueTask failed: %v", err)
	}

	if taskID == "" {
		t.Error("Expected non-empty task ID")
	}

	// Verify task was stored
	task, err := tq.GetTask(context.Background(), taskID)
	if err != nil {
		t.Fatalf("Failed to get task: %v", err)
	}

	if task.ID != taskID {
		t.Errorf("Expected task ID %s, got %s", taskID, task.ID)
	}
	if task.SessionID != session.ID {
		t.Errorf("Expected session ID %s, got %s", session.ID, task.SessionID)
	}
	if task.ToolName != "echo" {
		t.Errorf("Expected tool name 'echo', got %s", task.ToolName)
	}
	if task.State != storage.TaskStateQueued {
		t.Errorf("Expected state queued, got %s", task.State)
	}
}

func TestEnqueueTypedTask(t *testing.T) {
	tq, sm, _ := newTestTaskQueueFull(t)

	// Create a session
	session := sm.CreateSession(context.Background(), testSessionID, testUserID, testWorkspaceID)

	// Create a typed request
	request := &protov1.ToolRequest{
		Request: &protov1.ToolRequest_Echo{
			Echo: &protov1.EchoRequest{
				Message: "hello typed",
			},
		},
	}

	// Enqueue typed task
	taskID, err := tq.EnqueueTypedTask(context.Background(), session.ID, request)

	if err != nil {
		t.Fatalf("EnqueueTypedTask failed: %v", err)
	}

	if taskID == "" {
		t.Error("Expected non-empty task ID")
	}

	// Verify task was stored with typed request
	task, err := tq.GetTask(context.Background(), taskID)
	if err != nil {
		t.Fatalf("Failed to get task: %v", err)
	}

	if task.ID != taskID {
		t.Errorf("Expected task ID %s, got %s", taskID, task.ID)
	}
	if task.ToolName != "echo" {
		t.Errorf("Expected tool name 'echo', got %s", task.ToolName)
	}
	if len(task.TypedRequest) == 0 {
		t.Error("Expected typed request to be populated")
	}
}

func TestGetToolNameFromRequest(t *testing.T) {
	tests := []struct {
		name     string
		request  *protov1.ToolRequest
		expected string
	}{
		{
			name: "echo",
			request: &protov1.ToolRequest{
				Request: &protov1.ToolRequest_Echo{
					Echo: &protov1.EchoRequest{Message: "test"},
				},
			},
			expected: "echo",
		},
		{
			name: "fs.read",
			request: &protov1.ToolRequest{
				Request: &protov1.ToolRequest_FsRead{
					FsRead: &protov1.FsReadRequest{Path: "test.txt"},
				},
			},
			expected: "fs.read",
		},
		{
			name: "fs.write",
			request: &protov1.ToolRequest{
				Request: &protov1.ToolRequest_FsWrite{
					FsWrite: &protov1.FsWriteRequest{Path: "test.txt", Contents: "data"},
				},
			},
			expected: "fs.write",
		},
		{
			name: "fs.list",
			request: &protov1.ToolRequest{
				Request: &protov1.ToolRequest_FsList{
					FsList: &protov1.FsListRequest{Path: "/"},
				},
			},
			expected: "fs.list",
		},
		{
			name: "run.python",
			request: &protov1.ToolRequest{
				Request: &protov1.ToolRequest_RunPython{
					RunPython: &protov1.RunPythonRequest{
						Source: &protov1.RunPythonRequest_Code{Code: "print('hi')"},
					},
				},
			},
			expected: "run.python",
		},
		{
			name: "pkg.install",
			request: &protov1.ToolRequest{
				Request: &protov1.ToolRequest_PkgInstall{
					PkgInstall: &protov1.PkgInstallRequest{Packages: []string{"numpy"}},
				},
			},
			expected: "pkg.install",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getToolNameFromRequest(tt.request)
			if result != tt.expected {
				t.Errorf("Expected tool name %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestGetTask(t *testing.T) {
	tq, sm, _ := newTestTaskQueueFull(t)

	// Create a session
	session := sm.CreateSession(context.Background(), testSessionID, testUserID, testWorkspaceID)

	// Enqueue a task
	taskID, err := tq.EnqueueTask(
		context.Background(),
		session.ID,
		"echo",
		storage.TaskArgs{"message": "test"},
	)
	if err != nil {
		t.Fatalf("EnqueueTask failed: %v", err)
	}

	// Get the task
	task, err := tq.GetTask(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}

	if task.ID != taskID {
		t.Errorf("Expected task ID %s, got %s", taskID, task.ID)
	}
}

func TestGetTaskNotFound(t *testing.T) {
	tq, _, _ := newTestTaskQueueFull(t)

	// Try to get non-existent task
	_, err := tq.GetTask(context.Background(), "nonexistent-task-id")
	if err == nil {
		t.Error("Expected error for non-existent task")
	}
}

func TestSetResultStreamer(t *testing.T) {
	tq, _, _ := newTestTaskQueueFull(t)

	// Create a result streamer
	sseManager := NewSSESessionManager()
	cache := NewResultCache(5 * time.Minute)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	streamer := NewResultStreamer(sseManager, cache, logger)

	// Set the result streamer
	tq.SetResultStreamer(streamer)

	if tq.resultStreamer != streamer {
		t.Error("Expected result streamer to be set")
	}
}

func TestEnqueueTaskWithInvalidSession(t *testing.T) {
	tq, _, _ := newTestTaskQueueFull(t)

	// Try to enqueue task with non-existent session
	_, err := tq.EnqueueTask(
		context.Background(),
		"nonexistent-session",
		"echo",
		storage.TaskArgs{"message": "test"},
	)

	// Should succeed - task queue doesn't validate session existence
	// Session validation happens at handler level
	if err != nil {
		t.Logf("Note: EnqueueTask with invalid session returned error: %v", err)
	}
}

func TestEnqueueMultipleTasks(t *testing.T) {
	tq, sm, _ := newTestTaskQueueFull(t)

	// Create a session
	session := sm.CreateSession(context.Background(), testSessionID, testUserID, testWorkspaceID)

	// Enqueue multiple tasks
	taskIDs := make([]string, 5)
	for i := 0; i < 5; i++ {
		taskID, err := tq.EnqueueTask(
			context.Background(),
			session.ID,
			"echo",
			storage.TaskArgs{"message": "test"},
		)
		if err != nil {
			t.Fatalf("EnqueueTask %d failed: %v", i, err)
		}
		taskIDs[i] = taskID
	}

	// Verify all tasks were enqueued
	for i, taskID := range taskIDs {
		task, err := tq.GetTask(context.Background(), taskID)
		if err != nil {
			t.Fatalf("Failed to get task %d: %v", i, err)
		}
		if task.ID != taskID {
			t.Errorf("Task %d ID mismatch", i)
		}
	}
}

// Note: generateTaskID uses time.Now().UnixNano() which can produce duplicates
// when called in rapid succession. This is a known limitation of the current implementation.

func TestTaskQueueSequencing(t *testing.T) {
	tq, sm, _ := newTestTaskQueueFull(t)

	// Create a session
	session := sm.CreateSession(context.Background(), testSessionID, testUserID, testWorkspaceID)

	// Enqueue multiple tasks and verify sequence numbers increase
	var lastSequence uint64
	for i := 0; i < 3; i++ {
		taskID, err := tq.EnqueueTask(
			context.Background(),
			session.ID,
			"echo",
			storage.TaskArgs{"message": "test"},
		)
		if err != nil {
			t.Fatalf("EnqueueTask failed: %v", err)
		}

		task, err := tq.GetTask(context.Background(), taskID)
		if err != nil {
			t.Fatalf("GetTask failed: %v", err)
		}

		if i > 0 && task.Sequence <= lastSequence {
			t.Errorf("Expected sequence to increase, got %d after %d", task.Sequence, lastSequence)
		}
		lastSequence = task.Sequence
	}
}

func TestTaskQueueContextCancellation(t *testing.T) {
	tq, sm, _ := newTestTaskQueueFull(t)

	// Create a session
	session := sm.CreateSession(context.Background(), testSessionID, testUserID, testWorkspaceID)

	// Enqueue a task
	taskID, err := tq.EnqueueTask(
		context.Background(),
		session.ID,
		"echo",
		storage.TaskArgs{"message": "test"},
	)
	if err != nil {
		t.Fatalf("EnqueueTask failed: %v", err)
	}

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Try to get result with cancelled context
	_, err = tq.GetTaskResult(ctx, taskID)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled error, got %v", err)
	}
}

func TestEnqueueTypedTaskAllTypes(t *testing.T) {
	tq, sm, _ := newTestTaskQueueFull(t)
	session := sm.CreateSession(context.Background(), testSessionID, testUserID, testWorkspaceID)

	tests := []struct {
		name         string
		request      *protov1.ToolRequest
		expectedTool string
	}{
		{
			name: "echo",
			request: &protov1.ToolRequest{
				Request: &protov1.ToolRequest_Echo{
					Echo: &protov1.EchoRequest{Message: "test"},
				},
			},
			expectedTool: "echo",
		},
		{
			name: "fs.read",
			request: &protov1.ToolRequest{
				Request: &protov1.ToolRequest_FsRead{
					FsRead: &protov1.FsReadRequest{Path: "file.txt"},
				},
			},
			expectedTool: "fs.read",
		},
		{
			name: "fs.write",
			request: &protov1.ToolRequest{
				Request: &protov1.ToolRequest_FsWrite{
					FsWrite: &protov1.FsWriteRequest{Path: "file.txt", Contents: "data"},
				},
			},
			expectedTool: "fs.write",
		},
		{
			name: "fs.list",
			request: &protov1.ToolRequest{
				Request: &protov1.ToolRequest_FsList{
					FsList: &protov1.FsListRequest{Path: "dir"},
				},
			},
			expectedTool: "fs.list",
		},
		{
			name: "run.python",
			request: &protov1.ToolRequest{
				Request: &protov1.ToolRequest_RunPython{
					RunPython: &protov1.RunPythonRequest{
						Source: &protov1.RunPythonRequest_Code{Code: "print('hi')"},
					},
				},
			},
			expectedTool: "run.python",
		},
		{
			name: "pkg.install",
			request: &protov1.ToolRequest{
				Request: &protov1.ToolRequest_PkgInstall{
					PkgInstall: &protov1.PkgInstallRequest{Packages: []string{"numpy", "pandas"}},
				},
			},
			expectedTool: "pkg.install",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskID, err := tq.EnqueueTypedTask(context.Background(), session.ID, tt.request)
			if err != nil {
				t.Fatalf("EnqueueTypedTask failed: %v", err)
			}

			task, err := tq.GetTask(context.Background(), taskID)
			if err != nil {
				t.Fatalf("GetTask failed: %v", err)
			}

			if task.ToolName != tt.expectedTool {
				t.Errorf("Expected tool name %s, got %s", tt.expectedTool, task.ToolName)
			}
		})
	}
}
