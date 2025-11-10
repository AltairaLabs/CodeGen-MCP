package coordinator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/storage"
	"google.golang.org/protobuf/proto"
)

// TaskQueueInterface defines the interface for task queue operations
type TaskQueueInterface interface {
	// EnqueueTask adds a new task to the queue for a session
	// Deprecated: Use EnqueueTypedTask for type-safe requests
	EnqueueTask(ctx context.Context, sessionID, toolName string, args storage.TaskArgs) (string, error)

	// EnqueueTypedTask adds a new task with strongly-typed request
	EnqueueTypedTask(ctx context.Context, sessionID string, request *protov1.ToolRequest) (string, error)

	// GetTaskResult waits for and returns the result of a task
	GetTaskResult(ctx context.Context, taskID string) (*TaskResult, error)

	// GetTask retrieves task information by ID
	GetTask(ctx context.Context, taskID string) (*storage.QueuedTask, error)

	// SetResultStreamer sets the result streamer for async notifications
	SetResultStreamer(streamer *ResultStreamer)

	// Start begins background task dispatch workers
	Start()

	// Stop gracefully shuts down the task queue
	Stop()
}

// TaskQueue manages asynchronous task queuing and dispatch
type TaskQueue struct {
	storage        storage.TaskQueueStorage
	sessionManager *SessionManager
	workerClient   WorkerClient
	dispatcher     *TaskDispatcher
	logger         *slog.Logger
	resultStreamer *ResultStreamer

	// Result notification channels
	resultChannels map[string]chan *TaskResult
	resultMu       sync.RWMutex

	// Background worker control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Configuration
	dispatchInterval time.Duration
	maxDispatchBatch int
}

// TaskQueueConfig holds configuration for TaskQueue
type TaskQueueConfig struct {
	DispatchInterval time.Duration // How often to check for ready tasks
	MaxDispatchBatch int           // Max tasks to dispatch per cycle
}

// DefaultTaskQueueConfig returns default configuration
func DefaultTaskQueueConfig() TaskQueueConfig {
	return TaskQueueConfig{
		DispatchInterval: 100 * time.Millisecond,
		MaxDispatchBatch: 10,
	}
}

// NewTaskQueue creates a new task queue manager
func NewTaskQueue(
	storage storage.TaskQueueStorage,
	sessionMgr *SessionManager,
	workerClient WorkerClient,
	dispatcher *TaskDispatcher,
	logger *slog.Logger,
	config TaskQueueConfig,
) *TaskQueue {
	ctx, cancel := context.WithCancel(context.Background())

	return &TaskQueue{
		storage:          storage,
		sessionManager:   sessionMgr,
		workerClient:     workerClient,
		dispatcher:       dispatcher,
		logger:           logger,
		resultChannels:   make(map[string]chan *TaskResult),
		ctx:              ctx,
		cancel:           cancel,
		dispatchInterval: config.DispatchInterval,
		maxDispatchBatch: config.MaxDispatchBatch,
	}
}

// Start begins background task dispatch workers
func (tq *TaskQueue) Start() {
	tq.logger.Info("Starting task queue background workers")

	// Start task dispatcher
	tq.wg.Add(1)
	go tq.dispatchLoop()

	// Start result timeout monitor
	tq.wg.Add(1)
	go tq.resultTimeoutLoop()
}

// Stop gracefully stops the task queue
func (tq *TaskQueue) Stop() {
	tq.logger.Info("Stopping task queue")
	tq.cancel()
	tq.wg.Wait()
	tq.logger.Info("Task queue stopped")
}

// SetResultStreamer sets the result streamer for async notifications
func (tq *TaskQueue) SetResultStreamer(streamer *ResultStreamer) {
	tq.resultStreamer = streamer
}

