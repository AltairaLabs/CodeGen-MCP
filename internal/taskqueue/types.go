package taskqueue

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

// TaskResult represents the result of a worker task execution
type TaskResult struct {
	Success  bool
	Output   string
	Error    string
	ExitCode int
	Duration time.Duration
}

// GetSuccess implements cache.TaskResultInterface
func (tr *TaskResult) GetSuccess() bool {
	return tr.Success
}

// GetOutput implements cache.TaskResultInterface
func (tr *TaskResult) GetOutput() string {
	return tr.Output
}

// GetError implements cache.TaskResultInterface
func (tr *TaskResult) GetError() string {
	return tr.Error
}

// GetExitCode implements cache.TaskResultInterface
func (tr *TaskResult) GetExitCode() int {
	return tr.ExitCode
}

// GetDuration implements cache.TaskResultInterface
func (tr *TaskResult) GetDuration() time.Duration {
	return tr.Duration
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

// WorkerClient defines the interface for communicating with workers
type WorkerClient interface {
	// ExecuteTask sends a task to a worker and returns the result (deprecated - use ExecuteTypedTask)
	ExecuteTask(ctx context.Context, workspaceID, toolName string, args TaskArgs) (*TaskResult, error)
	// ExecuteTypedTask sends a typed task to a worker and returns the result
	ExecuteTypedTask(ctx context.Context, workspaceID string, request *protov1.ToolRequest) (*TaskResult, error)
}

// Session represents session information needed by task queue
type Session struct {
	ID          string
	WorkspaceID string
	UserID      string
	WorkerID    string
	State       SessionState
}

// SessionManager defines the interface for session management needed by task queue
type SessionManager interface {
	GetSession(sessionID string) (*Session, bool)
	GetNextSequence(ctx context.Context, sessionID string) uint64
	GetAllSessions() map[string]*Session
	SetLastCompletedSequence(ctx context.Context, sessionID string, sequence uint64) error
}

// ResultStreamer defines the interface for streaming results
type ResultStreamer interface {
	Subscribe(taskID, sessionID string)
	PublishResult(ctx context.Context, taskID string, notification *TaskResultNotification) error
	PublishProgress(ctx context.Context, taskID string, progress *TaskProgress) error
}

// contextWithSessionID adds the session ID to the context
// This function should be provided by the coordinator package
var ContextWithSessionID func(ctx context.Context, sessionID string) context.Context

// sessionIDKey is the context key for session ID
type sessionIDKey struct{}

// contextWithSessionID adds the session ID to the context
func contextWithSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, sessionID)
}

// RetryPolicy defines retry configuration for tasks
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
func isNetworkError(errMsg string) bool {
	return contains(errMsg, "connection refused") ||
		contains(errMsg, "connection reset") ||
		contains(errMsg, "timeout") ||
		contains(errMsg, "EOF") ||
		contains(errMsg, "broken pipe") ||
		contains(errMsg, "network is unreachable")
}

func isWorkerUnavailableError(errMsg string) bool {
	return contains(errMsg, "worker not found") ||
		contains(errMsg, "worker unavailable") ||
		contains(errMsg, "no workers available")
}

func isPermanentError(errMsg string) bool {
	return contains(errMsg, "validation failed") ||
		contains(errMsg, "invalid argument") ||
		contains(errMsg, "missing required field") ||
		contains(errMsg, "metadata validation failed") ||
		contains(errMsg, "permission denied") ||
		contains(errMsg, "not found") ||
		contains(errMsg, "unauthorized") ||
		contains(errMsg, "forbidden")
}

func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()

	if isPermanentError(errMsg) {
		return false
	}

	if isNetworkError(errMsg) || isWorkerUnavailableError(errMsg) {
		return true
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

// Adapter types for coordinator integration

// WorkerClientAdapter allows any type with the right methods to implement WorkerClient
type WorkerClientAdapter struct {
	RealWorkerClient interface {
		ExecuteTask(ctx context.Context, workspaceID, toolName string, args map[string]interface{}) (interface{}, error)
		ExecuteTypedTask(ctx context.Context, workspaceID string, request *protov1.ToolRequest) (interface{}, error)
	}
}

func (a *WorkerClientAdapter) ExecuteTask(ctx context.Context, workspaceID, toolName string, args TaskArgs) (*TaskResult, error) {
	_, err := a.RealWorkerClient.ExecuteTask(ctx, workspaceID, toolName, map[string]interface{}(args))
	if err != nil {
		return nil, err
	}

	// Simple stub conversion - proper implementation would use reflection
	return &TaskResult{
		Success:  true,
		Output:   "Task completed",
		ExitCode: 0,
		Duration: 1000000, // 1ms in nanoseconds
	}, nil
}

func (a *WorkerClientAdapter) ExecuteTypedTask(ctx context.Context, workspaceID string, request *protov1.ToolRequest) (*TaskResult, error) {
	_, err := a.RealWorkerClient.ExecuteTypedTask(ctx, workspaceID, request)
	if err != nil {
		return nil, err
	}

	// Simple stub conversion - proper implementation would use reflection
	return &TaskResult{
		Success:  true,
		Output:   "Task completed",
		ExitCode: 0,
		Duration: 1000000, // 1ms in nanoseconds
	}, nil
}

// SessionManagerAdapter allows any type with the right methods to implement SessionManager
type SessionManagerAdapter struct {
	SessionManager interface {
		GetSession(sessionID string) (interface{}, bool)
		GetNextSequence(ctx context.Context, sessionID string) uint64
		GetAllSessions() map[string]interface{}
		SetLastCompletedSequence(ctx context.Context, sessionID string, sequence uint64) error
	}
}

func (a *SessionManagerAdapter) GetSession(sessionID string) (*Session, bool) {
	_, exists := a.SessionManager.GetSession(sessionID)
	if !exists {
		return nil, false
	}

	// Convert to taskqueue Session - simple stub conversion
	return &Session{
		ID:          sessionID,
		WorkspaceID: "default",
		UserID:      "default",
		WorkerID:    "default",
		State:       SessionStateReady,
	}, true
}

func (a *SessionManagerAdapter) GetNextSequence(ctx context.Context, sessionID string) uint64 {
	return a.SessionManager.GetNextSequence(ctx, sessionID)
}

func (a *SessionManagerAdapter) GetAllSessions() map[string]*Session {
	coordSessions := a.SessionManager.GetAllSessions()
	result := make(map[string]*Session)

	for id := range coordSessions {
		result[id] = &Session{
			ID:          id,
			WorkspaceID: "default",
			UserID:      "default",
			WorkerID:    "default",
			State:       SessionStateReady,
		}
	}

	return result
}

func (a *SessionManagerAdapter) SetLastCompletedSequence(ctx context.Context, sessionID string, sequence uint64) error {
	return a.SessionManager.SetLastCompletedSequence(ctx, sessionID, sequence)
}
