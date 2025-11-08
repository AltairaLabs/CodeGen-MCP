package coordinator

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
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

// RealWorkerClient routes tasks to workers via gRPC
type RealWorkerClient struct {
	registry       *WorkerRegistry
	sessionManager *SessionManager
	logger         *slog.Logger
}

// NewRealWorkerClient creates a worker client that routes to registered workers
func NewRealWorkerClient(
	registry *WorkerRegistry,
	sessionManager *SessionManager,
	logger *slog.Logger,
) *RealWorkerClient {
	return &RealWorkerClient{
		registry:       registry,
		sessionManager: sessionManager,
		logger:         logger,
	}
}

// ExecuteTask routes a task to the appropriate worker via gRPC
func (r *RealWorkerClient) ExecuteTask(
	ctx context.Context,
	workspaceID string,
	toolName string,
	args TaskArgs,
) (*TaskResult, error) {
	session, worker, err := r.getSessionAndWorker(ctx)
	if err != nil {
		return nil, err
	}

	// Convert TaskArgs to protobuf format
	protoArgs := make(map[string]string)
	for k, v := range args {
		protoArgs[k] = fmt.Sprintf("%v", v)
	}

	// Execute task on worker
	startTime := time.Now()
	stream, err := r.startTask(ctx, worker, session, toolName, workspaceID, protoArgs)
	if err != nil {
		return nil, err
	}

	// Process streaming responses
	finalResult, taskError, err := r.processTaskStream(ctx, stream, worker.WorkerID)
	if err != nil {
		return nil, err
	}

	duration := time.Since(startTime)

	// Build and return result
	result, err := r.buildTaskResult(finalResult, taskError, duration)
	if err != nil {
		return nil, err
	}

	r.logger.InfoContext(ctx, "Task executed on worker",
		"worker_id", worker.WorkerID,
		"session_id", session.ID,
		"tool_name", toolName,
		"success", result.Success,
		"duration_ms", duration.Milliseconds(),
	)

	return result, nil
}

// getSessionAndWorker retrieves the session and its assigned worker
func (r *RealWorkerClient) getSessionAndWorker(ctx context.Context) (*Session, *RegisteredWorker, error) {
	sessionID, ok := ctx.Value("session_id").(string)
	if !ok {
		sessionID = "default-session"
	}

	session, _ := r.sessionManager.GetSession(sessionID)
	if session == nil {
		return nil, nil, fmt.Errorf("session not found: %s", sessionID)
	}

	if session.WorkerID == "" {
		return nil, nil, fmt.Errorf("no worker assigned to session: %s", sessionID)
	}

	worker := r.registry.GetWorker(session.WorkerID)
	if worker == nil {
		return nil, nil, fmt.Errorf("worker not found: %s", session.WorkerID)
	}

	return session, worker, nil
}

// startTask initiates task execution on a worker
func (r *RealWorkerClient) startTask(
	ctx context.Context,
	worker *RegisteredWorker,
	session *Session,
	toolName string,
	workspaceID string,
	protoArgs map[string]string,
) (protov1.TaskExecution_ExecuteTaskClient, error) {
	stream, err := worker.Client.TaskExec.ExecuteTask(ctx, &protov1.TaskRequest{
		SessionId: session.WorkerSessionID,
		ToolName:  toolName,
		Arguments: protoArgs,
		Context: &protov1.TaskContext{
			WorkspaceId: workspaceID,
			UserId:      session.UserID,
		},
	})

	if err != nil {
		r.logger.ErrorContext(ctx, "Failed to start task on worker",
			"worker_id", worker.WorkerID,
			"session_id", session.ID,
			"tool_name", toolName,
			"error", err,
		)
		return nil, fmt.Errorf("worker task execution failed: %w", err)
	}

	return stream, nil
}

// processTaskStream processes streaming responses from worker
func (r *RealWorkerClient) processTaskStream(
	ctx context.Context,
	stream protov1.TaskExecution_ExecuteTaskClient,
	workerID string,
) (*protov1.TaskResult, *protov1.TaskError, error) {
	var finalResult *protov1.TaskResult
	var taskError *protov1.TaskError

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			r.logger.ErrorContext(ctx, "Error receiving task response",
				"worker_id", workerID,
				"error", err,
			)
			return nil, nil, fmt.Errorf("stream error: %w", err)
		}

		r.handleTaskResponse(ctx, resp)

		// Extract result/error from response
		switch payload := resp.Payload.(type) {
		case *protov1.TaskResponse_Result:
			finalResult = payload.Result
		case *protov1.TaskResponse_Error:
			taskError = payload.Error
		}
	}

	return finalResult, taskError, nil
}

// handleTaskResponse processes individual task response messages
func (r *RealWorkerClient) handleTaskResponse(ctx context.Context, resp *protov1.TaskResponse) {
	switch payload := resp.Payload.(type) {
	case *protov1.TaskResponse_Log:
		r.logger.DebugContext(ctx, "Worker log",
			"level", payload.Log.Level,
			"message", payload.Log.Message,
		)
	case *protov1.TaskResponse_Progress:
		r.logger.DebugContext(ctx, "Worker progress",
			"percent", payload.Progress.PercentComplete,
			"stage", payload.Progress.Stage,
		)
	}
}

// buildTaskResult constructs a TaskResult from protobuf response
func (r *RealWorkerClient) buildTaskResult(
	finalResult *protov1.TaskResult,
	taskError *protov1.TaskError,
	duration time.Duration,
) (*TaskResult, error) {
	if taskError != nil {
		return nil, fmt.Errorf("task failed: %s - %s", taskError.Code, taskError.Message)
	}

	if finalResult == nil {
		return nil, fmt.Errorf("no result received from worker")
	}

	// Extract output from the outputs map
	output := ""
	if finalResult.Outputs != nil {
		if val, ok := finalResult.Outputs["output"]; ok {
			output = val
		}
	}

	exitCode := 0
	if finalResult.Metadata != nil {
		exitCode = int(finalResult.Metadata.ExitCode)
	}

	return &TaskResult{
		Success:  finalResult.Status == protov1.TaskResult_STATUS_SUCCESS,
		Output:   output,
		ExitCode: exitCode,
		Duration: duration,
	}, nil
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
	// Check for context cancellation before processing
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Simulate some work with context awareness
	select {
	case <-time.After(mockDuration):
		// Normal execution
	case <-ctx.Done():
		return nil, ctx.Err()
	}

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
