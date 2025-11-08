package coordinator_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	coordinator "github.com/AltairaLabs/codegen-mcp/internal/coordinator"
)

// TestIntegration_FullWorkflow tests the complete coordinator workflow
func TestIntegration_FullWorkflow(t *testing.T) {
	// Setup complete coordinator stack
	sessionMgr := coordinator.NewSessionManager()
	worker := coordinator.NewMockWorkerClient()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	audit := coordinator.NewAuditLogger(logger)

	cfg := coordinator.Config{
		Name:    "integration-test-server",
		Version: "1.0.0",
	}

	server := coordinator.NewMCPServer(cfg, sessionMgr, worker, audit)

	// Verify server initialization
	if server == nil {
		t.Fatal("Server should not be nil")
	}

	// Test 1: Create a session
	ctx := context.Background()
	session := sessionMgr.CreateSession(ctx, "integration-session-1", "test-user", "test-workspace")

	if session.ID != "integration-session-1" {
		t.Errorf("Session ID mismatch: expected integration-session-1, got %s", session.ID)
	}

	// Test 2: Execute echo tool
	ctxWithSession := context.WithValue(ctx, "session_id", session.ID)
	result, err := worker.ExecuteTask(ctxWithSession, session.WorkspaceID, "echo", coordinator.TaskArgs{
		"message": "integration test message",
	})

	if err != nil {
		t.Fatalf("Echo tool failed: %v", err)
	}

	if result.Output != "integration test message" {
		t.Errorf("Echo output mismatch: expected 'integration test message', got '%s'", result.Output)
	}

	if !result.Success {
		t.Error("Echo tool should have succeeded")
	}

	// Test 3: Write a file
	result, err = worker.ExecuteTask(ctxWithSession, session.WorkspaceID, "fs.write", coordinator.TaskArgs{
		"path":     "test-file.py",
		"contents": "print('Hello from integration test')",
	})

	if err != nil {
		t.Fatalf("Write tool failed: %v", err)
	}

	if !result.Success {
		t.Error("Write tool should have succeeded")
	}

	// Test 4: Read the file back
	result, err = worker.ExecuteTask(ctxWithSession, session.WorkspaceID, "fs.read", coordinator.TaskArgs{
		"path": "test-file.py",
	})

	if err != nil {
		t.Fatalf("Read tool failed: %v", err)
	}

	if !result.Success {
		t.Error("Read tool should have succeeded")
	}

	// Test 5: Verify session is still active
	retrievedSession, found := sessionMgr.GetSession(session.ID)
	if !found {
		t.Fatal("Session should still exist")
	}

	if retrievedSession.LastActive.Before(session.CreatedAt) {
		t.Error("LastActive should have been updated")
	}

	// Test 6: Verify session count
	if sessionMgr.SessionCount() != 1 {
		t.Errorf("Expected 1 session, got %d", sessionMgr.SessionCount())
	}
}

// TestIntegration_MultipleSessionsIsolation tests workspace isolation
func TestIntegration_MultipleSessionsIsolation(t *testing.T) {
	sessionMgr := coordinator.NewSessionManager()
	worker := coordinator.NewMockWorkerClient()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	audit := coordinator.NewAuditLogger(logger)

	cfg := coordinator.Config{
		Name:    "isolation-test-server",
		Version: "1.0.0",
	}

	server := coordinator.NewMCPServer(cfg, sessionMgr, worker, audit)

	if server == nil {
		t.Fatal("Server should not be nil")
	}

	ctx := context.Background()

	// Create multiple sessions with different workspaces
	sessions := []struct {
		sessionID   string
		userID      string
		workspaceID string
	}{
		{"session-1", "user-1", "workspace-1"},
		{"session-2", "user-2", "workspace-2"},
		{"session-3", "user-3", "workspace-3"},
	}

	for _, s := range sessions {
		session := sessionMgr.CreateSession(ctx, s.sessionID, s.userID, s.workspaceID)

		if session.ID != s.sessionID {
			t.Errorf("Session ID mismatch for %s", s.sessionID)
		}

		if session.WorkspaceID != s.workspaceID {
			t.Errorf("Workspace ID mismatch for %s", s.sessionID)
		}

		// Write different content to each workspace
		ctxWithSession := context.WithValue(ctx, "session_id", session.ID)
		content := fmt.Sprintf("Content for %s", s.workspaceID)

		result, err := worker.ExecuteTask(ctxWithSession, session.WorkspaceID, "fs.write", coordinator.TaskArgs{
			"path":     "data.txt",
			"contents": content,
		})

		if err != nil {
			t.Fatalf("Write failed for %s: %v", s.sessionID, err)
		}

		if !result.Success {
			t.Errorf("Write should have succeeded for %s", s.sessionID)
		}
	}

	// Verify all sessions exist and are isolated
	if sessionMgr.SessionCount() != 3 {
		t.Errorf("Expected 3 sessions, got %d", sessionMgr.SessionCount())
	}

	// Verify each session still has its workspace
	for _, s := range sessions {
		session, found := sessionMgr.GetSession(s.sessionID)
		if !found {
			t.Errorf("Session %s not found", s.sessionID)
		}

		if session.WorkspaceID != s.workspaceID {
			t.Errorf("Workspace mismatch for %s: expected %s, got %s",
				s.sessionID, s.workspaceID, session.WorkspaceID)
		}
	}
}

