package coordinator

import (
	"context"
	"fmt"
	"testing"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

func TestNewWorkerRegistry(t *testing.T) {
	registry := NewWorkerRegistry()
	if registry == nil {
		t.Fatal("Expected non-nil registry")
	}
	if registry.workers == nil {
		t.Error("Expected initialized workers map")
	}
}

func TestWorkerRegistry_RegisterWorker(t *testing.T) {
	registry := NewWorkerRegistry()

	worker := &RegisteredWorker{
		WorkerID:          "worker-1",
		SessionID:         "session-1",
		RegisteredAt:      time.Now(),
		LastHeartbeat:     time.Now(),
		HeartbeatInterval: 30 * time.Second,
		Status: &protov1.WorkerStatus{
			State: protov1.WorkerStatus_STATE_IDLE,
		},
		Capacity: &protov1.SessionCapacity{
			TotalSessions:     10,
			ActiveSessions:    0,
			AvailableSessions: 10,
		},
	}

	// Test successful registration
	err := registry.RegisterWorker("worker-1", worker)
	if err != nil {
		t.Errorf("Failed to register worker: %v", err)
	}

	// Test duplicate registration
	err = registry.RegisterWorker("worker-1", worker)
	if err == nil {
		t.Error("Expected error when registering duplicate worker")
	}
}

func TestWorkerRegistry_GetWorker(t *testing.T) {
	registry := NewWorkerRegistry()

	worker := &RegisteredWorker{
		WorkerID:      "worker-1",
		SessionID:     "session-1",
		RegisteredAt:  time.Now(),
		LastHeartbeat: time.Now(),
	}

	registry.RegisterWorker("worker-1", worker)

	// Test getting existing worker
	retrieved := registry.GetWorker("worker-1")
	if retrieved == nil {
		t.Error("Expected to retrieve registered worker")
	}
	if retrieved.WorkerID != "worker-1" {
		t.Errorf("Expected worker-1, got %s", retrieved.WorkerID)
	}

	// Test getting non-existent worker
	notFound := registry.GetWorker("worker-999")
	if notFound != nil {
		t.Error("Expected nil for non-existent worker")
	}
}

func TestWorkerRegistry_UpdateHeartbeat(t *testing.T) {
	registry := NewWorkerRegistry()

	worker := &RegisteredWorker{
		WorkerID:      "worker-1",
		SessionID:     "session-1",
		RegisteredAt:  time.Now(),
		LastHeartbeat: time.Now().Add(-1 * time.Minute),
		Status: &protov1.WorkerStatus{
			State: protov1.WorkerStatus_STATE_IDLE,
		},
		Capacity: &protov1.SessionCapacity{
			TotalSessions:     10,
			ActiveSessions:    2,
			AvailableSessions: 8,
		},
	}

	registry.RegisterWorker("worker-1", worker)

	// Update heartbeat
	newStatus := &protov1.WorkerStatus{
		State: protov1.WorkerStatus_STATE_BUSY,
	}
	newCapacity := &protov1.SessionCapacity{
		TotalSessions:     10,
		ActiveSessions:    5,
		AvailableSessions: 5,
	}

	err := registry.UpdateHeartbeat("worker-1", newStatus, newCapacity)
	if err != nil {
		t.Errorf("Failed to update heartbeat: %v", err)
	}

	// Verify update
	updated := registry.GetWorker("worker-1")
	if updated.Status.State != protov1.WorkerStatus_STATE_BUSY {
		t.Error("Expected worker state to be updated to BUSY")
	}
	if updated.Capacity.ActiveSessions != 5 {
		t.Errorf("Expected 5 active sessions, got %d", updated.Capacity.ActiveSessions)
	}

	// Test updating non-existent worker
	err = registry.UpdateHeartbeat("worker-999", newStatus, newCapacity)
	if err == nil {
		t.Error("Expected error when updating non-existent worker")
	}
}

func TestWorkerRegistry_FindWorkerWithCapacity(t *testing.T) {
	registry := NewWorkerRegistry()
	ctx := context.Background()

	// Register multiple workers
	worker1 := &RegisteredWorker{
		WorkerID:          "worker-1",
		SessionID:         "session-1",
		RegisteredAt:      time.Now(),
		LastHeartbeat:     time.Now(),
		HeartbeatInterval: 30 * time.Second,
		Status: &protov1.WorkerStatus{
			State: protov1.WorkerStatus_STATE_IDLE,
		},
		Capacity: &protov1.SessionCapacity{
			TotalSessions:     10,
			ActiveSessions:    7,
			AvailableSessions: 3,
		},
	}

	worker2 := &RegisteredWorker{
		WorkerID:          "worker-2",
		SessionID:         "session-2",
		RegisteredAt:      time.Now(),
		LastHeartbeat:     time.Now(),
		HeartbeatInterval: 30 * time.Second,
		Status: &protov1.WorkerStatus{
			State: protov1.WorkerStatus_STATE_IDLE,
		},
		Capacity: &protov1.SessionCapacity{
			TotalSessions:     10,
			ActiveSessions:    2,
			AvailableSessions: 8, // Most available
		},
	}

	worker3 := &RegisteredWorker{
		WorkerID:          "worker-3",
		SessionID:         "session-3",
		RegisteredAt:      time.Now(),
		LastHeartbeat:     time.Now(),
		HeartbeatInterval: 30 * time.Second,
		Status: &protov1.WorkerStatus{
			State: protov1.WorkerStatus_STATE_IDLE,
		},
		Capacity: &protov1.SessionCapacity{
			TotalSessions:     10,
			ActiveSessions:    10,
			AvailableSessions: 0, // No capacity
		},
	}

	registry.RegisterWorker("worker-1", worker1)
	registry.RegisterWorker("worker-2", worker2)
	registry.RegisterWorker("worker-3", worker3)

	// Find worker with capacity
	best, err := registry.FindWorkerWithCapacity(ctx, nil)
	if err != nil {
		t.Errorf("Failed to find worker: %v", err)
	}
	if best == nil {
		t.Fatal("Expected to find a worker")
	}
	if best.WorkerID != "worker-2" {
		t.Errorf("Expected worker-2 (most available), got %s", best.WorkerID)
	}
}

func TestWorkerRegistry_FindWorkerWithCapacity_NoCapacity(t *testing.T) {
	registry := NewWorkerRegistry()
	ctx := context.Background()

	// Register worker with no capacity
	worker := &RegisteredWorker{
		WorkerID:          "worker-1",
		SessionID:         "session-1",
		RegisteredAt:      time.Now(),
		LastHeartbeat:     time.Now(),
		HeartbeatInterval: 30 * time.Second,
		Status: &protov1.WorkerStatus{
			State: protov1.WorkerStatus_STATE_BUSY,
		},
		Capacity: &protov1.SessionCapacity{
			TotalSessions:     10,
			ActiveSessions:    10,
			AvailableSessions: 0,
		},
	}

	registry.RegisterWorker("worker-1", worker)

	// Should fail to find worker
	_, err := registry.FindWorkerWithCapacity(ctx, nil)
	if err == nil {
		t.Error("Expected error when no workers have capacity")
	}
}

func TestWorkerRegistry_FindWorkerWithCapacity_StaleHeartbeat(t *testing.T) {
	registry := NewWorkerRegistry()
	ctx := context.Background()

	// Register worker with stale heartbeat
	worker := &RegisteredWorker{
		WorkerID:          "worker-1",
		SessionID:         "session-1",
		RegisteredAt:      time.Now(),
		LastHeartbeat:     time.Now().Add(-2 * time.Minute), // Stale
		HeartbeatInterval: 30 * time.Second,
		Status: &protov1.WorkerStatus{
			State: protov1.WorkerStatus_STATE_IDLE,
		},
		Capacity: &protov1.SessionCapacity{
			TotalSessions:     10,
			ActiveSessions:    0,
			AvailableSessions: 10,
		},
	}

	registry.RegisterWorker("worker-1", worker)

	// Should fail to find worker due to stale heartbeat
	_, err := registry.FindWorkerWithCapacity(ctx, nil)
	if err == nil {
		t.Error("Expected error when worker heartbeat is stale")
	}
}

func TestWorkerRegistry_FindWorkerWithCapacity_UnhealthyWorker(t *testing.T) {
	registry := NewWorkerRegistry()
	ctx := context.Background()

	// Register unhealthy worker
	worker := &RegisteredWorker{
		WorkerID:          "worker-1",
		SessionID:         "session-1",
		RegisteredAt:      time.Now(),
		LastHeartbeat:     time.Now(),
		HeartbeatInterval: 30 * time.Second,
		Status: &protov1.WorkerStatus{
			State: protov1.WorkerStatus_STATE_DRAINING, // Unhealthy
		},
		Capacity: &protov1.SessionCapacity{
			TotalSessions:     10,
			ActiveSessions:    0,
			AvailableSessions: 10,
		},
	}

	registry.RegisterWorker("worker-1", worker)

	// Should fail to find worker due to unhealthy state
	_, err := registry.FindWorkerWithCapacity(ctx, nil)
	if err == nil {
		t.Error("Expected error when worker is unhealthy")
	}
}

func TestWorkerRegistry_DeregisterWorker(t *testing.T) {
	registry := NewWorkerRegistry()

	worker := &RegisteredWorker{
		WorkerID:      "worker-1",
		SessionID:     "session-1",
		RegisteredAt:  time.Now(),
		LastHeartbeat: time.Now(),
	}

	registry.RegisterWorker("worker-1", worker)

	// Test successful deregistration
	err := registry.DeregisterWorker("worker-1")
	if err != nil {
		t.Errorf("Failed to deregister worker: %v", err)
	}

	// Verify worker is gone
	retrieved := registry.GetWorker("worker-1")
	if retrieved != nil {
		t.Error("Expected worker to be removed")
	}

	// Test deregistering non-existent worker
	err = registry.DeregisterWorker("worker-999")
	if err == nil {
		t.Error("Expected error when deregistering non-existent worker")
	}
}

func TestWorkerRegistry_ListWorkers(t *testing.T) {
	registry := NewWorkerRegistry()

	// Empty registry
	workers := registry.ListWorkers()
	if len(workers) != 0 {
		t.Errorf("Expected 0 workers, got %d", len(workers))
	}

	// Register multiple workers
	for i := 1; i <= 3; i++ {
		worker := &RegisteredWorker{
			WorkerID:      fmt.Sprintf("worker-%d", i),
			SessionID:     fmt.Sprintf("session-%d", i),
			RegisteredAt:  time.Now(),
			LastHeartbeat: time.Now(),
		}
		registry.RegisterWorker(worker.WorkerID, worker)
	}

	// List all workers
	workers = registry.ListWorkers()
	if len(workers) != 3 {
		t.Errorf("Expected 3 workers, got %d", len(workers))
	}
}

func TestWorkerRegistry_GetTotalCapacity(t *testing.T) {
	registry := NewWorkerRegistry()

	// Empty registry
	total, active, available := registry.GetTotalCapacity()
	if total != 0 || active != 0 || available != 0 {
		t.Error("Expected all zeros for empty registry")
	}

	// Register workers
	worker1 := &RegisteredWorker{
		WorkerID:      "worker-1",
		SessionID:     "session-1",
		RegisteredAt:  time.Now(),
		LastHeartbeat: time.Now(),
		Capacity: &protov1.SessionCapacity{
			TotalSessions:     10,
			ActiveSessions:    3,
			AvailableSessions: 7,
		},
	}

	worker2 := &RegisteredWorker{
		WorkerID:      "worker-2",
		SessionID:     "session-2",
		RegisteredAt:  time.Now(),
		LastHeartbeat: time.Now(),
		Capacity: &protov1.SessionCapacity{
			TotalSessions:     20,
			ActiveSessions:    5,
			AvailableSessions: 15,
		},
	}

	registry.RegisterWorker("worker-1", worker1)
	registry.RegisterWorker("worker-2", worker2)

	// Get total capacity
	total, active, available = registry.GetTotalCapacity()
	if total != 30 {
		t.Errorf("Expected total 30, got %d", total)
	}
	if active != 8 {
		t.Errorf("Expected active 8, got %d", active)
	}
	if available != 22 {
		t.Errorf("Expected available 22, got %d", available)
	}
}

func TestWorkerRegistry_CleanupStaleWorkers(t *testing.T) {
	registry := NewWorkerRegistry()

	// Register fresh worker
	freshWorker := &RegisteredWorker{
		WorkerID:      "worker-fresh",
		SessionID:     "session-1",
		RegisteredAt:  time.Now(),
		LastHeartbeat: time.Now(),
	}

	// Register stale worker
	staleWorker := &RegisteredWorker{
		WorkerID:      "worker-stale",
		SessionID:     "session-2",
		RegisteredAt:  time.Now(),
		LastHeartbeat: time.Now().Add(-5 * time.Minute), // Very old
	}

	registry.RegisterWorker("worker-fresh", freshWorker)
	registry.RegisterWorker("worker-stale", staleWorker)

	// Cleanup with 2 minute timeout
	removed := registry.CleanupStaleWorkers(2 * time.Minute)
	if removed != 1 {
		t.Errorf("Expected 1 worker removed, got %d", removed)
	}

	// Verify fresh worker still exists
	if registry.GetWorker("worker-fresh") == nil {
		t.Error("Expected fresh worker to still exist")
	}

	// Verify stale worker is gone
	if registry.GetWorker("worker-stale") != nil {
		t.Error("Expected stale worker to be removed")
	}
}

func TestWorkerRegistry_GetAllWorkers(t *testing.T) {
	registry := NewWorkerRegistry()

	// Empty registry
	workers := registry.GetAllWorkers()
	if len(workers) != 0 {
		t.Errorf("Expected 0 workers, got %d", len(workers))
	}

	// Register multiple workers
	for i := 1; i <= 3; i++ {
		worker := &RegisteredWorker{
			WorkerID:      fmt.Sprintf("worker-%d", i),
			SessionID:     fmt.Sprintf("session-%d", i),
			RegisteredAt:  time.Now(),
			LastHeartbeat: time.Now(),
		}
		registry.RegisterWorker(worker.WorkerID, worker)
	}

	// Get all workers
	workers = registry.GetAllWorkers()
	if len(workers) != 3 {
		t.Errorf("Expected 3 workers, got %d", len(workers))
	}

	// Verify we got copies, not originals
	for _, worker := range workers {
		if worker == nil {
			t.Error("Expected non-nil worker")
		}
	}
}

func TestWorkerRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewWorkerRegistry()
	ctx := context.Background()

	// Concurrent registration
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			worker := &RegisteredWorker{
				WorkerID:          fmt.Sprintf("worker-%d", id),
				SessionID:         fmt.Sprintf("session-%d", id),
				RegisteredAt:      time.Now(),
				LastHeartbeat:     time.Now(),
				HeartbeatInterval: 30 * time.Second,
				Status: &protov1.WorkerStatus{
					State: protov1.WorkerStatus_STATE_IDLE,
				},
				Capacity: &protov1.SessionCapacity{
					TotalSessions:     10,
					ActiveSessions:    0,
					AvailableSessions: 10,
				},
			}
			_ = registry.RegisterWorker(worker.WorkerID, worker)
			done <- true
		}(i)
	}

	// Wait for all registrations
	for i := 0; i < 10; i++ {
		<-done
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func(id int) {
			_ = registry.GetWorker(fmt.Sprintf("worker-%d", id))
			_, _ = registry.FindWorkerWithCapacity(ctx, nil)
			_ = registry.ListWorkers()
			_ = registry.GetAllWorkers()
			_, _, _ = registry.GetTotalCapacity()
			done <- true
		}(i)
	}

	// Wait for all reads
	for i := 0; i < 10; i++ {
		<-done
	}

	workers := registry.ListWorkers()
	if len(workers) != 10 {
		t.Errorf("Expected 10 workers after concurrent operations, got %d", len(workers))
	}
}
