package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

const (
	progressStarting  = 10
	progressCompleted = 100
	unknownToolName   = "unknown"
)

// MetadataRequirement defines a required metadata field for a tool
type MetadataRequirement struct {
	Key         string
	Description string
	Required    bool
}

// ToolMetadataValidator defines validation requirements for a tool
type ToolMetadataValidator interface {
	// GetRequiredMetadata returns the list of metadata requirements for this tool
	GetRequiredMetadata() []MetadataRequirement
}

// ValidateMetadata checks if the session metadata satisfies the tool's requirements
func ValidateMetadata(metadata map[string]string, requirements []MetadataRequirement) error {
	for _, req := range requirements {
		if req.Required {
			value, exists := metadata[req.Key]
			if !exists {
				return fmt.Errorf("required metadata field missing: %s (%s)", req.Key, req.Description)
			}
			if value == "" {
				return fmt.Errorf("required metadata field is empty: %s (%s)", req.Key, req.Description)
			}
		}
	}
	return nil
}

// TaskExecutor handles task execution within sessions
type TaskExecutor struct {
	sessionPool *SessionPool
	activeTasks map[string]*ActiveTask
	mu          sync.RWMutex
}

// getToolNameFromTypedRequest extracts the tool name from a typed request
func getToolNameFromTypedRequest(req *protov1.ToolRequest) string {
	if req == nil {
		return unknownToolName
	}
	switch req.Request.(type) {
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
		return unknownToolName
	}
}

// ActiveTask represents a running task
type ActiveTask struct {
	TaskID    string
	SessionID string
	ToolName  string
	StartTime time.Time
	Status    protov1.TaskResult_Status
	Cancel    context.CancelFunc
	mu        sync.RWMutex
}

// NewTaskExecutor creates a new task executor
func NewTaskExecutor(sessionPool *SessionPool) *TaskExecutor {
	return &TaskExecutor{
		sessionPool: sessionPool,
		activeTasks: make(map[string]*ActiveTask),
	}
}

// Execute executes a task and streams results
//
//nolint:lll // Protobuf types create inherently long function signatures
func (te *TaskExecutor) Execute(ctx context.Context, req *protov1.TaskRequest, stream protov1.TaskExecution_ExecuteTaskServer) error {
	// Get session
	session, err := te.sessionPool.GetSession(req.SessionId)
	if err != nil {
		return err
	}

	// Create cancellable context
	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Extract tool name from typed request
	toolName := getToolNameFromTypedRequest(req.TypedRequest)

	// Register active task
	task := &ActiveTask{
		TaskID:    req.TaskId,
		SessionID: req.SessionId,
		ToolName:  toolName,
		StartTime: time.Now(),
		Status:    protov1.TaskResult_STATUS_UNSPECIFIED,
		Cancel:    cancel,
	}
	te.mu.Lock()
	te.activeTasks[req.TaskId] = task
	te.mu.Unlock()

	defer func() {
		te.mu.Lock()
		delete(te.activeTasks, req.TaskId)
		te.mu.Unlock()
	}()

	// Update session activity
	if updateErr := te.sessionPool.UpdateSessionActivity(req.SessionId); updateErr != nil {
		return updateErr
	}

	// Increment active tasks
	if incErr := te.sessionPool.IncrementActiveTasks(req.SessionId); incErr != nil {
		return incErr
	}
	defer func() {
		_ = te.sessionPool.DecrementActiveTasks(req.SessionId) // Best effort cleanup
	}()

	// Apply timeout if specified
	if req.Constraints != nil && req.Constraints.TimeoutSeconds > 0 {
		var timeoutCancel context.CancelFunc
		taskCtx, timeoutCancel = context.WithTimeout(taskCtx, time.Duration(req.Constraints.TimeoutSeconds)*time.Second)
		defer timeoutCancel()
	}

	// Track task in session history
	if histErr := te.sessionPool.AddTaskToHistory(req.SessionId, req.TaskId); histErr != nil {
		// Non-fatal - log but continue
		_ = fmt.Errorf("failed to add task to history: %w", err)
	}

	// Execute tool
	startTime := time.Now()
	result, err := te.executeToolInSession(taskCtx, session, req, stream)
	duration := time.Since(startTime)

	if err != nil {
		// Send error response
		return stream.Send(&protov1.TaskResponse{
			TaskId: req.TaskId,
			Payload: &protov1.TaskResponse_Error{
				Error: &protov1.TaskError{
					Code:      "EXECUTION_ERROR",
					Message:   err.Error(),
					Details:   "",
					Retriable: false,
				},
			},
		})
	}

	// Update result with metadata
	if result.Metadata == nil {
		result.Metadata = &protov1.ExecutionMetadata{}
	}
	result.Metadata.StartTimeMs = startTime.UnixMilli()
	result.Metadata.EndTimeMs = time.Now().UnixMilli()
	result.Metadata.DurationMs = duration.Milliseconds()

	// Send final result
	return stream.Send(&protov1.TaskResponse{
		TaskId: req.TaskId,
		Payload: &protov1.TaskResponse_Result{
			Result: result,
		},
	})
}

// Cancel cancels a running task
func (te *TaskExecutor) Cancel(ctx context.Context, req *protov1.CancelRequest) (*protov1.CancelResponse, error) {
	te.mu.RLock()
	task, exists := te.activeTasks[req.TaskId]
	te.mu.RUnlock()

	if !exists {
		return &protov1.CancelResponse{
			Cancelled: false, //nolint:misspell // Field name from protobuf definition
			State:     "not_found",
		}, nil
	}

	task.mu.Lock()
	task.Cancel()
	task.Status = protov1.TaskResult_STATUS_CANCELLED //nolint:misspell // Constant from protobuf definition
	task.mu.Unlock()

	return &protov1.CancelResponse{
		Cancelled: true, //nolint:misspell // Field name from protobuf definition
		State:     "canceled",
	}, nil
}

