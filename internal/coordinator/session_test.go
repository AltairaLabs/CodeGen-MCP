package coordinator_test

import (
	"context"
	"testing"
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator"
)

func TestSessionManager_CreateSession(t *testing.T) {
	sm := coordinator.NewSessionManager()

	session := sm.CreateSession(context.Background(), "test-session", "user1", "workspace1")

	if session.ID != "test-session" {
		t.Errorf("Expected session ID 'test-session', got %s", session.ID)
	}
	if session.UserID != "user1" {
		t.Errorf("Expected user ID 'user1', got %s", session.UserID)
	}
	if session.WorkspaceID != "workspace1" {
		t.Errorf("Expected workspace ID 'workspace1', got %s", session.WorkspaceID)
	}
}

func TestSessionManager_GetSession(t *testing.T) {
	sm := coordinator.NewSessionManager()

	sm.CreateSession(context.Background(), "test-session", "user1", "workspace1")

	session, ok := sm.GetSession("test-session")
	if !ok {
		t.Fatal("Expected to find session")
	}
	if session.ID != "test-session" {
		t.Errorf("Expected session ID 'test-session', got %s", session.ID)
	}

	// Non-existent session
	_, ok = sm.GetSession("non-existent")
	if ok {
		t.Error("Expected not to find non-existent session")
	}
}

func TestSessionManager_DeleteSession(t *testing.T) {
	sm := coordinator.NewSessionManager()

	sm.CreateSession(context.Background(), "test-session", "user1", "workspace1")
	sm.DeleteSession("test-session")

	_, ok := sm.GetSession("test-session")
	if ok {
		t.Error("Expected session to be deleted")
	}
}

func TestSessionManager_CleanupStale(t *testing.T) {
	sm := coordinator.NewSessionManager()

	// Create two sessions
	sm.CreateSession(context.Background(), "session1", "user1", "workspace1")
	time.Sleep(50 * time.Millisecond)
	sm.CreateSession(context.Background(), "session2", "user2", "workspace2")

	// Cleanup sessions older than 30ms (should remove session1)
	deleted := sm.CleanupStale(30 * time.Millisecond)

	if deleted != 1 {
		t.Errorf("Expected 1 session to be deleted, got %d", deleted)
	}

	// Session1 should be gone
	_, ok := sm.GetSession("session1")
	if ok {
		t.Error("Expected session1 to be deleted")
	}

	// Session2 should still exist
	_, ok = sm.GetSession("session2")
	if !ok {
		t.Error("Expected session2 to still exist")
	}
}

func TestSessionManager_SessionCount(t *testing.T) {
	sm := coordinator.NewSessionManager()

	if sm.SessionCount() != 0 {
		t.Errorf("Expected 0 sessions, got %d", sm.SessionCount())
	}

	sm.CreateSession(context.Background(), "session1", "user1", "workspace1")
	sm.CreateSession(context.Background(), "session2", "user2", "workspace2")

	if sm.SessionCount() != 2 {
		t.Errorf("Expected 2 sessions, got %d", sm.SessionCount())
	}
}

func TestSessionManager_CreateSessionNoWorkersAvailable(t *testing.T) {
	registry := coordinator.NewWorkerRegistry()
	sm := coordinator.NewSessionManager(registry)

	// Don't register any workers

	// Create session - should succeed but no worker assigned
	session := sm.CreateSession(context.Background(), "test-session-1", "user1", "workspace1")

	if session.WorkerID != "" {
		t.Errorf("Expected empty worker ID, got %s", session.WorkerID)
	}

	if session.WorkerSessionID != "" {
		t.Errorf("Expected empty worker session ID, got %s", session.WorkerSessionID)
	}
}
