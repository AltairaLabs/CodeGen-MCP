package coordinator

import (
	"context"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/storage"
	"github.com/AltairaLabs/codegen-mcp/internal/types"
)

// sessionManagerAdapter adapts SessionManager to types.SessionManagerWithWorkers
type sessionManagerAdapter struct {
	sm             *SessionManager
	workerRegistry *WorkerRegistry
}

func newSessionManagerAdapter(sm *SessionManager, wr *WorkerRegistry) types.SessionManagerWithWorkers {
	return &sessionManagerAdapter{sm: sm, workerRegistry: wr}
}

func (a *sessionManagerAdapter) GetSession(sessionID string) (*types.Session, bool) {
	session, ok := a.sm.GetSession(sessionID)
	if !ok {
		return nil, false
	}
	return &types.Session{
		ID:          session.ID,
		UserID:      session.UserID,
		WorkspaceID: session.WorkspaceID,
		WorkerID:    session.WorkerID,
	}, true
}

func (a *sessionManagerAdapter) CreateSession(ctx context.Context, sessionID, userID, workspaceID string) *types.Session {
	session := a.sm.CreateSession(ctx, sessionID, userID, workspaceID)
	if session == nil {
		return nil
	}
	return &types.Session{
		ID:          session.ID,
		UserID:      session.UserID,
		WorkspaceID: session.WorkspaceID,
		WorkerID:    session.WorkerID,
	}
}

func (a *sessionManagerAdapter) HasWorkersAvailable() bool {
	if a.workerRegistry == nil {
		return false
	}
	_, _, available := a.workerRegistry.GetTotalCapacity()
	return available > 0
}

// taskQueueAdapter adapts taskqueue.TaskQueueInterface to types.TaskQueueInterface
type taskQueueAdapter struct {
	tq interface {
		EnqueueTypedTask(ctx context.Context, sessionID string, request *protov1.ToolRequest) (string, error)
		GetTask(ctx context.Context, taskID string) (*storage.QueuedTask, error)
	}
}

func newTaskQueueAdapter(tq interface {
	EnqueueTypedTask(ctx context.Context, sessionID string, request *protov1.ToolRequest) (string, error)
	GetTask(ctx context.Context, taskID string) (*storage.QueuedTask, error)
}) types.TaskQueueInterface {
	return &taskQueueAdapter{tq: tq}
}

func (a *taskQueueAdapter) EnqueueTypedTask(ctx context.Context, sessionID string, request *protov1.ToolRequest) (string, error) {
	return a.tq.EnqueueTypedTask(ctx, sessionID, request)
}

func (a *taskQueueAdapter) GetTask(ctx context.Context, taskID string) (*types.Task, error) {
	queuedTask, err := a.tq.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	// Safely convert uint64 sequence to int, limiting to int32 range to prevent overflow
	sequence := queuedTask.Sequence
	const maxInt32 uint64 = 1 << 31
	if sequence > maxInt32 {
		sequence %= maxInt32
	}
	return &types.Task{
		ID:       queuedTask.ID,
		Sequence: int(sequence),
	}, nil
}

// auditLoggerAdapter adapts AuditLogger to types.AuditLogger
type auditLoggerAdapter struct {
	al *AuditLogger
}

func newAuditLoggerAdapter(al *AuditLogger) types.AuditLogger {
	return &auditLoggerAdapter{al: al}
}

func (a *auditLoggerAdapter) LogToolCall(ctx context.Context, entry *types.AuditEntry) {
	a.al.LogToolCall(ctx, &AuditEntry{
		SessionID:   entry.SessionID,
		UserID:      entry.UserID,
		ToolName:    entry.ToolName,
		Arguments:   TaskArgs(entry.Arguments),
		WorkspaceID: entry.WorkspaceID,
	})
}

func (a *auditLoggerAdapter) LogToolResult(ctx context.Context, entry *types.AuditEntry) {
	a.al.LogToolResult(ctx, &AuditEntry{
		SessionID:   entry.SessionID,
		UserID:      entry.UserID,
		ToolName:    entry.ToolName,
		Arguments:   TaskArgs(entry.Arguments),
		WorkspaceID: entry.WorkspaceID,
		ErrorMsg:    entry.ErrorMsg,
	})
}

// resultStreamerAdapter adapts ResultStreamer to types.ResultStreamer
type resultStreamerAdapter struct {
	rs *ResultStreamer
}

func newResultStreamerAdapter(rs *ResultStreamer) types.ResultStreamer {
	return &resultStreamerAdapter{rs: rs}
}

func (a *resultStreamerAdapter) Subscribe(taskID, sessionID string) {
	a.rs.Subscribe(taskID, sessionID)
}
