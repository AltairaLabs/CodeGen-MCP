package coordinator

import (
	"time"
)

// AuditEntry represents a logged event for provenance tracking
type AuditEntry struct {
	Timestamp   time.Time
	SessionID   string
	UserID      string
	ToolName    string
	Arguments   map[string]interface{}
	Result      *TaskResult
	ErrorMsg    string
	TraceID     string
	WorkspaceID string
}
