package taskqueue

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/storage"
)

// Mock implementations for testing
type mockWorkerClient struct{}

func (m *mockWorkerClient) ExecuteTask(ctx context.Context, workspaceID, toolName string, args TaskArgs) (*TaskResult, error) {
	return &TaskResult{
		Success:  true,
		Output:   "test output",
		ExitCode: 0,
		Duration: 10 * time.Millisecond,
	}, nil
}

func (m *mockWorkerClient) ExecuteTypedTask(ctx context.Context, workspaceID string, request *protov1.ToolRequest) (*TaskResult, error) {
	return &TaskResult{
		Success:  true,
		Output:   "typed test output",
		ExitCode: 0,
		Duration: 10 * time.Millisecond,
	}, nil
}

const testSessionID = "test-session"

type mockSessionManager struct{}

func (m *mockSessionManager) GetSession(sessionID string) (*Session, bool) {
	if sessionID == testSessionID {
		return &Session{
			ID:          sessionID,
			WorkspaceID: "test-workspace",
			UserID:      "test-user",
			WorkerID:    "test-worker",
			State:       SessionStateReady,
		}, true
	}
	return nil, false
}

func (m *mockSessionManager) GetNextSequence(ctx context.Context, sessionID string) uint64 {
	return 1
}

func (m *mockSessionManager) GetAllSessions() map[string]*Session {
	return map[string]*Session{
		testSessionID: {
			ID:          testSessionID,
			WorkspaceID: "test-workspace",
			UserID:      "test-user",
			WorkerID:    "test-worker",
			State:       SessionStateReady,
		},
	}
}

func (m *mockSessionManager) SetLastCompletedSequence(ctx context.Context, sessionID string, sequence uint64) error {
	return nil
}

type mockStorage struct {
	tasks map[string]*storage.QueuedTask
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		tasks: make(map[string]*storage.QueuedTask),
	}
}

func (m *mockStorage) Enqueue(ctx context.Context, sessionID string, task *storage.QueuedTask) error {
	m.tasks[task.ID] = task
	return nil
}

func (m *mockStorage) Dequeue(ctx context.Context, sessionID string, limit int) ([]*storage.QueuedTask, error) {
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

func (m *mockStorage) UpdateTaskState(ctx context.Context, taskID string, state storage.TaskState) error {
	if task, exists := m.tasks[taskID]; exists {
		task.State = state
	}
	return nil
}

func (m *mockStorage) GetTask(ctx context.Context, taskID string) (*storage.QueuedTask, error) {
	if task, exists := m.tasks[taskID]; exists {
		return task, nil
	}
	return nil, nil
}

func (m *mockStorage) PurgeSessionQueue(ctx context.Context, sessionID string) (int, error) {
	count := 0
	for id, task := range m.tasks {
		if task.SessionID == sessionID {
			delete(m.tasks, id)
			count++
		}
	}
	return count, nil
}

func (m *mockStorage) GetQueueStats(ctx context.Context) (*storage.QueueStats, error) {
	return &storage.QueueStats{
		TotalQueued:     len(m.tasks),
		TotalDispatched: 0,
		QueuedBySession: make(map[string]int),
		OldestTaskAge:   0,
	}, nil
}

func (m *mockStorage) GetQueuedTasksForSession(ctx context.Context, sessionID string) ([]*storage.QueuedTask, error) {
	var tasks []*storage.QueuedTask
	for _, task := range m.tasks {
		if task.SessionID == sessionID {
			tasks = append(tasks, task)
		}
	}
	return tasks, nil
}

func (m *mockStorage) GetTasksReadyForRetry(ctx context.Context, limit int) ([]*storage.QueuedTask, error) {
	return []*storage.QueuedTask{}, nil
}

func (m *mockStorage) UpdateTaskForRetry(ctx context.Context, taskID string, nextRetryAt time.Time, errorMsg string) error {
	return nil
}

func (m *mockStorage) RequeueTaskForRetry(ctx context.Context, taskID string) error {
	return nil
}

func TestNewTaskQueue(t *testing.T) {
	storage := newMockStorage()
	sessionManager := &mockSessionManager{}
	workerClient := &mockWorkerClient{}
	dispatcher := NewTaskDispatcher(storage, workerClient, &RetryPolicy{})
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	config := DefaultTaskQueueConfig()

	tq := NewTaskQueue(storage, sessionManager, workerClient, dispatcher, logger, config)

	if tq == nil {
		t.Fatal("Expected non-nil TaskQueue")
	}
}

func TestTaskQueueEnqueue(t *testing.T) {
	storage := newMockStorage()
	sessionManager := &mockSessionManager{}
	workerClient := &mockWorkerClient{}
	dispatcher := NewTaskDispatcher(storage, workerClient, &RetryPolicy{})
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	config := DefaultTaskQueueConfig()

	tq := NewTaskQueue(storage, sessionManager, workerClient, dispatcher, logger, config)

	ctx := context.Background()
	taskID, err := tq.EnqueueTask(ctx, testSessionID, "echo", TaskArgs{"message": "hello"})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if taskID == "" {
		t.Fatal("Expected non-empty task ID")
	}
}

func TestDefaultTaskQueueConfig(t *testing.T) {
	config := DefaultTaskQueueConfig()

	if config.DispatchInterval <= 0 {
		t.Error("Expected positive DispatchInterval")
	}

	if config.MaxDispatchBatch <= 0 {
		t.Error("Expected positive MaxDispatchBatch")
	}
}
