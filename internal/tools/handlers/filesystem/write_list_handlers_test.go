package filesystem

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestWriteHandler(t *testing.T) {
	mockSession := &mockSessionManager{workersAvailable: true}
	mockQueue := &mockTaskQueue{}
	mockAudit := &mockAuditLogger{}
	mockStreamer := &mockResultStreamer{}
	handler := NewWriteHandler(mockSession, mockQueue, mockAudit, mockStreamer)

	// Test successful write
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "fs.write",
			Arguments: map[string]interface{}{
				"path":     "test.txt",
				"contents": "hello world",
			},
		},
	}

	ctx := context.WithValue(context.Background(), "session_id", "test-session")
	result, err := handler.Handle(ctx, request)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Verify task was enqueued
	if len(mockQueue.tasks) == 0 {
		t.Error("Expected task to be enqueued")
	}
}

func TestWriteHandler_InvalidPath(t *testing.T) {
	mockSession := &mockSessionManager{workersAvailable: true}
	mockQueue := &mockTaskQueue{}
	mockAudit := &mockAuditLogger{}
	mockStreamer := &mockResultStreamer{}
	handler := NewWriteHandler(mockSession, mockQueue, mockAudit, mockStreamer)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "fs.write",
			Arguments: map[string]interface{}{
				"path":     "../../../etc/passwd",
				"contents": "malicious",
			},
		},
	}

	result, err := handler.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result.IsError != true {
		t.Error("Expected error result for path traversal")
	}
}

func TestListHandler(t *testing.T) {
	mockSession := &mockSessionManager{workersAvailable: true}
	mockQueue := &mockTaskQueue{}
	mockAudit := &mockAuditLogger{}
	mockStreamer := &mockResultStreamer{}
	handler := NewListHandler(mockSession, mockQueue, mockAudit, mockStreamer)

	// Test successful list
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "fs.list",
			Arguments: map[string]interface{}{
				"path": "subdir",
			},
		},
	}

	ctx := context.WithValue(context.Background(), "session_id", "test-session")
	result, err := handler.Handle(ctx, request)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Verify task was enqueued
	if len(mockQueue.tasks) == 0 {
		t.Error("Expected task to be enqueued")
	}
}

func TestListHandler_InvalidPath(t *testing.T) {
	mockSession := &mockSessionManager{workersAvailable: true}
	mockQueue := &mockTaskQueue{}
	mockAudit := &mockAuditLogger{}
	mockStreamer := &mockResultStreamer{}
	handler := NewListHandler(mockSession, mockQueue, mockAudit, mockStreamer)

	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "fs.list",
			Arguments: map[string]interface{}{
				"path": "../../../etc",
			},
		},
	}

	result, err := handler.Handle(context.Background(), request)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result.IsError != true {
		t.Error("Expected error result for path traversal")
	}
}
