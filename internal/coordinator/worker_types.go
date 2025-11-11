package coordinator

import (
	"context"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

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
