package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator"
)

var (
	errSessionNil      = errors.New("session cannot be nil")
	errSessionIDEmpty2 = errors.New("session ID cannot be empty")
	errSessionNotFound = errors.New("session not found")
)

// InMemorySessionStateStorage implements coordinator.SessionStateStorage using in-memory maps
type InMemorySessionStateStorage struct {
	mu                    sync.RWMutex
	sessions              map[string]*coordinator.Session
	conversationToSession map[string]string // conversationID -> sessionID mapping
	sequences             map[string]uint64 // sessionID -> next sequence number
	completedSequences    map[string]uint64 // sessionID -> last completed sequence
}

// NewInMemorySessionStateStorage creates a new in-memory session state storage
func NewInMemorySessionStateStorage() *InMemorySessionStateStorage {
	return &InMemorySessionStateStorage{
		sessions:              make(map[string]*coordinator.Session),
		conversationToSession: make(map[string]string),
		sequences:             make(map[string]uint64),
		completedSequences:    make(map[string]uint64),
	}
}

// CreateSession creates a new session with initial state
func (s *InMemorySessionStateStorage) CreateSession(
	ctx context.Context,
	session *coordinator.Session,
) error {
	if session == nil {
		return errSessionNil
	}
	if session.ID == "" {
		return errSessionIDEmpty2
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.sessions[session.ID]; exists {
		return fmt.Errorf("session with ID %s already exists", session.ID)
	}

	// Store a copy to prevent external modifications
	sessionCopy := *session
	if session.Metadata != nil {
		sessionCopy.Metadata = make(map[string]string, len(session.Metadata))
		for k, v := range session.Metadata {
			sessionCopy.Metadata[k] = v
		}
	}
	if session.TaskHistory != nil {
		sessionCopy.TaskHistory = make([]string, len(session.TaskHistory))
		copy(sessionCopy.TaskHistory, session.TaskHistory)
	}

	s.sessions[session.ID] = &sessionCopy
	s.sequences[session.ID] = 0
	s.completedSequences[session.ID] = 0

	return nil
}

// GetSession retrieves session by ID
func (s *InMemorySessionStateStorage) GetSession(
	ctx context.Context,
	sessionID string,
) (*coordinator.Session, error) {
	if sessionID == "" {
		return nil, errSessionIDEmpty2
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return nil, nil
	}

	// Return a copy to prevent external modifications
	sessionCopy := *session
	if session.Metadata != nil {
		sessionCopy.Metadata = make(map[string]string, len(session.Metadata))
		for k, v := range session.Metadata {
			sessionCopy.Metadata[k] = v
		}
	}
	if session.TaskHistory != nil {
		sessionCopy.TaskHistory = make([]string, len(session.TaskHistory))
		copy(sessionCopy.TaskHistory, session.TaskHistory)
	}

	return &sessionCopy, nil
}

// UpdateSessionState updates the state of a session
func (s *InMemorySessionStateStorage) UpdateSessionState(
	ctx context.Context,
	sessionID string,
	state coordinator.SessionState,
	message string,
) error {
	if sessionID == "" {
		return errSessionIDEmpty2
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return errSessionNotFound
	}

	session.State = state
	session.StateMessage = message

	return nil
}

// SetSessionMetadata sets arbitrary metadata for a session
func (s *InMemorySessionStateStorage) SetSessionMetadata(
	ctx context.Context,
	sessionID string,
	metadata map[string]string,
) error {
	if sessionID == "" {
		return errSessionIDEmpty2
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return errSessionNotFound
	}

	// Initialize metadata map if needed
	if session.Metadata == nil {
		session.Metadata = make(map[string]string)
	}

	// Copy metadata
	for k, v := range metadata {
		session.Metadata[k] = v
	}

	return nil
}

// GetSessionMetadata retrieves metadata for a session
func (s *InMemorySessionStateStorage) GetSessionMetadata(
	ctx context.Context,
	sessionID string,
) (map[string]string, error) {
	if sessionID == "" {
		return make(map[string]string), nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return make(map[string]string), nil
	}

	// Return a copy
	result := make(map[string]string, len(session.Metadata))
	for k, v := range session.Metadata {
		result[k] = v
	}

	return result, nil
}

// GetSessionByConversationID looks up session by external conversation ID
func (s *InMemorySessionStateStorage) GetSessionByConversationID(
	ctx context.Context,
	conversationID string,
) (*coordinator.Session, error) {
	if conversationID == "" {
		return nil, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	sessionID, exists := s.conversationToSession[conversationID]
	if !exists {
		return nil, nil
	}

	session, exists := s.sessions[sessionID]
	if !exists {
		return nil, nil
	}

	// Return a copy
	sessionCopy := *session
	if session.Metadata != nil {
		sessionCopy.Metadata = make(map[string]string, len(session.Metadata))
		for k, v := range session.Metadata {
			sessionCopy.Metadata[k] = v
		}
	}
	if session.TaskHistory != nil {
		sessionCopy.TaskHistory = make([]string, len(session.TaskHistory))
		copy(sessionCopy.TaskHistory, session.TaskHistory)
	}

	return &sessionCopy, nil
}

// DeleteSession removes a session and its metadata
func (s *InMemorySessionStateStorage) DeleteSession(
	ctx context.Context,
	sessionID string,
) error {
	if sessionID == "" {
		return errSessionIDEmpty2
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove from all maps (idempotent - no error if not found)
	delete(s.sessions, sessionID)
	delete(s.sequences, sessionID)
	delete(s.completedSequences, sessionID)

	// Remove from conversation mapping
	for convID, sessID := range s.conversationToSession {
		if sessID == sessionID {
			delete(s.conversationToSession, convID)
		}
	}

	return nil
}

// GetLastCompletedSequence gets the last successfully completed task sequence number
func (s *InMemorySessionStateStorage) GetLastCompletedSequence(
	ctx context.Context,
	sessionID string,
) (uint64, error) {
	if sessionID == "" {
		return 0, errSessionIDEmpty2
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	seq, exists := s.completedSequences[sessionID]
	if !exists {
		return 0, nil
	}

	return seq, nil
}

// SetLastCompletedSequence updates the last completed sequence number
func (s *InMemorySessionStateStorage) SetLastCompletedSequence(
	ctx context.Context,
	sessionID string,
	sequence uint64,
) error {
	if sessionID == "" {
		return errSessionIDEmpty2
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.sessions[sessionID]; !exists {
		return errSessionNotFound
	}

	s.completedSequences[sessionID] = sequence

	return nil
}

// GetNextSequence atomically increments and returns the next sequence number
func (s *InMemorySessionStateStorage) GetNextSequence(
	ctx context.Context,
	sessionID string,
) (uint64, error) {
	if sessionID == "" {
		return 0, errSessionIDEmpty2
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.sessions[sessionID]; !exists {
		return 0, errSessionNotFound
	}

	s.sequences[sessionID]++
	return s.sequences[sessionID], nil
}

// ListSessions retrieves all active sessions
func (s *InMemorySessionStateStorage) ListSessions(
	ctx context.Context,
) ([]*coordinator.Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*coordinator.Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		// Return copies
		sessionCopy := *session
		if session.Metadata != nil {
			sessionCopy.Metadata = make(map[string]string, len(session.Metadata))
			for k, v := range session.Metadata {
				sessionCopy.Metadata[k] = v
			}
		}
		if session.TaskHistory != nil {
			sessionCopy.TaskHistory = make([]string, len(session.TaskHistory))
			copy(sessionCopy.TaskHistory, session.TaskHistory)
		}
		result = append(result, &sessionCopy)
	}

	return result, nil
}

// ListSessionsByWorkerID retrieves all sessions assigned to a specific worker
func (s *InMemorySessionStateStorage) ListSessionsByWorkerID(
	ctx context.Context,
	workerID string,
) ([]*coordinator.Session, error) {
	if workerID == "" {
		return []*coordinator.Session{}, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*coordinator.Session, 0)
	for _, session := range s.sessions {
		if session.WorkerID == workerID {
			// Return copies
			sessionCopy := *session
			if session.Metadata != nil {
				sessionCopy.Metadata = make(map[string]string, len(session.Metadata))
				for k, v := range session.Metadata {
					sessionCopy.Metadata[k] = v
				}
			}
			if session.TaskHistory != nil {
				sessionCopy.TaskHistory = make([]string, len(session.TaskHistory))
				copy(sessionCopy.TaskHistory, session.TaskHistory)
			}
			result = append(result, &sessionCopy)
		}
	}

	return result, nil
}

// UpdateSessionActivity updates the LastActive timestamp for a session
func (s *InMemorySessionStateStorage) UpdateSessionActivity(
	ctx context.Context,
	sessionID string,
) error {
	if sessionID == "" {
		return errSessionIDEmpty2
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session, exists := s.sessions[sessionID]
	if !exists {
		return errSessionNotFound
	}

	session.LastActive = time.Now()

	return nil
}
