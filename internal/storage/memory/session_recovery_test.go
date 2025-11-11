package memory

import (
	"context"
	"testing"
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator"
)

// TestListSessionsByWorkerID tests the storage layer implementation
// for querying sessions by worker ID (used for reconnection recovery).
func TestListSessionsByWorkerID(t *testing.T) {
	ctx := context.Background()

	storage := NewInMemorySessionStateStorage()

	// Create sessions for different workers
	session1 := &coordinator.Session{
		ID:         "session-1",
		WorkerID:   "worker-1",
		State:      coordinator.SessionStateCreating,
		Metadata:   map[string]string{"tag": "test"},
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
	}

	session2 := &coordinator.Session{
		ID:         "session-2",
		WorkerID:   "worker-1",
		State:      coordinator.SessionStateReady,
		Metadata:   map[string]string{"tag": "prod"},
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
	}

	session3 := &coordinator.Session{
		ID:         "session-3",
		WorkerID:   "worker-2",
		State:      coordinator.SessionStateReady,
		Metadata:   map[string]string{"tag": "test"},
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
	}

	if err := storage.CreateSession(ctx, session1); err != nil {
		t.Fatalf("Failed to create session1: %v", err)
	}
	if err := storage.CreateSession(ctx, session2); err != nil {
		t.Fatalf("Failed to create session2: %v", err)
	}
	if err := storage.CreateSession(ctx, session3); err != nil {
		t.Fatalf("Failed to create session3: %v", err)
	}

	// Query by worker ID for worker-1
	sessions, err := storage.ListSessionsByWorkerID(ctx, "worker-1")
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}

	if len(sessions) != 2 {
		t.Errorf("Expected 2 sessions for worker-1, got %d", len(sessions))
	}

	// Verify sessions belong to worker-1 and metadata is preserved
	for _, s := range sessions {
		if s.WorkerID != "worker-1" {
			t.Errorf("Expected workerID 'worker-1', got '%s'", s.WorkerID)
		}
		if s.Metadata["tag"] == "" {
			t.Error("Expected metadata to be preserved")
		}
	}

	// Query by worker ID for worker-2
	sessions2, err := storage.ListSessionsByWorkerID(ctx, "worker-2")
	if err != nil {
		t.Fatalf("Failed to list sessions for worker-2: %v", err)
	}

	if len(sessions2) != 1 {
		t.Errorf("Expected 1 session for worker-2, got %d", len(sessions2))
	}
}

// TestListSessionsByWorkerIDEmpty verifies behavior when querying
// sessions for a worker that has no sessions.
func TestListSessionsByWorkerIDEmpty(t *testing.T) {
	ctx := context.Background()

	storage := NewInMemorySessionStateStorage()

	// Query sessions for non-existent worker
	sessions, err := storage.ListSessionsByWorkerID(ctx, "nonexistent-worker")
	if err != nil {
		t.Fatalf("Expected no error for nonexistent worker, got: %v", err)
	}

	if len(sessions) != 0 {
		t.Errorf("Expected 0 sessions for nonexistent worker, got %d", len(sessions))
	}
}

// TestListSessionsByWorkerIDMultipleStates verifies that sessions
// in all states are returned (creating, ready, failed, terminating).
func TestListSessionsByWorkerIDMultipleStates(t *testing.T) {
	ctx := context.Background()

	storage := NewInMemorySessionStateStorage()

	creating := &coordinator.Session{
		ID:         "session-creating",
		WorkerID:   "worker-1",
		State:      coordinator.SessionStateCreating,
		Metadata:   map[string]string{},
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
	}

	ready := &coordinator.Session{
		ID:         "session-ready",
		WorkerID:   "worker-1",
		State:      coordinator.SessionStateReady,
		Metadata:   map[string]string{},
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
	}

	failed := &coordinator.Session{
		ID:         "session-failed",
		WorkerID:   "worker-1",
		State:      coordinator.SessionStateFailed,
		Metadata:   map[string]string{},
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
	}

	for _, session := range []*coordinator.Session{creating, ready, failed} {
		if err := storage.CreateSession(ctx, session); err != nil {
			t.Fatalf("Failed to create session %s: %v", session.ID, err)
		}
	}

	sessions, err := storage.ListSessionsByWorkerID(ctx, "worker-1")
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}

	if len(sessions) != 3 {
		t.Errorf("Expected 3 sessions for worker-1 (all states), got %d", len(sessions))
	}

	// Verify all states are present
	states := make(map[coordinator.SessionState]bool)
	for _, s := range sessions {
		states[s.State] = true
	}

	if !states[coordinator.SessionStateCreating] {
		t.Error("Expected to find creating session")
	}
	if !states[coordinator.SessionStateReady] {
		t.Error("Expected to find ready session")
	}
	if !states[coordinator.SessionStateFailed] {
		t.Error("Expected to find failed session")
	}
}
