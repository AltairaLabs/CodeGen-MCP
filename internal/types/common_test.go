package types

import (
	"testing"
)

func TestSessionStruct(t *testing.T) {
	session := &Session{
		ID:          "test-id",
		UserID:      "user-1",
		WorkspaceID: "workspace-1",
		WorkerID:    "worker-1",
	}

	if session.ID != "test-id" {
		t.Errorf("Expected ID 'test-id', got %s", session.ID)
	}
	if session.UserID != "user-1" {
		t.Errorf("Expected UserID 'user-1', got %s", session.UserID)
	}
	if session.WorkspaceID != "workspace-1" {
		t.Errorf("Expected WorkspaceID 'workspace-1', got %s", session.WorkspaceID)
	}
	if session.WorkerID != "worker-1" {
		t.Errorf("Expected WorkerID 'worker-1', got %s", session.WorkerID)
	}
}

func TestTaskStruct(t *testing.T) {
	task := &Task{
		ID:       "task-123",
		Sequence: 42,
	}

	if task.ID != "task-123" {
		t.Errorf("Expected ID 'task-123', got %s", task.ID)
	}
	if task.Sequence != 42 {
		t.Errorf("Expected Sequence 42, got %d", task.Sequence)
	}
}

func TestAuditEntryStruct(t *testing.T) {
	entry := &AuditEntry{
		SessionID:   "session-1",
		UserID:      "user-1",
		ToolName:    "echo",
		Arguments:   map[string]interface{}{"message": "hello"},
		WorkspaceID: "workspace-1",
		ErrorMsg:    "",
	}

	if entry.SessionID != "session-1" {
		t.Errorf("Expected SessionID 'session-1', got %s", entry.SessionID)
	}
	if entry.ToolName != "echo" {
		t.Errorf("Expected ToolName 'echo', got %s", entry.ToolName)
	}
	if entry.Arguments["message"] != "hello" {
		t.Errorf("Expected message 'hello', got %v", entry.Arguments["message"])
	}
}

func TestTaskResponseStruct(t *testing.T) {
	response := &TaskResponse{
		TaskID:    "task-1",
		Status:    "queued",
		SessionID: "session-1",
		Sequence:  1,
		Message:   "Task queued successfully",
	}

	if response.TaskID != "task-1" {
		t.Errorf("Expected TaskID 'task-1', got %s", response.TaskID)
	}
	if response.Status != "queued" {
		t.Errorf("Expected Status 'queued', got %s", response.Status)
	}
}
