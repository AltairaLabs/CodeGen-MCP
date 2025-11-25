package taskqueue

import (
	"context"
	"fmt"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/storage"
	"google.golang.org/protobuf/proto"
)

// This file contains task dispatching infrastructure code that's difficult to unit test
// due to goroutines, channels, and complex integration requirements.

// dispatchTask executes a single task by sending it to a worker
func (tq *TaskQueue) dispatchTask(session *Session, task *storage.QueuedTask) {
	defer tq.wg.Done()

	ctx := tq.ctx
	tq.logger.InfoContext(ctx, "Dispatching task to worker",
		"task_id", task.ID,
		"session_id", session.ID,
		"tool_name", task.ToolName,
		"sequence", task.Sequence,
		"retry_count", task.RetryCount,
	)

	// Update task state to dispatched
	dispatchedAt := time.Now()
	task.DispatchedAt = &dispatchedAt
	if err := tq.storage.UpdateTaskState(ctx, task.ID, storage.TaskStateDispatched); err != nil {
		tq.logger.ErrorContext(ctx, "Failed to update task state to dispatched",
			"task_id", task.ID,
			"error", err,
		)
	}

	// Deserialize typed request from task
	if len(task.TypedRequest) == 0 {
		tq.handleTaskError(ctx, task, fmt.Errorf("task has no typed request data"))
		return
	}

	var toolRequest protov1.ToolRequest
	if err := proto.Unmarshal(task.TypedRequest, &toolRequest); err != nil {
		tq.handleTaskError(ctx, task, fmt.Errorf("failed to deserialize typed request: %w", err))
		return
	}

	// Execute task via worker client with typed request
	ctxWithSession := contextWithSessionID(ctx, session.ID)
	result, err := tq.workerClient.ExecuteTypedTask(
		ctxWithSession,
		session.WorkspaceID,
		&toolRequest,
	)

	// Handle result or error
	if err != nil {
		tq.handleTaskError(ctx, task, err)
		return
	}

	tq.handleTaskSuccess(ctx, task, result)
}

// handleTaskSuccess processes successful task completion
func (tq *TaskQueue) handleTaskSuccess(ctx context.Context, task *storage.QueuedTask, result *TaskResult) {
	tq.logger.InfoContext(ctx, "Task completed successfully",
		"task_id", task.ID,
		"session_id", task.SessionID,
		"sequence", task.Sequence,
	)

	completedAt := time.Now()
	task.CompletedAt = &completedAt

	// Publish result to subscribers via SSE and cache it FIRST
	// This must happen before updating task state to avoid race condition
	// where clients see "completed" status but result isn't cached yet
	if tq.resultStreamer != nil {
		notification := &TaskResultNotification{
			TaskID:      task.ID,
			Status:      "completed",
			Result:      result,
			CompletedAt: completedAt,
		}

		if err := tq.resultStreamer.PublishResult(ctx, task.ID, notification); err != nil {
			tq.logger.ErrorContext(ctx, "Failed to publish result",
				"task_id", task.ID,
				"error", err,
			)
		}
	}

	// Now update task state to completed (after caching result)
	if err := tq.storage.UpdateTaskState(ctx, task.ID, storage.TaskStateCompleted); err != nil {
		tq.logger.ErrorContext(ctx, "Failed to update task state to completed",
			"task_id", task.ID,
			"error", err,
		)
	}

	// Update last completed sequence
	if err := tq.sessionManager.SetLastCompletedSequence(ctx, task.SessionID, task.Sequence); err != nil {
		tq.logger.ErrorContext(ctx, "Failed to update last completed sequence",
			"task_id", task.ID,
			"session_id", task.SessionID,
			"sequence", task.Sequence,
			"error", err,
		)
	}

	// Send result to waiting client (for backward compatibility)
	tq.sendTaskResult(task.ID, result)
}

// handleTaskError processes task failure and determines retry
func (tq *TaskQueue) handleTaskError(ctx context.Context, task *storage.QueuedTask, taskErr error) {
	tq.logger.ErrorContext(ctx, "Task execution failed",
		"task_id", task.ID,
		"session_id", task.SessionID,
		"error", taskErr,
		"retry_count", task.RetryCount,
	)

	// Use dispatcher to handle retry logic
	willRetry, err := tq.dispatcher.HandleTaskFailure(ctx, task, taskErr)
	if err != nil {
		tq.logger.ErrorContext(ctx, "Failed to handle task failure",
			"task_id", task.ID,
			"error", err,
		)
		// Fall through to mark as failed
		willRetry = false
	}

	if willRetry {
		tq.logger.InfoContext(ctx, "Task scheduled for retry",
			"task_id", task.ID,
			"retry_count", task.RetryCount+1,
		)
		// Task will be retried by retry scheduler
		return
	}

	// Mark task as permanently failed
	if err := tq.storage.UpdateTaskState(ctx, task.ID, storage.TaskStateFailed); err != nil {
		tq.logger.ErrorContext(ctx, "Failed to update task state to failed",
			"task_id", task.ID,
			"error", err,
		)
	}

	// Send error result to client
	errorResult := &TaskResult{
		Success:  false,
		Output:   fmt.Sprintf("Task failed: %v", taskErr),
		ExitCode: 1,
	}

	// Publish failure notification to subscribers via SSE
	if tq.resultStreamer != nil {
		notification := &TaskResultNotification{
			TaskID:      task.ID,
			Status:      "failed",
			Error:       taskErr.Error(),
			CompletedAt: time.Now(),
		}

		if err := tq.resultStreamer.PublishResult(ctx, task.ID, notification); err != nil {
			tq.logger.ErrorContext(ctx, "Failed to publish failure notification",
				"task_id", task.ID,
				"error", err,
			)
		}
	}

	tq.sendTaskResult(task.ID, errorResult)
}

// sendTaskResult sends result to the waiting client
func (tq *TaskQueue) sendTaskResult(taskID string, result *TaskResult) {
	tq.resultMu.RLock()
	resultChan, exists := tq.resultChannels[taskID]
	tq.resultMu.RUnlock()

	if !exists {
		tq.logger.Warn("No result channel for task", "task_id", taskID)
		return
	}

	select {
	case resultChan <- result:
		// Result sent successfully
	case <-tq.ctx.Done():
		// Queue is shutting down
	default:
		tq.logger.Warn("Result channel full, dropping result", "task_id", taskID)
	}
}

// cleanupExpiredResults removes result channels for old tasks
func (tq *TaskQueue) cleanupExpiredResults() {
	// Clean up result channels that have been waiting too long
	// This prevents memory leaks from abandoned tasks
	tq.resultMu.Lock()
	defer tq.resultMu.Unlock()

	now := time.Now()
	for taskID, ch := range tq.resultChannels {
		// Check if task exists and is old
		task, err := tq.storage.GetTask(tq.ctx, taskID)
		if err != nil || task == nil {
			// Task doesn't exist, remove channel
			close(ch)
			delete(tq.resultChannels, taskID)
			continue
		}

		// Remove channels for tasks older than timeout
		if now.Sub(task.CreatedAt) > task.Timeout {
			tq.logger.Warn("Cleaning up expired result channel",
				"task_id", taskID,
				"age", now.Sub(task.CreatedAt),
			)
			close(ch)
			delete(tq.resultChannels, taskID)
		}
	}
}
