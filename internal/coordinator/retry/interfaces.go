package retry

import (
	"context"
	"time"
)

// TaskInterface represents a task that can be retried
// This interface abstracts away the specific task implementation
type TaskInterface interface {
	GetID() string
	GetRetryCount() int
	GetMaxRetries() int
	GetNextRetryAt() *time.Time
	GetError() string
}

// StorageInterface defines the storage operations needed for retry scheduling
// This interface avoids import cycles by defining only the retry-specific methods
type StorageInterface interface {
	GetTasksReadyForRetry(ctx context.Context, limit int) ([]TaskInterface, error)
	RequeueTaskForRetry(ctx context.Context, taskID string) error
	UpdateTaskForRetry(ctx context.Context, taskID string, nextRetryAt time.Time, errorMsg string) error
}
