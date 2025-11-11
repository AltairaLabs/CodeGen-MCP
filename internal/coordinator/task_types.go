package coordinator

import (
	"time"
)

// TaskResponse represents the immediate response when a task is enqueued
type TaskResponse struct {
	TaskID    string    `json:"task_id"`
	Status    string    `json:"status"` // "queued", "dispatched", "completed", "failed"
	SessionID string    `json:"session_id"`
	Sequence  uint64    `json:"sequence"`
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// TaskResultNotification represents a notification sent when a task completes
type TaskResultNotification struct {
	TaskID      string        `json:"task_id"`
	Status      string        `json:"status"` // "completed", "failed", "progress"
	Result      *TaskResult   `json:"result,omitempty"`
	Progress    *TaskProgress `json:"progress,omitempty"`
	Error       string        `json:"error,omitempty"`
	CompletedAt time.Time     `json:"completed_at,omitempty"`
}

// TaskProgress represents progress updates for long-running tasks
type TaskProgress struct {
	Percentage int    `json:"percentage"` // 0-100
	Message    string `json:"message"`
	Stage      string `json:"stage"`
}

// QueuedTask represents a task in the queue with retry and sequencing support
type QueuedTask struct {
	ID             string        // Unique task ID
	SessionID      string        // Target session ID
	ConversationID string        // External conversation ID (for future use)
	ToolName       string        // Tool to execute
	Args           TaskArgs      // Tool arguments
	Sequence       uint64        // Monotonic sequence number per session
	State          string        // Current state (queued, dispatched, completed, failed, retrying)
	RetryCount     int           // Number of retry attempts
	MaxRetries     int           // Maximum retry attempts
	CreatedAt      time.Time     // When task was created
	DispatchedAt   *time.Time    // When task was sent to worker (nil if not dispatched)
	CompletedAt    *time.Time    // When task completed (nil if not completed)
	Timeout        time.Duration // Max time to wait in queue
	Error          string        // Last error message
	NextRetryAt    *time.Time    // When to retry next (nil if not retrying)
}
