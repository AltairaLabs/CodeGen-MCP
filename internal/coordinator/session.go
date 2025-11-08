package coordinator

import (
	"context"
	"sync"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

// SessionManager manages MCP client sessions and workspace isolation
type SessionManager struct {
	sessions       map[string]*Session
	mu             sync.RWMutex
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

// CreateSession creates a new session for an MCP client and assigns a worker
func (sm *SessionManager) CreateSession(ctx context.Context, sessionID, userID, workspaceID string) *Session {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session := &Session{
		ID:          sessionID,
		WorkspaceID: workspaceID,
		UserID:      userID,
		CreatedAt:   time.Now(),
		LastActive:  time.Now(),
		Metadata:    make(map[string]string),
	}

	// If worker registry is available, try to assign a worker
	if sm.workerRegistry != nil {
		worker, err := sm.assignWorkerToSession(ctx, session)
		if err == nil && worker != nil {
			session.WorkerID = worker.WorkerID

			// Create session on the worker
			workerSessionID, err := sm.createWorkerSession(ctx, worker, session)
			if err == nil {
				session.WorkerSessionID = workerSessionID
			}
		}
	}

	sm.sessions[sessionID] = session
	return session
}

// assignWorkerToSession selects a worker with available capacity
func (sm *SessionManager) assignWorkerToSession(ctx context.Context, _ *Session) (*RegisteredWorker, error) {
	// Use default session config for now
	// In a real implementation, this could be customized per session
	return sm.workerRegistry.FindWorkerWithCapacity(ctx, nil)
}

// createWorkerSession calls the worker's CreateSession RPC
func (sm *SessionManager) createWorkerSession(
	ctx context.Context,
	worker *RegisteredWorker,
	session *Session,
) (string, error) {
	// Import protov1 needed at package level
	resp, err := worker.Client.SessionMgmt.CreateSession(ctx, &protov1.CreateSessionRequest{
		WorkerId:    worker.WorkerID,
		WorkspaceId: session.WorkspaceID,
		UserId:      session.UserID,
		Config:      &protov1.SessionConfig{},
	})

	if err != nil {
		return "", err
	}

	return resp.SessionId, nil
}

// GetSession retrieves a session by ID
func (sm *SessionManager) GetSession(sessionID string) (*Session, bool) {
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
	sm.mu.Lock()
	defer sm.mu.Unlock()

	delete(sm.sessions, sessionID)
}

// CleanupStale removes sessions inactive for the specified duration
func (sm *SessionManager) CleanupStale(maxAge time.Duration) int {
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
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return len(sm.sessions)
}
