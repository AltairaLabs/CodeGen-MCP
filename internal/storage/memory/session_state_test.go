package memory

import (
	"context"
	"testing"
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator"
)

func TestNewInMemorySessionStateStorage(t *testing.T) {
	s := NewInMemorySessionStateStorage()
	if s == nil {
		t.Fatal("expected non-nil storage")
	}
	if s.sessions == nil {
		t.Error("sessions map should be initialized")
	}
	if s.conversationToSession == nil {
		t.Error("conversationToSession map should be initialized")
	}
	if s.sequences == nil {
		t.Error("sequences map should be initialized")
	}
	if s.completedSequences == nil {
		t.Error("completedSequences map should be initialized")
	}
}

func TestCreateSession(t *testing.T) {
	tests := []struct {
		name    string
		session *coordinator.Session
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil session",
			session: nil,
			wantErr: true,
			errMsg:  "session cannot be nil",
		},
		{
			name: "empty session ID",
			session: &coordinator.Session{
				ID: "",
			},
			wantErr: true,
			errMsg:  "session ID cannot be empty",
		},
		{
			name: "valid session",
			session: &coordinator.Session{
				ID:          "session1",
				WorkspaceID: "workspace1",
				UserID:      "user1",
				CreatedAt:   time.Now(),
				LastActive:  time.Now(),
				State:       coordinator.SessionStateCreating,
			},
			wantErr: false,
		},
		{
			name: "session with metadata",
			session: &coordinator.Session{
				ID:          "session2",
				WorkspaceID: "workspace2",
				UserID:      "user2",
				CreatedAt:   time.Now(),
				Metadata:    map[string]string{"key": "value"},
				State:       coordinator.SessionStateReady,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewInMemorySessionStateStorage()
			err := s.CreateSession(context.Background(), tt.session)

			if (err != nil) != tt.wantErr {
				t.Errorf("CreateSession() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr && err.Error() != tt.errMsg {
				t.Errorf("CreateSession() error message = %v, want %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestCreateSessionDuplicate(t *testing.T) {
	s := NewInMemorySessionStateStorage()
	session := &coordinator.Session{
		ID:          "session1",
		WorkspaceID: "workspace1",
		UserID:      "user1",
		CreatedAt:   time.Now(),
		State:       coordinator.SessionStateReady,
	}

	// First create should succeed
	err := s.CreateSession(context.Background(), session)
	if err != nil {
		t.Fatalf("First CreateSession failed: %v", err)
	}

	// Second create should fail
	err = s.CreateSession(context.Background(), session)
	if err == nil {
		t.Fatal("Expected error for duplicate session, got nil")
	}
}

func TestGetSession(t *testing.T) {
	s := NewInMemorySessionStateStorage()

	session := &coordinator.Session{
		ID:          "session1",
		WorkspaceID: "workspace1",
		UserID:      "user1",
		CreatedAt:   time.Now(),
		Metadata:    map[string]string{"key": "value"},
		TaskHistory: []string{"task1", "task2"},
		State:       coordinator.SessionStateReady,
	}

	err := s.CreateSession(context.Background(), session)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Get existing session
	retrieved, err := s.GetSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Expected session, got nil")
	}
	if retrieved.ID != session.ID {
		t.Errorf("ID = %v, want %v", retrieved.ID, session.ID)
	}
	if retrieved.WorkspaceID != session.WorkspaceID {
		t.Errorf("WorkspaceID = %v, want %v", retrieved.WorkspaceID, session.WorkspaceID)
	}
	if len(retrieved.Metadata) != 1 {
		t.Errorf("Expected 1 metadata entry, got %d", len(retrieved.Metadata))
	}
	if len(retrieved.TaskHistory) != 2 {
		t.Errorf("Expected 2 task history entries, got %d", len(retrieved.TaskHistory))
	}

	// Verify we got a copy (modifying returned session shouldn't affect stored one)
	retrieved.Metadata["key2"] = "value2"
	retrieved2, _ := s.GetSession(context.Background(), "session1")
	if len(retrieved2.Metadata) != 1 {
		t.Error("Metadata was modified in storage (not a copy)")
	}
}

func TestGetSessionNotFound(t *testing.T) {
	s := NewInMemorySessionStateStorage()
	session, err := s.GetSession(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if session != nil {
		t.Errorf("Expected nil session, got %v", session)
	}
}

func TestGetSessionEmptyID(t *testing.T) {
	s := NewInMemorySessionStateStorage()
	_, err := s.GetSession(context.Background(), "")
	if err == nil {
		t.Fatal("Expected error for empty session ID, got nil")
	}
}

func TestUpdateSessionState(t *testing.T) {
	s := NewInMemorySessionStateStorage()

	session := &coordinator.Session{
		ID:        "session1",
		State:     coordinator.SessionStateCreating,
		CreatedAt: time.Now(),
	}

	err := s.CreateSession(context.Background(), session)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Update state
	err = s.UpdateSessionState(
		context.Background(),
		"session1",
		coordinator.SessionStateReady,
		"Session is ready",
	)
	if err != nil {
		t.Fatalf("UpdateSessionState failed: %v", err)
	}

	// Verify state changed
	retrieved, err := s.GetSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if retrieved.State != coordinator.SessionStateReady {
		t.Errorf("State = %v, want %v", retrieved.State, coordinator.SessionStateReady)
	}
	if retrieved.StateMessage != "Session is ready" {
		t.Errorf("StateMessage = %v, want 'Session is ready'", retrieved.StateMessage)
	}
}

func TestUpdateSessionStateNotFound(t *testing.T) {
	s := NewInMemorySessionStateStorage()
	err := s.UpdateSessionState(
		context.Background(),
		"nonexistent",
		coordinator.SessionStateReady,
		"",
	)
	if err == nil {
		t.Fatal("Expected error for nonexistent session, got nil")
	}
}

func TestSetSessionMetadata(t *testing.T) {
	s := NewInMemorySessionStateStorage()

	session := &coordinator.Session{
		ID:        "session1",
		CreatedAt: time.Now(),
	}

	err := s.CreateSession(context.Background(), session)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Set metadata
	metadata := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	err = s.SetSessionMetadata(context.Background(), "session1", metadata)
	if err != nil {
		t.Fatalf("SetSessionMetadata failed: %v", err)
	}

	// Verify metadata
	retrieved, err := s.GetSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if len(retrieved.Metadata) != 2 {
		t.Errorf("Expected 2 metadata entries, got %d", len(retrieved.Metadata))
	}
	if retrieved.Metadata["key1"] != "value1" {
		t.Errorf("Metadata[key1] = %v, want 'value1'", retrieved.Metadata["key1"])
	}

	// Overwrite existing key
	newMetadata := map[string]string{"key1": "newvalue"}
	err = s.SetSessionMetadata(context.Background(), "session1", newMetadata)
	if err != nil {
		t.Fatalf("SetSessionMetadata failed: %v", err)
	}

	retrieved, _ = s.GetSession(context.Background(), "session1")
	if retrieved.Metadata["key1"] != "newvalue" {
		t.Errorf("Metadata[key1] = %v, want 'newvalue'", retrieved.Metadata["key1"])
	}
}

func TestGetSessionMetadata(t *testing.T) {
	s := NewInMemorySessionStateStorage()

	session := &coordinator.Session{
		ID:        "session1",
		CreatedAt: time.Now(),
		Metadata:  map[string]string{"key": "value"},
	}

	err := s.CreateSession(context.Background(), session)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Get metadata
	metadata, err := s.GetSessionMetadata(context.Background(), "session1")
	if err != nil {
		t.Fatalf("GetSessionMetadata failed: %v", err)
	}
	if len(metadata) != 1 {
		t.Errorf("Expected 1 metadata entry, got %d", len(metadata))
	}
	if metadata["key"] != "value" {
		t.Errorf("Metadata[key] = %v, want 'value'", metadata["key"])
	}

	// Verify we got a copy
	metadata["key2"] = "value2"
	metadata2, _ := s.GetSessionMetadata(context.Background(), "session1")
	if len(metadata2) != 1 {
		t.Error("Metadata was modified in storage (not a copy)")
	}
}

func TestGetSessionMetadataNotFound(t *testing.T) {
	s := NewInMemorySessionStateStorage()
	metadata, err := s.GetSessionMetadata(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("GetSessionMetadata failed: %v", err)
	}
	if len(metadata) != 0 {
		t.Errorf("Expected empty metadata, got %d entries", len(metadata))
	}
}

func TestDeleteSession(t *testing.T) {
	s := NewInMemorySessionStateStorage()

	session := &coordinator.Session{
		ID:        "session1",
		CreatedAt: time.Now(),
		Metadata:  map[string]string{"key": "value"},
	}

	err := s.CreateSession(context.Background(), session)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Delete session
	err = s.DeleteSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	// Verify session is gone
	retrieved, err := s.GetSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if retrieved != nil {
		t.Error("Session should be deleted")
	}

	// Delete again should be idempotent
	err = s.DeleteSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("DeleteSession should be idempotent: %v", err)
	}
}

func TestSequenceOperations(t *testing.T) {
	s := NewInMemorySessionStateStorage()

	session := &coordinator.Session{
		ID:        "session1",
		CreatedAt: time.Now(),
	}

	err := s.CreateSession(context.Background(), session)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Initial sequence should be 0
	seq, err := s.GetLastCompletedSequence(context.Background(), "session1")
	if err != nil {
		t.Fatalf("GetLastCompletedSequence failed: %v", err)
	}
	if seq != 0 {
		t.Errorf("Initial sequence = %d, want 0", seq)
	}

	// Get next sequence (should start at 1)
	next, err := s.GetNextSequence(context.Background(), "session1")
	if err != nil {
		t.Fatalf("GetNextSequence failed: %v", err)
	}
	if next != 1 {
		t.Errorf("First sequence = %d, want 1", next)
	}

	// Get next sequence again (should be 2)
	next, err = s.GetNextSequence(context.Background(), "session1")
	if err != nil {
		t.Fatalf("GetNextSequence failed: %v", err)
	}
	if next != 2 {
		t.Errorf("Second sequence = %d, want 2", next)
	}

	// Set completed sequence
	err = s.SetLastCompletedSequence(context.Background(), "session1", 1)
	if err != nil {
		t.Fatalf("SetLastCompletedSequence failed: %v", err)
	}

	// Verify completed sequence
	seq, err = s.GetLastCompletedSequence(context.Background(), "session1")
	if err != nil {
		t.Fatalf("GetLastCompletedSequence failed: %v", err)
	}
	if seq != 1 {
		t.Errorf("Completed sequence = %d, want 1", seq)
	}
}

func TestSequenceOperationsNotFound(t *testing.T) {
	s := NewInMemorySessionStateStorage()

	_, err := s.GetNextSequence(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent session")
	}

	err = s.SetLastCompletedSequence(context.Background(), "nonexistent", 1)
	if err == nil {
		t.Fatal("Expected error for nonexistent session")
	}
}

func TestListSessions(t *testing.T) {
	s := NewInMemorySessionStateStorage()

	// Empty list
	sessions, err := s.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("Expected 0 sessions, got %d", len(sessions))
	}

	// Create multiple sessions
	for i := 1; i <= 3; i++ {
		session := &coordinator.Session{
			ID:        string(rune('0' + i)),
			CreatedAt: time.Now(),
		}
		if err := s.CreateSession(context.Background(), session); err != nil {
			t.Fatalf("CreateSession failed: %v", err)
		}
	}

	// List all sessions
	sessions, err = s.ListSessions(context.Background())
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("Expected 3 sessions, got %d", len(sessions))
	}

	// Verify we got copies
	sessions[0].Metadata = map[string]string{"modified": "true"}
	sessions2, _ := s.ListSessions(context.Background())
	if sessions2[0].Metadata != nil && sessions2[0].Metadata["modified"] == "true" {
		t.Error("Session was modified in storage (not a copy)")
	}
}

func TestUpdateSessionActivity(t *testing.T) {
	s := NewInMemorySessionStateStorage()

	initialTime := time.Now().Add(-1 * time.Hour)
	session := &coordinator.Session{
		ID:         "session1",
		CreatedAt:  initialTime,
		LastActive: initialTime,
	}

	err := s.CreateSession(context.Background(), session)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Update activity
	time.Sleep(10 * time.Millisecond) // Small delay to ensure time difference
	err = s.UpdateSessionActivity(context.Background(), "session1")
	if err != nil {
		t.Fatalf("UpdateSessionActivity failed: %v", err)
	}

	// Verify LastActive was updated
	retrieved, err := s.GetSession(context.Background(), "session1")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if retrieved.LastActive.Before(initialTime) {
		t.Error("LastActive should be updated to a more recent time")
	}
	if retrieved.LastActive.Equal(initialTime) {
		t.Error("LastActive should be different from initial time")
	}
}

func TestUpdateSessionActivityNotFound(t *testing.T) {
	s := NewInMemorySessionStateStorage()
	err := s.UpdateSessionActivity(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent session, got nil")
	}
}

func TestConcurrentOperations(t *testing.T) {
	s := NewInMemorySessionStateStorage()

	session := &coordinator.Session{
		ID:        "session1",
		CreatedAt: time.Now(),
	}

	err := s.CreateSession(context.Background(), session)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	const numGoroutines = 10
	done := make(chan bool, numGoroutines*2)

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		go func() {
			_, _ = s.GetSession(context.Background(), "session1")
			done <- true
		}()
	}

	// Concurrent sequence increments
	for i := 0; i < numGoroutines; i++ {
		go func() {
			_, _ = s.GetNextSequence(context.Background(), "session1")
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < numGoroutines*2; i++ {
		<-done
	}

	// Verify sequence was incremented correctly
	// GetNextSequence starts at 1, so after 10 calls we should be at 10
	next, err := s.GetNextSequence(context.Background(), "session1")
	if err != nil {
		t.Fatalf("GetNextSequence failed: %v", err)
	}
	if next != 11 {
		t.Errorf("Expected sequence 11 after 10 increments, got %d", next)
	}
}

func TestGetSessionByConversationID(t *testing.T) {
	s := NewInMemorySessionStateStorage()

	// Test returns nil for non-existent conversation
	session, err := s.GetSessionByConversationID(context.Background(), "conv1")
	if err != nil {
		t.Fatalf("GetSessionByConversationID failed: %v", err)
	}
	if session != nil {
		t.Error("Expected nil for non-existent conversation ID")
	}

	// Note: This method requires the conversationToSession mapping to be populated
	// by external code. For now, we just test that it doesn't crash.
	session, err = s.GetSessionByConversationID(context.Background(), "")
	if err != nil {
		t.Fatalf("GetSessionByConversationID failed: %v", err)
	}
	if session != nil {
		t.Error("Expected nil for empty conversation ID")
	}
}
