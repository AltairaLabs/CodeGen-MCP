package retry

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/storage"
)

// Simple mock that implements the minimum interface needed
type simpleMockStorage struct{}

func (s *simpleMockStorage) GetTasksReadyForRetry(ctx context.Context, limit int) ([]*storage.QueuedTask, error) {
	return []*storage.QueuedTask{}, nil
}

func (s *simpleMockStorage) RequeueTaskForRetry(ctx context.Context, taskID string) error {
	return nil
}

func (s *simpleMockStorage) UpdateTaskForRetry(ctx context.Context, taskID string, nextRetryAt time.Time, errorMsg string) error {
	return nil
}

// Implement other required methods as no-ops for testing
func (s *simpleMockStorage) Enqueue(ctx context.Context, sessionID string, task *storage.QueuedTask) error {
	return nil
}
func (s *simpleMockStorage) Dequeue(ctx context.Context, sessionID string, limit int) ([]*storage.QueuedTask, error) {
	return nil, nil
}
func (s *simpleMockStorage) UpdateTaskState(ctx context.Context, taskID string, state storage.TaskState) error {
	return nil
}
func (s *simpleMockStorage) GetTask(ctx context.Context, taskID string) (*storage.QueuedTask, error) {
	return nil, nil
}
func (s *simpleMockStorage) PurgeSessionQueue(ctx context.Context, sessionID string) (int, error) {
	return 0, nil
}
func (s *simpleMockStorage) GetQueueStats(ctx context.Context) (*storage.QueueStats, error) {
	return nil, nil
}
func (s *simpleMockStorage) GetQueuedTasksForSession(ctx context.Context, sessionID string) ([]*storage.QueuedTask, error) {
	return nil, nil
}

func TestNewSchedulerClean(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	storage := &simpleMockStorage{}

	scheduler := NewScheduler(storage, logger)
	if scheduler == nil {
		t.Fatal("Expected non-nil scheduler")
	}
}

func TestSchedulerProcessRetriesNoTasksClean(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	storage := &simpleMockStorage{}

	scheduler := NewScheduler(storage, logger)

	ctx := context.Background()
	err := scheduler.processRetries(ctx)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}
