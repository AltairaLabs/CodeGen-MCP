package adapters

import (
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/retry"
	"github.com/AltairaLabs/codegen-mcp/internal/storage"
)

// StorageQueuedTaskAdapter adapts storage.QueuedTask to implement retry.TaskInterface
type StorageQueuedTaskAdapter struct {
	*storage.QueuedTask
}

// NewStorageQueuedTaskAdapter creates a new adapter for storage.QueuedTask
func NewStorageQueuedTaskAdapter(sqt *storage.QueuedTask) retry.TaskInterface {
	return &StorageQueuedTaskAdapter{QueuedTask: sqt}
}

// GetID implements retry.TaskInterface
func (sqta *StorageQueuedTaskAdapter) GetID() string {
	return sqta.ID
}

// GetRetryCount implements retry.TaskInterface
func (sqta *StorageQueuedTaskAdapter) GetRetryCount() int {
	return sqta.RetryCount
}

// GetMaxRetries implements retry.TaskInterface
func (sqta *StorageQueuedTaskAdapter) GetMaxRetries() int {
	return sqta.MaxRetries
}

// GetNextRetryAt implements retry.TaskInterface
func (sqta *StorageQueuedTaskAdapter) GetNextRetryAt() *time.Time {
	return sqta.NextRetryAt
}

// GetError implements retry.TaskInterface
func (sqta *StorageQueuedTaskAdapter) GetError() string {
	return sqta.Error
}