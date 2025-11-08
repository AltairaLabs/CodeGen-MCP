package worker

import (
	"context"
	"fmt"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

// WorkerServer implements the gRPC services for the worker
type WorkerServer struct {
	protov1.UnimplementedSessionManagementServer
	protov1.UnimplementedTaskExecutionServer
	protov1.UnimplementedArtifactServiceServer

	workerID     string
	sessionPool  *SessionPool
	taskExecutor *TaskExecutor
	checkpointer *Checkpointer
}

// NewWorkerServer creates a new worker server
func NewWorkerServer(workerID string, maxSessions int32, baseWorkspace string) *WorkerServer {
	sessionPool := NewSessionPool(workerID, maxSessions, baseWorkspace)
	taskExecutor := NewTaskExecutor(sessionPool)
	checkpointer := NewCheckpointer(sessionPool, baseWorkspace)

	return &WorkerServer{
		workerID:     workerID,
		sessionPool:  sessionPool,
		taskExecutor: taskExecutor,
		checkpointer: checkpointer,
	}
}

// CreateSession implements SessionManagement.CreateSession
func (ws *WorkerServer) CreateSession(ctx context.Context, req *protov1.CreateSessionRequest) (*protov1.CreateSessionResponse, error) {
	return ws.sessionPool.CreateSession(ctx, req)
}

// DestroySession implements SessionManagement.DestroySession
func (ws *WorkerServer) DestroySession(ctx context.Context, req *protov1.DestroySessionRequest) (*protov1.DestroySessionResponse, error) {
	var checkpointID string
	var err error

	// Checkpoint if requested
	if req.SaveCheckpoint {
		checkpointID, err = ws.checkpointer.CreateCheckpoint(ctx, req.SessionId, false)
		if err != nil {
			return nil, fmt.Errorf("failed to checkpoint session: %w", err)
		}
	}

	// Get artifacts before destroying
	session, err := ws.sessionPool.GetSession(req.SessionId)
	if err != nil {
		return nil, err
	}
	session.mu.RLock()
	artifacts := make([]string, len(session.ArtifactIDs))
	copy(artifacts, session.ArtifactIDs)
	session.mu.RUnlock()

	// Destroy session
	if err := ws.sessionPool.DestroySession(ctx, req.SessionId, req.SaveCheckpoint); err != nil {
		return nil, err
	}

	return &protov1.DestroySessionResponse{
		Destroyed:    true,
		CheckpointId: checkpointID,
		Artifacts:    artifacts,
	}, nil
}

// CheckpointSession implements SessionManagement.CheckpointSession
func (ws *WorkerServer) CheckpointSession(ctx context.Context, req *protov1.CheckpointRequest) (*protov1.CheckpointResponse, error) {
	return ws.checkpointer.Checkpoint(ctx, req)
}

// RestoreSession implements SessionManagement.RestoreSession
func (ws *WorkerServer) RestoreSession(ctx context.Context, req *protov1.RestoreRequest) (*protov1.RestoreResponse, error) {
	return ws.checkpointer.Restore(ctx, req)
}

// GetSessionStatus implements SessionManagement.GetSessionStatus
func (ws *WorkerServer) GetSessionStatus(ctx context.Context, req *protov1.SessionStatusRequest) (*protov1.SessionStatusResponse, error) {
	session, err := ws.sessionPool.GetSession(req.SessionId)
	if err != nil {
		return nil, err
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	sessionInfo := &protov1.SessionInfo{
		SessionId:      session.SessionID,
		WorkspaceId:    session.WorkspaceID,
		CreatedAtMs:    session.CreatedAt.UnixMilli(),
		LastActivityMs: session.LastActivity.UnixMilli(),
		State:          session.State,
		Usage:          session.ResourceUsage,
		ActiveTasks:    session.ActiveTasks,
	}

	return &protov1.SessionStatusResponse{
		SessionId:        req.SessionId,
		WorkerId:         ws.workerID,
		State:            session.State,
		Info:             sessionInfo,
		RecentTasks:      session.RecentTasks,
		LastCheckpointId: session.LastCheckpointID,
	}, nil
}

// ExecuteTask implements TaskExecution.ExecuteTask
func (ws *WorkerServer) ExecuteTask(req *protov1.TaskRequest, stream protov1.TaskExecution_ExecuteTaskServer) error {
	return ws.taskExecutor.Execute(stream.Context(), req, stream)
}

// CancelTask implements TaskExecution.CancelTask
func (ws *WorkerServer) CancelTask(ctx context.Context, req *protov1.CancelRequest) (*protov1.CancelResponse, error) {
	return ws.taskExecutor.Cancel(ctx, req)
}

// GetTaskStatus implements TaskExecution.GetTaskStatus
func (ws *WorkerServer) GetTaskStatus(ctx context.Context, req *protov1.StatusRequest) (*protov1.StatusResponse, error) {
	return ws.taskExecutor.GetStatus(ctx, req)
}

// GetUploadURL implements ArtifactService.GetUploadURL
func (ws *WorkerServer) GetUploadURL(ctx context.Context, req *protov1.UploadRequest) (*protov1.UploadResponse, error) {
	// Generate artifact ID
	artifactID := fmt.Sprintf("artifact-%s-%d", req.SessionId, time.Now().UnixNano())

	// For now, use local file storage. In production, this would generate a pre-signed S3/MinIO URL
	artifactPath := fmt.Sprintf("/artifacts/%s/%s", req.SessionId, artifactID)

	return &protov1.UploadResponse{
		UploadUrl:   fmt.Sprintf("file://%s", artifactPath),
		ArtifactId:  artifactID,
		ExpiresAtMs: time.Now().Add(1 * time.Hour).UnixMilli(),
	}, nil
}

// RecordArtifact implements ArtifactService.RecordArtifact
func (ws *WorkerServer) RecordArtifact(ctx context.Context, req *protov1.ArtifactMetadata) (*protov1.ArtifactResponse, error) {
	// Store artifact metadata in session (get session ID from task ID prefix)
	// For now, we'll need the session ID passed separately or tracked via task
	// Let's use the artifact ID to track it

	return &protov1.ArtifactResponse{
		Recorded:    true,
		DownloadUrl: fmt.Sprintf("file:///artifacts/%s", req.ArtifactId),
	}, nil
}

// GetCapacity returns the current session capacity
func (ws *WorkerServer) GetCapacity() *protov1.SessionCapacity {
	return ws.sessionPool.GetCapacity()
}
