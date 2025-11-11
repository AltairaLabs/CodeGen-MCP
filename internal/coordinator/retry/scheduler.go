package retry

import (
	"context"
	"log/slog"
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/config"
	"github.com/AltairaLabs/codegen-mcp/internal/storage"
)

const (
	// CheckInterval is how often to check for retry-ready tasks
	CheckInterval = config.DefaultRetryCheckInterval
	// BatchSize is the maximum tasks to requeue per check
	BatchSize = config.DefaultRetryBatchSize
)

// Scheduler works directly with storage types - clean and simple
type Scheduler struct {
	storage storage.TaskQueueStorage
	logger  *slog.Logger
}

// NewScheduler creates a new retry scheduler that works directly with storage types
// No interfaces, adapters, or type conversions needed - much cleaner architecture
func NewScheduler(storage storage.TaskQueueStorage, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		storage: storage,
		logger:  logger,
	}
}

// NewDirectScheduler is an alias for NewScheduler for backward compatibility
func NewDirectScheduler(storage storage.TaskQueueStorage, logger *slog.Logger) *Scheduler {
	return NewScheduler(storage, logger)
}

// Start begins the retry scheduler background goroutine
// It will run until ctx is canceled
func (rs *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(CheckInterval)
	defer ticker.Stop()

	rs.logger.Info("Retry scheduler started",
		"check_interval", CheckInterval,
		"batch_size", BatchSize,
	)

	for {
		select {
		case <-ticker.C:
			if err := rs.processRetries(ctx); err != nil {
				rs.logger.Error("Failed to process retries", "error", err)
			}
		case <-ctx.Done():
			rs.logger.Info("Retry scheduler stopped")
			return
		}
	}
}

// processRetries checks for tasks ready to retry and requeues them - clean implementation
func (rs *Scheduler) processRetries(ctx context.Context) error {
	// Direct call to storage - no interface conversion needed
	tasks, err := rs.storage.GetTasksReadyForRetry(ctx, BatchSize)
	if err != nil {
		return err
	}

	if len(tasks) == 0 {
		return nil
	}

	rs.logger.Debug("Processing retry tasks", "count", len(tasks))

	for _, task := range tasks {
		// Direct method calls - no interface overhead
		if err := rs.storage.RequeueTaskForRetry(ctx, task.ID); err != nil {
			rs.logger.Error("Failed to requeue task for retry",
				"task_id", task.ID,
				"error", err,
			)
			continue
		}

		rs.logger.Info("Task requeued for retry",
			"task_id", task.ID,
			"retry_count", task.RetryCount,
		)
	}

	return nil
}
