package worker

import (
	"context"
	"os"
	"testing"
)

func TestAbandonSession(t *testing.T) {
	// Create temp directory for test workspaces
	tempDir := t.TempDir()

	pool := NewSessionPool("test-worker", 5, tempDir)

	ctx := context.Background()

	// Create a session
	sessionID := "test-session-1"
	err := pool.CreateSessionWithID(ctx, sessionID, "workspace1", "user1", map[string]string{}, map[string]string{})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Verify session exists
	session, err := pool.GetSession(sessionID)
	if err != nil {
		t.Fatalf("Session should exist: %v", err)
	}
	if session == nil {
		t.Fatal("Session should not be nil")
	}

	workspacePath := session.WorkspacePath

	// Verify workspace directory exists
	if _, err := os.Stat(workspacePath); os.IsNotExist(err) {
		t.Fatalf("Workspace directory should exist: %s", workspacePath)
	}

	// Abandon the session
	err = pool.AbandonSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("Failed to abandon session: %v", err)
	}

	// Verify session is removed from pool
	_, err = pool.GetSession(sessionID)
	if err == nil {
		t.Error("Session should not exist after abandonment")
	}

	// Verify workspace directory is cleaned up
	if _, err := os.Stat(workspacePath); !os.IsNotExist(err) {
		t.Error("Workspace directory should be removed after abandonment")
	}
}

func TestAbandonSessionNonexistent(t *testing.T) {
	tempDir := t.TempDir()
	pool := NewSessionPool("test-worker", 5, tempDir)
	ctx := context.Background()

	// Abandon nonexistent session should not error
	err := pool.AbandonSession(ctx, "nonexistent-session")
	if err != nil {
		t.Errorf("Abandoning nonexistent session should not error: %v", err)
	}
}

func TestAbandonAllSessions(t *testing.T) {
	t.Skip("Skipping slow test (5+ seconds) - TODO: optimize or move to integration tests")
	tempDir := t.TempDir()
	pool := NewSessionPool("test-worker", 5, tempDir)
	ctx := context.Background()

	// Create multiple sessions
	sessionIDs := []string{"session1", "session2", "session3"}
	workspacePaths := make([]string, len(sessionIDs))

	for i, sessionID := range sessionIDs {
		err := pool.CreateSessionWithID(ctx, sessionID, "workspace"+sessionID, "user1", map[string]string{}, map[string]string{})
		if err != nil {
			t.Fatalf("Failed to create session %s: %v", sessionID, err)
		}

		session, _ := pool.GetSession(sessionID)
		workspacePaths[i] = session.WorkspacePath
	}

	// Verify all sessions exist
	capacity := pool.GetCapacity()
	if capacity.ActiveSessions != 3 {
		t.Errorf("Expected 3 active sessions, got %d", capacity.ActiveSessions)
	}

	// Abandon all sessions
	count, err := pool.AbandonAllSessions(ctx)
	if err != nil {
		t.Fatalf("Failed to abandon all sessions: %v", err)
	}

	if count != 3 {
		t.Errorf("Expected to abandon 3 sessions, got %d", count)
	}

	// Verify all sessions are removed
	capacity = pool.GetCapacity()
	if capacity.ActiveSessions != 0 {
		t.Errorf("Expected 0 active sessions after abandonment, got %d", capacity.ActiveSessions)
	}

	// Verify all workspace directories are cleaned up
	for i, workspacePath := range workspacePaths {
		if _, err := os.Stat(workspacePath); !os.IsNotExist(err) {
			t.Errorf("Workspace directory for session %d should be removed: %s", i, workspacePath)
		}
	}
}

func TestAbandonAllSessionsEmpty(t *testing.T) {
	tempDir := t.TempDir()
	pool := NewSessionPool("test-worker", 5, tempDir)
	ctx := context.Background()

	// Abandon all when no sessions exist
	count, err := pool.AbandonAllSessions(ctx)
	if err != nil {
		t.Errorf("Abandoning empty pool should not error: %v", err)
	}

	if count != 0 {
		t.Errorf("Expected 0 abandoned sessions, got %d", count)
	}
}

func TestAbandonSessionCleansUpMetadata(t *testing.T) {
	tempDir := t.TempDir()
	pool := NewSessionPool("test-worker", 5, tempDir)
	ctx := context.Background()

	sessionID := "session-with-metadata"
	metadata := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	// Create session with metadata
	err := pool.CreateSessionWithID(ctx, sessionID, "workspace1", "user1", map[string]string{}, metadata)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Set last completed sequence
	err = pool.SetLastCompletedSequence(sessionID, 42)
	if err != nil {
		t.Fatalf("Failed to set sequence: %v", err)
	}

	// Verify metadata and sequence exist
	retrievedMetadata, err := pool.GetSessionMetadata(sessionID)
	if err != nil {
		t.Fatalf("Failed to get metadata: %v", err)
	}
	if len(retrievedMetadata) != 2 {
		t.Error("Metadata should exist before abandonment")
	}

	seq, err := pool.GetLastCompletedSequence(sessionID)
	if err != nil {
		t.Fatalf("Failed to get sequence: %v", err)
	}
	if seq != 42 {
		t.Errorf("Expected sequence 42, got %d", seq)
	}

	// Abandon session
	err = pool.AbandonSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("Failed to abandon session: %v", err)
	}

	// Verify metadata is cleaned up
	retrievedMetadata, _ = pool.GetSessionMetadata(sessionID)
	if len(retrievedMetadata) != 0 {
		t.Error("Metadata should be cleaned up after abandonment")
	}

	// Verify sequence is reset
	seq, err = pool.GetLastCompletedSequence(sessionID)
	if err == nil && seq != 0 {
		t.Error("Sequence should be reset after abandonment")
	}
}

func TestAbandonSessionWithActiveTasks(t *testing.T) {
	tempDir := t.TempDir()
	pool := NewSessionPool("test-worker", 5, tempDir)
	ctx := context.Background()

	sessionID := "session-with-tasks"
	err := pool.CreateSessionWithID(ctx, sessionID, "workspace1", "user1", map[string]string{}, map[string]string{})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Increment active tasks
	err = pool.IncrementActiveTasks(sessionID)
	if err != nil {
		t.Fatalf("Failed to increment tasks: %v", err)
	}

	session, _ := pool.GetSession(sessionID)
	if session.ActiveTasks != 1 {
		t.Errorf("Expected 1 active task, got %d", session.ActiveTasks)
	}

	// Abandon session should work even with active tasks
	err = pool.AbandonSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("Failed to abandon session with active tasks: %v", err)
	}

	// Verify session is removed
	_, err = pool.GetSession(sessionID)
	if err == nil {
		t.Error("Session should be removed even with active tasks")
	}
}
