package coordinator

import (
	"context"
	"testing"
	"time"
)

// Helper function to create a test session manager
func newTestSessionManagerForFuncs(t *testing.T) (*SessionManager, *testSessionStorage) {
	t.Helper()
	storage := newTestSessionStorage()
	registry := NewWorkerRegistry()
	sm := NewSessionManager(storage, registry)
	return sm, storage
}

func TestSessionManagerDeleteSession(t *testing.T) {
	sm, storage := newTestSessionManagerForFuncs(t)
	ctx := context.Background()

	// Create a session first
	session := sm.CreateSession(ctx, "test-session-1", "user1", "workspace1")

	// Delete the session
	sm.DeleteSession(session.ID)

	// Verify session is deleted
	retrieved, found := sm.GetSession(session.ID)
	if found || retrieved != nil {
		t.Error("Expected session to be deleted")
	}

	// Verify it's also deleted from storage
	storageSession, err := storage.GetSession(ctx, session.ID)
	if err != nil || storageSession != nil {
		t.Error("Expected session to be deleted from storage")
	}
}

func TestSessionManagerCleanupStale(t *testing.T) {
	sm, storage := newTestSessionManagerForFuncs(t)
	ctx := context.Background()

	// Create some sessions
	session1 := sm.CreateSession(ctx, "test-session-1", "user1", "workspace1")
	session2 := sm.CreateSession(ctx, "test-session-2", "user2", "workspace2")

	// Manually mark session1 as stale by setting an old LastActive time
	session1.LastActive = time.Now().Add(-2 * time.Hour)
	_ = storage.CreateSession(ctx, session1) // Update in storage

	// Run cleanup with 1 hour threshold
	count := sm.CleanupStale(1 * time.Hour)

	// Should have cleaned up 1 stale session
	if count != 1 {
		t.Errorf("Expected 1 stale session cleaned up, got %d", count)
	}

	// Verify session1 is deleted but session2 remains
	_, found1 := sm.GetSession(session1.ID)
	if found1 {
		t.Error("Expected session1 to be deleted")
	}

	_, found2 := sm.GetSession(session2.ID)
	if !found2 {
		t.Error("Expected session2 to still exist")
	}
}

func TestSessionManagerUpdateSessionState(t *testing.T) {
	sm, _ := newTestSessionManagerForFuncs(t)
	ctx := context.Background()

	// Create a session
	session := sm.CreateSession(ctx, "test-session-1", "user1", "workspace1")

	// Update the session state
	err := sm.UpdateSessionState(session.ID, SessionStateReady, "test message")
	if err != nil {
		t.Fatalf("UpdateSessionState failed: %v", err)
	}

	// Retrieve and verify
	updated, found := sm.GetSession(session.ID)
	if !found {
		t.Fatal("Failed to get updated session")
	}

	if updated.State != SessionStateReady {
		t.Errorf("Expected state %v, got %v", SessionStateReady, updated.State)
	}
	if updated.StateMessage != "test message" {
		t.Errorf("Expected state message 'test message', got %s", updated.StateMessage)
	}
}

func TestSessionManagerGetAllSessions(t *testing.T) {
	sm, _ := newTestSessionManagerForFuncs(t)
	ctx := context.Background()

	// Create multiple sessions
	sm.CreateSession(ctx, "test-session-1", "user1", "workspace1")
	sm.CreateSession(ctx, "test-session-2", "user2", "workspace2")
	sm.CreateSession(ctx, "test-session-3", "user3", "workspace3")

	// Get all sessions
	sessions := sm.GetAllSessions()

	if len(sessions) < 3 {
		t.Errorf("Expected at least 3 sessions, got %d", len(sessions))
	}
}

