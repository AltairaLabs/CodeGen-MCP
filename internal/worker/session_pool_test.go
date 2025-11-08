package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

func TestNewSessionPool(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := NewSessionPool("test-worker", 5, baseWorkspace)

	if pool == nil {
		t.Fatal("Expected session pool to be created")
	}

	if pool.workerID != "test-worker" {
		t.Errorf("Expected worker ID 'test-worker', got '%s'", pool.workerID)
	}

	if pool.maxSessions != 5 {
		t.Errorf("Expected maxSessions 5, got %d", pool.maxSessions)
	}
}

func TestSessionPool_CreateSession(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := NewSessionPool("test-worker", 5, baseWorkspace)

	req := &protov1.CreateSessionRequest{
		WorkspaceId: "test-workspace",
		UserId:      "test-user",
		Config: &protov1.SessionConfig{
			Language: "python",
		},
	}

	resp, err := pool.CreateSession(context.Background(), req)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	if resp.SessionId == "" {
		t.Error("Expected session ID to be set")
	}

	if resp.WorkerId != "test-worker" {
		t.Errorf("Expected worker ID 'test-worker', got '%s'", resp.WorkerId)
	}

	if resp.State != protov1.SessionState_SESSION_STATE_READY {
		t.Errorf("Expected state READY, got %v", resp.State)
	}

	// Verify workspace was created
	if _, err := os.Stat(resp.WorkspacePath); os.IsNotExist(err) {
		t.Errorf("Workspace directory was not created: %s", resp.WorkspacePath)
	}
}

func TestSessionPool_GetSession(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := NewSessionPool("test-worker", 5, baseWorkspace)

	// Create a session
	req := &protov1.CreateSessionRequest{
		WorkspaceId: "test-workspace",
		UserId:      "test-user",
	}

	resp, err := pool.CreateSession(context.Background(), req)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Get the session
	session, err := pool.GetSession(resp.SessionId)
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	if session.SessionID != resp.SessionId {
		t.Errorf("Expected session ID '%s', got '%s'", resp.SessionId, session.SessionID)
	}

	// Try to get non-existent session
	_, err = pool.GetSession("non-existent")
	if err == nil {
		t.Error("Expected error when getting non-existent session")
	}
}

func TestSessionPool_DestroySession(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := NewSessionPool("test-worker", 5, baseWorkspace)

	// Create a session
	req := &protov1.CreateSessionRequest{
		WorkspaceId: "test-workspace",
		UserId:      "test-user",
	}

	resp, err := pool.CreateSession(context.Background(), req)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Destroy the session
	err = pool.DestroySession(context.Background(), resp.SessionId, false)
	if err != nil {
		t.Fatalf("Failed to destroy session: %v", err)
	}

	// Verify session no longer exists
	_, err = pool.GetSession(resp.SessionId)
	if err == nil {
		t.Error("Expected error when getting destroyed session")
	}

	// Verify workspace was removed
	if _, err := os.Stat(resp.WorkspacePath); !os.IsNotExist(err) {
		t.Error("Workspace directory was not removed")
	}
}

func TestSessionPool_Capacity(t *testing.T) {
	baseWorkspace := t.TempDir()
	maxSessions := int32(3)
	pool := NewSessionPool("test-worker", maxSessions, baseWorkspace)

	// Create sessions up to capacity
	for i := 0; i < int(maxSessions); i++ {
		req := &protov1.CreateSessionRequest{
			WorkspaceId: "test-workspace",
			UserId:      "test-user",
		}

		_, err := pool.CreateSession(context.Background(), req)
		if err != nil {
			t.Fatalf("Failed to create session %d: %v", i, err)
		}
	}

	// Try to create one more session (should fail)
	req := &protov1.CreateSessionRequest{
		WorkspaceId: "test-workspace",
		UserId:      "test-user",
	}

	_, err := pool.CreateSession(context.Background(), req)
	if err == nil {
		t.Error("Expected error when creating session beyond capacity")
	}

	// Verify capacity info
	capacity := pool.GetCapacity()
	if capacity.TotalSessions != maxSessions {
		t.Errorf("Expected total sessions %d, got %d", maxSessions, capacity.TotalSessions)
	}

	if capacity.ActiveSessions != maxSessions {
		t.Errorf("Expected active sessions %d, got %d", maxSessions, capacity.ActiveSessions)
	}

	if capacity.AvailableSessions != 0 {
		t.Errorf("Expected available sessions 0, got %d", capacity.AvailableSessions)
	}
}

func TestSessionPool_UpdateSessionActivity(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := NewSessionPool("test-worker", 5, baseWorkspace)

	req := &protov1.CreateSessionRequest{
		WorkspaceId: "test-workspace",
		UserId:      "test-user",
	}

	resp, err := pool.CreateSession(context.Background(), req)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	session, _ := pool.GetSession(resp.SessionId)
	originalActivity := session.LastActivity

	time.Sleep(10 * time.Millisecond)

	err = pool.UpdateSessionActivity(resp.SessionId)
	if err != nil {
		t.Fatalf("Failed to update session activity: %v", err)
	}

	session, _ = pool.GetSession(resp.SessionId)
	if !session.LastActivity.After(originalActivity) {
		t.Error("Expected last activity to be updated")
	}
}

