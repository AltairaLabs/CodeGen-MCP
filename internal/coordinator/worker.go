package coordinator

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// AuditLogger handles audit logging for MCP tool calls
type AuditLogger struct {
	logger *slog.Logger
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(logger *slog.Logger) *AuditLogger {
	return &AuditLogger{
		logger: logger,
	}
}

// LogToolCall logs a tool invocation with all relevant context
func (al *AuditLogger) LogToolCall(ctx context.Context, entry *AuditEntry) {
	al.logger.InfoContext(ctx, "tool_call",
		"session_id", entry.SessionID,
		"user_id", entry.UserID,
		"tool_name", entry.ToolName,
		"workspace_id", entry.WorkspaceID,
		"trace_id", entry.TraceID,
		"timestamp", entry.Timestamp,
	)
}

// LogToolResult logs a tool execution result
func (al *AuditLogger) LogToolResult(ctx context.Context, entry *AuditEntry) {
	if entry.Result != nil {
		al.logger.InfoContext(ctx, "tool_result",
			"session_id", entry.SessionID,
			"tool_name", entry.ToolName,
			"success", entry.Result.Success,
			"exit_code", entry.Result.ExitCode,
			"duration_ms", entry.Result.Duration.Milliseconds(),
			"trace_id", entry.TraceID,
		)
	} else if entry.ErrorMsg != "" {
		al.logger.ErrorContext(ctx, "tool_error",
			"session_id", entry.SessionID,
			"tool_name", entry.ToolName,
			"error", entry.ErrorMsg,
			"trace_id", entry.TraceID,
		)
	}
}

// MockWorkerClient is a simple in-memory worker client for testing and POC
type MockWorkerClient struct{}

// NewMockWorkerClient creates a mock worker client
func NewMockWorkerClient() *MockWorkerClient {
	return &MockWorkerClient{}
}

// ExecuteTask simulates task execution (POC implementation)
const mockDuration = 10 * time.Millisecond

func (m *MockWorkerClient) ExecuteTask(
	ctx context.Context,
	workspaceID string,
	toolName string,
	args TaskArgs,
) (*TaskResult, error) {
	// Simulate some work
	time.Sleep(mockDuration)

	switch toolName {
	case toolEcho:
		message, ok := args["message"].(string)
		if !ok {
			return nil, fmt.Errorf("message argument must be a string")
		}
		return &TaskResult{
			Success:  true,
			Output:   message,
			ExitCode: 0,
			Duration: mockDuration,
		}, nil

	case toolFsRead:
		path, ok := args["path"].(string)
		if !ok {
			return nil, fmt.Errorf("path argument must be a string")
		}
		// Mock file content
		return &TaskResult{
			Success:  true,
			Output:   fmt.Sprintf("Content of %s in workspace %s", path, workspaceID),
			ExitCode: 0,
			Duration: mockDuration,
		}, nil

	case toolFsWrite:
		path, ok := args["path"].(string)
		if !ok {
			return nil, fmt.Errorf("path argument must be a string")
		}
		contents, ok := args["contents"].(string)
		if !ok {
			return nil, fmt.Errorf("contents argument must be a string")
		}
		return &TaskResult{
			Success:  true,
			Output:   fmt.Sprintf("Wrote %d bytes to %s", len(contents), path),
			ExitCode: 0,
			Duration: mockDuration,
		}, nil

	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}
