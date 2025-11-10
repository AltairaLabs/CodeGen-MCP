package coordinator

import (
	"context"
	"log/slog"
	"os"
	"testing"
)

func TestMockWorkerClient_ExecuteTask_Echo(t *testing.T) {
	worker := NewMockWorkerClient()

	result, err := worker.ExecuteTask(context.Background(), "workspace1", "echo", TaskArgs{
		"message": "hello world",
	})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if !result.Success {
		t.Error("Expected success=true")
	}
	if result.Output != "hello world" {
		t.Errorf("Expected output 'hello world', got %s", result.Output)
	}
	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}
}

func TestMockWorkerClient_ExecuteTask_FsRead(t *testing.T) {
	worker := NewMockWorkerClient()

	result, err := worker.ExecuteTask(context.Background(), "workspace1", "fs.read", TaskArgs{
		"path": "test.txt",
	})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if !result.Success {
		t.Error("Expected success=true")
	}
	if result.Output != "Content of test.txt in workspace workspace1" {
		t.Errorf("Unexpected output: %s", result.Output)
	}
}

func TestMockWorkerClient_ExecuteTask_FsWrite(t *testing.T) {
	worker := NewMockWorkerClient()

	result, err := worker.ExecuteTask(context.Background(), "workspace1", "fs.write", TaskArgs{
		"path":     "test.txt",
		"contents": "hello",
	})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if !result.Success {
		t.Error("Expected success=true")
	}
	if result.Output != "Wrote 5 bytes to test.txt" {
		t.Errorf("Unexpected output: %s", result.Output)
	}
}

func TestMockWorkerClient_ExecuteTask_UnknownTool(t *testing.T) {
	worker := NewMockWorkerClient()

	_, err := worker.ExecuteTask(context.Background(), "workspace1", "unknown", TaskArgs{})

	if err == nil {
		t.Fatal("Expected error for unknown tool")
	}
}

func TestMockWorkerClient_ExecuteTask_InvalidArgs(t *testing.T) {
	worker := NewMockWorkerClient()

	// Missing message argument
	_, err := worker.ExecuteTask(context.Background(), "workspace1", "echo", TaskArgs{})
	if err == nil {
		t.Error("Expected error for missing message argument")
	}

	// Wrong type for path
	_, err = worker.ExecuteTask(context.Background(), "workspace1", "fs.read", TaskArgs{
		"path": 123, // Should be string
	})
	if err == nil {
		t.Error("Expected error for wrong argument type")
	}
}

func TestAuditLogger_LogToolCall(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := NewAuditLogger(logger)

	// Should not panic
	audit.LogToolCall(context.Background(), &AuditEntry{
		SessionID:   "session1",
		UserID:      "user1",
		ToolName:    "echo",
		WorkspaceID: "workspace1",
	})
}

func TestAuditLogger_LogToolResult(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := NewAuditLogger(logger)

	// Should not panic with result
	audit.LogToolResult(context.Background(), &AuditEntry{
		SessionID: "session1",
		ToolName:  "echo",
		Result: &TaskResult{
			Success:  true,
			Output:   "test",
			ExitCode: 0,
		},
	})

	// Should not panic with error
	audit.LogToolResult(context.Background(), &AuditEntry{
		SessionID: "session1",
		ToolName:  "echo",
		ErrorMsg:  "test error",
	})
}
