// Package types provides shared types used across the codegen-mcp codebase
package types

import (
	"context"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

// Session represents a user session with workspace and worker assignment
type Session struct {
	ID          string
	UserID      string
	WorkspaceID string
	WorkerID    string
}

// Task represents a task in the queue with ID and sequence number
type Task struct {
	ID       string
	Sequence int
}

// AuditEntry represents an audit log entry for tool calls and results
type AuditEntry struct {
	SessionID   string
	UserID      string
	ToolName    string
	Arguments   map[string]interface{}
	WorkspaceID string
	ErrorMsg    string
}

// TaskResponse represents the response structure for async tasks
type TaskResponse struct {
	TaskID    string    `json:"task_id"`
	Status    string    `json:"status"`
	SessionID string    `json:"session_id"`
	Sequence  int       `json:"sequence"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// SessionManager provides session management operations
type SessionManager interface {
	GetSession(sessionID string) (*Session, bool)
	CreateSession(ctx context.Context, sessionID, userID, workspaceID string) *Session
}

// SessionManagerWithWorkers extends SessionManager with worker availability checking
type SessionManagerWithWorkers interface {
	SessionManager
	HasWorkersAvailable() bool
}

// TaskQueueInterface provides task queue operations
type TaskQueueInterface interface {
	EnqueueTypedTask(ctx context.Context, sessionID string, request *protov1.ToolRequest) (string, error)
	GetTask(ctx context.Context, taskID string) (*Task, error)
}

// AuditLogger provides audit logging operations
type AuditLogger interface {
	LogToolCall(ctx context.Context, entry *AuditEntry)
	LogToolResult(ctx context.Context, entry *AuditEntry)
}

// ResultStreamer provides result streaming subscription
type ResultStreamer interface {
	Subscribe(taskID, sessionID string)
}