// EnqueueTask adds a task to the queue and returns immediately
func (tq *TaskQueue) EnqueueTask(
	ctx context.Context,
	sessionID string,
	toolName string,
	args storage.TaskArgs,
) (string, error) {
	// Get next sequence number for this session
	sequence := tq.sessionManager.GetNextSequence(ctx, sessionID)

	// Create queued task
	task := &storage.QueuedTask{
		ID:         generateTaskID(),
		SessionID:  sessionID,
		ToolName:   toolName,
		Args:       args,
		Sequence:   sequence,
		State:      storage.TaskStateQueued,
		RetryCount: 0,
		MaxRetries: 5,
		CreatedAt:  time.Now(),
		Timeout:    2 * time.Minute,
	}

	// Create result channel for this task
	resultChan := make(chan *TaskResult, 1)

	// Register result channel
	tq.resultMu.Lock()
	tq.resultChannels[task.ID] = resultChan
	tq.resultMu.Unlock()

	// Enqueue task
	if err := tq.storage.Enqueue(ctx, sessionID, task); err != nil {
		// Clean up result channel on error
		tq.resultMu.Lock()
		delete(tq.resultChannels, task.ID)
		tq.resultMu.Unlock()
		return "", fmt.Errorf("failed to enqueue task: %w", err)
	}

	tq.logger.InfoContext(ctx, "Task enqueued",
		"task_id", task.ID,
		"session_id", sessionID,
		"tool_name", toolName,
		"sequence", sequence,
	)

	return task.ID, nil
}

// EnqueueTypedTask adds a task with strongly-typed request to the queue
func (tq *TaskQueue) EnqueueTypedTask(
	ctx context.Context,
	sessionID string,
	request *protov1.ToolRequest,
) (string, error) {
	// Extract tool name from typed request
	toolName := getToolNameFromRequest(request)

	// Serialize the typed request
	typedBytes, err := proto.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal typed request: %w", err)
	}

	// Get next sequence number for this session
	sequence := tq.sessionManager.GetNextSequence(ctx, sessionID)

	// Create queued task with typed request
	task := &storage.QueuedTask{
		ID:           generateTaskID(),
		SessionID:    sessionID,
		ToolName:     toolName,
		Args:         nil,        // Legacy field, not used with typed requests
		TypedRequest: typedBytes, // Serialized protobuf
		Sequence:     sequence,
		State:        storage.TaskStateQueued,
		RetryCount:   0,
		MaxRetries:   5,
		CreatedAt:    time.Now(),
		Timeout:      2 * time.Minute,
	}

	// Create result channel for this task
	resultChan := make(chan *TaskResult, 1)

	// Register result channel
	tq.resultMu.Lock()
	tq.resultChannels[task.ID] = resultChan
	tq.resultMu.Unlock()

	// Enqueue task
	if err := tq.storage.Enqueue(ctx, sessionID, task); err != nil {
		// Clean up result channel on error
		tq.resultMu.Lock()
		delete(tq.resultChannels, task.ID)
		tq.resultMu.Unlock()
		return "", fmt.Errorf("failed to enqueue task: %w", err)
	}

	tq.logger.InfoContext(ctx, "Typed task enqueued",
		"task_id", task.ID,
		"session_id", sessionID,
		"tool_name", toolName,
		"sequence", sequence,
	)

	return task.ID, nil
}

