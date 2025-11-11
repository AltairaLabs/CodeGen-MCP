package taskqueue

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/config"
	"github.com/AltairaLabs/codegen-mcp/internal/storage"
	"google.golang.org/protobuf/proto"
)

// TaskQueueInterface defines the interface for task queue operations
type TaskQueueInterface interface {
	// EnqueueTask adds a new task to the queue for a session
	// Deprecated: Use EnqueueTypedTask for type-safe requests
	EnqueueTask(ctx context.Context, sessionID, toolName string, args TaskArgs) (string, error)

	// EnqueueTypedTask adds a new task with strongly-typed request
	EnqueueTypedTask(ctx context.Context, sessionID string, request *protov1.ToolRequest) (string, error)

	// GetTaskResult waits for and returns the result of a task
	GetTaskResult(ctx context.Context, taskID string) (*TaskResult, error)

	// GetTask retrieves task information by ID
	GetTask(ctx context.Context, taskID string) (*storage.QueuedTask, error)

	// SetResultStreamer sets the result streamer for async notifications
	SetResultStreamer(streamer ResultStreamer)

	// Start begins background task dispatch workers
	Start()

	// Stop gracefully shuts down the task queue
	Stop()
}

// TaskQueue manages asynchronous task queuing and dispatch
type TaskQueue struct {
	storage        storage.TaskQueueStorage
	sessionManager SessionManager
	workerClient   WorkerClient
	dispatcher     *TaskDispatcher
	logger         *slog.Logger
	resultStreamer ResultStreamer

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

// TaskQueueConfig is an alias to the config package type for backward compatibility
type TaskQueueConfig = config.TaskQueueConfig

// DefaultTaskQueueConfig returns default configuration
func DefaultTaskQueueConfig() TaskQueueConfig {
	return config.DefaultTaskQueueConfig()
}

// NewTaskQueue creates a new task queue manager
func NewTaskQueue(
	storage storage.TaskQueueStorage,
	sessionMgr SessionManager,
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
func (tq *TaskQueue) SetResultStreamer(streamer ResultStreamer) {
	tq.resultStreamer = streamer
}

// EnqueueTask adds a task to the queue and returns immediately
func (tq *TaskQueue) EnqueueTask(
	ctx context.Context,
	sessionID string,
	toolName string,
	args TaskArgs,
) (string, error) {
	// Get next sequence number for this session
	sequence := tq.sessionManager.GetNextSequence(ctx, sessionID)

	// Create queued task
	task := &storage.QueuedTask{
		ID:         generateTaskID(),
		SessionID:  sessionID,
		ToolName:   toolName,
		Args:       storage.TaskArgs(args),
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
// Note: dispatchLoop has been moved to task_queue_loops.go (untestable infrastructure code)

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

// Note: The following functions have been moved to task_queue_dispatch.go (untestable infrastructure):
// - dispatchTask
// - handleTaskSuccess
// - handleTaskError
// - sendTaskResult
// - cleanupExpiredResults

// generateTaskID generates a unique task ID
func generateTaskID() string {
	return fmt.Sprintf("task-%d", time.Now().UnixNano())
}