// GetStatus gets the status of a task
func (te *TaskExecutor) GetStatus(ctx context.Context, req *protov1.StatusRequest) (*protov1.StatusResponse, error) {
	te.mu.RLock()
	task, exists := te.activeTasks[req.TaskId]
	te.mu.RUnlock()

	if !exists {
		return &protov1.StatusResponse{
			TaskId:    req.TaskId,
			Status:    protov1.TaskResult_STATUS_UNSPECIFIED,
			ElapsedMs: 0,
		}, nil
	}

	task.mu.RLock()
	defer task.mu.RUnlock()

	return &protov1.StatusResponse{
		TaskId:    req.TaskId,
		Status:    task.Status,
		ElapsedMs: time.Since(task.StartTime).Milliseconds(),
	}, nil
}

// executeToolInSession executes a specific tool within a session
//
//nolint:lll // Protobuf types create inherently long function signatures
func (te *TaskExecutor) executeToolInSession(ctx context.Context, session *WorkerSession, req *protov1.TaskRequest, stream protov1.TaskExecution_ExecuteTaskServer) (*protov1.TaskResult, error) {
	// Extract tool name from typed request
	toolName := getToolNameFromTypedRequest(req.TypedRequest)

	// Validate metadata requirements before execution
	metadata, metaErr := te.sessionPool.GetSessionMetadata(req.SessionId)
	if metaErr != nil {
		return nil, fmt.Errorf("failed to retrieve session metadata: %w", metaErr)
	}

	// Get metadata requirements for this tool
	requirements := te.getToolMetadataRequirements(toolName)
	if len(requirements) > 0 {
		if err := ValidateMetadata(metadata, requirements); err != nil {
			return nil, fmt.Errorf("metadata validation failed: %w", err)
		}
	}

	// Send progress update
	_ = stream.Send(&protov1.TaskResponse{
		TaskId: req.TaskId,
		Payload: &protov1.TaskResponse_Progress{
			Progress: &protov1.ProgressUpdate{
				PercentComplete: progressStarting,
				Stage:           "starting",
				Message:         fmt.Sprintf("Executing %s", toolName),
			},
		},
	})

	// Send log entry
	_ = stream.Send(&protov1.TaskResponse{
		TaskId: req.TaskId,
		Payload: &protov1.TaskResponse_Log{
			Log: &protov1.LogEntry{
				Level:       protov1.LogEntry_LEVEL_INFO,
				Message:     fmt.Sprintf("Starting tool execution: %s", toolName),
				TimestampMs: time.Now().UnixMilli(),
				Source:      "task_executor",
			},
		},
	})

	// Route to typed tool handler
	if req.TypedRequest == nil {
		return nil, fmt.Errorf("task request missing typed request")
	}

	result, err := te.executeTypedTool(ctx, session, req.TypedRequest, stream)
	if err != nil {
		return nil, err
	}

	// Send completion progress
	_ = stream.Send(&protov1.TaskResponse{
		TaskId: req.TaskId,
		Payload: &protov1.TaskResponse_Progress{
			Progress: &protov1.ProgressUpdate{
				PercentComplete: progressCompleted,
				Stage:           "completed",
				Message:         "Task completed successfully",
			},
		},
	})

	return result, nil
}

// getToolMetadataRequirements returns metadata requirements for a specific tool
// Tools can be extended to declare their metadata dependencies here
func (te *TaskExecutor) getToolMetadataRequirements(toolName string) []MetadataRequirement {
	// Define metadata requirements per tool
	// This can be extended as new tools are added
	//
	// Example requirements (currently none are enforced):
	//
	// case "run.python":
	//     return []MetadataRequirement{
	//         {Key: "python_version", Description: "Python interpreter version", Required: true},
	//         {Key: "virtual_env", Description: "Virtual environment path", Required: false},
	//     }
	// case "pkg.install":
	//     return []MetadataRequirement{
	//         {Key: "package_manager", Description: "Package manager to use", Required: true},
	//     }

	// No metadata requirements currently defined
	return []MetadataRequirement{}
}

// executeTypedTool executes a tool using strongly-typed request/response
//
//nolint:lll // Protobuf types create inherently long function signatures
func (te *TaskExecutor) executeTypedTool(ctx context.Context, session *WorkerSession, request *protov1.ToolRequest, stream protov1.TaskExecution_ExecuteTaskServer) (*protov1.TaskResult, error) {
	// Route based on the type of request
	switch req := request.Request.(type) {
	case *protov1.ToolRequest_Echo:
		return te.handleEchoTyped(ctx, session, req.Echo)
	case *protov1.ToolRequest_FsRead:
		return te.handleFsReadTyped(ctx, session, req.FsRead)
	case *protov1.ToolRequest_FsWrite:
		return te.handleFsWriteTyped(ctx, session, req.FsWrite)
	case *protov1.ToolRequest_FsList:
		return te.handleFsListTyped(ctx, session, req.FsList)
	case *protov1.ToolRequest_RunPython:
		return te.handleRunPythonTyped(ctx, session, req.RunPython, stream)
	case *protov1.ToolRequest_PkgInstall:
		return te.handlePkgInstallTyped(ctx, session, req.PkgInstall, stream)
	default:
		return nil, fmt.Errorf("unknown typed tool request: %T", request.Request)
	}
}
