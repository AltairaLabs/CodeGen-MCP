package memory

import (
	"context"
	"testing"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator"
)

// TestListSessionsByWorkerID tests the storage layer implementation
// for querying sessions by worker ID (used for reconnection recovery).
func TestListSessionsByWorkerID(t *testing.T) {
	ctx := context.Background()

	storage := NewInMemorySessionStateStorage()

	// Create sessions for different workers
	session1 := &coordinator.Session{
		ID:       "session-1",
		WorkerID: "worker-1",
		Status:   coordinator.SessionStatusCreated,
		Metadata: map[string]string{"tag": "test"},
	}

	session2 := &coordinator.Session{
		ID:       "session-2",
		WorkerID: "worker-1",
		Status:   coordinator.SessionStatusActive,
		Metadata: map[string]string{"tag": "prod"},
	}

	session3 := &coordinator.Session{
		ID:       "session-3",
		WorkerID: "worker-2",
		Status:   coordinator.SessionStatusActive,
		Metadata: map[string]string{"tag": "test"},
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

// TestListSessionsByWorkerIDAfterTermination verifies that terminated
// sessions are still returned when querying by worker ID (for recovery purposes).
func TestListSessionsByWorkerIDAfterTermination(t *testing.T) {
	ctx := context.Background()

	storage := NewInMemorySessionStateStorage()

	session := &coordinator.Session{
		ID:       "session-1",
		WorkerID: "worker-1",
		Status:   coordinator.SessionStatusActive,
		Metadata: map[string]string{},
	}

	if err := storage.CreateSession(ctx, session); err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Terminate session
	if err := storage.UpdateSessionStatus(ctx, "session-1", coordinator.SessionStatusTerminated); err != nil {
		t.Fatalf("Failed to terminate session: %v", err)
	}

	// Query should still return terminated session
	sessions, err := storage.ListSessionsByWorkerID(ctx, "worker-1")
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}

	if len(sessions) != 1 {
		t.Errorf("Expected 1 session (including terminated), got %d", len(sessions))
	}

	if sessions[0].Status != coordinator.SessionStatusTerminated {
		t.Errorf("Expected status 'terminated', got '%s'", sessions[0].Status)
	}
}

// TestListSessionsByWorkerIDMultipleStatuses verifies that sessions
// in all states are returned (created, active, terminated).
func TestListSessionsByWorkerIDMultipleStatuses(t *testing.T) {
	ctx := context.Background()

	storage := NewInMemorySessionStateStorage()

	created := &coordinator.Session{
		ID:       "session-created",
		WorkerID: "worker-1",
		Status:   coordinator.SessionStatusCreated,
		Metadata: map[string]string{},
	}

	active := &coordinator.Session{
		ID:       "session-active",
		WorkerID: "worker-1",
		Status:   coordinator.SessionStatusActive,
		Metadata: map[string]string{},
	}

	terminated := &coordinator.Session{
		ID:       "session-terminated",
		WorkerID: "worker-1",
		Status:   coordinator.SessionStatusTerminated,
		Metadata: map[string]string{},
	}

	for _, session := range []*coordinator.Session{created, active, terminated} {
		if err := storage.CreateSession(ctx, session); err != nil {
			t.Fatalf("Failed to create session %s: %v", session.ID, err)
		}
	}

	sessions, err := storage.ListSessionsByWorkerID(ctx, "worker-1")
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}

	if len(sessions) != 3 {
		t.Errorf("Expected 3 sessions for worker-1 (all statuses), got %d", len(sessions))
	}

	// Verify all statuses are present
	statuses := make(map[coordinator.SessionStatus]bool)
	for _, s := range sessions {
		statuses[s.Status] = true
	}

	if !statuses[coordinator.SessionStatusCreated] {
		t.Error("Expected to find created session")
	}
	if !statuses[coordinator.SessionStatusActive] {
		t.Error("Expected to find active session")
	}
	if !statuses[coordinator.SessionStatusTerminated] {
		t.Error("Expected to find terminated session")
	}
}
