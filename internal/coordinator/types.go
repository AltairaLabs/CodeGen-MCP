package coordinator

import (
	"context"
	"time"
)

// Session represents an MCP client session with workspace isolation
type Session struct {
	ID          string
	WorkspaceID string
	UserID      string
	CreatedAt   time.Time
	LastActive  time.Time
	Metadata    map[string]string
}

// WorkerClient defines the interface for communicating with workers
type WorkerClient interface {
	// ExecuteTask sends a task to a worker and returns the result
	ExecuteTask(ctx context.Context, workspaceID, toolName string, args TaskArgs) (*TaskResult, error)
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
