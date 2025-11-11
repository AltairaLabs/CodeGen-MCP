package coordinator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/cache"
	"github.com/AltairaLabs/codegen-mcp/internal/taskqueue"
)

// ResultStreamer manages streaming task results to SSE clients
type ResultStreamer struct {
	sseManager  *SSESessionManager
	resultCache cache.CacheInterface
	logger      *slog.Logger

	// Subscriptions: taskID -> list of session IDs waiting for result
	subscriptions map[string][]string
	subMu         sync.RWMutex
}

// NewResultStreamer creates a new result streamer
func NewResultStreamer(
	sseMgr *SSESessionManager,
	resultCache cache.CacheInterface,
	logger *slog.Logger,
) *ResultStreamer {
	return &ResultStreamer{
		sseManager:    sseMgr,
		resultCache:   resultCache,
		logger:        logger,
		subscriptions: make(map[string][]string),
	}
}

// Subscribe registers a session to receive notifications for a task
func (rs *ResultStreamer) Subscribe(taskID, sessionID string) {
	rs.subMu.Lock()
	defer rs.subMu.Unlock()

	rs.subscriptions[taskID] = append(rs.subscriptions[taskID], sessionID)
	rs.logger.Debug("Session subscribed to task",
		"task_id", taskID,
		"session_id", sessionID,
	)
}

// Unsubscribe removes a session from receiving notifications for a task
func (rs *ResultStreamer) Unsubscribe(taskID, sessionID string) {
	rs.subMu.Lock()
	defer rs.subMu.Unlock()

	sessions := rs.subscriptions[taskID]
	for i, sid := range sessions {
		if sid == sessionID {
			// Remove this session from the list
			rs.subscriptions[taskID] = append(sessions[:i], sessions[i+1:]...)
			break
		}
	}

	// Clean up if no more subscribers
	if len(rs.subscriptions[taskID]) == 0 {
		delete(rs.subscriptions, taskID)
	}
}

// PublishResult publishes a task result to all subscribed sessions
func (rs *ResultStreamer) PublishResult(
	ctx context.Context,
	taskID string,
	result *TaskResultNotification,
) error {
	// Cache the result first
	if result.Result != nil {
		// Convert coordinator TaskResult to taskqueue TaskResult
		taskQueueResult := &taskqueue.TaskResult{
			Success:  result.Result.Success,
			Output:   result.Result.Output,
			Error:    result.Result.Error,
			ExitCode: result.Result.ExitCode,
			Duration: result.Result.Duration,
		}
		if err := rs.resultCache.Store(ctx, taskID, taskQueueResult); err != nil {
			rs.logger.Error("Failed to cache result",
				"task_id", taskID,
				"error", err,
			)
		}
	}

	// Get subscribers
	rs.subMu.RLock()
	sessionIDs := rs.subscriptions[taskID]
	rs.subMu.RUnlock()

	if len(sessionIDs) == 0 {
		rs.logger.Debug("No subscribers for task", "task_id", taskID)
		return nil
	}

	rs.logger.Info("Publishing result to subscribers",
		"task_id", taskID,
		"subscribers", len(sessionIDs),
		"status", result.Status,
	)

	// Send notification to each subscribed session
	var errors []error
	for _, sessionID := range sessionIDs {
		if err := rs.sendNotification(sessionID, result); err != nil {
			rs.logger.Error("Failed to send notification",
				"task_id", taskID,
				"session_id", sessionID,
				"error", err,
			)
			errors = append(errors, err)
		}
	}

	// Cleanup subscriptions for completed or failed tasks
	if result.Status == "completed" || result.Status == "failed" {
		rs.subMu.Lock()
		delete(rs.subscriptions, taskID)
		rs.subMu.Unlock()
	}

	// Return error if any notifications failed
	if len(errors) > 0 {
		return fmt.Errorf("failed to send %d notifications", len(errors))
	}

	return nil
}

// sendNotification sends an SSE notification to a specific session
func (rs *ResultStreamer) sendNotification(
	sessionID string,
	notification *TaskResultNotification,
) error {
	sseSession := rs.sseManager.GetSession(sessionID)
	if sseSession == nil {
		return fmt.Errorf("SSE session not found: %s", sessionID)
	}

	// Use MCP notification protocol
	// MCP defines notifications/tools/list_changed for tool updates
	// We'll send task result as a custom notification
	notificationData := map[string]interface{}{
		"method": "notifications/tasks/result",
		"params": notification,
	}

	return sseSession.SendNotification(notificationData)
}

// PublishProgress publishes progress updates for a task
func (rs *ResultStreamer) PublishProgress(
	ctx context.Context,
	taskID string,
	progress *TaskProgress,
) error {
	notification := &TaskResultNotification{
		TaskID:   taskID,
		Status:   "progress",
		Progress: progress,
	}

	rs.subMu.RLock()
	sessionIDs := rs.subscriptions[taskID]
	rs.subMu.RUnlock()

	if len(sessionIDs) == 0 {
		rs.logger.Debug("No subscribers for progress update", "task_id", taskID)
		return nil
	}

	rs.logger.Debug("Publishing progress update",
		"task_id", taskID,
		"subscribers", len(sessionIDs),
		"percentage", progress.Percentage,
	)

	var errors []error
	for _, sessionID := range sessionIDs {
		if err := rs.sendNotification(sessionID, notification); err != nil {
			rs.logger.Error("Failed to send progress",
				"task_id", taskID,
				"session_id", sessionID,
				"error", err,
			)
			errors = append(errors, err)
		}
	}

	// Return error if any notifications failed
	if len(errors) > 0 {
		return fmt.Errorf("failed to send %d progress notifications", len(errors))
	}

	return nil
}

// GetSubscriberCount returns the number of subscribers for a task
func (rs *ResultStreamer) GetSubscriberCount(taskID string) int {
	rs.subMu.RLock()
	defer rs.subMu.RUnlock()
	return len(rs.subscriptions[taskID])
}

// ClearSubscriptions removes all subscriptions (useful for cleanup/testing)
func (rs *ResultStreamer) ClearSubscriptions() {
	rs.subMu.Lock()
	defer rs.subMu.Unlock()
	rs.subscriptions = make(map[string][]string)
}
