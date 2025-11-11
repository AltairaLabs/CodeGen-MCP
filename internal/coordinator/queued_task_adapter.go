package coordinator

import (
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/retry"
)

// QueuedTaskAdapter adapts QueuedTask to implement retry.TaskInterface
// This allows QueuedTask to work with the retry package without circular dependencies
type QueuedTaskAdapter struct {
	*QueuedTask
}

// NewQueuedTaskAdapter creates a new adapter for QueuedTask
func NewQueuedTaskAdapter(qt *QueuedTask) retry.TaskInterface {
	return &QueuedTaskAdapter{QueuedTask: qt}
}

// GetID implements retry.TaskInterface
func (qta *QueuedTaskAdapter) GetID() string {
	return qta.ID
}

// GetRetryCount implements retry.TaskInterface
func (qta *QueuedTaskAdapter) GetRetryCount() int {
	return qta.RetryCount
}

// GetMaxRetries implements retry.TaskInterface
func (qta *QueuedTaskAdapter) GetMaxRetries() int {
	return qta.MaxRetries
}

// GetNextRetryAt implements retry.TaskInterface
func (qta *QueuedTaskAdapter) GetNextRetryAt() *time.Time {
	return qta.NextRetryAt
}

// GetError implements retry.TaskInterface
func (qta *QueuedTaskAdapter) GetError() string {
	return qta.Error
}
