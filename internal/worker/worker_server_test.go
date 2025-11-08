package worker

import (
	"context"
	"testing"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

func TestWorkerServerCreation(t *testing.T) {
	baseWorkspace := t.TempDir()
	server := NewWorkerServer("worker-1", 5, baseWorkspace)

	if server == nil {
		t.Fatal("Expected non-nil server")
	}

	if server.workerID != "worker-1" {
		t.Errorf("Expected worker ID 'worker-1', got '%s'", server.workerID)
	}
}

func TestWorkerServerCreateSession(t *testing.T) {
	baseWorkspace := t.TempDir()
	server := NewWorkerServer("worker-1", 5, baseWorkspace)

	req := &protov1.CreateSessionRequest{
		WorkspaceId: "ws-1",
		UserId:      "user-1",
	}

	resp, err := server.CreateSession(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if resp.SessionId == "" {
		t.Error("Expected non-empty session ID")
	}
}

func TestWorkerServerDestroySession(t *testing.T) {
	baseWorkspace := t.TempDir()
	server := NewWorkerServer("worker-1", 5, baseWorkspace)

	createResp, _ := server.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "ws-1",
		UserId:      "user-1",
	})

	req := &protov1.DestroySessionRequest{
		SessionId: createResp.SessionId,
	}

	resp, err := server.DestroySession(context.Background(), req)
	if err != nil {
		t.Fatalf("DestroySession failed: %v", err)
	}

	if !resp.Destroyed {
		t.Error("Expected successful destroy")
	}
}

func TestWorkerServerGetSessionStatus(t *testing.T) {
	baseWorkspace := t.TempDir()
	server := NewWorkerServer("worker-1", 5, baseWorkspace)

	createResp, _ := server.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "ws-1",
		UserId:      "user-1",
	})

	req := &protov1.SessionStatusRequest{
		SessionId: createResp.SessionId,
	}

	resp, err := server.GetSessionStatus(context.Background(), req)
	if err != nil {
		t.Fatalf("GetSessionStatus failed: %v", err)
	}

	if resp.SessionId != createResp.SessionId {
		t.Errorf("Expected session ID '%s', got '%s'", createResp.SessionId, resp.SessionId)
	}
}

func TestWorkerServerCheckpointSession(t *testing.T) {
	baseWorkspace := t.TempDir()
	server := NewWorkerServer("worker-1", 5, baseWorkspace)

	createResp, _ := server.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "ws-1",
		UserId:      "user-1",
	})

	req := &protov1.CheckpointRequest{
		SessionId:   createResp.SessionId,
		Incremental: false,
	}

	resp, err := server.CheckpointSession(context.Background(), req)
	if err != nil {
		t.Fatalf("CheckpointSession failed: %v", err)
	}

	if resp.CheckpointId == "" {
		t.Error("Expected non-empty checkpoint ID")
	}
}

func TestWorkerServerGetCapacity(t *testing.T) {
	baseWorkspace := t.TempDir()
	server := NewWorkerServer("worker-1", 5, baseWorkspace)

	capacity := server.GetCapacity()
	if capacity == nil {
		t.Fatal("Expected non-nil capacity")
	}

	if capacity.TotalSessions != 5 {
		t.Errorf("Expected total sessions 5, got %d", capacity.TotalSessions)
	}
}

func TestWorkerServerRestoreSession(t *testing.T) {
	baseWorkspace := t.TempDir()
	server := NewWorkerServer("worker-1", 5, baseWorkspace)

	// Create a session and checkpoint it
	createResp, _ := server.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "ws-1",
		UserId:      "user-1",
	})

	// Create checkpoint
	checkpointResp, err := server.CheckpointSession(context.Background(), &protov1.CheckpointRequest{
		SessionId:   createResp.SessionId,
		Incremental: false,
	})
	if err != nil {
		t.Fatalf("Failed to create checkpoint: %v", err)
	}

	// Restore from checkpoint
	restoreResp, err := server.RestoreSession(context.Background(), &protov1.RestoreRequest{
		CheckpointId: checkpointResp.CheckpointId,
	})

	if err != nil {
		t.Fatalf("Failed to restore session: %v", err)
	}

	if restoreResp.SessionId == "" {
		t.Error("Expected non-empty restored session ID")
	}

	if restoreResp.WorkerId != "worker-1" {
		t.Errorf("Expected worker ID 'worker-1', got '%s'", restoreResp.WorkerId)
	}
}

func TestWorkerServerExecuteTask(t *testing.T) {
	baseWorkspace := t.TempDir()
	server := NewWorkerServer("worker-1", 5, baseWorkspace)

	createResp, _ := server.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "ws-1",
		UserId:      "user-1",
	})

	req := &protov1.TaskRequest{
		TaskId:    "task-1",
		SessionId: createResp.SessionId,
		ToolName:  "echo",
		Arguments: map[string]string{"message": "hello"},
	}

	mockStream := &mockTaskStream{}
	err := server.ExecuteTask(req, mockStream)
	if err != nil {
		t.Fatalf("ExecuteTask failed: %v", err)
	}

	if len(mockStream.responses) == 0 {
		t.Error("Expected at least one response")
	}
}

