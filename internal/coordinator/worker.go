package coordinator

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

// sessionIDKey is the context key for session ID
type sessionIDKey struct{}

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
func (r *RealWorkerClient) checkSessionState(session *Session) error {
	switch session.State {
	case SessionStateFailed:
		return fmt.Errorf("session creation failed: %s", session.StateMessage)
	case SessionStateTerminating:
		return fmt.Errorf("session is terminating")
	case SessionStateCreating, SessionStateReady:
		return nil
	default:
		return fmt.Errorf("session in unknown state: %s", session.State)
	}
}

// Note: waitForTaskResponse has been moved to worker_streams.go (untestable gRPC stream infrastructure)

func (r *RealWorkerClient) syncSessionMetadata(ctx context.Context, sessionID string, metadata map[string]string) {
	if err := r.sessionManager.storage.SetSessionMetadata(ctx, sessionID, metadata); err != nil {
		r.logger.WarnContext(ctx, "Failed to sync session metadata from worker", "session_id", sessionID, "error", err)
	} else {
		r.logger.DebugContext(ctx, "Synced session metadata from worker", "session_id", sessionID, "metadata_keys", len(metadata))
	}
}

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

	if err := r.checkSessionState(session); err != nil {
		return nil, err
	}

	protoArgs := make(map[string]string)
	for k, v := range args {
		protoArgs[k] = fmt.Sprintf("%v", v)
	}

	taskID := fmt.Sprintf("task-%d", time.Now().UnixNano())
	sequence := r.sessionManager.GetNextSequence(ctx, session.ID)

	taskCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	startTime := time.Now()
	responseChan, err := r.sendTaskToWorker(taskSendParams{
		ctx:         taskCtx,
		worker:      worker,
		session:     session,
		taskID:      taskID,
		toolName:    toolName,
		workspaceID: workspaceID,
		protoArgs:   protoArgs,
		sequence:    sequence,
	})
	if err != nil {
		return nil, err
	}

	finalResult, taskError, err := r.waitForTaskResponse(taskCtx, worker, session, taskID, responseChan)
	if err != nil {
		return nil, err
	}

	duration := time.Since(startTime)
	result, err := r.buildTaskResultFromStream(finalResult, taskError, duration, toolName)
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

// Note: ExecuteTypedTask has been moved to worker_streams.go (untestable gRPC stream infrastructure)

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
// taskSendParams holds parameters for sending tasks to workers
type taskSendParams struct {
	ctx         context.Context
	worker      *RegisteredWorker
	session     *Session
	taskID      string
	workspaceID string
	sequence    uint64
	// For legacy sendTaskToWorker
	toolName  string
	protoArgs map[string]string
	// For sendTypedTaskToWorker
	request *protov1.ToolRequest
}

func (r *RealWorkerClient) sendTaskToWorker(params taskSendParams) (chan *protov1.TaskStreamResponse, error) {
	params.worker.mu.Lock()
	defer params.worker.mu.Unlock()

	// Check if worker has an active task stream
	if params.worker.TaskStream == nil {
		return nil, fmt.Errorf("worker %s has no active task stream", params.worker.WorkerID)
	}

	// Create channel for receiving responses and track with tool name
	responseChan := make(chan *protov1.TaskStreamResponse, 10)
	params.worker.PendingTasks[params.taskID] = &PendingTask{
		ResponseChan: responseChan,
		ToolName:     params.toolName,
	}

	// DEPRECATED: This method should not be used anymore - use sendTypedTaskToWorker instead
	return nil, fmt.Errorf("sendTaskToWorker is deprecated, use ExecuteTypedTask instead")
}

// Note: sendTypedTaskToWorker has been moved to worker_streams.go (untestable gRPC stream infrastructure)

// getToolNameFromTypedRequest extracts the tool name from a ToolRequest
func getToolNameFromTypedRequest(request *protov1.ToolRequest) string {
	switch request.Request.(type) {
	case *protov1.ToolRequest_Echo:
		return "echo"
	case *protov1.ToolRequest_FsRead:
		return "fs.read"
	case *protov1.ToolRequest_FsWrite:
		return "fs.write"
	case *protov1.ToolRequest_FsList:
		return "fs.list"
	case *protov1.ToolRequest_RunPython:
		return "run.python"
	case *protov1.ToolRequest_PkgInstall:
		return "pkg.install"
	default:
		return "unknown"
	}
}

// Note: buildTaskResultFromStream has been moved to worker_streams.go (untestable gRPC stream infrastructure)

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

// ExecuteTypedTask simulates typed task execution
func (m *MockWorkerClient) ExecuteTypedTask(
	ctx context.Context,
	workspaceID string,
	request *protov1.ToolRequest,
) (*TaskResult, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Simulate some work
	select {
	case <-time.After(mockDuration):
		// Normal execution
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Handle each tool type
	switch req := request.Request.(type) {
	case *protov1.ToolRequest_Echo:
		return &TaskResult{
			Success:  true,
			Output:   req.Echo.Message,
			ExitCode: 0,
			Duration: mockDuration,
		}, nil

	case *protov1.ToolRequest_FsRead:
		return &TaskResult{
			Success:  true,
			Output:   fmt.Sprintf("Content of %s in workspace %s", req.FsRead.Path, workspaceID),
			ExitCode: 0,
			Duration: mockDuration,
		}, nil

	case *protov1.ToolRequest_FsWrite:
		return &TaskResult{
			Success:  true,
			Output:   fmt.Sprintf("Wrote %d bytes to %s", len(req.FsWrite.Contents), req.FsWrite.Path),
			ExitCode: 0,
			Duration: mockDuration,
		}, nil

	default:
		return nil, fmt.Errorf("unknown tool type: %T", request.Request)
	}
}
