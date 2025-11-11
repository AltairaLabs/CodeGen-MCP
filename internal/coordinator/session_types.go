package coordinator

import (
	"time"
)

// SessionState represents the lifecycle state of a session
type SessionState string

const (
	// SessionStateCreating indicates the session is being created on the worker
	SessionStateCreating SessionState = "creating"
	// SessionStateReady indicates the session is ready to accept tasks
	SessionStateReady SessionState = "ready"
	// SessionStateFailed indicates session creation failed on the worker
	SessionStateFailed SessionState = "failed"
	// SessionStateTerminating indicates the session is being shut down
	SessionStateTerminating SessionState = "terminating"
)

// Session represents an MCP client session with workspace isolation
type Session struct {
	ID          string
	WorkspaceID string
	UserID      string
	CreatedAt   time.Time
	LastActive  time.Time
	Metadata    map[string]string
	// Worker routing and state
	WorkerID        string       // Assigned worker (session affinity)
	WorkerSessionID string       // Session ID on the worker
	State           SessionState // Current lifecycle state
	StateMessage    string       // Optional state details (e.g., error message)
	LastCheckpoint  string       // Last checkpoint ID for recovery
	TaskHistory     []string     // Recent task IDs
}
