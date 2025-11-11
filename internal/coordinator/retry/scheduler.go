package retry

import (
	"context"
	"log/slog"
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/config"
)

const (
	// CheckInterval is how often to check for retry-ready tasks
	CheckInterval = config.DefaultRetryCheckInterval
	// BatchSize is the maximum tasks to requeue per check
	BatchSize = config.DefaultRetryBatchSize
)

// Scheduler periodically checks for tasks ready to be retried
// and requeues them for execution
type Scheduler struct {
	storage StorageInterface
	logger  *slog.Logger
}

// NewScheduler creates a new retry scheduler
func NewScheduler(storage StorageInterface, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		storage: storage,
		logger:  logger,
	}
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

// processRetries checks for tasks ready to retry and requeues them
func (rs *Scheduler) processRetries(ctx context.Context) error {
	tasks, err := rs.storage.GetTasksReadyForRetry(ctx, BatchSize)
	if err != nil {
		return err
	}

	if len(tasks) == 0 {
		return nil
	}

	rs.logger.Debug("Processing retry tasks", "count", len(tasks))

	for _, task := range tasks {
		if err := rs.storage.RequeueTaskForRetry(ctx, task.GetID()); err != nil {
			rs.logger.Error("Failed to requeue task for retry",
				"task_id", task.GetID(),
				"error", err,
			)
			continue
		}

		rs.logger.Info("Task requeued for retry",
			"task_id", task.GetID(),
			"retry_count", task.GetRetryCount(),
		)
	}

	return nil
}
