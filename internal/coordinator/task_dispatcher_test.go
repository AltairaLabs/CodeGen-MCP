package coordinator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/storage"
)

// MockTaskDispatcherStorage implements storage.TaskQueueStorage for testing
type MockTaskDispatcherStorage struct {
	updateForRetryError error
	updateForRetryCalls []updateForRetryCall
}

type updateForRetryCall struct {
	taskID      string
	nextRetryAt time.Time
	errorMsg    string
}

func (m *MockTaskDispatcherStorage) UpdateTaskForRetry(ctx context.Context, taskID string, nextRetryAt time.Time, errorMsg string) error {
	m.updateForRetryCalls = append(m.updateForRetryCalls, updateForRetryCall{
		taskID:      taskID,
		nextRetryAt: nextRetryAt,
		errorMsg:    errorMsg,
	})
	return m.updateForRetryError
}

func (m *MockTaskDispatcherStorage) GetTasksReadyForRetry(ctx context.Context, limit int) ([]*storage.QueuedTask, error) {
	return nil, nil
}

func (m *MockTaskDispatcherStorage) RequeueTaskForRetry(ctx context.Context, taskID string) error {
	return nil
}

// Stub implementations for other TaskQueueStorage methods
func (m *MockTaskDispatcherStorage) Enqueue(ctx context.Context, sessionID string, task *storage.QueuedTask) error {
	return nil
}

func (m *MockTaskDispatcherStorage) Dequeue(ctx context.Context, sessionID string, limit int) ([]*storage.QueuedTask, error) {
	return nil, nil
}

func (m *MockTaskDispatcherStorage) UpdateTaskState(ctx context.Context, taskID string, state storage.TaskState) error {
	return nil
}

func (m *MockTaskDispatcherStorage) GetTask(ctx context.Context, taskID string) (*storage.QueuedTask, error) {
	return nil, nil
}

func (m *MockTaskDispatcherStorage) PurgeSessionQueue(ctx context.Context, sessionID string) (int, error) {
	return 0, nil
}

func (m *MockTaskDispatcherStorage) GetQueueStats(ctx context.Context) (*storage.QueueStats, error) {
	return nil, nil
}

func (m *MockTaskDispatcherStorage) GetQueuedTasksForSession(ctx context.Context, sessionID string) ([]*storage.QueuedTask, error) {
	return nil, nil
}

func TestNewTaskDispatcher_WithPolicy(t *testing.T) {
	stor := &MockTaskDispatcherStorage{}
	workerClient := &MockWorkerClient{}
	policy := DefaultRetryPolicy()

	dispatcher := NewTaskDispatcher(stor, workerClient, &policy)

	if dispatcher == nil {
		t.Fatal("Expected non-nil dispatcher")
	}
	if dispatcher.storage != stor {
		t.Error("Expected storage to be set")
	}
	if dispatcher.workerClient != workerClient {
		t.Error("Expected workerClient to be set")
	}
	if dispatcher.retryPolicy != &policy {
		t.Error("Expected retryPolicy to be set")
	}
}

func TestNewTaskDispatcher_NilPolicy(t *testing.T) {
	stor := &MockTaskDispatcherStorage{}
	workerClient := &MockWorkerClient{}

	dispatcher := NewTaskDispatcher(stor, workerClient, nil)

	if dispatcher == nil {
		t.Fatal("Expected non-nil dispatcher")
	}
	if dispatcher.retryPolicy == nil {
		t.Fatal("Expected retryPolicy to be set to default")
	}

	defaultPolicy := DefaultRetryPolicy()
	if dispatcher.retryPolicy.MaxRetries != defaultPolicy.MaxRetries {
		t.Errorf("Expected MaxRetries=%d, got %d", defaultPolicy.MaxRetries, dispatcher.retryPolicy.MaxRetries)
	}
	if dispatcher.retryPolicy.InitialDelay != defaultPolicy.InitialDelay {
		t.Errorf("Expected InitialDelay=%v, got %v", defaultPolicy.InitialDelay, dispatcher.retryPolicy.InitialDelay)
	}
}

func TestHandleTaskFailure_RetryableError(t *testing.T) {
	stor := &MockTaskDispatcherStorage{}
	policy := RetryPolicy{
		MaxRetries:        3,
		InitialDelay:      1 * time.Second,
		MaxDelay:          30 * time.Second,
		BackoffMultiplier: 2.0,
	}

	dispatcher := NewTaskDispatcher(stor, &MockWorkerClient{}, &policy)

	task := &storage.QueuedTask{
		ID:         "task-1",
		SessionID:  "session-1",
		RetryCount: 0,
		MaxRetries: 3,
	}

	taskErr := errors.New("connection refused")
	ctx := context.Background()

	willRetry, err := dispatcher.HandleTaskFailure(ctx, task, taskErr)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !willRetry {
		t.Error("Expected task to be scheduled for retry")
	}

	if len(stor.updateForRetryCalls) != 1 {
		t.Fatalf("Expected 1 UpdateTaskForRetry call, got %d", len(stor.updateForRetryCalls))
	}

	call := stor.updateForRetryCalls[0]
	if call.taskID != "task-1" {
		t.Errorf("Expected taskID=task-1, got %s", call.taskID)
	}
	if call.errorMsg != "Task failed: connection refused" {
		t.Errorf("Expected error message to contain task error, got: %s", call.errorMsg)
	}
}

