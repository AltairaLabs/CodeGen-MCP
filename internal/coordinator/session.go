package coordinator

import (
	"context"
	"fmt"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

// SessionStateStorage defines the interface for session metadata storage.
// This is defined here (not in storage package) to avoid import cycles:
// - coordinator package needs this interface to define SessionManager
// - storage implementations need to import coordinator for Session type
// Storage implementations in internal/coordinator/storage/memory (and future Redis/DB)
// directly implement this interface.
type SessionStateStorage interface {
	CreateSession(ctx context.Context, session *Session) error
	GetSession(ctx context.Context, sessionID string) (*Session, error)
	UpdateSessionState(ctx context.Context, sessionID string, state SessionState, message string) error
	SetSessionMetadata(ctx context.Context, sessionID string, metadata map[string]string) error
	GetSessionMetadata(ctx context.Context, sessionID string) (map[string]string, error)
	GetSessionByConversationID(ctx context.Context, conversationID string) (*Session, error)
	DeleteSession(ctx context.Context, sessionID string) error
	GetLastCompletedSequence(ctx context.Context, sessionID string) (uint64, error)
	SetLastCompletedSequence(ctx context.Context, sessionID string, sequence uint64) error
	GetNextSequence(ctx context.Context, sessionID string) (uint64, error)
	ListSessions(ctx context.Context) ([]*Session, error)
	ListSessionsByWorkerID(ctx context.Context, workerID string) ([]*Session, error)
	UpdateSessionActivity(ctx context.Context, sessionID string) error
}

// SessionManager manages MCP client sessions and workspace isolation
type SessionManager struct {
	storage        SessionStateStorage // Storage backend (required)
	workerRegistry *WorkerRegistry
}

// NewSessionManager creates a new session manager with storage backend
func NewSessionManager(
	storageBackend SessionStateStorage,
	workerRegistry *WorkerRegistry,
) *SessionManager {
	return &SessionManager{
		storage:        storageBackend,
		workerRegistry: workerRegistry,
	}
}

// CreateSession creates a new session for an MCP client and assigns a worker
func (sm *SessionManager) CreateSession(ctx context.Context, sessionID, userID, workspaceID string) *Session {
	session := &Session{
		ID:          sessionID,
		WorkspaceID: workspaceID,
		UserID:      userID,
		CreatedAt:   time.Now(),
		LastActive:  time.Now(),
		Metadata:    make(map[string]string),
		State:       SessionStateCreating, // Start in creating state
	}

	// If worker registry is available, try to assign a worker
	if sm.workerRegistry != nil {
		worker, err := sm.assignWorkerToSession(ctx, session)
		if err == nil && worker != nil {
			session.WorkerID = worker.WorkerID

			// Create session on the worker (async)
			workerSessionID, createErr := sm.createWorkerSession(ctx, worker, session)
			if createErr == nil {
				session.WorkerSessionID = workerSessionID
				// State remains "creating" until worker confirms
			} else {
				session.State = SessionStateFailed
				session.StateMessage = createErr.Error()
			}
		} else {
			session.State = SessionStateFailed
			if err != nil {
				session.StateMessage = err.Error()
			} else {
				session.StateMessage = "no worker available"
			}
		}
	}

	// Store in storage backend
	if err := sm.storage.CreateSession(ctx, session); err != nil {
		// Update session state to failed if storage fails
		session.State = SessionStateFailed
		session.StateMessage = fmt.Sprintf("storage error: %v", err)
	}

	return session
}

// assignWorkerToSession selects a worker with available capacity
func (sm *SessionManager) assignWorkerToSession(ctx context.Context, _ *Session) (*RegisteredWorker, error) {
	// Use default session config for now
	// In a real implementation, this could be customized per session
	return sm.workerRegistry.FindWorkerWithCapacity(ctx, nil)
}

// createWorkerSession creates a session on the worker via task stream (async)
func (sm *SessionManager) createWorkerSession(
	ctx context.Context,
	worker *RegisteredWorker,
	session *Session,
) (string, error) {
	// Generate unique worker session ID
	workerSessionID := fmt.Sprintf("ws-%s-%d", session.ID, time.Now().UnixNano())

	if worker.TaskStream == nil {
		return "", fmt.Errorf("worker %s has no active task stream", worker.WorkerID)
	}

	// Send session creation message to worker via task stream (fire and forget)
	createMsg := &protov1.TaskStreamMessage{
		Message: &protov1.TaskStreamMessage_SessionCreate{
			SessionCreate: &protov1.SessionCreateRequest{
				SessionId:   workerSessionID,
				WorkspaceId: session.WorkspaceID,
				UserId:      session.UserID,
				EnvVars:     make(map[string]string), // TODO: populate from session config
			},
		},
	}

	if err := worker.TaskStream.Send(createMsg); err != nil {
		return "", fmt.Errorf("failed to send session creation to worker: %w", err)
	}

	// Return immediately - session creation happens asynchronously
	// The worker will create the session and send confirmation, but we don't wait
	// Tool calls will wait briefly if session doesn't exist yet
	return workerSessionID, nil
}

// GetSession retrieves a session by ID
func (sm *SessionManager) GetSession(sessionID string) (*Session, bool) {
	session, err := sm.storage.GetSession(context.Background(), sessionID)
	if err != nil {
		return nil, false
	}
	if session != nil {
		// Update last active time
		_ = sm.storage.UpdateSessionActivity(context.Background(), sessionID)
		return session, true
	}
	return nil, false
}

// GetSessionsByWorkerID retrieves all sessions assigned to a worker
func (sm *SessionManager) GetSessionsByWorkerID(workerID string) ([]*Session, error) {
	return sm.storage.ListSessionsByWorkerID(context.Background(), workerID)
}

// DeleteSession removes a session
func (sm *SessionManager) DeleteSession(sessionID string) {
	_ = sm.storage.DeleteSession(context.Background(), sessionID)
}

// CleanupStale removes sessions inactive for the specified duration
func (sm *SessionManager) CleanupStale(maxAge time.Duration) int {
	// Get all sessions and delete stale ones
	sessions, err := sm.storage.ListSessions(context.Background())
	if err != nil {
		return 0
	}
	now := time.Now()
	deleted := 0
	for _, session := range sessions {
		if now.Sub(session.LastActive) > maxAge {
			_ = sm.storage.DeleteSession(context.Background(), session.ID)
			deleted++
		}
	}
	return deleted
}

// SessionCount returns the number of active sessions
func (sm *SessionManager) SessionCount() int {
	sessions, err := sm.storage.ListSessions(context.Background())
	if err != nil {
		return 0
	}
	return len(sessions)
}

// UpdateSessionState updates the state of a session
func (sm *SessionManager) UpdateSessionState(sessionID string, state SessionState, message string) error {
	return sm.storage.UpdateSessionState(context.Background(), sessionID, state, message)
}

// GetAllSessions returns all active sessions
// This replaces direct access to sm.sessions map
func (sm *SessionManager) GetAllSessions() map[string]*Session {
	sessions, err := sm.storage.ListSessions(context.Background())
	if err != nil {
		return make(map[string]*Session)
	}
	result := make(map[string]*Session, len(sessions))
	for _, session := range sessions {
		result[session.ID] = session
	}
	return result
}

// GetNextSequence atomically increments and returns the next sequence number for a session
func (sm *SessionManager) GetNextSequence(ctx context.Context, sessionID string) uint64 {
	sequence, err := sm.storage.GetNextSequence(ctx, sessionID)
	if err != nil {
		// Return 0 if error occurred
		return 0
	}
	return sequence
}

// SetLastCompletedSequence sets the last completed sequence for a session
func (sm *SessionManager) SetLastCompletedSequence(ctx context.Context, sessionID string, sequence uint64) error {
	return sm.storage.SetLastCompletedSequence(ctx, sessionID, sequence)
}
