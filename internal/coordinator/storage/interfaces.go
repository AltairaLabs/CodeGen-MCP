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
}

// SessionStateStorage defines the interface for session metadata storage
type SessionStateStorage interface {
	// CreateSession creates a new session with initial state
	// Returns error if session already exists or validation fails
	CreateSession(ctx context.Context, session *coordinator.Session) error

	// GetSession retrieves session by ID
	// Returns nil, nil if session not found
	GetSession(ctx context.Context, sessionID string) (*coordinator.Session, error)

	// UpdateSessionState updates the state of a session
	// Returns error if session not found
	UpdateSessionState(ctx context.Context, sessionID string, state coordinator.SessionState, message string) error

	// SetSessionMetadata sets arbitrary metadata for a session
	// Overwrites existing metadata with same keys
	// Returns error if session not found
	SetSessionMetadata(ctx context.Context, sessionID string, metadata map[string]string) error

	// GetSessionMetadata retrieves metadata for a session
	// Returns empty map if session has no metadata or session not found
	GetSessionMetadata(ctx context.Context, sessionID string) (map[string]string, error)

	// GetSessionByConversationID looks up session by external conversation ID
	// Returns nil, nil if no session found for this conversation ID
	// Future use: map external conversation IDs to internal session IDs
	GetSessionByConversationID(ctx context.Context, conversationID string) (*coordinator.Session, error)

	// DeleteSession removes a session and its metadata
	// Returns error if session not found (idempotent - no error if already deleted)
	DeleteSession(ctx context.Context, sessionID string) error

	// GetLastCompletedSequence gets the last successfully completed task sequence number
	// Returns 0 if no tasks have completed yet
	GetLastCompletedSequence(ctx context.Context, sessionID string) (uint64, error)

	// SetLastCompletedSequence updates the last completed sequence number
	// Used for deduplication - worker checks this before executing tasks
	// Returns error if session not found
	SetLastCompletedSequence(ctx context.Context, sessionID string, sequence uint64) error

	// GetNextSequence atomically increments and returns the next sequence number for a session
	// Used when enqueuing new tasks to assign monotonic sequence numbers
	// Returns error if session not found
	GetNextSequence(ctx context.Context, sessionID string) (uint64, error)

	// ListSessions retrieves all active sessions
	// Useful for debugging and cleanup operations
	ListSessions(ctx context.Context) ([]*coordinator.Session, error)

	// UpdateSessionActivity updates the LastActive timestamp for a session
	// Used to track session activity for cleanup of stale sessions
	UpdateSessionActivity(ctx context.Context, sessionID string) error
}

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
