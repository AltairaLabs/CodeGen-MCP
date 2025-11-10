package coordinator

import (
	"encoding/json"
	"errors"
	"testing"
)

// mockNotificationSender is a test implementation of SSENotificationSender
type mockNotificationSender struct {
	notifications []map[string]interface{}
	sendErr       error
}

func (m *mockNotificationSender) SendNotification(notification map[string]interface{}) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.notifications = append(m.notifications, notification)
	return nil
}

func TestNewSSESessionManager(t *testing.T) {
	manager := NewSSESessionManager()
	if manager == nil {
		t.Fatal("Expected non-nil manager")
	}
	if manager.sessions == nil {
		t.Error("Expected initialized sessions map")
	}
	if count := manager.GetSessionCount(); count != 0 {
		t.Errorf("Expected 0 sessions, got %d", count)
	}
}

func TestSSESessionManager_RegisterSession(t *testing.T) {
	manager := NewSSESessionManager()
	sender := &mockNotificationSender{}

	manager.RegisterSession("session-1", sender)

	session := manager.GetSession("session-1")
	if session == nil {
		t.Fatal("Expected session to be registered")
	}
	if session.SessionID != "session-1" {
		t.Errorf("Expected session ID 'session-1', got %q", session.SessionID)
	}
	if session.sender != sender {
		t.Error("Expected sender to match registered sender")
	}
}

func TestSSESessionManager_RegisterSession_Multiple(t *testing.T) {
	manager := NewSSESessionManager()
	sender1 := &mockNotificationSender{}
	sender2 := &mockNotificationSender{}

	manager.RegisterSession("session-1", sender1)
	manager.RegisterSession("session-2", sender2)

	if count := manager.GetSessionCount(); count != 2 {
		t.Errorf("Expected 2 sessions, got %d", count)
	}

	session1 := manager.GetSession("session-1")
	session2 := manager.GetSession("session-2")

	if session1 == nil || session2 == nil {
		t.Fatal("Expected both sessions to be registered")
	}
	if session1.sender != sender1 || session2.sender != sender2 {
		t.Error("Expected senders to match their respective registered senders")
	}
}

func TestSSESessionManager_RegisterSession_Overwrite(t *testing.T) {
	manager := NewSSESessionManager()
	sender1 := &mockNotificationSender{}
	sender2 := &mockNotificationSender{}

	manager.RegisterSession("session-1", sender1)
	manager.RegisterSession("session-1", sender2)

	session := manager.GetSession("session-1")
	if session == nil {
		t.Fatal("Expected session to be registered")
	}
	if session.sender != sender2 {
		t.Error("Expected sender to be overwritten with sender2")
	}
	if count := manager.GetSessionCount(); count != 1 {
		t.Errorf("Expected 1 session after overwrite, got %d", count)
	}
}

func TestSSESessionManager_GetSession_NotFound(t *testing.T) {
	manager := NewSSESessionManager()
	session := manager.GetSession("non-existent")
	if session != nil {
		t.Error("Expected nil for non-existent session")
	}
}

func TestSSESessionManager_UnregisterSession(t *testing.T) {
	manager := NewSSESessionManager()
	sender := &mockNotificationSender{}

	manager.RegisterSession("session-1", sender)
	if count := manager.GetSessionCount(); count != 1 {
		t.Errorf("Expected 1 session before unregister, got %d", count)
	}

	manager.UnregisterSession("session-1")

	session := manager.GetSession("session-1")
	if session != nil {
		t.Error("Expected session to be unregistered")
	}
	if count := manager.GetSessionCount(); count != 0 {
		t.Errorf("Expected 0 sessions after unregister, got %d", count)
	}
}

func TestSSESessionManager_UnregisterSession_NotFound(t *testing.T) {
	manager := NewSSESessionManager()
	// Should not panic when unregistering non-existent session
	manager.UnregisterSession("non-existent")
	if count := manager.GetSessionCount(); count != 0 {
		t.Errorf("Expected 0 sessions, got %d", count)
	}
}

func TestSSESessionManager_GetAllSessions(t *testing.T) {
	manager := NewSSESessionManager()

	// Test empty sessions
	sessions := manager.GetAllSessions()
	if len(sessions) != 0 {
		t.Errorf("Expected 0 sessions, got %d", len(sessions))
	}

	// Add sessions
	sender1 := &mockNotificationSender{}
	sender2 := &mockNotificationSender{}
	sender3 := &mockNotificationSender{}

	manager.RegisterSession("session-1", sender1)
	manager.RegisterSession("session-2", sender2)
	manager.RegisterSession("session-3", sender3)

	sessions = manager.GetAllSessions()
	if len(sessions) != 3 {
		t.Errorf("Expected 3 sessions, got %d", len(sessions))
	}

	// Verify all sessions are present
	sessionIDs := make(map[string]bool)
	for _, session := range sessions {
		sessionIDs[session.SessionID] = true
	}

	expectedIDs := []string{"session-1", "session-2", "session-3"}
	for _, id := range expectedIDs {
		if !sessionIDs[id] {
			t.Errorf("Expected session %q to be in results", id)
		}
	}
}