func TestSessionPool_ActiveTasks(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := NewSessionPool("test-worker", 5, baseWorkspace)

	req := &protov1.CreateSessionRequest{
		WorkspaceId: "test-workspace",
		UserId:      "test-user",
	}

	resp, err := pool.CreateSession(context.Background(), req)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Increment active tasks
	err = pool.IncrementActiveTasks(resp.SessionId)
	if err != nil {
		t.Fatalf("Failed to increment active tasks: %v", err)
	}

	session, _ := pool.GetSession(resp.SessionId)
	if session.ActiveTasks != 1 {
		t.Errorf("Expected 1 active task, got %d", session.ActiveTasks)
	}

	if session.State != protov1.SessionState_SESSION_STATE_BUSY {
		t.Errorf("Expected state BUSY, got %v", session.State)
	}

	// Decrement active tasks
	err = pool.DecrementActiveTasks(resp.SessionId)
	if err != nil {
		t.Fatalf("Failed to decrement active tasks: %v", err)
	}

	session, _ = pool.GetSession(resp.SessionId)
	if session.ActiveTasks != 0 {
		t.Errorf("Expected 0 active tasks, got %d", session.ActiveTasks)
	}

	if session.State != protov1.SessionState_SESSION_STATE_IDLE {
		t.Errorf("Expected state IDLE, got %v", session.State)
	}
}

func TestSessionPool_TaskHistory(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := NewSessionPool("test-worker", 5, baseWorkspace)

	req := &protov1.CreateSessionRequest{
		WorkspaceId: "test-workspace",
		UserId:      "test-user",
	}

	resp, err := pool.CreateSession(context.Background(), req)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Add tasks to history
	for i := 0; i < 12; i++ {
		err = pool.AddTaskToHistory(resp.SessionId, "task-"+string(rune('0'+i)))
		if err != nil {
			t.Fatalf("Failed to add task to history: %v", err)
		}
	}

	session, _ := pool.GetSession(resp.SessionId)

	// Should keep only last 10 tasks
	if len(session.RecentTasks) != 10 {
		t.Errorf("Expected 10 recent tasks, got %d", len(session.RecentTasks))
	}

	// Most recent task should be first
	if session.RecentTasks[0] != "task-;" {
		t.Errorf("Expected most recent task 'task-;', got '%s'", session.RecentTasks[0])
	}
}

func TestSessionPool_CheckpointTracking(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := NewSessionPool("test-worker", 5, baseWorkspace)

	req := &protov1.CreateSessionRequest{
		WorkspaceId: "test-workspace",
		UserId:      "test-user",
	}

	resp, err := pool.CreateSession(context.Background(), req)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	checkpointID := "checkpoint-123"
	err = pool.SetLastCheckpoint(resp.SessionId, checkpointID)
	if err != nil {
		t.Fatalf("Failed to set last checkpoint: %v", err)
	}

	session, _ := pool.GetSession(resp.SessionId)
	if session.LastCheckpointID != checkpointID {
		t.Errorf("Expected checkpoint ID '%s', got '%s'", checkpointID, session.LastCheckpointID)
	}
}

func TestSessionPool_ArtifactTracking(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := NewSessionPool("test-worker", 5, baseWorkspace)

	req := &protov1.CreateSessionRequest{
		WorkspaceId: "test-workspace",
		UserId:      "test-user",
	}

	resp, err := pool.CreateSession(context.Background(), req)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Add artifacts
	artifacts := []string{"artifact-1", "artifact-2", "artifact-3"}
	for _, artifactID := range artifacts {
		err = pool.AddArtifact(resp.SessionId, artifactID)
		if err != nil {
			t.Fatalf("Failed to add artifact: %v", err)
		}
	}

	session, _ := pool.GetSession(resp.SessionId)
	if len(session.ArtifactIDs) != 3 {
		t.Errorf("Expected 3 artifacts, got %d", len(session.ArtifactIDs))
	}

	for i, artifactID := range artifacts {
		if session.ArtifactIDs[i] != artifactID {
			t.Errorf("Expected artifact ID '%s', got '%s'", artifactID, session.ArtifactIDs[i])
		}
	}
}

func TestSessionPool_WorkspaceStructure(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := NewSessionPool("test-worker", 5, baseWorkspace)

	req := &protov1.CreateSessionRequest{
		WorkspaceId: "test-workspace",
		UserId:      "test-user",
	}

	resp, err := pool.CreateSession(context.Background(), req)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Verify workspace structure
	expectedDirs := []string{"src", "tests", "artifacts"}
	for _, dir := range expectedDirs {
		dirPath := filepath.Join(resp.WorkspacePath, dir)
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			t.Errorf("Expected directory '%s' to exist", dir)
		}
	}
}

func TestGenerateSessionID(t *testing.T) {
	id1 := generateSessionID()

	// Add a small delay to ensure different timestamps
	time.Sleep(1 * time.Millisecond)
	id2 := generateSessionID()

	if id1 == id2 {
		t.Error("Expected unique session IDs")
	}

	if id1 == "" {
		t.Error("Expected non-empty session ID")
	}
}
