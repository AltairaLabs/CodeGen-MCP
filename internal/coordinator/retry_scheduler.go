package coordinator

import (
	"context"
	"log/slog"
	"time"
)

const (
	retryCheckInterval = 1 * time.Second // How often to check for retry-ready tasks
	retryBatchSize     = 100             // Maximum tasks to requeue per check
)

// TaskRetryStorage defines the storage operations needed for retry scheduling
// This interface avoids import cycles by defining only the retry-specific methods
type TaskRetryStorage interface {
	GetTasksReadyForRetry(ctx context.Context, limit int) ([]*QueuedTask, error)
	RequeueTaskForRetry(ctx context.Context, taskID string) error
	UpdateTaskForRetry(ctx context.Context, taskID string, nextRetryAt time.Time, errorMsg string) error
}

// RetryScheduler periodically checks for tasks ready to be retried
// and requeues them for execution
type RetryScheduler struct {
	storage TaskRetryStorage
	logger  *slog.Logger
}

// NewRetryScheduler creates a new retry scheduler
func NewRetryScheduler(storage TaskRetryStorage, logger *slog.Logger) *RetryScheduler {
	return &RetryScheduler{
		storage: storage,
		logger:  logger,
	}
}

// Start begins the retry scheduler background goroutine
// It will run until ctx is canceled
func (rs *RetryScheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(retryCheckInterval)
	defer ticker.Stop()

	rs.logger.Info("Retry scheduler started",
		"check_interval", retryCheckInterval,
		"batch_size", retryBatchSize,
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

// processRetries checks for retry-ready tasks and requeues them
func (rs *RetryScheduler) processRetries(ctx context.Context) error {
	// Get tasks ready for retry
	tasks, err := rs.storage.GetTasksReadyForRetry(ctx, retryBatchSize)
	if err != nil {
		return err
	}

	if len(tasks) == 0 {
		return nil
	}

	rs.logger.Debug("Processing retry-ready tasks", "count", len(tasks))

	// Requeue each task
	requeuedCount := 0
	for _, task := range tasks {
		if err := rs.storage.RequeueTaskForRetry(ctx, task.ID); err != nil {
			rs.logger.Error("Failed to requeue task for retry",
				"task_id", task.ID,
				"session_id", task.SessionID,
				"retry_count", task.RetryCount,
				"error", err,
			)
			continue
		}

		requeuedCount++
		rs.logger.Debug("Task requeued for retry",
			"task_id", task.ID,
			"session_id", task.SessionID,
			"retry_count", task.RetryCount,
		)
	}

	if requeuedCount > 0 {
		rs.logger.Info("Requeued tasks for retry",
			"count", requeuedCount,
			"total_checked", len(tasks),
		)
	}

	return nil
}
