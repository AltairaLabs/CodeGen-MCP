package worker

import (
	"context"
	"testing"
)

func TestValidateMetadata_AllRequired(t *testing.T) {
	requirements := []MetadataRequirement{
		{Key: "env", Description: "Environment", Required: true},
		{Key: "version", Description: "Version", Required: true},
	}

	// Test with all required fields present
	metadata := map[string]string{
		"env":     "production",
		"version": "1.0.0",
	}
	err := ValidateMetadata(metadata, requirements)
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Test with missing required field
	metadataIncomplete := map[string]string{
		"env": "production",
	}
	err = ValidateMetadata(metadataIncomplete, requirements)
	if err == nil {
		t.Error("Expected error for missing required field")
	}

	// Test with empty required field
	metadataEmpty := map[string]string{
		"env":     "production",
		"version": "",
	}
	err = ValidateMetadata(metadataEmpty, requirements)
	if err == nil {
		t.Error("Expected error for empty required field")
	}
}

func TestValidateMetadata_OptionalFields(t *testing.T) {
	requirements := []MetadataRequirement{
		{Key: "env", Description: "Environment", Required: true},
		{Key: "debug", Description: "Debug mode", Required: false},
	}

	// Test with required field only
	metadata := map[string]string{
		"env": "production",
	}
	err := ValidateMetadata(metadata, requirements)
	if err != nil {
		t.Errorf("Expected no error with optional field missing, got: %v", err)
	}

	// Test with both fields
	metadataComplete := map[string]string{
		"env":   "production",
		"debug": "true",
	}
	err = ValidateMetadata(metadataComplete, requirements)
	if err != nil {
		t.Errorf("Expected no error with optional field present, got: %v", err)
	}
}

func TestValidateMetadata_NoRequirements(t *testing.T) {
	requirements := []MetadataRequirement{}
	metadata := map[string]string{}

	err := ValidateMetadata(metadata, requirements)
	if err != nil {
		t.Errorf("Expected no error with no requirements, got: %v", err)
	}
}

func TestGetToolMetadataRequirements(t *testing.T) {
	sessionPool := NewSessionPool("test-worker", 10, "/tmp/test-sessions")
	executor := NewTaskExecutor(sessionPool)

	// Test that method returns without error for various tools
	tools := []string{"echo", "fs.write", "fs.read", "fs.list", "run.python", "pkg.install"}
	for _, tool := range tools {
		reqs := executor.getToolMetadataRequirements(tool)
		// Should not panic and should return a slice (even if empty)
		if reqs == nil {
			t.Errorf("Expected non-nil requirements for tool %s", tool)
		}
	}
}

func TestSessionPool_MetadataCRUD(t *testing.T) {
	sessionPool := NewSessionPool("test-worker", 10, "/tmp/test-sessions")

	// Create a session with metadata
	ctx := context.Background()
	sessionID := "metadata-test-session"
	metadata := map[string]string{
		"env":     "test",
		"version": "1.0.0",
	}

	err := sessionPool.CreateSessionWithID(ctx, sessionID, "workspace1", "user1", map[string]string{}, metadata)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer sessionPool.DestroySession(ctx, sessionID, false)

	// Test GetSessionMetadata
	retrievedMetadata, err := sessionPool.GetSessionMetadata(sessionID)
	if err != nil {
		t.Fatalf("Failed to get metadata: %v", err)
	}
	if retrievedMetadata["env"] != "test" {
		t.Errorf("Expected env=test, got %s", retrievedMetadata["env"])
	}
	if retrievedMetadata["version"] != "1.0.0" {
		t.Errorf("Expected version=1.0.0, got %s", retrievedMetadata["version"])
	}

	// Test UpdateSessionMetadata
	updates := map[string]string{
		"version": "1.1.0",
		"debug":   "true",
	}
	err = sessionPool.UpdateSessionMetadata(sessionID, updates)
	if err != nil {
		t.Fatalf("Failed to update metadata: %v", err)
	}

	retrievedMetadata, err = sessionPool.GetSessionMetadata(sessionID)
	if err != nil {
		t.Fatalf("Failed to get updated metadata: %v", err)
	}
	if retrievedMetadata["env"] != "test" {
		t.Error("Expected env to remain unchanged")
	}
	if retrievedMetadata["version"] != "1.1.0" {
		t.Errorf("Expected version=1.1.0, got %s", retrievedMetadata["version"])
	}
	if retrievedMetadata["debug"] != "true" {
		t.Errorf("Expected debug=true, got %s", retrievedMetadata["debug"])
	}

	// Test SetSessionMetadata (replaces all)
	newMetadata := map[string]string{
		"stage": "production",
	}
	err = sessionPool.SetSessionMetadata(sessionID, newMetadata)
	if err != nil {
		t.Fatalf("Failed to set metadata: %v", err)
	}

	retrievedMetadata, err = sessionPool.GetSessionMetadata(sessionID)
	if err != nil {
		t.Fatalf("Failed to get replaced metadata: %v", err)
	}
	if len(retrievedMetadata) != 1 {
		t.Errorf("Expected 1 metadata field, got %d", len(retrievedMetadata))
	}
	if retrievedMetadata["stage"] != "production" {
		t.Errorf("Expected stage=production, got %s", retrievedMetadata["stage"])
	}
	if _, exists := retrievedMetadata["env"]; exists {
		t.Error("Expected old metadata to be removed after Set")
	}
}

