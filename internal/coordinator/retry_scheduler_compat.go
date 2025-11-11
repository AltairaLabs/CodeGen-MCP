package coordinator

import (
	"log/slog"

	"github.com/AltairaLabs/codegen-mcp/internal/adapters"
	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/retry"
	"github.com/AltairaLabs/codegen-mcp/internal/storage"
)

// RetryScheduler is a compatibility type alias to retry.Scheduler
type RetryScheduler = retry.Scheduler

// NewRetryScheduler creates a new retry scheduler using the retry package
// This maintains backward compatibility with the existing coordinator API
func NewRetryScheduler(storage storage.TaskQueueStorage, logger *slog.Logger) *RetryScheduler {
	adapter := adapters.NewTaskRetryStorageAdapter(storage)
	return retry.NewScheduler(adapter, logger)
}
