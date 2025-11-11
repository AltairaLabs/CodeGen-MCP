package cache

import (
	"context"
	"time"
)

// TaskResultInterface abstracts the task result data for caching
// This allows the cache package to work with any task result implementation
// without importing the coordinator types package
type TaskResultInterface interface {
	GetSuccess() bool
	GetOutput() string
	GetError() string
	GetExitCode() int
	GetDuration() time.Duration
}

// CacheInterface defines the contract for result caching
// This interface allows for different cache implementations and easier testing
type CacheInterface interface {
	Store(ctx context.Context, taskID string, result TaskResultInterface) error
	Get(ctx context.Context, taskID string) (TaskResultInterface, error)
	Delete(ctx context.Context, taskID string)
	Size() int
	Clear()
	Close()
}