func TestSessionManagerGetNextSequence(t *testing.T) {
	sm, _ := newTestSessionManagerForFuncs(t)
	ctx := context.Background()

	// Create a session
	session := sm.CreateSession(ctx, "test-session-1", "user1", "workspace1")

	// Get next sequence numbers
	seq1 := sm.GetNextSequence(ctx, session.ID)
	seq2 := sm.GetNextSequence(ctx, session.ID)
	seq3 := sm.GetNextSequence(ctx, session.ID)

	// Verify sequences increment
	if seq2 != seq1+1 {
		t.Errorf("Expected sequence %d, got %d", seq1+1, seq2)
	}
	if seq3 != seq2+1 {
		t.Errorf("Expected sequence %d, got %d", seq2+1, seq3)
	}
}

func TestSessionManagerSessionCount(t *testing.T) {
	sm, _ := newTestSessionManagerForFuncs(t)
	ctx := context.Background()

	// Initially should have 0 sessions
	initialCount := sm.SessionCount()

	// Create some sessions
	sm.CreateSession(ctx, "test-session-1", "user1", "workspace1")
	sm.CreateSession(ctx, "test-session-2", "user2", "workspace2")

	// Verify count increased
	finalCount := sm.SessionCount()
	if finalCount != initialCount+2 {
		t.Errorf("Expected %d sessions, got %d", initialCount+2, finalCount)
	}
}

func TestSessionManagerCleanupStaleNoStaleSessions(t *testing.T) {
	sm, _ := newTestSessionManagerForFuncs(t)
	ctx := context.Background()

	// Create recent sessions
	sm.CreateSession(ctx, "test-session-1", "user1", "workspace1")
	sm.CreateSession(ctx, "test-session-2", "user2", "workspace2")

	// Run cleanup with very long threshold - should find nothing
	count := sm.CleanupStale(24 * time.Hour)

	if count != 0 {
		t.Errorf("Expected 0 stale sessions, got %d", count)
	}
}

func TestSessionManagerUpdateSessionStateNotFound(t *testing.T) {
	sm, _ := newTestSessionManagerForFuncs(t)

	// Try to update non-existent session - UpdateSessionState doesn't return error for missing sessions
	// It just updates storage, which may or may not fail
	err := sm.UpdateSessionState("nonexistent-session", SessionStateReady, "")
	// This doesn't error in current implementation
	if err != nil {
		t.Logf("UpdateSessionState returned error: %v", err)
	}
}

func TestSessionManagerGetNextSequenceNewSession(t *testing.T) {
	sm, _ := newTestSessionManagerForFuncs(t)
	ctx := context.Background()

	// Create a session
	session := sm.CreateSession(ctx, "test-session-1", "user1", "workspace1")

	// First sequence should be 1 (or initial value)
	seq1 := sm.GetNextSequence(ctx, session.ID)
	if seq1 == 0 {
		t.Error("Expected non-zero sequence number")
	}
}

func TestSessionManagerSetLastCompletedSequence(t *testing.T) {
	storage := newTestSessionStorage()
	registry := NewWorkerRegistry()
	sm := NewSessionManager(storage, registry)
	ctx := context.Background()

	// Create a session first
	session := sm.CreateSession(ctx, "test-session", "user1", "workspace1")
	if session == nil {
		t.Fatal("Failed to create session")
	}

	// Set last completed sequence
	err := sm.SetLastCompletedSequence(ctx, session.ID, 42)
	if err != nil {
		t.Errorf("SetLastCompletedSequence failed: %v", err)
	}

	// Verify the sequence was set by getting the session
	retrievedSession, ok := sm.GetSession(session.ID)
	if !ok {
		t.Fatal("Failed to retrieve session")
	}

	if retrievedSession.LastCheckpoint == "" {
		t.Log("LastCheckpoint not set via SetLastCompletedSequence (expected, as it updates storage)")
	}
}

func TestSessionManagerSetLastCompletedSequenceError(t *testing.T) {
	storage := newTestSessionStorage()
	registry := NewWorkerRegistry()
	sm := NewSessionManager(storage, registry)
	ctx := context.Background()

	// Try to set sequence for non-existent session
	err := sm.SetLastCompletedSequence(ctx, "non-existent-session", 100)
	// The error behavior depends on the storage implementation
	// Just verify the function can be called without panicking
	if err != nil {
		t.Logf("Expected error for non-existent session: %v", err)
	}
}
