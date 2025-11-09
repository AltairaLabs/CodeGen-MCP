package coordinator

import (
	"context"
	"fmt"
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

// ExecuteTask routes a task to the appropriate worker via bidirectional stream
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

	// Check session state - if still creating, task will wait on worker side
	// If failed, return error immediately
	switch session.State {
	case SessionStateFailed:
		return nil, fmt.Errorf("session creation failed: %s", session.StateMessage)
	case SessionStateTerminating:
		return nil, fmt.Errorf("session is terminating")
	case SessionStateCreating, SessionStateReady:
		// Continue - worker will handle the wait if needed
	default:
		return nil, fmt.Errorf("session in unknown state: %s", session.State)
	}

	// Convert TaskArgs to protobuf format
	protoArgs := make(map[string]string)
	for k, v := range args {
		protoArgs[k] = fmt.Sprintf("%v", v)
	}

	// Generate unique task ID
	taskID := fmt.Sprintf("task-%d", time.Now().UnixNano())

	// Get next sequence number for this session (for deduplication)
	// Returns 0 if storage is not available (means no sequence tracking)
	sequence := r.sessionManager.GetNextSequence(ctx, session.ID)

	// Add timeout to prevent hanging forever if worker disconnects
	taskCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	// Send task to worker over stream
	startTime := time.Now()
	responseChan, err := r.sendTaskToWorker(taskCtx, worker, session, taskID, toolName, workspaceID, protoArgs, sequence)
	if err != nil {
		return nil, err
	}

	// Wait for responses from worker
	var finalResult *protov1.TaskStreamResult
	var taskError *protov1.TaskStreamError

	for {
		select {
		case <-taskCtx.Done():
			// Cleanup pending task
			worker.mu.Lock()
			delete(worker.PendingTasks, taskID)
			worker.mu.Unlock()
			return nil, fmt.Errorf("task timeout: %w", taskCtx.Err())

		case response, ok := <-responseChan:
			if !ok {
				// Channel closed, task failed
				return nil, fmt.Errorf("task response channel closed")
			}

			// Handle different response types
			switch payload := response.Payload.(type) {
			case *protov1.TaskStreamResponse_Result:
				finalResult = payload.Result

				// Sync session metadata from worker (worker is master)
				if finalResult.SessionMetadata != nil && len(finalResult.SessionMetadata) > 0 {
					if err := r.sessionManager.storage.SetSessionMetadata(ctx, session.ID, finalResult.SessionMetadata); err != nil {
						r.logger.WarnContext(ctx, "Failed to sync session metadata from worker",
							"session_id", session.ID,
							"error", err,
						)
					} else {
						r.logger.DebugContext(ctx, "Synced session metadata from worker",
							"session_id", session.ID,
							"metadata_keys", len(finalResult.SessionMetadata),
						)
					}
				}

				// Task complete, cleanup and return
				worker.mu.Lock()
				delete(worker.PendingTasks, taskID)
				worker.mu.Unlock()
				goto buildResult

			case *protov1.TaskStreamResponse_Error:
				taskError = payload.Error
				// Task errored, cleanup and return
				worker.mu.Lock()
				delete(worker.PendingTasks, taskID)
				worker.mu.Unlock()
				goto buildResult

			case *protov1.TaskStreamResponse_Log:
				r.logger.DebugContext(ctx, "Worker log",
					"level", payload.Log.Level,
					"message", payload.Log.Message,
				)

			case *protov1.TaskStreamResponse_Progress:
				r.logger.DebugContext(ctx, "Worker progress",
					"stage", payload.Progress.Stage,
					"percent", payload.Progress.PercentComplete,
				)
			}
		}
	}

buildResult:
	duration := time.Since(startTime)

	// Build and return result
	result, err := r.buildTaskResultFromStream(finalResult, taskError, duration)
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
	sessionID, ok := ctx.Value(sessionIDKey{}).(string)
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

// sendTaskToWorker sends a task assignment to worker over the bidirectional stream
func (r *RealWorkerClient) sendTaskToWorker(
	ctx context.Context,
	worker *RegisteredWorker,
	session *Session,
	taskID string,
	toolName string,
	workspaceID string,
	protoArgs map[string]string,
	sequence uint64,
) (chan *protov1.TaskStreamResponse, error) {
	worker.mu.Lock()
	defer worker.mu.Unlock()

	// Check if worker has an active task stream
	if worker.TaskStream == nil {
		return nil, fmt.Errorf("worker %s has no active task stream", worker.WorkerID)
	}

	// Create channel for receiving responses
	responseChan := make(chan *protov1.TaskStreamResponse, 10)
	worker.PendingTasks[taskID] = responseChan

	// Send task assignment
	assignment := &protov1.TaskStreamMessage{
		Message: &protov1.TaskStreamMessage_Assignment{
			Assignment: &protov1.TaskAssignment{
				TaskId:    taskID,
				SessionId: session.WorkerSessionID,
				ToolName:  toolName,
				Arguments: protoArgs,
				Context: &protov1.TaskContext{
					WorkspaceId: workspaceID,
					UserId:      session.UserID,
				},
				Sequence: sequence, // Monotonic sequence number for deduplication
			},
		},
	}

	err := worker.TaskStream.Send(assignment)
	if err != nil {
		delete(worker.PendingTasks, taskID)
		close(responseChan)
		return nil, fmt.Errorf("failed to send task to worker: %w", err)
	}

	r.logger.InfoContext(ctx, "Sent task to worker via stream",
		"worker_id", worker.WorkerID,
		"task_id", taskID,
		"tool_name", toolName,
	)

	return responseChan, nil
}

// buildTaskResultFromStream builds a TaskResult from stream response messages
func (r *RealWorkerClient) buildTaskResultFromStream(
	finalResult *protov1.TaskStreamResult,
	taskError *protov1.TaskStreamError,
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
		Success:  finalResult.Status == protov1.TaskStreamResult_STATUS_SUCCESS,
		Output:   output,
		ExitCode: exitCode,
		Duration: duration,
	}, nil
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
