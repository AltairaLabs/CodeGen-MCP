package memory

import (
	"context"
	"testing"
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/storage"
)

func TestNewInMemoryWorkerRegistryStorage(t *testing.T) {
	s := NewInMemoryWorkerRegistryStorage()
	if s == nil {
		t.Fatal("expected non-nil storage")
	}
	if s.workers == nil {
		t.Error("workers map should be initialized")
	}
	if s.lastCapacityIndex != 0 {
		t.Error("lastCapacityIndex should be initialized to 0")
	}
}

func TestRegisterWorker(t *testing.T) {
	tests := []struct {
		name    string
		worker  *storage.RegisteredWorker
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil worker",
			worker:  nil,
			wantErr: true,
			errMsg:  "worker cannot be nil",
		},
		{
			name: "empty worker ID",
			worker: &storage.RegisteredWorker{
				WorkerID: "",
			},
			wantErr: true,
			errMsg:  "worker ID cannot be empty",
		},
		{
			name: "valid worker",
			worker: &storage.RegisteredWorker{
				WorkerID:       "worker1",
				Hostname:       "localhost",
				GRPCPort:       50051,
				MaxSessions:    10,
				ActiveSessions: 0,
				CoordinatorID:  "coord1",
			},
			wantErr: false,
		},
		{
			name: "worker with metadata",
			worker: &storage.RegisteredWorker{
				WorkerID:    "worker2",
				Hostname:    "worker2.local",
				GRPCPort:    50052,
				MaxSessions: 5,
				Metadata:    map[string]string{"region": "us-west"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewInMemoryWorkerRegistryStorage()
			err := s.RegisterWorker(context.Background(), tt.worker)

			if (err != nil) != tt.wantErr {
				t.Errorf("RegisterWorker() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err.Error() != tt.errMsg {
				t.Errorf("RegisterWorker() error message = %v, want %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestRegisterWorkerUpdate(t *testing.T) {
	s := NewInMemoryWorkerRegistryStorage()

	worker := &storage.RegisteredWorker{
		WorkerID:       "worker1",
		Hostname:       "localhost",
		GRPCPort:       50051,
		MaxSessions:    10,
		ActiveSessions: 2,
	}

	// First registration
	err := s.RegisterWorker(context.Background(), worker)
	if err != nil {
		t.Fatalf("RegisterWorker failed: %v", err)
	}

	// Get registered worker to check RegisteredAt
	retrieved, _ := s.GetWorker(context.Background(), "worker1")
	originalRegisteredAt := retrieved.RegisteredAt

	time.Sleep(10 * time.Millisecond)

	// Update worker
	worker.ActiveSessions = 5
	err = s.RegisterWorker(context.Background(), worker)
	if err != nil {
		t.Fatalf("RegisterWorker update failed: %v", err)
	}

	// Verify update
	retrieved, err = s.GetWorker(context.Background(), "worker1")
	if err != nil {
		t.Fatalf("GetWorker failed: %v", err)
	}
	if retrieved.ActiveSessions != 5 {
		t.Errorf("ActiveSessions = %d, want 5", retrieved.ActiveSessions)
	}
	// RegisteredAt should be preserved
	if !retrieved.RegisteredAt.Equal(originalRegisteredAt) {
		t.Error("RegisteredAt should be preserved on update")
	}
}

func TestGetWorker(t *testing.T) {
	s := NewInMemoryWorkerRegistryStorage()

	worker := &storage.RegisteredWorker{
		WorkerID:       "worker1",
		Hostname:       "localhost",
		GRPCPort:       50051,
		MaxSessions:    10,
		ActiveSessions: 3,
		Metadata:       map[string]string{"key": "value"},
	}

	err := s.RegisterWorker(context.Background(), worker)
	if err != nil {
		t.Fatalf("RegisterWorker failed: %v", err)
	}

	// Get existing worker
	retrieved, err := s.GetWorker(context.Background(), "worker1")
	if err != nil {
		t.Fatalf("GetWorker failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Expected worker, got nil")
	}
	if retrieved.WorkerID != worker.WorkerID {
		t.Errorf("WorkerID = %v, want %v", retrieved.WorkerID, worker.WorkerID)
	}
	if retrieved.ActiveSessions != 3 {
		t.Errorf("ActiveSessions = %d, want 3", retrieved.ActiveSessions)
	}

	// Verify we got a copy
	retrieved.Metadata["key2"] = "value2"
	retrieved2, _ := s.GetWorker(context.Background(), "worker1")
	if len(retrieved2.Metadata) != 1 {
		t.Error("Metadata was modified in storage (not a copy)")
	}
}

func TestGetWorkerNotFound(t *testing.T) {
	s := NewInMemoryWorkerRegistryStorage()
	worker, err := s.GetWorker(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("GetWorker failed: %v", err)
	}
	if worker != nil {
		t.Errorf("Expected nil worker, got %v", worker)
	}
}

func TestGetAllWorkers(t *testing.T) {
	s := NewInMemoryWorkerRegistryStorage()

	// Empty list
	workers, err := s.GetAllWorkers(context.Background())
	if err != nil {
		t.Fatalf("GetAllWorkers failed: %v", err)
	}
	if len(workers) != 0 {
		t.Errorf("Expected 0 workers, got %d", len(workers))
	}

	// Register multiple workers
	for i := 1; i <= 3; i++ {
		worker := &storage.RegisteredWorker{
			WorkerID:    string(rune('0' + i)),
			Hostname:    "localhost",
			GRPCPort:    50050 + i,
			MaxSessions: 10,
		}
		if err := s.RegisterWorker(context.Background(), worker); err != nil {
			t.Fatalf("RegisterWorker failed: %v", err)
		}
	}

	// Get all workers
	workers, err = s.GetAllWorkers(context.Background())
	if err != nil {
		t.Fatalf("GetAllWorkers failed: %v", err)
	}
	if len(workers) != 3 {
		t.Errorf("Expected 3 workers, got %d", len(workers))
	}
}

func TestUpdateWorkerHeartbeat(t *testing.T) {
	s := NewInMemoryWorkerRegistryStorage()

	initialTime := time.Now().Add(-1 * time.Hour)
	worker := &storage.RegisteredWorker{
		WorkerID:      "worker1",
		Hostname:      "localhost",
		GRPCPort:      50051,
		MaxSessions:   10,
		LastHeartbeat: initialTime,
	}

	err := s.RegisterWorker(context.Background(), worker)
	if err != nil {
		t.Fatalf("RegisterWorker failed: %v", err)
	}

	// Update heartbeat
	time.Sleep(10 * time.Millisecond)
	err = s.UpdateWorkerHeartbeat(context.Background(), "worker1")
	if err != nil {
		t.Fatalf("UpdateWorkerHeartbeat failed: %v", err)
	}

	// Verify LastHeartbeat was updated
	retrieved, err := s.GetWorker(context.Background(), "worker1")
	if err != nil {
		t.Fatalf("GetWorker failed: %v", err)
	}
	if retrieved.LastHeartbeat.Before(initialTime) {
		t.Error("LastHeartbeat should be updated to a more recent time")
	}
	if retrieved.LastHeartbeat.Equal(initialTime) {
		t.Error("LastHeartbeat should be different from initial time")
	}
}

func TestUpdateWorkerHeartbeatNotFound(t *testing.T) {
	s := NewInMemoryWorkerRegistryStorage()
	err := s.UpdateWorkerHeartbeat(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent worker, got nil")
	}
}

func TestUnregisterWorker(t *testing.T) {
	s := NewInMemoryWorkerRegistryStorage()

	worker := &storage.RegisteredWorker{
		WorkerID:    "worker1",
		Hostname:    "localhost",
		GRPCPort:    50051,
		MaxSessions: 10,
	}

	err := s.RegisterWorker(context.Background(), worker)
	if err != nil {
		t.Fatalf("RegisterWorker failed: %v", err)
	}

	// Unregister worker
	err = s.UnregisterWorker(context.Background(), "worker1")
	if err != nil {
		t.Fatalf("UnregisterWorker failed: %v", err)
	}

	// Verify worker is gone
	retrieved, err := s.GetWorker(context.Background(), "worker1")
	if err != nil {
		t.Fatalf("GetWorker failed: %v", err)
	}
	if retrieved != nil {
		t.Error("Worker should be unregistered")
	}

	// Unregister again should be idempotent
	err = s.UnregisterWorker(context.Background(), "worker1")
	if err != nil {
		t.Fatalf("UnregisterWorker should be idempotent: %v", err)
	}
}

func TestFindWorkerWithCapacity(t *testing.T) {
	s := NewInMemoryWorkerRegistryStorage()

	// No workers
	worker, err := s.FindWorkerWithCapacity(context.Background())
	if err != nil {
		t.Fatalf("FindWorkerWithCapacity failed: %v", err)
	}
	if worker != nil {
		t.Error("Expected nil worker when no workers registered")
	}

	// Register workers with varying capacity
	workers := []*storage.RegisteredWorker{
		{
			WorkerID:       "worker1",
			Hostname:       "host1",
			GRPCPort:       50051,
			MaxSessions:    10,
			ActiveSessions: 10, // Full
		},
		{
			WorkerID:       "worker2",
			Hostname:       "host2",
			GRPCPort:       50052,
			MaxSessions:    10,
			ActiveSessions: 5, // Has capacity
		},
		{
			WorkerID:       "worker3",
			Hostname:       "host3",
			GRPCPort:       50053,
			MaxSessions:    10,
			ActiveSessions: 3, // Has capacity
		},
	}

	for _, w := range workers {
		if err := s.RegisterWorker(context.Background(), w); err != nil {
			t.Fatalf("RegisterWorker failed: %v", err)
		}
	}

	// Find worker with capacity (should use round-robin)
	found, err := s.FindWorkerWithCapacity(context.Background())
	if err != nil {
		t.Fatalf("FindWorkerWithCapacity failed: %v", err)
	}
	if found == nil {
		t.Fatal("Expected to find worker with capacity")
	}
	if found.ActiveSessions >= found.MaxSessions {
		t.Error("Found worker should have capacity")
	}

	// Find again - should get different worker due to round-robin
	found2, err := s.FindWorkerWithCapacity(context.Background())
	if err != nil {
		t.Fatalf("FindWorkerWithCapacity failed: %v", err)
	}
	if found2 == nil {
		t.Fatal("Expected to find worker with capacity")
	}
	// Note: We can't guarantee which worker we'll get, just that it has capacity
	if found2.ActiveSessions >= found2.MaxSessions {
		t.Error("Found worker should have capacity")
	}
}

func TestFindWorkerWithCapacityNoneAvailable(t *testing.T) {
	s := NewInMemoryWorkerRegistryStorage()

	// Register worker at full capacity
	worker := &storage.RegisteredWorker{
		WorkerID:       "worker1",
		Hostname:       "localhost",
		GRPCPort:       50051,
		MaxSessions:    5,
		ActiveSessions: 5, // Full
	}

	err := s.RegisterWorker(context.Background(), worker)
	if err != nil {
		t.Fatalf("RegisterWorker failed: %v", err)
	}

	// Should return nil when no capacity available
	found, err := s.FindWorkerWithCapacity(context.Background())
	if err != nil {
		t.Fatalf("FindWorkerWithCapacity failed: %v", err)
	}
	if found != nil {
		t.Error("Expected nil when no workers have capacity")
	}
}

func TestGetWorkersByCoordinator(t *testing.T) {
	s := NewInMemoryWorkerRegistryStorage()

	// Register workers for different coordinators
	workers := []*storage.RegisteredWorker{
		{
			WorkerID:      "worker1",
			Hostname:      "host1",
			GRPCPort:      50051,
			MaxSessions:   10,
			CoordinatorID: "coord1",
		},
		{
			WorkerID:      "worker2",
			Hostname:      "host2",
			GRPCPort:      50052,
			MaxSessions:   10,
			CoordinatorID: "coord1",
		},
		{
			WorkerID:      "worker3",
			Hostname:      "host3",
			GRPCPort:      50053,
			MaxSessions:   10,
			CoordinatorID: "coord2",
		},
	}

	for _, w := range workers {
		if err := s.RegisterWorker(context.Background(), w); err != nil {
			t.Fatalf("RegisterWorker failed: %v", err)
		}
	}

	// Get workers for coord1
	coord1Workers, err := s.GetWorkersByCoordinator(context.Background(), "coord1")
	if err != nil {
		t.Fatalf("GetWorkersByCoordinator failed: %v", err)
	}
	if len(coord1Workers) != 2 {
		t.Errorf("Expected 2 workers for coord1, got %d", len(coord1Workers))
	}

	// Get workers for coord2
	coord2Workers, err := s.GetWorkersByCoordinator(context.Background(), "coord2")
	if err != nil {
		t.Fatalf("GetWorkersByCoordinator failed: %v", err)
	}
	if len(coord2Workers) != 1 {
		t.Errorf("Expected 1 worker for coord2, got %d", len(coord2Workers))
	}

	// Get workers for non-existent coordinator
	noWorkers, err := s.GetWorkersByCoordinator(context.Background(), "coord3")
	if err != nil {
		t.Fatalf("GetWorkersByCoordinator failed: %v", err)
	}
	if len(noWorkers) != 0 {
		t.Errorf("Expected 0 workers for coord3, got %d", len(noWorkers))
	}
}

func TestGetWorkersByCoordinatorEmptyID(t *testing.T) {
	s := NewInMemoryWorkerRegistryStorage()
	_, err := s.GetWorkersByCoordinator(context.Background(), "")
	if err == nil {
		t.Fatal("Expected error for empty coordinator ID, got nil")
	}
}

func TestUpdateWorkerSessions(t *testing.T) {
	s := NewInMemoryWorkerRegistryStorage()

	worker := &storage.RegisteredWorker{
		WorkerID:       "worker1",
		Hostname:       "localhost",
		GRPCPort:       50051,
		MaxSessions:    10,
		ActiveSessions: 2,
	}

	err := s.RegisterWorker(context.Background(), worker)
	if err != nil {
		t.Fatalf("RegisterWorker failed: %v", err)
	}

	// Update active sessions
	err = s.UpdateWorkerSessions(context.Background(), "worker1", 5)
	if err != nil {
		t.Fatalf("UpdateWorkerSessions failed: %v", err)
	}

	// Verify update
	retrieved, err := s.GetWorker(context.Background(), "worker1")
	if err != nil {
		t.Fatalf("GetWorker failed: %v", err)
	}
	if retrieved.ActiveSessions != 5 {
		t.Errorf("ActiveSessions = %d, want 5", retrieved.ActiveSessions)
	}
}

func TestUpdateWorkerSessionsNotFound(t *testing.T) {
	s := NewInMemoryWorkerRegistryStorage()
	err := s.UpdateWorkerSessions(context.Background(), "nonexistent", 5)
	if err == nil {
		t.Fatal("Expected error for nonexistent worker, got nil")
	}
}

func TestListStaleWorkers(t *testing.T) {
	s := NewInMemoryWorkerRegistryStorage()

	now := time.Now()
	workers := []*storage.RegisteredWorker{
		{
			WorkerID:      "worker1",
			Hostname:      "host1",
			GRPCPort:      50051,
			MaxSessions:   10,
			LastHeartbeat: now.Add(-10 * time.Minute), // Stale
		},
		{
			WorkerID:      "worker2",
			Hostname:      "host2",
			GRPCPort:      50052,
			MaxSessions:   10,
			LastHeartbeat: now.Add(-30 * time.Second), // Fresh
		},
		{
			WorkerID:      "worker3",
			Hostname:      "host3",
			GRPCPort:      50053,
			MaxSessions:   10,
			LastHeartbeat: now.Add(-15 * time.Minute), // Stale
		},
	}

	for _, w := range workers {
		if err := s.RegisterWorker(context.Background(), w); err != nil {
			t.Fatalf("RegisterWorker failed: %v", err)
		}
	}

	// List workers stale for more than 5 minutes
	staleWorkers, err := s.ListStaleWorkers(context.Background(), 5*time.Minute)
	if err != nil {
		t.Fatalf("ListStaleWorkers failed: %v", err)
	}
	if len(staleWorkers) != 2 {
		t.Errorf("Expected 2 stale workers, got %d", len(staleWorkers))
	}

	// List workers stale for more than 20 minutes
	veryStaleWorkers, err := s.ListStaleWorkers(context.Background(), 20*time.Minute)
	if err != nil {
		t.Fatalf("ListStaleWorkers failed: %v", err)
	}
	if len(veryStaleWorkers) != 0 {
		t.Errorf("Expected 0 very stale workers, got %d", len(veryStaleWorkers))
	}
}

func TestConcurrentWorkerOperations(t *testing.T) {
	s := NewInMemoryWorkerRegistryStorage()

	worker := &storage.RegisteredWorker{
		WorkerID:       "worker1",
		Hostname:       "localhost",
		GRPCPort:       50051,
		MaxSessions:    10,
		ActiveSessions: 0,
	}

	err := s.RegisterWorker(context.Background(), worker)
	if err != nil {
		t.Fatalf("RegisterWorker failed: %v", err)
	}

	const numGoroutines = 10
	done := make(chan bool, numGoroutines*2)

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		go func() {
			_, _ = s.GetWorker(context.Background(), "worker1")
			done <- true
		}()
	}

	// Concurrent heartbeat updates
	for i := 0; i < numGoroutines; i++ {
		go func() {
			_ = s.UpdateWorkerHeartbeat(context.Background(), "worker1")
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines*2; i++ {
		<-done
	}

	// Verify worker still exists and is valid
	retrieved, err := s.GetWorker(context.Background(), "worker1")
	if err != nil {
		t.Fatalf("GetWorker failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Worker should still exist")
	}
}
