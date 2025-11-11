package adapters

import (
	"context"
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/retry"
	"github.com/AltairaLabs/codegen-mcp/internal/storage"
)

// TaskRetryStorageAdapter adapts TaskQueueStorage to implement retry.StorageInterface
type TaskRetryStorageAdapter struct {
	storage storage.TaskQueueStorage
}

// NewTaskRetryStorageAdapter creates a new adapter for TaskQueueStorage
func NewTaskRetryStorageAdapter(storage storage.TaskQueueStorage) retry.StorageInterface {
	return &TaskRetryStorageAdapter{storage: storage}
}

// GetTasksReadyForRetry implements retry.StorageInterface
func (tsa *TaskRetryStorageAdapter) GetTasksReadyForRetry(ctx context.Context, limit int) ([]retry.TaskInterface, error) {
	queuedTasks, err := tsa.storage.GetTasksReadyForRetry(ctx, limit)
	if err != nil {
		return nil, err
	}

	// Convert []*storage.QueuedTask to []retry.TaskInterface
	tasks := make([]retry.TaskInterface, len(queuedTasks))
	for i, qt := range queuedTasks {
		tasks[i] = NewStorageQueuedTaskAdapter(qt)
	}

	return tasks, nil
}

// RequeueTaskForRetry implements retry.StorageInterface
func (tsa *TaskRetryStorageAdapter) RequeueTaskForRetry(ctx context.Context, taskID string) error {
	return tsa.storage.RequeueTaskForRetry(ctx, taskID)
}

// UpdateTaskForRetry implements retry.StorageInterface
func (tsa *TaskRetryStorageAdapter) UpdateTaskForRetry(ctx context.Context, taskID string, nextRetryAt time.Time, errorMsg string) error {
	return tsa.storage.UpdateTaskForRetry(ctx, taskID, nextRetryAt, errorMsg)
}
