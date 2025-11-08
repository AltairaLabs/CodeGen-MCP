package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

// TaskExecutor handles task execution within sessions
type TaskExecutor struct {
	sessionPool *SessionPool
	activeTasks map[string]*ActiveTask
	mu          sync.RWMutex
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
func (te *TaskExecutor) Execute(ctx context.Context, req *protov1.TaskRequest, stream protov1.TaskExecution_ExecuteTaskServer) error {
	// Get session
	session, err := te.sessionPool.GetSession(req.SessionId)
	if err != nil {
		return err
	}

	// Create cancellable context
	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Register active task
	task := &ActiveTask{
		TaskID:    req.TaskId,
		SessionID: req.SessionId,
		ToolName:  req.ToolName,
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
func (te *TaskExecutor) executeToolInSession(ctx context.Context, session *WorkerSession, req *protov1.TaskRequest, stream protov1.TaskExecution_ExecuteTaskServer) (*protov1.TaskResult, error) {
	// Send progress update
	_ = stream.Send(&protov1.TaskResponse{
		TaskId: req.TaskId,
		Payload: &protov1.TaskResponse_Progress{
			Progress: &protov1.ProgressUpdate{
				PercentComplete: 10,
				Stage:           "starting",
				Message:         fmt.Sprintf("Executing %s", req.ToolName),
			},
		},
	})

	// Send log entry
	_ = stream.Send(&protov1.TaskResponse{
		TaskId: req.TaskId,
		Payload: &protov1.TaskResponse_Log{
			Log: &protov1.LogEntry{
				Level:       protov1.LogEntry_LEVEL_INFO,
				Message:     fmt.Sprintf("Starting tool execution: %s", req.ToolName),
				TimestampMs: time.Now().UnixMilli(),
				Source:      "task_executor",
			},
		},
	})

	// Route to appropriate tool handler
	var result *protov1.TaskResult
	var err error

	switch req.ToolName {
	case "fs.write":
		result, err = te.handleFsWrite(ctx, session, req)
	case "fs.read":
		result, err = te.handleFsRead(ctx, session, req)
	case "fs.list":
		result, err = te.handleFsList(ctx, session, req)
	case "run.python":
		result, err = te.handleRunPython(ctx, session, req, stream)
	case "pkg.install":
		result, err = te.handlePkgInstall(ctx, session, req, stream)
	case "echo":
		result, err = te.handleEcho(ctx, session, req)
	default:
		return nil, fmt.Errorf("unknown tool: %s", req.ToolName)
	}

	if err != nil {
		return nil, err
	}

	// Send completion progress
	_ = stream.Send(&protov1.TaskResponse{
		TaskId: req.TaskId,
		Payload: &protov1.TaskResponse_Progress{
			Progress: &protov1.ProgressUpdate{
				PercentComplete: 100,
				Stage:           "completed",
				Message:         "Task completed successfully",
			},
		},
	})

	return result, nil
}