func TestHandleTaskFailure_NonRetryableError(t *testing.T) {
	stor := &MockTaskDispatcherStorage{}
	policy := DefaultRetryPolicy()

	dispatcher := NewTaskDispatcher(stor, &MockWorkerClient{}, &policy)

	task := &storage.QueuedTask{
		ID:         "task-1",
		SessionID:  "session-1",
		RetryCount: 0,
		MaxRetries: 3,
	}

	taskErr := errors.New("validation failed: invalid input")
	ctx := context.Background()

	willRetry, err := dispatcher.HandleTaskFailure(ctx, task, taskErr)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if willRetry {
		t.Error("Expected task NOT to be scheduled for retry (non-retryable error)")
	}

	if len(stor.updateForRetryCalls) != 0 {
		t.Errorf("Expected 0 UpdateTaskForRetry calls for non-retryable error, got %d", len(stor.updateForRetryCalls))
	}
}

func TestHandleTaskFailure_MaxRetriesExceeded(t *testing.T) {
	stor := &MockTaskDispatcherStorage{}
	policy := DefaultRetryPolicy()

	dispatcher := NewTaskDispatcher(stor, &MockWorkerClient{}, &policy)

	task := &storage.QueuedTask{
		ID:         "task-1",
		SessionID:  "session-1",
		RetryCount: 3,
		MaxRetries: 3,
	}

	taskErr := errors.New("connection refused")
	ctx := context.Background()

	willRetry, err := dispatcher.HandleTaskFailure(ctx, task, taskErr)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if willRetry {
		t.Error("Expected task NOT to be scheduled for retry (max retries exceeded)")
	}

	if len(stor.updateForRetryCalls) != 0 {
		t.Errorf("Expected 0 UpdateTaskForRetry calls when max retries exceeded, got %d", len(stor.updateForRetryCalls))
	}
}

func TestHandleTaskFailure_StorageError(t *testing.T) {
	stor := &MockTaskDispatcherStorage{
		updateForRetryError: errors.New("database connection failed"),
	}
	policy := DefaultRetryPolicy()

	dispatcher := NewTaskDispatcher(stor, &MockWorkerClient{}, &policy)

	task := &storage.QueuedTask{
		ID:         "task-1",
		SessionID:  "session-1",
		RetryCount: 0,
		MaxRetries: 3,
	}

	taskErr := errors.New("connection refused")
	ctx := context.Background()

	willRetry, err := dispatcher.HandleTaskFailure(ctx, task, taskErr)

	if err == nil {
		t.Fatal("Expected error from storage, got nil")
	}
	if willRetry {
		t.Error("Expected willRetry=false when storage error occurs")
	}
	if err.Error() != "failed to update task for retry: database connection failed" {
		t.Errorf("Expected wrapped error message, got: %v", err)
	}
}

func TestHandleTaskFailure_MultipleRetries(t *testing.T) {
	stor := &MockTaskDispatcherStorage{}
	policy := RetryPolicy{
		MaxRetries:        3,
		InitialDelay:      1 * time.Second,
		MaxDelay:          30 * time.Second,
		BackoffMultiplier: 2.0,
	}

	dispatcher := NewTaskDispatcher(stor, &MockWorkerClient{}, &policy)
	ctx := context.Background()
	taskErr := errors.New("connection refused")

	tests := []struct {
		retryCount  int
		shouldRetry bool
		description string
	}{
		{0, true, "First failure - should retry"},
		{1, true, "Second failure - should retry"},
		{2, true, "Third failure - should retry"},
		{3, false, "Fourth failure - max retries reached"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			task := &storage.QueuedTask{
				ID:         "task-1",
				SessionID:  "session-1",
				RetryCount: tt.retryCount,
				MaxRetries: 3,
			}

			willRetry, err := dispatcher.HandleTaskFailure(ctx, task, taskErr)

			if err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
			if willRetry != tt.shouldRetry {
				t.Errorf("Expected willRetry=%v, got %v", tt.shouldRetry, willRetry)
			}
		})
	}
}

func TestHandleTaskFailure_DifferentErrorTypes(t *testing.T) {
	stor := &MockTaskDispatcherStorage{}
	policy := DefaultRetryPolicy()
	dispatcher := NewTaskDispatcher(stor, &MockWorkerClient{}, &policy)
	ctx := context.Background()

	task := &storage.QueuedTask{
		ID:         "task-1",
		SessionID:  "session-1",
		RetryCount: 0,
		MaxRetries: 3,
	}

	tests := []struct {
		name        string
		err         error
		shouldRetry bool
	}{
		{"Connection refused", errors.New("connection refused"), true},
		{"EOF", errors.New("EOF"), true},
		{"Timeout", errors.New("dial tcp: i/o timeout"), true},
		{"Unavailable", errors.New("service unavailable"), true},
		{"Validation", errors.New("validation failed"), false},
		{"Permission denied", errors.New("permission denied"), false},
		{"Not found", errors.New("not found"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stor.updateForRetryCalls = nil

			willRetry, err := dispatcher.HandleTaskFailure(ctx, task, tt.err)

			if err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
			if willRetry != tt.shouldRetry {
				t.Errorf("Expected willRetry=%v for %s, got %v", tt.shouldRetry, tt.name, willRetry)
			}

			if tt.shouldRetry && len(stor.updateForRetryCalls) != 1 {
				t.Errorf("Expected UpdateTaskForRetry to be called for retryable error, got %d calls", len(stor.updateForRetryCalls))
			}
			if !tt.shouldRetry && len(stor.updateForRetryCalls) != 0 {
				t.Errorf("Expected UpdateTaskForRetry NOT to be called for non-retryable error, got %d calls", len(stor.updateForRetryCalls))
			}
		})
	}
}
