package retry

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"
)

// mockTask implements TaskInterface for testing
type mockTask struct {
	id           string
	retryCount   int
	maxRetries   int
	nextRetryAt  *time.Time
	error        string
}

func (m *mockTask) GetID() string              { return m.id }
func (m *mockTask) GetRetryCount() int         { return m.retryCount }
func (m *mockTask) GetMaxRetries() int         { return m.maxRetries }
func (m *mockTask) GetNextRetryAt() *time.Time { return m.nextRetryAt }
func (m *mockTask) GetError() string           { return m.error }

// mockStorage implements StorageInterface for testing
type mockStorage struct {
	mu                 sync.Mutex
	tasksReadyForRetry []TaskInterface
	requeuedTasks      []string
	updatedTasks       map[string]time.Time
	getTasksError      error
	requeueErrorFunc   func(taskID string) error
	updateError        error
}

func (m *mockStorage) GetTasksReadyForRetry(ctx context.Context, limit int) ([]TaskInterface, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.getTasksError != nil {
		return nil, m.getTasksError
	}
	
	if len(m.tasksReadyForRetry) <= limit {
		return m.tasksReadyForRetry, nil
	}
	return m.tasksReadyForRetry[:limit], nil
}

func (m *mockStorage) RequeueTaskForRetry(ctx context.Context, taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.requeueErrorFunc != nil {
		if err := m.requeueErrorFunc(taskID); err != nil {
			return err
		}
	}
	
	m.requeuedTasks = append(m.requeuedTasks, taskID)
	return nil
}

func (m *mockStorage) UpdateTaskForRetry(ctx context.Context, taskID string, nextRetryAt time.Time, errorMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.updateError != nil {
		return m.updateError
	}
	
	if m.updatedTasks == nil {
		m.updatedTasks = make(map[string]time.Time)
	}
	m.updatedTasks[taskID] = nextRetryAt
	return nil
}

func TestNewScheduler(t *testing.T) {
	storage := &mockStorage{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	
	scheduler := NewScheduler(storage, logger)
	
	if scheduler == nil {
		t.Fatal("Expected non-nil scheduler")
	}
	if scheduler.storage != storage {
		t.Error("Expected storage to be set")
	}
	if scheduler.logger != logger {
		t.Error("Expected logger to be set")
	}
}

func TestSchedulerProcessRetriesNoTasks(t *testing.T) {
	storage := &mockStorage{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	scheduler := NewScheduler(storage, logger)
	
	err := scheduler.processRetries(context.Background())
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestSchedulerProcessRetriesWithTasks(t *testing.T) {
	now := time.Now()
	tasks := []TaskInterface{
		&mockTask{
			id:          "task1",
			retryCount:  1,
			maxRetries:  3,
			nextRetryAt: &now,
		},
		&mockTask{
			id:          "task2",
			retryCount:  2,
			maxRetries:  3,
			nextRetryAt: &now,
		},
	}
	
	storage := &mockStorage{
		tasksReadyForRetry: tasks,
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	scheduler := NewScheduler(storage, logger)
	
	err := scheduler.processRetries(context.Background())
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	
	// Check that tasks were requeued
	storage.mu.Lock()
	defer storage.mu.Unlock()
	
	if len(storage.requeuedTasks) != 2 {
		t.Errorf("Expected 2 requeued tasks, got %d", len(storage.requeuedTasks))
	}
	
	expectedIDs := []string{"task1", "task2"}
	for i, id := range expectedIDs {
		if storage.requeuedTasks[i] != id {
			t.Errorf("Expected task %s to be requeued, got %s", id, storage.requeuedTasks[i])
		}
	}
}

func TestSchedulerProcessRetriesError(t *testing.T) {
	storage := &mockStorage{
		getTasksError: errors.New("storage error"),
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	scheduler := NewScheduler(storage, logger)
	
	err := scheduler.processRetries(context.Background())
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if err.Error() != "storage error" {
		t.Errorf("Expected 'storage error', got %v", err)
	}
}

func TestSchedulerProcessRetriesRequeueError(t *testing.T) {
	tasks := []TaskInterface{
		&mockTask{id: "task1"},
		&mockTask{id: "task2"},
	}
	
	storage := &mockStorage{
		tasksReadyForRetry: tasks,
		requeueErrorFunc: func(taskID string) error {
			if taskID == "task1" {
				return errors.New("requeue error")
			}
			return nil
		},
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	scheduler := NewScheduler(storage, logger)
	
	// This should not return an error even if individual requeues fail
	err := scheduler.processRetries(context.Background())
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	
	// Check that only the successful requeue happened
	storage.mu.Lock()
	defer storage.mu.Unlock()
	
	if len(storage.requeuedTasks) != 1 {
		t.Errorf("Expected 1 requeued task, got %d", len(storage.requeuedTasks))
	}
	if storage.requeuedTasks[0] != "task2" {
		t.Errorf("Expected task2 to be requeued, got %s", storage.requeuedTasks[0])
	}
}