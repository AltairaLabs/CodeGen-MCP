package taskqueue

import (
	"context"
	"fmt"

	"github.com/AltairaLabs/codegen-mcp/internal/storage"
)

// TaskDispatcher handles task dispatch with retry logic
// This will be used by the task queue system (Phase 5) when dispatching tasks
type TaskDispatcher struct {
	storageImpl  storage.TaskQueueStorage
	workerClient WorkerClient
	retryPolicy  *RetryPolicy
}

// NewTaskDispatcher creates a new task dispatcher with retry support
func NewTaskDispatcher(storageImpl storage.TaskQueueStorage, workerClient WorkerClient, policy *RetryPolicy) *TaskDispatcher {
	if policy == nil {
		defaultPolicy := DefaultRetryPolicy()
		policy = &defaultPolicy
	}

	return &TaskDispatcher{
		storageImpl:  storageImpl,
		workerClient: workerClient,
		retryPolicy:  policy,
	}
}

// HandleTaskFailure processes a task failure and determines if retry is needed
// Returns true if task was scheduled for retry, false if task failed permanently
func (td *TaskDispatcher) HandleTaskFailure(
	ctx context.Context,
	task *storage.QueuedTask,
	taskErr error,
) (bool, error) {
	// Check if error is retryable
	if !IsRetryableError(taskErr) {
		// Permanent error - no retry
		return false, nil
	}

	// Check if max retries exceeded
	if task.RetryCount >= task.MaxRetries {
		// Max retries exceeded - no retry
		return false, nil
	}

	// Calculate next retry time using exponential backoff
	nextRetryAt := td.retryPolicy.CalculateNextRetryTime(task.RetryCount)

	// Mark task for retry
	errorMsg := fmt.Sprintf("Task failed: %v", taskErr)
	if err := td.storageImpl.UpdateTaskForRetry(ctx, task.ID, nextRetryAt, errorMsg); err != nil {
		return false, fmt.Errorf("failed to update task for retry: %w", err)
	}

	return true, nil
}

// Example usage in task queue system (Phase 5):
//
// // After ExecuteTask fails:
// willRetry, err := dispatcher.HandleTaskFailure(ctx, queuedTask, taskErr)
// if err != nil {
//     log.Error("Failed to handle task failure", "error", err)
//     return
// }
//
// if willRetry {
//     log.Info("Task scheduled for retry",
//         "task_id", queuedTask.ID,
//         "retry_count", queuedTask.RetryCount+1,
//         "next_retry_at", time.Now().Add(policy.CalculateNextRetry(queuedTask.RetryCount)),
//     )
// } else {
//     // Mark task as permanently failed
//     storage.UpdateTaskState(ctx, queuedTask.ID, TaskStateFailed)
// }
