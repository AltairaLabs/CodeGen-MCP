package storage

import (
	"context"
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator"
)

// TaskState represents the lifecycle state of a queued task
type TaskState string

const (
	// TaskStateQueued indicates the task is waiting for session to be ready
	TaskStateQueued TaskState = "queued"
	// TaskStateDispatched indicates the task has been sent to the worker
	TaskStateDispatched TaskState = "dispatched"
	// TaskStateCompleted indicates the task completed successfully
	TaskStateCompleted TaskState = "completed"
	// TaskStateFailed indicates the task failed permanently
	TaskStateFailed TaskState = "failed"
	// TaskStateTimeout indicates the task timed out waiting for session
	TaskStateTimeout TaskState = "timeout"
	// TaskStateRetrying indicates the task is waiting for retry
	TaskStateRetrying TaskState = "retrying"
)

// QueuedTask represents a task in the queue with retry and sequencing support
type QueuedTask struct {
	ID             string               // Unique task ID
	SessionID      string               // Target session ID
	ConversationID string               // External conversation ID (future use)
	ToolName       string               // Tool to execute
	Args           coordinator.TaskArgs // Tool arguments
	Sequence       uint64               // Monotonic sequence number per session
	State          TaskState            // Current state
	RetryCount     int                  // Number of retry attempts
	MaxRetries     int                  // Maximum retry attempts
	CreatedAt      time.Time            // When task was created
	DispatchedAt   *time.Time           // When task was sent to worker (nil if not dispatched)
	CompletedAt    *time.Time           // When task completed (nil if not completed)
	Timeout        time.Duration        // Max time to wait in queue
	Error          string               // Last error message
	NextRetryAt    *time.Time           // When to retry next (nil if not retrying)
}

// QueueStats provides statistics about the task queue
type QueueStats struct {
	TotalQueued     int            // Total tasks currently queued
	TotalDispatched int            // Total tasks currently dispatched
	QueuedBySession map[string]int // Tasks queued per session
	OldestTaskAge   time.Duration  // Age of oldest queued task
}

// TaskQueueStorage defines the interface for pluggable task queue backends
type TaskQueueStorage interface {
	// Enqueue adds a task to the queue for a session
	// Returns error if queue is full or task already exists
	Enqueue(ctx context.Context, sessionID string, task *QueuedTask) error

	// Dequeue retrieves tasks ready for dispatch (session is ready)
	// limit parameter controls maximum number of tasks to return (0 = no limit)
	// Returns empty slice if no tasks are ready
	Dequeue(ctx context.Context, sessionID string, limit int) ([]*QueuedTask, error)

	// UpdateTaskState updates the state of a task
	// Returns error if task not found
	UpdateTaskState(ctx context.Context, taskID string, state TaskState) error

	// GetTask retrieves a specific task by ID
	// Returns nil, nil if task not found
	GetTask(ctx context.Context, taskID string) (*QueuedTask, error)

	// PurgeSessionQueue removes all tasks for a session
	// Useful when session creation fails or session is terminated
	// Returns number of tasks purged
	PurgeSessionQueue(ctx context.Context, sessionID string) (int, error)

	// GetQueueStats returns statistics about queued tasks
	// Useful for monitoring and capacity planning
	GetQueueStats(ctx context.Context) (*QueueStats, error)

	// GetQueuedTasksForSession retrieves all queued tasks for a session
	// Useful for debugging and observability
	GetQueuedTasksForSession(ctx context.Context, sessionID string) ([]*QueuedTask, error)

	// GetTasksReadyForRetry retrieves tasks that are ready to be retried
	// Returns tasks where NextRetryAt <= now and State == TaskStateRetrying
	// limit parameter controls maximum number of tasks to return (0 = no limit)
	GetTasksReadyForRetry(ctx context.Context, limit int) ([]*QueuedTask, error)

	// UpdateTaskForRetry updates a task to prepare it for retry
	// Increments RetryCount, sets NextRetryAt, sets State to TaskStateRetrying
	// Returns error if task not found or max retries exceeded
	UpdateTaskForRetry(ctx context.Context, taskID string, nextRetryAt time.Time, errorMsg string) error

	// RequeueTaskForRetry moves a task from retrying state back to queued
	// Used by retry scheduler when retry time arrives
	// Returns error if task not found or not in retrying state
	RequeueTaskForRetry(ctx context.Context, taskID string) error
}

// NOTE: SessionStateStorage interface is defined in coordinator/session.go
// to avoid import cycles. Storage implementations directly implement the
// coordinator.SessionStateStorage interface, not a separate storage interface.
// See coordinator.SessionStateStorage for the interface definition.

// RegisteredWorker represents a worker registered with the coordinator
// This is a copy of the coordinator.RegisteredWorker type for storage purposes
type RegisteredWorker struct {
	WorkerID       string
	Hostname       string
	GRPCPort       int
	MaxSessions    int
	ActiveSessions int
	RegisteredAt   time.Time
	LastHeartbeat  time.Time
	CoordinatorID  string            // Which coordinator this worker is connected to
	Metadata       map[string]string // Arbitrary worker metadata
}

// WorkerRegistryStorage defines the interface for worker registry backends
type WorkerRegistryStorage interface {
	// RegisterWorker adds or updates a worker registration
	// Updates LastHeartbeat, ActiveSessions if worker already exists
	RegisterWorker(ctx context.Context, worker *RegisteredWorker) error

	// GetWorker retrieves a worker by ID
	// Returns nil, nil if worker not found
	GetWorker(ctx context.Context, workerID string) (*RegisteredWorker, error)

	// GetAllWorkers retrieves all registered workers
	// Returns empty slice if no workers registered
	GetAllWorkers(ctx context.Context) ([]*RegisteredWorker, error)

	// UpdateWorkerHeartbeat updates last seen timestamp
	// Used to detect stale/disconnected workers
	// Returns error if worker not found
	UpdateWorkerHeartbeat(ctx context.Context, workerID string) error

	// UnregisterWorker removes a worker from the registry
	// Returns error if worker not found (idempotent - no error if already removed)
	UnregisterWorker(ctx context.Context, workerID string) error

	// FindWorkerWithCapacity finds a worker with available session capacity
	// Returns nil, nil if no worker has capacity
	// Selection strategy is implementation-defined (round-robin, least-loaded, etc.)
	FindWorkerWithCapacity(ctx context.Context) (*RegisteredWorker, error)

	// GetWorkersByCoordinator gets workers connected to specific coordinator
	// Useful in multi-coordinator setups for failover and load balancing
	// Returns empty slice if no workers for this coordinator
	GetWorkersByCoordinator(ctx context.Context, coordinatorID string) ([]*RegisteredWorker, error)

	// UpdateWorkerSessions updates the active session count for a worker
	// Used when sessions are created or terminated
	// Returns error if worker not found
	UpdateWorkerSessions(ctx context.Context, workerID string, activeSessions int) error

	// ListStaleWorkers returns workers that haven't sent heartbeat within timeout
	// Useful for cleanup operations
	ListStaleWorkers(ctx context.Context, timeout time.Duration) ([]*RegisteredWorker, error)
}
