package coordinator

import (
	"context"
	"fmt"
	"sync"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

// WorkerRegistry manages registered workers and their capacity
type WorkerRegistry struct {
	mu      sync.RWMutex
	workers map[string]*RegisteredWorker
}

// RegisteredWorker represents a worker registered with the coordinator
type RegisteredWorker struct {
	WorkerID          string
	SessionID         string // Worker's registration session ID
	Client            WorkerServiceClient
	Capabilities      *protov1.WorkerCapabilities
	Limits            *protov1.ResourceLimits
	Status            *protov1.WorkerStatus
	Capacity          *protov1.SessionCapacity
	LastHeartbeat     time.Time
	RegisteredAt      time.Time
	HeartbeatInterval time.Duration
	mu                sync.RWMutex
}

// WorkerServiceClient combines all worker gRPC service clients
type WorkerServiceClient struct {
	SessionMgmt protov1.SessionManagementClient
	TaskExec    protov1.TaskExecutionClient
	Artifacts   protov1.ArtifactServiceClient
	// conn is reserved for future connection lifecycle management
}

// NewWorkerRegistry creates a new worker registry
func NewWorkerRegistry() *WorkerRegistry {
	return &WorkerRegistry{
		workers: make(map[string]*RegisteredWorker),
	}
}

// RegisterWorker adds a new worker to the registry
func (wr *WorkerRegistry) RegisterWorker(workerID string, worker *RegisteredWorker) error {
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
	defer worker.mu.Unlock()

	worker.Status = status
	worker.Capacity = capacity
	worker.LastHeartbeat = time.Now()

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

		// Check if worker is healthy
		if worker.Status.State != protov1.WorkerStatus_STATE_IDLE &&
			worker.Status.State != protov1.WorkerStatus_STATE_BUSY {
			worker.mu.RUnlock()
			continue
		}

		// Check if heartbeat is recent (within 2x interval)
		if time.Since(worker.LastHeartbeat) > worker.HeartbeatInterval*2 {
			worker.mu.RUnlock()
			continue
		}

		// Check capacity
		if worker.Capacity != nil && worker.Capacity.AvailableSessions > 0 {
			if worker.Capacity.AvailableSessions > maxAvailable {
				maxAvailable = worker.Capacity.AvailableSessions
				bestWorker = worker
			}
		}

		worker.mu.RUnlock()
	}

	if bestWorker == nil {
		return nil, fmt.Errorf("no workers with available capacity")
	}

	return bestWorker, nil
}

// DeregisterWorker removes a worker from the registry
func (wr *WorkerRegistry) DeregisterWorker(workerID string) error {
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
			delete(wr.workers, workerID)
			removed++
		}
	}

	return removed
}