func TestSSESessionManager_GetSessionCount(t *testing.T) {
	manager := NewSSESessionManager()
	sender := &mockNotificationSender{}

	if count := manager.GetSessionCount(); count != 0 {
		t.Errorf("Expected 0 sessions initially, got %d", count)
	}

	manager.RegisterSession("session-1", sender)
	if count := manager.GetSessionCount(); count != 1 {
		t.Errorf("Expected 1 session, got %d", count)
	}

	manager.RegisterSession("session-2", sender)
	if count := manager.GetSessionCount(); count != 2 {
		t.Errorf("Expected 2 sessions, got %d", count)
	}

	manager.UnregisterSession("session-1")
	if count := manager.GetSessionCount(); count != 1 {
		t.Errorf("Expected 1 session after unregister, got %d", count)
	}

	manager.UnregisterSession("session-2")
	if count := manager.GetSessionCount(); count != 0 {
		t.Errorf("Expected 0 sessions after all unregistered, got %d", count)
	}
}

func TestSSESession_SendNotification(t *testing.T) {
	sender := &mockNotificationSender{}
	session := &SSESession{
		SessionID: "test-session",
		sender:    sender,
	}

	notification := map[string]interface{}{
		"type":    "progress",
		"message": "test message",
	}

	err := session.SendNotification(notification)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if len(sender.notifications) != 1 {
		t.Fatalf("Expected 1 notification, got %d", len(sender.notifications))
	}

	received := sender.notifications[0]
	if received["type"] != "progress" {
		t.Errorf("Expected type 'progress', got %v", received["type"])
	}
	if received["message"] != "test message" {
		t.Errorf("Expected message 'test message', got %v", received["message"])
	}
}

func TestSSESession_SendNotification_Error(t *testing.T) {
	expectedErr := errors.New("send failed")
	sender := &mockNotificationSender{sendErr: expectedErr}
	session := &SSESession{
		SessionID: "test-session",
		sender:    sender,
	}

	notification := map[string]interface{}{
		"type": "test",
	}

	err := session.SendNotification(notification)
	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
}

func TestNewChannelNotificationSender(t *testing.T) {
	ch := make(chan string, 10)
	sender := NewChannelNotificationSender(ch)

	if sender == nil {
		t.Fatal("Expected non-nil sender")
	}
	if sender.ch == nil {
		t.Error("Expected channel to be set")
	}
}

func TestChannelNotificationSender_SendNotification(t *testing.T) {
	ch := make(chan string, 10)
	sender := NewChannelNotificationSender(ch)

	notification := map[string]interface{}{
		"type":    "result",
		"message": "task completed",
		"data": map[string]interface{}{
			"value": 42,
		},
	}

	err := sender.SendNotification(notification)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	select {
	case msg := <-ch:
		var received map[string]interface{}
		if err := json.Unmarshal([]byte(msg), &received); err != nil {
			t.Fatalf("Failed to unmarshal notification: %v", err)
		}

		if received["type"] != "result" {
			t.Errorf("Expected type 'result', got %v", received["type"])
		}
		if received["message"] != "task completed" {
			t.Errorf("Expected message 'task completed', got %v", received["message"])
		}

		// Check nested data
		data, ok := received["data"].(map[string]interface{})
		if !ok {
			t.Fatal("Expected data to be a map")
		}
		if value, ok := data["value"].(float64); !ok || value != 42 {
			t.Errorf("Expected data.value to be 42, got %v", data["value"])
		}

	default:
		t.Error("Expected message to be sent to channel")
	}
}

func TestChannelNotificationSender_SendNotification_InvalidJSON(t *testing.T) {
	ch := make(chan string, 10)
	sender := NewChannelNotificationSender(ch)

	// Create notification with invalid JSON value (channel cannot be marshaled)
	notification := map[string]interface{}{
		"invalid": make(chan int),
	}

	err := sender.SendNotification(notification)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestSSESessionManager_Concurrency(t *testing.T) {
	manager := NewSSESessionManager()
	done := make(chan bool)

	// Register sessions concurrently
	for i := 0; i < 10; i++ {
		go func(id int) {
			sender := &mockNotificationSender{}
			sessionID := string(rune('A' + id))
			manager.RegisterSession(sessionID, sender)
			done <- true
		}(i)
	}

	// Wait for all registrations
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify count
	count := manager.GetSessionCount()
	if count != 10 {
		t.Errorf("Expected 10 sessions after concurrent registration, got %d", count)
	}

	// Unregister concurrently
	for i := 0; i < 10; i++ {
		go func(id int) {
			sessionID := string(rune('A' + id))
			manager.UnregisterSession(sessionID)
			done <- true
		}(i)
	}

	// Wait for all unregistrations
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify count
	count = manager.GetSessionCount()
	if count != 0 {
		t.Errorf("Expected 0 sessions after concurrent unregistration, got %d", count)
	}
}
