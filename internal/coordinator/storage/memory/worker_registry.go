package memory

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/storage"
)

var (
	errWorkerNil          = errors.New("worker cannot be nil")
	errWorkerIDEmpty      = errors.New("worker ID cannot be empty")
	errWorkerNotFound     = errors.New("worker not found")
	errCoordinatorIDEmpty = errors.New("coordinator ID cannot be empty")
)

// InMemoryWorkerRegistryStorage implements WorkerRegistryStorage using in-memory maps
type InMemoryWorkerRegistryStorage struct {
	mu                sync.RWMutex
	workers           map[string]*storage.RegisteredWorker
	lastCapacityIndex int // For round-robin worker selection
}

// NewInMemoryWorkerRegistryStorage creates a new in-memory worker registry storage
func NewInMemoryWorkerRegistryStorage() *InMemoryWorkerRegistryStorage {
	return &InMemoryWorkerRegistryStorage{
		workers:           make(map[string]*storage.RegisteredWorker),
		lastCapacityIndex: 0,
	}
}

// RegisterWorker adds or updates a worker registration
func (s *InMemoryWorkerRegistryStorage) RegisterWorker(
	ctx context.Context,
	worker *storage.RegisteredWorker,
) error {
	if worker == nil {
		return errWorkerNil
	}
	if worker.WorkerID == "" {
		return errWorkerIDEmpty
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Store a copy to prevent external modifications
	workerCopy := *worker
	if worker.Metadata != nil {
		workerCopy.Metadata = make(map[string]string, len(worker.Metadata))
		for k, v := range worker.Metadata {
			workerCopy.Metadata[k] = v
		}
	}

	// Update LastHeartbeat if not set
	if workerCopy.LastHeartbeat.IsZero() {
		workerCopy.LastHeartbeat = time.Now()
	}
	// Update RegisteredAt if this is a new worker
	if _, exists := s.workers[worker.WorkerID]; !exists {
		if workerCopy.RegisteredAt.IsZero() {
			workerCopy.RegisteredAt = time.Now()
		}
	} else {
		// Preserve original RegisteredAt for existing workers
		workerCopy.RegisteredAt = s.workers[worker.WorkerID].RegisteredAt
	}

	s.workers[worker.WorkerID] = &workerCopy

	return nil
}

// GetWorker retrieves a worker by ID
func (s *InMemoryWorkerRegistryStorage) GetWorker(
	ctx context.Context,
	workerID string,
) (*storage.RegisteredWorker, error) {
	if workerID == "" {
		return nil, errWorkerIDEmpty
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	worker, exists := s.workers[workerID]
	if !exists {
		return nil, nil
	}

	// Return a copy
	workerCopy := *worker
	if worker.Metadata != nil {
		workerCopy.Metadata = make(map[string]string, len(worker.Metadata))
		for k, v := range worker.Metadata {
			workerCopy.Metadata[k] = v
		}
	}

	return &workerCopy, nil
}

// GetAllWorkers retrieves all registered workers
func (s *InMemoryWorkerRegistryStorage) GetAllWorkers(
	ctx context.Context,
) ([]*storage.RegisteredWorker, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*storage.RegisteredWorker, 0, len(s.workers))
	for _, worker := range s.workers {
		// Return copies
		workerCopy := *worker
		if worker.Metadata != nil {
			workerCopy.Metadata = make(map[string]string, len(worker.Metadata))
			for k, v := range worker.Metadata {
				workerCopy.Metadata[k] = v
			}
		}
		result = append(result, &workerCopy)
	}

	return result, nil
}

// UpdateWorkerHeartbeat updates last seen timestamp
func (s *InMemoryWorkerRegistryStorage) UpdateWorkerHeartbeat(
	ctx context.Context,
	workerID string,
) error {
	if workerID == "" {
		return errWorkerIDEmpty
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	worker, exists := s.workers[workerID]
	if !exists {
		return errWorkerNotFound
	}

	worker.LastHeartbeat = time.Now()

	return nil
}

// UnregisterWorker removes a worker from the registry
func (s *InMemoryWorkerRegistryStorage) UnregisterWorker(
	ctx context.Context,
	workerID string,
) error {
	if workerID == "" {
		return errWorkerIDEmpty
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Idempotent - no error if worker doesn't exist
	delete(s.workers, workerID)

	return nil
}

// FindWorkerWithCapacity finds a worker with available session capacity
// Uses round-robin selection among workers with capacity
func (s *InMemoryWorkerRegistryStorage) FindWorkerWithCapacity(
	ctx context.Context,
) (*storage.RegisteredWorker, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.workers) == 0 {
		return nil, nil
	}

	// Build list of workers with capacity
	availableWorkers := make([]*storage.RegisteredWorker, 0, len(s.workers))
	for _, worker := range s.workers {
		if worker.ActiveSessions < worker.MaxSessions {
			availableWorkers = append(availableWorkers, worker)
		}
	}

	if len(availableWorkers) == 0 {
		return nil, nil
	}

	// Round-robin selection
	s.lastCapacityIndex = (s.lastCapacityIndex + 1) % len(availableWorkers)
	selectedWorker := availableWorkers[s.lastCapacityIndex]

	// Return a copy
	workerCopy := *selectedWorker
	if selectedWorker.Metadata != nil {
		workerCopy.Metadata = make(map[string]string, len(selectedWorker.Metadata))
		for k, v := range selectedWorker.Metadata {
			workerCopy.Metadata[k] = v
		}
	}

	return &workerCopy, nil
}

// GetWorkersByCoordinator gets workers connected to specific coordinator
func (s *InMemoryWorkerRegistryStorage) GetWorkersByCoordinator(
	ctx context.Context,
	coordinatorID string,
) ([]*storage.RegisteredWorker, error) {
	if coordinatorID == "" {
		return nil, errCoordinatorIDEmpty
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*storage.RegisteredWorker, 0)
	for _, worker := range s.workers {
		if worker.CoordinatorID == coordinatorID {
			// Return copies
			workerCopy := *worker
			if worker.Metadata != nil {
				workerCopy.Metadata = make(map[string]string, len(worker.Metadata))
				for k, v := range worker.Metadata {
					workerCopy.Metadata[k] = v
				}
			}
			result = append(result, &workerCopy)
		}
	}

	return result, nil
}

// UpdateWorkerSessions updates the active session count for a worker
func (s *InMemoryWorkerRegistryStorage) UpdateWorkerSessions(
	ctx context.Context,
	workerID string,
	activeSessions int,
) error {
	if workerID == "" {
		return errWorkerIDEmpty
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	worker, exists := s.workers[workerID]
	if !exists {
		return errWorkerNotFound
	}

	worker.ActiveSessions = activeSessions

	return nil
}

// ListStaleWorkers returns workers that haven't sent heartbeat within timeout
func (s *InMemoryWorkerRegistryStorage) ListStaleWorkers(
	ctx context.Context,
	timeout time.Duration,
) ([]*storage.RegisteredWorker, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cutoff := time.Now().Add(-timeout)
	result := make([]*storage.RegisteredWorker, 0)

	for _, worker := range s.workers {
		if worker.LastHeartbeat.Before(cutoff) {
			// Return copies
			workerCopy := *worker
			if worker.Metadata != nil {
				workerCopy.Metadata = make(map[string]string, len(worker.Metadata))
				for k, v := range worker.Metadata {
					workerCopy.Metadata[k] = v
				}
			}
			result = append(result, &workerCopy)
		}
	}

	return result, nil
}
