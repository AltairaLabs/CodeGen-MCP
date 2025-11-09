package coordinator

import (
	"context"
	"fmt"
	"sync"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

// SessionStateStorage defines the interface for session metadata storage
// This is defined here to avoid import cycles with the storage package
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
	UpdateSessionActivity(ctx context.Context, sessionID string) error
}

// SessionManager manages MCP client sessions and workspace isolation
type SessionManager struct {
	sessions       map[string]*Session
	mu             sync.RWMutex
	storage        SessionStateStorage // Optional storage backend
	workerRegistry *WorkerRegistry
}

// NewSessionManager creates a new session manager
// workerRegistry can be nil for backward compatibility with tests
func NewSessionManager(workerRegistry ...*WorkerRegistry) *SessionManager {
	sm := &SessionManager{
		sessions: make(map[string]*Session),
	}
	if len(workerRegistry) > 0 && workerRegistry[0] != nil {
		sm.workerRegistry = workerRegistry[0]
	}
	return sm
}

// NewSessionManagerWithStorage creates a session manager with custom storage backend
func NewSessionManagerWithStorage(
	storageBackend SessionStateStorage,
	workerRegistry *WorkerRegistry,
) *SessionManager {
	return &SessionManager{
		sessions:       make(map[string]*Session), // Keep for legacy direct access
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

	// Store in storage backend if available, otherwise use map
	if sm.storage != nil {
		if err := sm.storage.CreateSession(ctx, session); err != nil {
			// Storage is for persistence/recovery - failure is non-critical
			// Log error but continue with in-memory session
			// TODO: Add proper logging when logger is available in SessionManager
			_ = err
		}
	} else {
		sm.mu.Lock()
		sm.sessions[sessionID] = session
		sm.mu.Unlock()
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
	if sm.storage != nil {
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

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	session, ok := sm.sessions[sessionID]
	if ok {
		// Update last active time (we'll need to lock for write)
		sm.mu.RUnlock()
		sm.mu.Lock()
		session.LastActive = time.Now()
		sm.mu.Unlock()
		sm.mu.RLock()
	}
	return session, ok
}

// DeleteSession removes a session
func (sm *SessionManager) DeleteSession(sessionID string) {
	if sm.storage != nil {
		_ = sm.storage.DeleteSession(context.Background(), sessionID)
		return
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, sessionID)
}

// CleanupStale removes sessions inactive for the specified duration
func (sm *SessionManager) CleanupStale(maxAge time.Duration) int {
	if sm.storage != nil {
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

	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	deleted := 0

	for id, session := range sm.sessions {
		if now.Sub(session.LastActive) > maxAge {
			delete(sm.sessions, id)
			deleted++
		}
	}

	return deleted
}

// SessionCount returns the number of active sessions
func (sm *SessionManager) SessionCount() int {
	if sm.storage != nil {
		sessions, err := sm.storage.ListSessions(context.Background())
		if err != nil {
			return 0
		}
		return len(sessions)
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.sessions)
}

// UpdateSessionState updates the state of a session
func (sm *SessionManager) UpdateSessionState(sessionID string, state SessionState, message string) error {
	if sm.storage != nil {
		return sm.storage.UpdateSessionState(context.Background(), sessionID, state, message)
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, ok := sm.sessions[sessionID]
	if !ok {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	session.State = state
	session.StateMessage = message
	return nil
}

// GetAllSessions returns all active sessions
// This replaces direct access to sm.sessions map
func (sm *SessionManager) GetAllSessions() map[string]*Session {
	if sm.storage != nil {
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

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make(map[string]*Session, len(sm.sessions))
	for id, session := range sm.sessions {
		result[id] = session
	}
	return result
}
