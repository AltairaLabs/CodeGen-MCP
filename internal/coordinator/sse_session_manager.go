package coordinator

import (
	"encoding/json"
	"sync"
)

// SSENotificationSender defines the interface for sending SSE notifications
// This abstracts the actual SSE connection so we can work with any SSE implementation
type SSENotificationSender interface {
	SendNotification(notification map[string]interface{}) error
}

// SSESession represents an SSE connection for a session
type SSESession struct {
	SessionID string
	sender    SSENotificationSender
}

// SendNotification sends a notification through this SSE session
func (s *SSESession) SendNotification(notification map[string]interface{}) error {
	return s.sender.SendNotification(notification)
}

// SSESessionManager manages SSE connections for result streaming
type SSESessionManager struct {
	sessions map[string]*SSESession
	mu       sync.RWMutex
}

// NewSSESessionManager creates a new SSE session manager
func NewSSESessionManager() *SSESessionManager {
	return &SSESessionManager{
		sessions: make(map[string]*SSESession),
	}
}

// RegisterSession registers an SSE session with a notification sender
func (sm *SSESessionManager) RegisterSession(sessionID string, sender SSENotificationSender) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.sessions[sessionID] = &SSESession{
		SessionID: sessionID,
		sender:    sender,
	}
}

// GetSession retrieves an SSE session by ID
func (sm *SSESessionManager) GetSession(sessionID string) *SSESession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.sessions[sessionID]
}

// UnregisterSession removes an SSE session
func (sm *SSESessionManager) UnregisterSession(sessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, sessionID)
}

// GetAllSessions returns all active SSE sessions
func (sm *SSESessionManager) GetAllSessions() []*SSESession {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sessions := make([]*SSESession, 0, len(sm.sessions))
	for _, session := range sm.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

// GetSessionCount returns the number of active SSE sessions
func (sm *SSESessionManager) GetSessionCount() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.sessions)
}

// ChannelNotificationSender implements SSENotificationSender using a channel
// This is useful for testing and for buffered notification delivery
type ChannelNotificationSender struct {
	ch chan<- string
}

// NewChannelNotificationSender creates a notification sender that writes JSON to a channel
func NewChannelNotificationSender(ch chan<- string) *ChannelNotificationSender {
	return &ChannelNotificationSender{ch: ch}
}

// SendNotification sends a notification by marshaling it to JSON and sending to the channel
func (s *ChannelNotificationSender) SendNotification(notification map[string]interface{}) error {
	data, err := json.Marshal(notification)
	if err != nil {
		return err
	}
	s.ch <- string(data)
	return nil
}
