package coordinator

import (
	"context"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

// SessionState represents the lifecycle state of a session
type SessionState string

const (
	// SessionStateCreating indicates the session is being created on the worker
	SessionStateCreating SessionState = "creating"
	// SessionStateReady indicates the session is ready to accept tasks
	SessionStateReady SessionState = "ready"
	// SessionStateFailed indicates session creation failed on the worker
	SessionStateFailed SessionState = "failed"
	// SessionStateTerminating indicates the session is being shut down
	SessionStateTerminating SessionState = "terminating"
)

// Session represents an MCP client session with workspace isolation
type Session struct {
	ID          string
	WorkspaceID string
	UserID      string
	CreatedAt   time.Time
	LastActive  time.Time
	Metadata    map[string]string
	// Worker routing and state
	WorkerID        string       // Assigned worker (session affinity)
	WorkerSessionID string       // Session ID on the worker
	State           SessionState // Current lifecycle state
	StateMessage    string       // Optional state details (e.g., error message)
	LastCheckpoint  string       // Last checkpoint ID for recovery
	TaskHistory     []string     // Recent task IDs
}

// WorkerClient defines the interface for communicating with workers
type WorkerClient interface {
	// ExecuteTask sends a task to a worker and returns the result (deprecated - use ExecuteTypedTask)
	ExecuteTask(ctx context.Context, workspaceID, toolName string, args TaskArgs) (*TaskResult, error)
	// ExecuteTypedTask sends a typed task to a worker and returns the result
	ExecuteTypedTask(ctx context.Context, workspaceID string, request *protov1.ToolRequest) (*TaskResult, error)
}

// TaskResult represents the result of a worker task execution
type TaskResult struct {
	Success  bool
	Output   string
	Error    string
	ExitCode int
	Duration time.Duration
}

// TaskArgs is a convenience alias for task argument maps
type TaskArgs map[string]interface{}

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

// AuditEntry represents a logged event for provenance tracking
type AuditEntry struct {
	Timestamp   time.Time
	SessionID   string
	UserID      string
	ToolName    string
	Arguments   map[string]interface{}
	Result      *TaskResult
	ErrorMsg    string
	TraceID     string
	WorkspaceID string
}

// RetryPolicy defines how tasks should be retried on failure
type RetryPolicy struct {
	MaxRetries        int           // Maximum number of retry attempts (0 = no retries)
	InitialDelay      time.Duration // Initial delay before first retry
	MaxDelay          time.Duration // Maximum delay between retries
	BackoffMultiplier float64       // Multiplier for exponential backoff (e.g., 2.0)
}

// DefaultRetryPolicy returns the default retry policy for tasks
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:        3,
		InitialDelay:      1 * time.Second,
		MaxDelay:          30 * time.Second,
		BackoffMultiplier: 2.0,
	}
}

// NetworkErrorRetryPolicy returns a retry policy for network errors (more retries)
func NetworkErrorRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:        5,
		InitialDelay:      500 * time.Millisecond,
		MaxDelay:          60 * time.Second,
		BackoffMultiplier: 2.0,
	}
}

// NoRetryPolicy returns a policy that disables retries
func NoRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:        0,
		InitialDelay:      0,
		MaxDelay:          0,
		BackoffMultiplier: 0,
	}
}

// CalculateNextRetryTime calculates when a task should be retried based on the retry policy
// Uses exponential backoff with jitter to prevent thundering herd
func (p RetryPolicy) CalculateNextRetryTime(retryCount int) time.Time {
	if retryCount >= p.MaxRetries {
		return time.Time{} // Return zero time if max retries exceeded
	}

	// Calculate exponential backoff: initialDelay * (backoffMultiplier ^ retryCount)
	delay := float64(p.InitialDelay)
	for i := 0; i < retryCount; i++ {
		delay *= p.BackoffMultiplier
	}

	// Cap at max delay
	if time.Duration(delay) > p.MaxDelay {
		delay = float64(p.MaxDelay)
	}

	// Add jitter (±25% randomness to prevent thundering herd)
	jitterPercent := float64(time.Now().UnixNano()%1000) / 1000.0 // 0.0 to 1.0
	jitter := delay * 0.25 * (2*jitterPercent - 1)                // -25% to +25%
	finalDelay := time.Duration(delay + jitter)

	return time.Now().Add(finalDelay)
}

// IsRetryableError determines if an error should trigger a retry
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()

	// Network errors are retryable
	if contains(errMsg, "connection refused") ||
		contains(errMsg, "connection reset") ||
		contains(errMsg, "timeout") ||
		contains(errMsg, "EOF") ||
		contains(errMsg, "broken pipe") ||
		contains(errMsg, "network is unreachable") {
		return true
	}

	// Worker unavailable errors are retryable
	if contains(errMsg, "worker not found") ||
		contains(errMsg, "worker unavailable") ||
		contains(errMsg, "no workers available") {
		return true
	}

	// Permanent errors are NOT retryable
	if contains(errMsg, "validation failed") ||
		contains(errMsg, "invalid argument") ||
		contains(errMsg, "missing required field") ||
		contains(errMsg, "metadata validation failed") ||
		contains(errMsg, "permission denied") ||
		contains(errMsg, "not found") ||
		contains(errMsg, "unauthorized") ||
		contains(errMsg, "forbidden") {
		return false
	}

	// Default: transient errors are retryable
	return true
}

// contains is a helper function to check if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || len(s) > len(substr) &&
			(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
				findInString(s, substr)))
}

func findInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