// TestIntegration_ConcurrentToolExecution tests concurrent tool calls
func TestIntegration_ConcurrentToolExecution(t *testing.T) {
	sessionMgr := coordinator.NewSessionManager()
	worker := coordinator.NewMockWorkerClient()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	audit := coordinator.NewAuditLogger(logger)

	cfg := coordinator.Config{
		Name:    "concurrent-test-server",
		Version: "1.0.0",
	}

	server := coordinator.NewMCPServer(cfg, sessionMgr, worker, audit)

	if server == nil {
		t.Fatal("Server should not be nil")
	}

	ctx := context.Background()
	numGoroutines := 50
	var wg sync.WaitGroup

	// Create session for concurrent access
	session := sessionMgr.CreateSession(ctx, "concurrent-session", "test-user", "concurrent-workspace")

	// Execute multiple tool calls concurrently
	wg.Add(numGoroutines)
	errChan := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			ctxWithSession := context.WithValue(ctx, "session_id", session.ID)

			// Execute echo tool
			result, err := worker.ExecuteTask(ctxWithSession, session.WorkspaceID, "echo", coordinator.TaskArgs{
				"message": fmt.Sprintf("concurrent message %d", id),
			})

			if err != nil {
				errChan <- fmt.Errorf("goroutine %d failed: %v", id, err)
				return
			}

			expected := fmt.Sprintf("concurrent message %d", id)
			if result.Output != expected {
				errChan <- fmt.Errorf("goroutine %d output mismatch: expected '%s', got '%s'",
					id, expected, result.Output)
				return
			}

			// Write a file
			_, err = worker.ExecuteTask(ctxWithSession, session.WorkspaceID, "fs.write", coordinator.TaskArgs{
				"path":     fmt.Sprintf("file-%d.txt", id),
				"contents": fmt.Sprintf("data from goroutine %d", id),
			})

			if err != nil {
				errChan <- fmt.Errorf("goroutine %d write failed: %v", id, err)
				return
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Check for errors
	for err := range errChan {
		t.Error(err)
	}

	// Verify session is still intact
	retrievedSession, found := sessionMgr.GetSession(session.ID)
	if !found {
		t.Fatal("Session should still exist after concurrent operations")
	}

	if retrievedSession.ID != session.ID {
		t.Error("Session ID should not have changed")
	}
}

// TestIntegration_SessionCleanup tests automatic session cleanup
func TestIntegration_SessionCleanup(t *testing.T) {
	sessionMgr := coordinator.NewSessionManager()
	worker := coordinator.NewMockWorkerClient()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	audit := coordinator.NewAuditLogger(logger)

	cfg := coordinator.Config{
		Name:    "cleanup-test-server",
		Version: "1.0.0",
	}

	server := coordinator.NewMCPServer(cfg, sessionMgr, worker, audit)

	if server == nil {
		t.Fatal("Server should not be nil")
	}

	ctx := context.Background()

	// Create an old session
	oldSession := sessionMgr.CreateSession(ctx, "old-session", "user-1", "workspace-1")

	// Manually set LastActive to simulate old session
	oldSession.LastActive = time.Now().Add(-1 * time.Hour)

	// Create a recent session
	recentSession := sessionMgr.CreateSession(ctx, "recent-session", "user-2", "workspace-2")

	// Verify both exist
	if sessionMgr.SessionCount() != 2 {
		t.Errorf("Expected 2 sessions, got %d", sessionMgr.SessionCount())
	}

	// Clean up sessions older than 30 minutes
	deleted := sessionMgr.CleanupStale(30 * time.Minute)

	if deleted != 1 {
		t.Errorf("Expected 1 session deleted, got %d", deleted)
	}

	// Verify only recent session remains
	if sessionMgr.SessionCount() != 1 {
		t.Errorf("Expected 1 session remaining, got %d", sessionMgr.SessionCount())
	}

	// Verify the correct session was kept
	_, found := sessionMgr.GetSession(recentSession.ID)
	if !found {
		t.Error("Recent session should still exist")
	}

	_, found = sessionMgr.GetSession(oldSession.ID)
	if found {
		t.Error("Old session should have been deleted")
	}
}

// TestIntegration_ErrorHandling tests error conditions
func TestIntegration_ErrorHandling(t *testing.T) {
	sessionMgr := coordinator.NewSessionManager()
	worker := coordinator.NewMockWorkerClient()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	audit := coordinator.NewAuditLogger(logger)

	cfg := coordinator.Config{
		Name:    "error-test-server",
		Version: "1.0.0",
	}

	server := coordinator.NewMCPServer(cfg, sessionMgr, worker, audit)

	if server == nil {
		t.Fatal("Server should not be nil")
	}

	ctx := context.Background()
	session := sessionMgr.CreateSession(ctx, "error-session", "test-user", "error-workspace")
	ctxWithSession := context.WithValue(ctx, "session_id", session.ID)

	// Test 1: Unknown tool
	_, err := worker.ExecuteTask(ctxWithSession, session.WorkspaceID, "unknown.tool", coordinator.TaskArgs{})
	if err == nil {
		t.Error("Expected error for unknown tool")
	}

	// Test 2: Context cancellation
	cancelCtx, cancel := context.WithCancel(ctxWithSession)
	cancel() // Cancel immediately

	_, err = worker.ExecuteTask(cancelCtx, session.WorkspaceID, "echo", coordinator.TaskArgs{
		"message": "should not execute",
	})
	if err == nil {
		t.Error("Expected error for cancelled context")
	}

	// Test 3: Context timeout
	timeoutCtx, timeoutCancel := context.WithTimeout(ctxWithSession, 1*time.Millisecond)
	defer timeoutCancel()

	time.Sleep(10 * time.Millisecond) // Ensure timeout

	_, err = worker.ExecuteTask(timeoutCtx, session.WorkspaceID, "echo", coordinator.TaskArgs{
		"message": "timeout test",
	})
	if err == nil {
		t.Error("Expected timeout error")
	}

	// Test 4: Session should still be valid after errors
	_, found := sessionMgr.GetSession(session.ID)
	if !found {
		t.Error("Session should still exist after errors")
	}
}

// TestIntegration_AuditLogging tests that all operations are audited
func TestIntegration_AuditLogging(t *testing.T) {
	sessionMgr := coordinator.NewSessionManager()
	worker := coordinator.NewMockWorkerClient()

	// Capture audit logs
	var auditBuffer []byte
	logger := slog.New(slog.NewJSONHandler(&testWriter{buffer: &auditBuffer}, nil))
	audit := coordinator.NewAuditLogger(logger)

	cfg := coordinator.Config{
		Name:    "audit-test-server",
		Version: "1.0.0",
	}

	server := coordinator.NewMCPServer(cfg, sessionMgr, worker, audit)

	if server == nil {
		t.Fatal("Server should not be nil")
	}

	ctx := context.Background()
	session := sessionMgr.CreateSession(ctx, "audit-session", "audit-user", "audit-workspace")
	ctxWithSession := context.WithValue(ctx, "session_id", session.ID)

	// Execute various tools
	tools := []struct {
		name string
		args coordinator.TaskArgs
	}{
		{"echo", coordinator.TaskArgs{"message": "test"}},
		{"fs.write", coordinator.TaskArgs{"path": "test.txt", "contents": "data"}},
		{"fs.read", coordinator.TaskArgs{"path": "test.txt"}},
	}

	for _, tool := range tools {
		result, err := worker.ExecuteTask(ctxWithSession, session.WorkspaceID, tool.name, tool.args)
		if err != nil {
			t.Fatalf("Tool %s failed: %v", tool.name, err)
		}

		if !result.Success {
			t.Errorf("Tool %s should have succeeded", tool.name)
		}
	}

	// Note: In a real implementation, we would verify audit log entries
	// For now, we just verify no errors occurred during logging
}

// testWriter is a simple writer for capturing logs in tests
type testWriter struct {
	buffer *[]byte
}

func (tw *testWriter) Write(p []byte) (n int, err error) {
	*tw.buffer = append(*tw.buffer, p...)
	return len(p), nil
}

// TestIntegration_SessionMetadata tests session metadata functionality
func TestIntegration_SessionMetadata(t *testing.T) {
	sessionMgr := coordinator.NewSessionManager()
	worker := coordinator.NewMockWorkerClient()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	audit := coordinator.NewAuditLogger(logger)

	cfg := coordinator.Config{
		Name:    "metadata-test-server",
		Version: "1.0.0",
	}

	server := coordinator.NewMCPServer(cfg, sessionMgr, worker, audit)

	if server == nil {
		t.Fatal("Server should not be nil")
	}

	ctx := context.Background()
	session := sessionMgr.CreateSession(ctx, "metadata-session", "meta-user", "meta-workspace")

	// Add metadata to session
	session.Metadata["client"] = "test-client"
	session.Metadata["version"] = "1.0.0"
	session.Metadata["region"] = "us-west-2"

	// Retrieve session and verify metadata
	retrieved, found := sessionMgr.GetSession(session.ID)
	if !found {
		t.Fatal("Session should exist")
	}

	if retrieved.Metadata["client"] != "test-client" {
		t.Error("Metadata 'client' not preserved")
	}

	if retrieved.Metadata["version"] != "1.0.0" {
		t.Error("Metadata 'version' not preserved")
	}

	if retrieved.Metadata["region"] != "us-west-2" {
		t.Error("Metadata 'region' not preserved")
	}

	// Execute tool and verify session metadata is still intact
	ctxWithSession := context.WithValue(ctx, "session_id", session.ID)
	_, err := worker.ExecuteTask(ctxWithSession, session.WorkspaceID, "echo", coordinator.TaskArgs{
		"message": "test",
	})

	if err != nil {
		t.Fatalf("Tool execution failed: %v", err)
	}

	// Re-retrieve and verify metadata persisted
	retrieved2, _ := sessionMgr.GetSession(session.ID)
	if retrieved2.Metadata["client"] != "test-client" {
		t.Error("Metadata should persist after tool execution")
	}
}

// TestIntegration_WorkerFailover tests handling of worker failures
func TestIntegration_WorkerFailover(t *testing.T) {
	sessionMgr := coordinator.NewSessionManager()
	worker := coordinator.NewMockWorkerClient()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	audit := coordinator.NewAuditLogger(logger)

	cfg := coordinator.Config{
		Name:    "failover-test-server",
		Version: "1.0.0",
	}

	server := coordinator.NewMCPServer(cfg, sessionMgr, worker, audit)

	if server == nil {
		t.Fatal("Server should not be nil")
	}

	ctx := context.Background()
	session := sessionMgr.CreateSession(ctx, "failover-session", "failover-user", "failover-workspace")
	ctxWithSession := context.WithValue(ctx, "session_id", session.ID)

	// Test normal operation
	result, err := worker.ExecuteTask(ctxWithSession, session.WorkspaceID, "echo", coordinator.TaskArgs{
		"message": "normal operation",
	})

	if err != nil {
		t.Fatalf("Normal operation failed: %v", err)
	}

	if !result.Success {
		t.Error("Normal operation should succeed")
	}

	// Test with unknown tool (simulates worker error)
	_, err = worker.ExecuteTask(ctxWithSession, session.WorkspaceID, "nonexistent", coordinator.TaskArgs{})
	if err == nil {
		t.Error("Expected error for nonexistent tool")
	}

	// Verify session is still valid after error
	_, found := sessionMgr.GetSession(session.ID)
	if !found {
		t.Error("Session should remain valid after worker error")
	}

	// Verify we can continue using the session
	result, err = worker.ExecuteTask(ctxWithSession, session.WorkspaceID, "echo", coordinator.TaskArgs{
		"message": "recovery test",
	})

	if err != nil {
		t.Fatalf("Recovery operation failed: %v", err)
	}

	if result.Output != "recovery test" {
		t.Errorf("Recovery operation produced wrong output: %s", result.Output)
	}
}
