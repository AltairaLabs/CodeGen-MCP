package coordinator

import (
	"context"
	"fmt"
	"sync"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

// WorkerRegistryStorage defines the storage backend interface for worker registry
// Note: This is a simplified interface compared to storage.WorkerRegistryStorage
// because the coordinator's RegisteredWorker has runtime fields like TaskStream
// that cannot be persisted. This interface focuses on metadata persistence.
type WorkerRegistryStorage interface {
	// RegisterWorker stores basic worker metadata
	RegisterWorker(ctx context.Context, workerID string, metadata map[string]interface{}) error
	// GetWorker retrieves worker metadata
	GetWorker(ctx context.Context, workerID string) (map[string]interface{}, error)
	// GetAllWorkers retrieves all worker metadata
	GetAllWorkers(ctx context.Context) (map[string]map[string]interface{}, error)
	// UnregisterWorker removes worker metadata
	UnregisterWorker(ctx context.Context, workerID string) error
	// UpdateWorkerHeartbeat updates last heartbeat timestamp
	UpdateWorkerHeartbeat(ctx context.Context, workerID string, lastHeartbeat time.Time) error
}

// WorkerRegistry manages registered workers and their capacity
type WorkerRegistry struct {
	mu      sync.RWMutex
	workers map[string]*RegisteredWorker
	storage WorkerRegistryStorage // Optional storage backend for metadata persistence
}

// PendingTask tracks a task awaiting response with its associated tool name
type PendingTask struct {
	ResponseChan chan *protov1.TaskStreamResponse
	ToolName     string
}

// RegisteredWorker represents a worker registered with the coordinator
type RegisteredWorker struct {
	WorkerID              string
	SessionID             string                                         // Worker's registration session ID
	TaskStream            protov1.WorkerLifecycle_TaskStreamServer       // Bidirectional stream for tasks
	PendingTasks          map[string]*PendingTask                        // Tasks awaiting responses with tool context
	PendingSessionCreates map[string]chan *protov1.SessionCreateResponse // Channels waiting for session create responses
	Capabilities          *protov1.WorkerCapabilities
	Limits                *protov1.ResourceLimits
	Status                *protov1.WorkerStatus
	Capacity              *protov1.SessionCapacity
	LastHeartbeat         time.Time
	RegisteredAt          time.Time
	HeartbeatInterval     time.Duration
	mu                    sync.RWMutex
}

// NewWorkerRegistry creates a new worker registry
func NewWorkerRegistry() *WorkerRegistry {
	return &WorkerRegistry{
		workers: make(map[string]*RegisteredWorker),
		storage: nil,
	}
}

// RegisterWorker adds a new worker to the registry
func (wr *WorkerRegistry) RegisterWorker(workerID string, worker *RegisteredWorker) error {
	if wr.storage != nil {
		// Store basic metadata (non-runtime fields only)
		metadata := map[string]interface{}{
			"worker_id":          workerID,
			"session_id":         worker.SessionID,
			"registered_at":      worker.RegisteredAt,
			"last_heartbeat":     worker.LastHeartbeat,
			"heartbeat_interval": worker.HeartbeatInterval,
		}
		if err := wr.storage.RegisterWorker(context.Background(), workerID, metadata); err != nil {
			return fmt.Errorf("failed to store worker metadata: %w", err)
		}
	}

	wr.mu.Lock()
	defer wr.mu.Unlock()

	if _, exists := wr.workers[workerID]; exists {
		return fmt.Errorf("worker already registered: %s", workerID)
	}

	wr.workers[workerID] = worker
	return nil
}

// GetWorker retrieves a worker by ID
func (wr *WorkerRegistry) GetWorker(workerID string) *RegisteredWorker {
	// Always return from in-memory map (contains runtime state)
	// Storage is only for persistence/recovery scenarios
	wr.mu.RLock()
	defer wr.mu.RUnlock()

	return wr.workers[workerID]
}

// UpdateHeartbeat updates worker heartbeat and capacity information
//
//nolint:lll // Protobuf types create inherently long function signatures
func (wr *WorkerRegistry) UpdateHeartbeat(workerID string, status *protov1.WorkerStatus, capacity *protov1.SessionCapacity) error {
	wr.mu.RLock()
	worker, exists := wr.workers[workerID]
	wr.mu.RUnlock()

	if !exists {
		return fmt.Errorf("worker not found: %s", workerID)
	}

	worker.mu.Lock()
	worker.Status = status
	worker.Capacity = capacity
	now := time.Now()
	worker.LastHeartbeat = now
	worker.mu.Unlock()

	// Update storage backend if available
	if wr.storage != nil {
		if err := wr.storage.UpdateWorkerHeartbeat(context.Background(), workerID, now); err != nil {
			// Storage is for persistence/recovery - failure is non-critical
			// In-memory update succeeded, that's what matters for runtime
			// Log error but don't fail the operation
			_ = err
		}
	}

	return nil
}

// FindWorkerWithCapacity finds a worker with available session capacity
//
//nolint:lll // Protobuf types create inherently long function signatures
func (wr *WorkerRegistry) FindWorkerWithCapacity(ctx context.Context, config *protov1.SessionConfig) (*RegisteredWorker, error) {
	wr.mu.RLock()
	defer wr.mu.RUnlock()

	var bestWorker *RegisteredWorker
	maxAvailable := int32(0)

	for _, worker := range wr.workers {
		worker.mu.RLock()

		if !wr.isWorkerHealthy(worker) {
			worker.mu.RUnlock()
			continue
		}

		availableSessions := wr.getAvailableSessionCount(worker)
		if availableSessions > maxAvailable {
			maxAvailable = availableSessions
			bestWorker = worker
		}

		worker.mu.RUnlock()
	}

	if bestWorker == nil {
		return nil, fmt.Errorf("no workers with available capacity")
	}

	return bestWorker, nil
}

// isWorkerHealthy checks if a worker is in a healthy state and has recent heartbeat
func (wr *WorkerRegistry) isWorkerHealthy(worker *RegisteredWorker) bool {
	// Check if worker status is initialized and healthy (if status exists)
	if worker.Status != nil {
		if worker.Status.State != protov1.WorkerStatus_STATE_IDLE &&
			worker.Status.State != protov1.WorkerStatus_STATE_BUSY {
			return false
		}
	}

	// Check if heartbeat is recent (within 2x interval)
	if time.Since(worker.LastHeartbeat) > worker.HeartbeatInterval*2 {
		return false
	}

	return true
}

// getAvailableSessionCount returns the number of available sessions for a worker
func (wr *WorkerRegistry) getAvailableSessionCount(worker *RegisteredWorker) int32 {
	// Use runtime Capacity if available, otherwise fall back to Capabilities
	if worker.Capacity != nil {
		return worker.Capacity.AvailableSessions
	}
	if worker.Capabilities != nil {
		// Worker just registered, hasn't sent heartbeat yet - use max capacity
		return worker.Capabilities.MaxSessions
	}
	return 0
}

// DeregisterWorker removes a worker from the registry
func (wr *WorkerRegistry) DeregisterWorker(workerID string) error {
	if wr.storage != nil {
		// Remove from storage backend
		if err := wr.storage.UnregisterWorker(context.Background(), workerID); err != nil {
			// Storage is for persistence/recovery - failure is non-critical
			// We'll still remove from memory
			// Log error but don't fail the operation
			_ = err
		}
	}

	wr.mu.Lock()
	defer wr.mu.Unlock()

	if _, exists := wr.workers[workerID]; !exists {
		return fmt.Errorf("worker not found: %s", workerID)
	}

	delete(wr.workers, workerID)
	return nil
}

// ListWorkers returns all registered workers
func (wr *WorkerRegistry) ListWorkers() []*RegisteredWorker {
	wr.mu.RLock()
	defer wr.mu.RUnlock()

	workers := make([]*RegisteredWorker, 0, len(wr.workers))
	for _, worker := range wr.workers {
		workers = append(workers, worker)
	}
	return workers
}

// GetTotalCapacity returns the total session capacity across all workers
func (wr *WorkerRegistry) GetTotalCapacity() (total, active, available int32) {
	wr.mu.RLock()
	defer wr.mu.RUnlock()

	for _, worker := range wr.workers {
		worker.mu.RLock()
		if worker.Capacity != nil {
			total += worker.Capacity.TotalSessions
			active += worker.Capacity.ActiveSessions
			available += worker.Capacity.AvailableSessions
		}
		worker.mu.RUnlock()
	}

	return total, active, available
}

// CleanupStaleWorkers removes workers that haven't sent heartbeats
func (wr *WorkerRegistry) CleanupStaleWorkers(timeout time.Duration) int {
	wr.mu.Lock()
	defer wr.mu.Unlock()

	removed := 0
	now := time.Now()

	for workerID, worker := range wr.workers {
		worker.mu.RLock()
		lastHeartbeat := worker.LastHeartbeat
		worker.mu.RUnlock()

		if now.Sub(lastHeartbeat) > timeout {
			// Remove from storage backend if available
			if wr.storage != nil {
				_ = wr.storage.UnregisterWorker(context.Background(), workerID)
			}
			delete(wr.workers, workerID)
			removed++
		}
	}

	return removed
}

// GetAllWorkers returns a map of all registered workers
// This method is for external access to avoid direct field access
func (wr *WorkerRegistry) GetAllWorkers() map[string]*RegisteredWorker {
	wr.mu.RLock()
	defer wr.mu.RUnlock()

	// Create a copy to avoid external modifications
	result := make(map[string]*RegisteredWorker, len(wr.workers))
	for id, worker := range wr.workers {
		result[id] = worker
	}
	return result
}
