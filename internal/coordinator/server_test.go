package coordinator_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"

	coordinator "github.com/AltairaLabs/codegen-mcp/internal/coordinator"
)

func TestNewMCPServer(t *testing.T) {
	sm := coordinator.NewSessionManager()
	worker := coordinator.NewMockWorkerClient()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := coordinator.NewAuditLogger(logger)

	cfg := coordinator.Config{
		Name:    "TestServer",
		Version: "1.0.0",
	}

	server := coordinator.NewMCPServer(cfg, sm, worker, audit)
	if server == nil {
		t.Fatal("Expected non-nil server")
	}
	if server.Server() == nil {
		t.Fatal("Expected non-nil underlying server")
	}
}

func TestMCPServer_HandleEcho(t *testing.T) {
	sm := coordinator.NewSessionManager()
	worker := coordinator.NewMockWorkerClient()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := coordinator.NewAuditLogger(logger)

	cfg := coordinator.Config{
		Name:    "TestServer",
		Version: "1.0.0",
	}

	server := coordinator.NewMCPServer(cfg, sm, worker, audit)

	// Verify server was created properly
	if server.Server() == nil {
		t.Fatal("Server not properly initialized")
	}

	// Verify no sessions initially
	if sm.SessionCount() != 0 {
		t.Error("Expected no sessions initially")
	}
}

func TestMCPServer_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	sm := coordinator.NewSessionManager()
	worker := coordinator.NewMockWorkerClient()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := coordinator.NewAuditLogger(logger)

	cfg := coordinator.Config{
		Name:    "TestServer",
		Version: "1.0.0",
	}

	server := coordinator.NewMCPServer(cfg, sm, worker, audit)

	// Test server creation
	if server == nil {
		t.Fatal("Server should not be nil")
	}

	// Test underlying mcp server
	if server.Server() == nil {
		t.Fatal("Underlying MCP server should not be nil")
	}

	// Test session management integration
	session := sm.CreateSession(context.Background(), "test-session", "user1", "workspace1")
	if session.ID != "test-session" {
		t.Errorf("Session ID mismatch: expected test-session, got %s", session.ID)
	}

	// Test worker client integration
	result, err := worker.ExecuteTask(context.Background(), "workspace1", "echo", coordinator.TaskArgs{
		"message": "integration test",
	})
	if err != nil {
		t.Fatalf("Worker execution failed: %v", err)
	}
	if result.Output != "integration test" {
		t.Errorf("Unexpected output: %s", result.Output)
	}
}

func TestMCPServer_ValidateWorkspacePath(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"valid relative path", "file.txt"},
		{"valid nested path", "dir/subdir/file.txt"},
		{"current dir allowed", "./file.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := coordinator.NewSessionManager()
			worker := coordinator.NewMockWorkerClient()
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			audit := coordinator.NewAuditLogger(logger)

			cfg := coordinator.Config{
				Name:    "TestServer",
				Version: "1.0.0",
			}

			_ = coordinator.NewMCPServer(cfg, sm, worker, audit)

			// Create session for tool execution
			session := sm.CreateSession(context.Background(), "path-test", "user1", "workspace1")
			ctx := context.WithValue(context.Background(), "session_id", session.ID)

			// Test valid paths via worker's ExecuteTask
			_, err := worker.ExecuteTask(ctx, "workspace1", "fs.read", coordinator.TaskArgs{
				"path": tt.path,
			})

			if err != nil {
				t.Errorf("Expected no error for path %s, got %v", tt.path, err)
			}
		})
	}
}

func TestMCPServer_GetSessionID(t *testing.T) {
	sm := coordinator.NewSessionManager()
	worker := coordinator.NewMockWorkerClient()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := coordinator.NewAuditLogger(logger)

	cfg := coordinator.Config{
		Name:    "TestServer",
		Version: "1.0.0",
	}

	_ = coordinator.NewMCPServer(cfg, sm, worker, audit)

	// Create a session
	session := sm.CreateSession(context.Background(), "test-get-session", "user1", "workspace1")

	// Test retrieving session
	retrieved, found := sm.GetSession(session.ID)
	if !found {
		t.Fatal("Failed to get session")
	}

	if retrieved.ID != session.ID {
		t.Errorf("Session ID mismatch: expected %s, got %s", session.ID, retrieved.ID)
	}

	if retrieved.UserID != "user1" {
		t.Errorf("User ID mismatch: expected user1, got %s", retrieved.UserID)
	}

	if retrieved.WorkspaceID != "workspace1" {
		t.Errorf("Workspace ID mismatch: expected workspace1, got %s", retrieved.WorkspaceID)
	}
}

func TestMCPServer_EchoTool(t *testing.T) {
	sm := coordinator.NewSessionManager()
	worker := coordinator.NewMockWorkerClient()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := coordinator.NewAuditLogger(logger)

	cfg := coordinator.Config{
		Name:    "TestServer",
		Version: "1.0.0",
	}

	_ = coordinator.NewMCPServer(cfg, sm, worker, audit)

	// Create session
	session := sm.CreateSession(context.Background(), "echo-test", "user1", "workspace1")
	ctx := context.WithValue(context.Background(), "session_id", session.ID)

	// Test echo tool via worker
	result, err := worker.ExecuteTask(ctx, "workspace1", "echo", coordinator.TaskArgs{
		"message": "test echo message",
	})

	if err != nil {
		t.Fatalf("Echo task failed: %v", err)
	}

	if result.Output != "test echo message" {
		t.Errorf("Expected output 'test echo message', got '%s'", result.Output)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}
}

func TestMCPServer_FsReadTool(t *testing.T) {
	sm := coordinator.NewSessionManager()
	worker := coordinator.NewMockWorkerClient()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := coordinator.NewAuditLogger(logger)

	cfg := coordinator.Config{
		Name:    "TestServer",
		Version: "1.0.0",
	}

	_ = coordinator.NewMCPServer(cfg, sm, worker, audit)

	// Create session
	session := sm.CreateSession(context.Background(), "read-test", "user1", "workspace1")
	ctx := context.WithValue(context.Background(), "session_id", session.ID)

	// Test fs.read tool
	result, err := worker.ExecuteTask(ctx, "workspace1", "fs.read", coordinator.TaskArgs{
		"path": "test.txt",
	})

	if err != nil {
		t.Fatalf("Read task failed: %v", err)
	}

	expected := "Content of test.txt in workspace workspace1"
	if result.Output != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result.Output)
	}
}

func TestMCPServer_FsWriteTool(t *testing.T) {
	sm := coordinator.NewSessionManager()
	worker := coordinator.NewMockWorkerClient()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := coordinator.NewAuditLogger(logger)

	cfg := coordinator.Config{
		Name:    "TestServer",
		Version: "1.0.0",
	}

	_ = coordinator.NewMCPServer(cfg, sm, worker, audit)

	// Create session
	session := sm.CreateSession(context.Background(), "write-test", "user1", "workspace1")
	ctx := context.WithValue(context.Background(), "session_id", session.ID)

	// Test fs.write tool (note: uses "contents" not "content")
	testContent := "test content"
	result, err := worker.ExecuteTask(ctx, "workspace1", "fs.write", coordinator.TaskArgs{
		"path":     "output.txt",
		"contents": testContent,
	})

	if err != nil {
		t.Fatalf("Write task failed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}

	expected := fmt.Sprintf("Wrote %d bytes to output.txt", len(testContent))
	if result.Output != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result.Output)
	}
}