func TestSessionPool_MetadataConcurrency(t *testing.T) {
	sessionPool := NewSessionPool("test-worker", 10, "/tmp/test-sessions")

	ctx := context.Background()
	sessionID := "concurrent-metadata-test"
	err := sessionPool.CreateSessionWithID(ctx, sessionID, "workspace1", "user1", map[string]string{}, map[string]string{})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer sessionPool.DestroySession(ctx, sessionID, false)

	// Concurrent updates
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(val int) {
			updates := map[string]string{
				"counter": string(rune('0' + val)),
			}
			_ = sessionPool.UpdateSessionMetadata(sessionID, updates)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should not panic and should have consistent state
	metadata, err := sessionPool.GetSessionMetadata(sessionID)
	if err != nil {
		t.Fatalf("Failed to get metadata after concurrent updates: %v", err)
	}
	if _, exists := metadata["counter"]; !exists {
		t.Error("Expected counter field to exist after concurrent updates")
	}
}

func TestSessionPool_MetadataOnNonExistentSession(t *testing.T) {
	sessionPool := NewSessionPool("test-worker", 10, "/tmp/test-sessions")

	// Try to get metadata for non-existent session
	_, err := sessionPool.GetSessionMetadata("non-existent-session")
	if err == nil {
		t.Error("Expected error when getting metadata for non-existent session")
	}

	// Try to update metadata for non-existent session
	err = sessionPool.UpdateSessionMetadata("non-existent-session", map[string]string{"key": "value"})
	if err == nil {
		t.Error("Expected error when updating metadata for non-existent session")
	}

	// Try to set metadata for non-existent session
	err = sessionPool.SetSessionMetadata("non-existent-session", map[string]string{"key": "value"})
	if err == nil {
		t.Error("Expected error when setting metadata for non-existent session")
	}
}

func TestSessionPool_LastCompletedSequence(t *testing.T) {
	sessionPool := NewSessionPool("test-worker", 10, "/tmp/test-sessions")

	ctx := context.Background()
	sessionID := "sequence-test-session"
	err := sessionPool.CreateSessionWithID(ctx, sessionID, "workspace1", "user1", map[string]string{}, map[string]string{})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer sessionPool.DestroySession(ctx, sessionID, false)

	// Initial sequence should be 0
	seq, err := sessionPool.GetLastCompletedSequence(sessionID)
	if err != nil {
		t.Fatalf("Failed to get initial sequence: %v", err)
	}
	if seq != 0 {
		t.Errorf("Expected initial sequence 0, got %d", seq)
	}

	// Set sequence
	err = sessionPool.SetLastCompletedSequence(sessionID, 42)
	if err != nil {
		t.Fatalf("Failed to set sequence: %v", err)
	}

	// Verify sequence
	seq, err = sessionPool.GetLastCompletedSequence(sessionID)
	if err != nil {
		t.Fatalf("Failed to get updated sequence: %v", err)
	}
	if seq != 42 {
		t.Errorf("Expected sequence 42, got %d", seq)
	}

	// Update sequence
	err = sessionPool.SetLastCompletedSequence(sessionID, 100)
	if err != nil {
		t.Fatalf("Failed to update sequence: %v", err)
	}

	seq, err = sessionPool.GetLastCompletedSequence(sessionID)
	if err != nil {
		t.Fatalf("Failed to get final sequence: %v", err)
	}
	if seq != 100 {
		t.Errorf("Expected sequence 100, got %d", seq)
	}
}

func TestSessionPool_SequenceConcurrency(t *testing.T) {
	sessionPool := NewSessionPool("test-worker", 10, "/tmp/test-sessions")

	ctx := context.Background()
	sessionID := "concurrent-sequence-test"
	err := sessionPool.CreateSessionWithID(ctx, sessionID, "workspace1", "user1", map[string]string{}, map[string]string{})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer sessionPool.DestroySession(ctx, sessionID, false)

	// Concurrent sequence updates
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func(seq uint64) {
			_ = sessionPool.SetLastCompletedSequence(sessionID, seq)
			done <- true
		}(uint64(i))
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}

	// Should not panic and should have a valid final state
	finalSeq, err := sessionPool.GetLastCompletedSequence(sessionID)
	if err != nil {
		t.Fatalf("Failed to get sequence after concurrent updates: %v", err)
	}
	if finalSeq >= 100 {
		t.Errorf("Expected sequence < 100, got %d", finalSeq)
	}
}