func TestWorkerServerCancelTask(t *testing.T) {
	baseWorkspace := t.TempDir()
	server := NewWorkerServer("worker-1", 5, baseWorkspace)

	createResp, _ := server.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "ws-1",
		UserId:      "user-1",
	})

	req := &protov1.CancelRequest{
		TaskId:    "task-1",
		SessionId: createResp.SessionId,
	}

	resp, err := server.CancelTask(context.Background(), req)
	if err != nil {
		t.Fatalf("CancelTask failed: %v", err)
	}

	// Should return cancelled=false for non-existent task
	if resp.Cancelled {
		t.Error("Expected cancelled=false for non-existent task")
	}
}

func TestWorkerServerGetTaskStatus(t *testing.T) {
	baseWorkspace := t.TempDir()
	server := NewWorkerServer("worker-1", 5, baseWorkspace)

	createResp, _ := server.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "ws-1",
		UserId:      "user-1",
	})

	req := &protov1.StatusRequest{
		TaskId:    "task-1",
		SessionId: createResp.SessionId,
	}

	resp, err := server.GetTaskStatus(context.Background(), req)
	if err != nil {
		t.Fatalf("GetTaskStatus failed: %v", err)
	}

	if resp.Status == protov1.TaskResult_STATUS_UNSPECIFIED {
		// Expected for non-existent task
		t.Log("Task not found as expected")
	}
}

func TestWorkerServerGetUploadURL(t *testing.T) {
	baseWorkspace := t.TempDir()
	server := NewWorkerServer("worker-1", 5, baseWorkspace)

	createResp, _ := server.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "ws-1",
		UserId:      "user-1",
	})

	req := &protov1.UploadRequest{
		SessionId: createResp.SessionId,
		SizeBytes: 100,
	}

	resp, err := server.GetUploadURL(context.Background(), req)
	if err != nil {
		t.Fatalf("GetUploadURL failed: %v", err)
	}

	if resp.UploadUrl == "" {
		t.Error("Expected non-empty upload URL")
	}
}

func TestWorkerServerRecordArtifact(t *testing.T) {
	baseWorkspace := t.TempDir()
	server := NewWorkerServer("worker-1", 5, baseWorkspace)

	createResp, _ := server.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "ws-1",
		UserId:      "user-1",
	})

	metadata := &protov1.ArtifactMetadata{
		ArtifactId:  "artifact-1",
		ContentType: "text/plain",
		SizeBytes:   100,
	}

	resp, err := server.RecordArtifact(context.Background(), metadata)
	if err != nil {
		t.Fatalf("RecordArtifact failed: %v", err)
	}

	if !resp.Recorded {
		t.Error("Expected artifact to be recorded")
	}

	_ = createResp // Use variable
}

func TestWorkerServerDestroyWithCheckpoint(t *testing.T) {
	baseWorkspace := t.TempDir()
	server := NewWorkerServer("worker-1", 5, baseWorkspace)

	createResp, _ := server.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "ws-1",
		UserId:      "user-1",
	})

	// Destroy with checkpoint
	req := &protov1.DestroySessionRequest{
		SessionId:      createResp.SessionId,
		SaveCheckpoint: true,
	}

	resp, err := server.DestroySession(context.Background(), req)
	if err != nil {
		t.Fatalf("DestroySession with checkpoint failed: %v", err)
	}

	if !resp.Destroyed {
		t.Error("Expected session to be destroyed")
	}

	if resp.CheckpointId == "" {
		t.Error("Expected checkpoint ID when SaveCheckpoint is true")
	}
}

func TestWorkerServerMultipleSessions(t *testing.T) {
	baseWorkspace := t.TempDir()
	server := NewWorkerServer("worker-1", 5, baseWorkspace)

	// Create multiple sessions
	sessions := make([]string, 3)
	for i := 0; i < 3; i++ {
		resp, _ := server.CreateSession(context.Background(), &protov1.CreateSessionRequest{
			WorkspaceId: "ws-1",
			UserId:      "user-1",
		})
		sessions[i] = resp.SessionId
	}

	// Check capacity reflects active sessions
	capacity := server.GetCapacity()
	if capacity.ActiveSessions != 3 {
		t.Errorf("Expected 3 active sessions, got %d", capacity.ActiveSessions)
	}

	// Destroy all sessions
	for _, sessionID := range sessions {
		server.DestroySession(context.Background(), &protov1.DestroySessionRequest{
			SessionId: sessionID,
		})
	}

	// Check capacity is back to 0
	capacity = server.GetCapacity()
	if capacity.ActiveSessions != 0 {
		t.Errorf("Expected 0 active sessions after destroy, got %d", capacity.ActiveSessions)
	}
}

func TestWorkerServerErrorPaths(t *testing.T) {
	baseWorkspace := t.TempDir()
	server := NewWorkerServer("worker-1", 5, baseWorkspace)

	// Test status for non-existent session
	_, err := server.GetSessionStatus(context.Background(), &protov1.SessionStatusRequest{
		SessionId: "nonexistent",
	})
	if err == nil {
		t.Error("Expected error for non-existent session status")
	}

	// Test destroy non-existent session
	_, err = server.DestroySession(context.Background(), &protov1.DestroySessionRequest{
		SessionId: "nonexistent",
	})
	if err == nil {
		t.Error("Expected error for destroying non-existent session")
	}

	// Test checkpoint non-existent session
	_, err = server.CheckpointSession(context.Background(), &protov1.CheckpointRequest{
		SessionId: "nonexistent",
	})
	if err == nil {
		t.Error("Expected error for checkpointing non-existent session")
	}
}