// getToolNameFromRequest extracts the tool name from a ToolRequest
func getToolNameFromRequest(request *protov1.ToolRequest) string {
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

// GetTaskResult waits for task result (blocking until task completes)
func (tq *TaskQueue) GetTaskResult(ctx context.Context, taskID string) (*TaskResult, error) {
	tq.resultMu.RLock()
	resultChan, exists := tq.resultChannels[taskID]
	tq.resultMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	select {
	case result := <-resultChan:
		// Clean up result channel
		tq.resultMu.Lock()
		delete(tq.resultChannels, taskID)
		tq.resultMu.Unlock()
		return result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// GetTask retrieves task information by ID
func (tq *TaskQueue) GetTask(ctx context.Context, taskID string) (*storage.QueuedTask, error) {
	return tq.storage.GetTask(ctx, taskID)
}

// dispatchLoop continuously checks for ready tasks and dispatches them
func (tq *TaskQueue) dispatchLoop() {
	defer tq.wg.Done()

	ticker := time.NewTicker(tq.dispatchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tq.dispatchReadyTasks()
		case <-tq.ctx.Done():
			return
		}
	}
}

// dispatchReadyTasks finds ready tasks and dispatches them to workers
func (tq *TaskQueue) dispatchReadyTasks() {
	// Get all sessions (we need to check each session's queue)
	// Optimized: Could maintain a priority queue of sessions with pending tasks
	// in future to avoid scanning all sessions
	sessionsMap := tq.sessionManager.GetAllSessions()

	for _, session := range sessionsMap {
		// Skip sessions that aren't ready
		if session.State != SessionStateReady {
			continue
		}

		// Dequeue ready tasks for this session
		tasks, err := tq.storage.Dequeue(tq.ctx, session.ID, tq.maxDispatchBatch)
		if err != nil {
			tq.logger.ErrorContext(tq.ctx, "Failed to dequeue tasks",
				"session_id", session.ID,
				"error", err,
			)
			continue
		}

		if len(tasks) == 0 {
			continue
		}

		// Dispatch each task
		for _, task := range tasks {
			tq.wg.Add(1)
			go tq.dispatchTask(session, task)
		}
	}
}

// dispatchTask sends a single task to the worker
func (tq *TaskQueue) dispatchTask(session *Session, task *storage.QueuedTask) {
	defer tq.wg.Done()

	ctx := tq.ctx
	tq.logger.InfoContext(ctx, "Dispatching task to worker",
		"task_id", task.ID,
		"session_id", session.ID,
		"tool_name", task.ToolName,
		"sequence", task.Sequence,
		"retry_count", task.RetryCount,
	)

	// Update task state to dispatched
	dispatchedAt := time.Now()
	task.DispatchedAt = &dispatchedAt
	if err := tq.storage.UpdateTaskState(ctx, task.ID, storage.TaskStateDispatched); err != nil {
		tq.logger.ErrorContext(ctx, "Failed to update task state to dispatched",
			"task_id", task.ID,
			"error", err,
		)
	}

	// Deserialize typed request from task
	if len(task.TypedRequest) == 0 {
		tq.handleTaskError(ctx, task, fmt.Errorf("task has no typed request data"))
		return
	}

	var toolRequest protov1.ToolRequest
	if err := proto.Unmarshal(task.TypedRequest, &toolRequest); err != nil {
		tq.handleTaskError(ctx, task, fmt.Errorf("failed to deserialize typed request: %w", err))
		return
	}

	// Execute task via worker client with typed request
	ctxWithSession := contextWithSessionID(ctx, session.ID)
	result, err := tq.workerClient.ExecuteTypedTask(
		ctxWithSession,
		session.WorkspaceID,
		&toolRequest,
	)

	// Handle result or error
	if err != nil {
		tq.handleTaskError(ctx, task, err)
		return
	}

	tq.handleTaskSuccess(ctx, task, result)
}

// handleTaskSuccess processes successful task completion
func (tq *TaskQueue) handleTaskSuccess(ctx context.Context, task *storage.QueuedTask, result *TaskResult) {
	tq.logger.InfoContext(ctx, "Task completed successfully",
		"task_id", task.ID,
		"session_id", task.SessionID,
		"sequence", task.Sequence,
	)

	// Update task state
	completedAt := time.Now()
	task.CompletedAt = &completedAt
	if err := tq.storage.UpdateTaskState(ctx, task.ID, storage.TaskStateCompleted); err != nil {
		tq.logger.ErrorContext(ctx, "Failed to update task state to completed",
			"task_id", task.ID,
			"error", err,
		)
	}

	// Update last completed sequence
	if err := tq.sessionManager.storage.SetLastCompletedSequence(ctx, task.SessionID, task.Sequence); err != nil {
		tq.logger.ErrorContext(ctx, "Failed to update last completed sequence",
			"task_id", task.ID,
			"session_id", task.SessionID,
			"sequence", task.Sequence,
			"error", err,
		)
	}

	// Publish result to subscribers via SSE
	if tq.resultStreamer != nil {
		notification := &TaskResultNotification{
			TaskID:      task.ID,
			Status:      "completed",
			Result:      result,
			CompletedAt: completedAt,
		}

		if err := tq.resultStreamer.PublishResult(ctx, task.ID, notification); err != nil {
			tq.logger.ErrorContext(ctx, "Failed to publish result",
				"task_id", task.ID,
				"error", err,
			)
		}
	}

	// Send result to waiting client (for backward compatibility)
	tq.sendTaskResult(task.ID, result)
}

// handleTaskError processes task failure and determines retry
func (tq *TaskQueue) handleTaskError(ctx context.Context, task *storage.QueuedTask, taskErr error) {
	tq.logger.ErrorContext(ctx, "Task execution failed",
		"task_id", task.ID,
		"session_id", task.SessionID,
		"error", taskErr,
		"retry_count", task.RetryCount,
	)

	// Use dispatcher to handle retry logic
	willRetry, err := tq.dispatcher.HandleTaskFailure(ctx, task, taskErr)
	if err != nil {
		tq.logger.ErrorContext(ctx, "Failed to handle task failure",
			"task_id", task.ID,
			"error", err,
		)
		// Fall through to mark as failed
		willRetry = false
	}

	if willRetry {
		tq.logger.InfoContext(ctx, "Task scheduled for retry",
			"task_id", task.ID,
			"retry_count", task.RetryCount+1,
		)
		// Task will be retried by retry scheduler
		return
	}

	// Mark task as permanently failed
	if err := tq.storage.UpdateTaskState(ctx, task.ID, storage.TaskStateFailed); err != nil {
		tq.logger.ErrorContext(ctx, "Failed to update task state to failed",
			"task_id", task.ID,
			"error", err,
		)
	}

	// Send error result to client
	errorResult := &TaskResult{
		Success:  false,
		Output:   fmt.Sprintf("Task failed: %v", taskErr),
		ExitCode: 1,
	}

	// Publish failure notification to subscribers via SSE
	if tq.resultStreamer != nil {
		notification := &TaskResultNotification{
			TaskID:      task.ID,
			Status:      "failed",
			Error:       taskErr.Error(),
			CompletedAt: time.Now(),
		}

		if err := tq.resultStreamer.PublishResult(ctx, task.ID, notification); err != nil {
			tq.logger.ErrorContext(ctx, "Failed to publish failure notification",
				"task_id", task.ID,
				"error", err,
			)
		}
	}

	tq.sendTaskResult(task.ID, errorResult)
}

// sendTaskResult sends result to the waiting client
func (tq *TaskQueue) sendTaskResult(taskID string, result *TaskResult) {
	tq.resultMu.RLock()
	resultChan, exists := tq.resultChannels[taskID]
	tq.resultMu.RUnlock()

	if !exists {
		tq.logger.Warn("No result channel for task", "task_id", taskID)
		return
	}

	select {
	case resultChan <- result:
		// Result sent successfully
	case <-tq.ctx.Done():
		// Queue is shutting down
	default:
		tq.logger.Warn("Result channel full, dropping result", "task_id", taskID)
	}
}

// resultTimeoutLoop monitors for tasks that have been waiting too long
func (tq *TaskQueue) resultTimeoutLoop() {
	defer tq.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			tq.cleanupExpiredResults()
		case <-tq.ctx.Done():
			return
		}
	}
}

// cleanupExpiredResults removes result channels for old tasks
func (tq *TaskQueue) cleanupExpiredResults() {
	// Clean up result channels that have been waiting too long
	// This prevents memory leaks from abandoned tasks
	tq.resultMu.Lock()
	defer tq.resultMu.Unlock()

	now := time.Now()
	for taskID, ch := range tq.resultChannels {
		// Check if task exists and is old
		task, err := tq.storage.GetTask(tq.ctx, taskID)
		if err != nil || task == nil {
			// Task doesn't exist, remove channel
			close(ch)
			delete(tq.resultChannels, taskID)
			continue
		}

		// Remove channels for tasks older than timeout
		if now.Sub(task.CreatedAt) > task.Timeout {
			tq.logger.Warn("Cleaning up expired result channel",
				"task_id", taskID,
				"age", now.Sub(task.CreatedAt),
			)
			close(ch)
			delete(tq.resultChannels, taskID)
		}
	}
}

// generateTaskID generates a unique task ID
func generateTaskID() string {
	return fmt.Sprintf("task-%d", time.Now().UnixNano())
}