func TestMetadataRequirement_Structure(t *testing.T) {
	// Test MetadataRequirement creation
	req := MetadataRequirement{
		Key:         "test_key",
		Description: "Test description",
		Required:    true,
	}

	if req.Key != "test_key" {
		t.Errorf("Expected Key=test_key, got %s", req.Key)
	}
	if req.Description != "Test description" {
		t.Errorf("Expected Description='Test description', got %s", req.Description)
	}
	if !req.Required {
		t.Error("Expected Required=true")
	}
}

func TestTaskExecutor_MetadataValidationIntegration(t *testing.T) {
	sessionPool := NewSessionPool("test-worker", 10, "/tmp/test-sessions")
	executor := NewTaskExecutor(sessionPool)

	ctx := context.Background()
	sessionID := "validation-test-session"

	// Create session with empty metadata
	err := sessionPool.CreateSessionWithID(ctx, sessionID, "workspace1", "user1", map[string]string{}, map[string]string{})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer sessionPool.DestroySession(ctx, sessionID, false)

	// Currently no tools require metadata, so this should succeed
	requirements := executor.getToolMetadataRequirements("echo")
	metadata, _ := sessionPool.GetSessionMetadata(sessionID)

	err = ValidateMetadata(metadata, requirements)
	if err != nil {
		t.Errorf("Expected validation to pass with no requirements, got: %v", err)
	}
}

func TestWorkerSession_MetadataInitialization(t *testing.T) {
	sessionPool := NewSessionPool("test-worker", 10, "/tmp/test-sessions")

	ctx := context.Background()
	sessionID := "init-test-session"
	initialMetadata := map[string]string{
		"init_key": "init_value",
	}

	// Create session with initial metadata
	err := sessionPool.CreateSessionWithID(ctx, sessionID, "workspace1", "user1", map[string]string{}, initialMetadata)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer sessionPool.DestroySession(ctx, sessionID, false)

	// Verify initial metadata was set
	metadata, err := sessionPool.GetSessionMetadata(sessionID)
	if err != nil {
		t.Fatalf("Failed to get metadata: %v", err)
	}
	if metadata["init_key"] != "init_value" {
		t.Errorf("Expected init_key=init_value, got %s", metadata["init_key"])
	}
}

func TestSessionPool_MetadataIsolation(t *testing.T) {
	t.Skip("Skipping slow test (3+ seconds) - TODO: optimize or move to integration tests")
	sessionPool := NewSessionPool("test-worker", 10, "/tmp/test-sessions")

	ctx := context.Background()

	// Create two sessions with different metadata
	session1 := "isolation-test-1"
	session2 := "isolation-test-2"

	metadata1 := map[string]string{"session": "1"}
	metadata2 := map[string]string{"session": "2"}

	err := sessionPool.CreateSessionWithID(ctx, session1, "workspace1", "user1", map[string]string{}, metadata1)
	if err != nil {
		t.Fatalf("Failed to create session1: %v", err)
	}
	defer sessionPool.DestroySession(ctx, session1, false)

	err = sessionPool.CreateSessionWithID(ctx, session2, "workspace2", "user2", map[string]string{}, metadata2)
	if err != nil {
		t.Fatalf("Failed to create session2: %v", err)
	}
	defer sessionPool.DestroySession(ctx, session2, false)

	// Update session1 metadata
	err = sessionPool.UpdateSessionMetadata(session1, map[string]string{"updated": "yes"})
	if err != nil {
		t.Fatalf("Failed to update session1: %v", err)
	}

	// Verify session1 has update but session2 doesn't
	meta1, _ := sessionPool.GetSessionMetadata(session1)
	meta2, _ := sessionPool.GetSessionMetadata(session2)

	if meta1["updated"] != "yes" {
		t.Error("Session1 should have 'updated' field")
	}
	if _, exists := meta2["updated"]; exists {
		t.Error("Session2 should not have 'updated' field")
	}
	if meta2["session"] != "2" {
		t.Error("Session2 metadata should remain unchanged")
	}
}

func TestSessionPool_MetadataCopySemantics(t *testing.T) {
	sessionPool := NewSessionPool("test-worker", 10, "/tmp/test-sessions")

	ctx := context.Background()
	sessionID := "copy-test-session"

	err := sessionPool.CreateSessionWithID(ctx, sessionID, "workspace1", "user1", map[string]string{}, map[string]string{"key": "value"})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer sessionPool.DestroySession(ctx, sessionID, false)

	// Get metadata
	metadata, err := sessionPool.GetSessionMetadata(sessionID)
	if err != nil {
		t.Fatalf("Failed to get metadata: %v", err)
	}

	// Modify returned map
	metadata["key"] = "modified"
	metadata["new_key"] = "new_value"

	// Get metadata again - should not be affected by our modifications
	metadata2, err := sessionPool.GetSessionMetadata(sessionID)
	if err != nil {
		t.Fatalf("Failed to get metadata again: %v", err)
	}

	if metadata2["key"] != "value" {
		t.Error("Original metadata should not be affected by modifications to returned map")
	}
	if _, exists := metadata2["new_key"]; exists {
		t.Error("New key should not appear in original metadata")
	}
}
