package coordinator

import (
	"context"
	"log/slog"
	"testing"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"google.golang.org/grpc"
)

// testSessionStorage is a simple in-memory implementation for testing
type testSessionStorage struct {
	sessions map[string]*Session
}

func newTestSessionStorage() *testSessionStorage {
	return &testSessionStorage{
		sessions: make(map[string]*Session),
	}
}

func (s *testSessionStorage) CreateSession(ctx context.Context, session *Session) error {
	s.sessions[session.ID] = session
	return nil
}

func (s *testSessionStorage) GetSession(ctx context.Context, sessionID string) (*Session, error) {
	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, nil
	}
	return session, nil
}

func (s *testSessionStorage) UpdateSessionState(ctx context.Context, sessionID string, state SessionState, message string) error {
	if session, ok := s.sessions[sessionID]; ok {
		session.State = state
		session.StateMessage = message
	}
	return nil
}

func (s *testSessionStorage) SetSessionMetadata(ctx context.Context, sessionID string, metadata map[string]string) error {
	if session, ok := s.sessions[sessionID]; ok {
		if session.Metadata == nil {
			session.Metadata = make(map[string]string)
		}
		for k, v := range metadata {
			session.Metadata[k] = v
		}
	}
	return nil
}

func (s *testSessionStorage) GetSessionMetadata(ctx context.Context, sessionID string) (map[string]string, error) {
	if session, ok := s.sessions[sessionID]; ok {
		return session.Metadata, nil
	}
	return make(map[string]string), nil
}

func (s *testSessionStorage) GetSessionByConversationID(ctx context.Context, conversationID string) (*Session, error) {
	for _, session := range s.sessions {
		if session.ConversationID == conversationID {
			return session, nil
		}
	}
	return nil, nil
}

func (s *testSessionStorage) DeleteSession(ctx context.Context, sessionID string) error {
	delete(s.sessions, sessionID)
	return nil
}

func (s *testSessionStorage) GetLastCompletedSequence(ctx context.Context, sessionID string) (uint64, error) {
	if session, ok := s.sessions[sessionID]; ok {
		return session.LastCompletedSeq, nil
	}
	return 0, nil
}

func (s *testSessionStorage) SetLastCompletedSequence(ctx context.Context, sessionID string, sequence uint64) error {
	if session, ok := s.sessions[sessionID]; ok {
		session.LastCompletedSeq = sequence
	}
	return nil
}

func (s *testSessionStorage) GetNextSequence(ctx context.Context, sessionID string) (uint64, error) {
	if session, ok := s.sessions[sessionID]; ok {
		session.NextSequence++
		return session.NextSequence, nil
	}
	return 0, nil
}

func (s *testSessionStorage) ListSessions(ctx context.Context) ([]*Session, error) {
	sessions := make([]*Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func (s *testSessionStorage) UpdateSessionActivity(ctx context.Context, sessionID string) error {
	if session, ok := s.sessions[sessionID]; ok {
		session.LastActive = time.Now()
	}
	return nil
}

func TestNewCoordinatorServer(t *testing.T) {
	registry := NewWorkerRegistry()
	storage := newTestSessionStorage()
	sessionMgr := NewSessionManager(storage, registry)

	// Test with logger
	logger := slog.Default()
	server := NewCoordinatorServer(registry, sessionMgr, logger)
	if server == nil {
		t.Fatal("Expected non-nil server")
	}
	if server.logger != logger {
		t.Error("Logger not set correctly")
	}

	// Test with nil logger (should use default)
	server = NewCoordinatorServer(registry, sessionMgr, nil)
	if server == nil {
		t.Fatal("Expected non-nil server")
	}
	if server.logger == nil {
		t.Error("Expected default logger when nil provided")
	}
}

func TestRegisterWorker_Success(t *testing.T) {
	registry := NewWorkerRegistry()
	sessionMgr := NewSessionManager(registry)
	server := NewCoordinatorServer(registry, sessionMgr, slog.Default())

	req := &protov1.RegisterRequest{
		WorkerId: "test-worker",
		Version:  "1.0.0",
		Capabilities: &protov1.WorkerCapabilities{
			MaxSessions: 10,
		},
	}

	resp, err := server.RegisterWorker(context.Background(), req)
	if err != nil {
		t.Fatalf("RegisterWorker failed: %v", err)
	}

	if !resp.Accepted {
		t.Errorf("Expected accepted=true, got false. Reason: %s", resp.Reason)
	}

	if resp.SessionId == "" {
		t.Error("Expected non-empty session ID")
	}

	if resp.HeartbeatIntervalSec != 30 {
		t.Errorf("Expected heartbeat interval 30s, got %d", resp.HeartbeatIntervalSec)
	}

	// Verify worker is in registry
	worker := registry.GetWorker("test-worker")
	if worker == nil {
		t.Fatal("Worker not found in registry")
	}
	if worker.WorkerID != "test-worker" {
		t.Errorf("Expected worker ID 'test-worker', got '%s'", worker.WorkerID)
	}
}

func TestRegisterWorker_MissingWorkerID(t *testing.T) {
	registry := NewWorkerRegistry()
	sessionMgr := NewSessionManager(registry)
	server := NewCoordinatorServer(registry, sessionMgr, slog.Default())

	req := &protov1.RegisterRequest{
		WorkerId: "",
		Version:  "1.0.0",
	}

	resp, err := server.RegisterWorker(context.Background(), req)
	if err != nil {
		t.Fatalf("RegisterWorker returned error: %v", err)
	}

	if resp.Accepted {
		t.Error("Expected accepted=false for missing worker ID")
	}

	if resp.Reason != "worker_id is required" {
		t.Errorf("Expected reason 'worker_id is required', got '%s'", resp.Reason)
	}
}

func TestRegisterWorker_Reregistration(t *testing.T) {
	registry := NewWorkerRegistry()
	sessionMgr := NewSessionManager(registry)
	server := NewCoordinatorServer(registry, sessionMgr, slog.Default())

	req := &protov1.RegisterRequest{
		WorkerId: "test-worker",
		Version:  "1.0.0",
		Capabilities: &protov1.WorkerCapabilities{
			MaxSessions: 10,
		},
	}

	// First registration
	resp1, err := server.RegisterWorker(context.Background(), req)
	if err != nil {
		t.Fatalf("First registration failed: %v", err)
	}
	if !resp1.Accepted {
		t.Fatal("First registration should be accepted")
	}
	firstSessionID := resp1.SessionId

	// Second registration (should deregister and re-register)
	resp2, err := server.RegisterWorker(context.Background(), req)
	if err != nil {
		t.Fatalf("Second registration failed: %v", err)
	}
	if !resp2.Accepted {
		t.Fatal("Second registration should be accepted")
	}

	// Should have a new session ID
	if resp2.SessionId == firstSessionID {
		t.Error("Expected new session ID on re-registration")
	}
}

func TestHeartbeat_Success(t *testing.T) {
	registry := NewWorkerRegistry()
	sessionMgr := NewSessionManager(registry)
	server := NewCoordinatorServer(registry, sessionMgr, slog.Default())

	// Register worker first
	regReq := &protov1.RegisterRequest{
		WorkerId: "test-worker",
		Version:  "1.0.0",
		Capabilities: &protov1.WorkerCapabilities{
			MaxSessions: 10,
		},
	}
	regResp, _ := server.RegisterWorker(context.Background(), regReq)

	// Send heartbeat
	hbReq := &protov1.HeartbeatRequest{
		WorkerId:  "test-worker",
		SessionId: regResp.SessionId,
		Status: &protov1.WorkerStatus{
			State:       protov1.WorkerStatus_STATE_IDLE,
			ActiveTasks: 0,
		},
		Capacity: &protov1.SessionCapacity{
			TotalSessions:     10,
			ActiveSessions:    0,
			AvailableSessions: 10,
		},
	}

	hbResp, err := server.Heartbeat(context.Background(), hbReq)
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	if !hbResp.ContinueServing {
		t.Error("Expected ContinueServing=true")
	}
}

func TestHeartbeat_UnregisteredWorker(t *testing.T) {
	registry := NewWorkerRegistry()
	sessionMgr := NewSessionManager(registry)
	server := NewCoordinatorServer(registry, sessionMgr, slog.Default())

	hbReq := &protov1.HeartbeatRequest{
		WorkerId:  "unknown-worker",
		SessionId: "some-session",
		Status:    &protov1.WorkerStatus{},
	}

	hbResp, err := server.Heartbeat(context.Background(), hbReq)
	if err == nil {
		t.Fatal("Expected error for unregistered worker")
	}

	if hbResp.ContinueServing {
		t.Error("Expected ContinueServing=false for unregistered worker")
	}
}

func TestHeartbeat_InvalidSessionID(t *testing.T) {
	registry := NewWorkerRegistry()
	sessionMgr := NewSessionManager(registry)
	server := NewCoordinatorServer(registry, sessionMgr, slog.Default())

	// Register worker
	regReq := &protov1.RegisterRequest{
		WorkerId: "test-worker",
		Version:  "1.0.0",
		Capabilities: &protov1.WorkerCapabilities{
			MaxSessions: 10,
		},
	}
	regResp, _ := server.RegisterWorker(context.Background(), regReq)

	// Send heartbeat with wrong session ID
	hbReq := &protov1.HeartbeatRequest{
		WorkerId:  "test-worker",
		SessionId: "wrong-session-id",
		Status:    &protov1.WorkerStatus{},
	}

	hbResp, err := server.Heartbeat(context.Background(), hbReq)
	if err == nil {
		t.Fatal("Expected error for invalid session ID")
	}

	if hbResp.ContinueServing {
		t.Error("Expected ContinueServing=false for invalid session ID")
	}

	// Verify correct session ID is stored
	worker := registry.GetWorker("test-worker")
	if worker.SessionID != regResp.SessionId {
		t.Error("Worker session ID was incorrectly modified")
	}
}

func TestDeregisterWorker_Success(t *testing.T) {
	registry := NewWorkerRegistry()
	sessionMgr := NewSessionManager(registry)
	server := NewCoordinatorServer(registry, sessionMgr, slog.Default())

	// Register worker
	regReq := &protov1.RegisterRequest{
		WorkerId: "test-worker",
		Version:  "1.0.0",
		Capabilities: &protov1.WorkerCapabilities{
			MaxSessions: 10,
		},
	}
	regResp, _ := server.RegisterWorker(context.Background(), regReq)

	// Deregister
	deregReq := &protov1.DeregisterRequest{
		WorkerId:  "test-worker",
		SessionId: regResp.SessionId,
		Reason:    "shutting down",
	}

	deregResp, err := server.DeregisterWorker(context.Background(), deregReq)
	if err != nil {
		t.Fatalf("DeregisterWorker failed: %v", err)
	}

	if !deregResp.Acknowledged {
		t.Error("Expected acknowledged=true")
	}

	// Verify worker removed from registry
	worker := registry.GetWorker("test-worker")
	if worker != nil {
		t.Error("Worker should be removed from registry")
	}
}

func TestDeregisterWorker_UnknownWorker(t *testing.T) {
	registry := NewWorkerRegistry()
	sessionMgr := NewSessionManager(registry)
	server := NewCoordinatorServer(registry, sessionMgr, slog.Default())

	deregReq := &protov1.DeregisterRequest{
		WorkerId:  "unknown-worker",
		SessionId: "some-session",
		Reason:    "test",
	}

	deregResp, err := server.DeregisterWorker(context.Background(), deregReq)
	if err == nil {
		t.Fatal("Expected error for unknown worker")
	}

	if deregResp.Acknowledged {
		t.Error("Expected acknowledged=false for unknown worker")
	}
}

func TestDeregisterWorker_InvalidSessionID(t *testing.T) {
	registry := NewWorkerRegistry()
	sessionMgr := NewSessionManager(registry)
	server := NewCoordinatorServer(registry, sessionMgr, slog.Default())

	// Register worker
	regReq := &protov1.RegisterRequest{
		WorkerId: "test-worker",
		Version:  "1.0.0",
		Capabilities: &protov1.WorkerCapabilities{
			MaxSessions: 10,
		},
	}
	regResp, _ := server.RegisterWorker(context.Background(), regReq)

	// Try to deregister with wrong session ID
	deregReq := &protov1.DeregisterRequest{
		WorkerId:  "test-worker",
		SessionId: "wrong-session",
		Reason:    "test",
	}

	deregResp, err := server.DeregisterWorker(context.Background(), deregReq)
	if err == nil {
		t.Fatal("Expected error for invalid session ID")
	}

	if deregResp.Acknowledged {
		t.Error("Expected acknowledged=false for invalid session ID")
	}

	// Verify worker still in registry
	worker := registry.GetWorker("test-worker")
	if worker == nil {
		t.Error("Worker should still be in registry")
	}
	if worker.SessionID != regResp.SessionId {
		t.Error("Worker session ID should be unchanged")
	}
}

func TestStartCleanupLoop(t *testing.T) {
	registry := NewWorkerRegistry()
	sessionMgr := NewSessionManager(registry)
	server := NewCoordinatorServer(registry, sessionMgr, slog.Default())

	// Register a worker
	regReq := &protov1.RegisterRequest{
		WorkerId: "stale-worker",
		Version:  "1.0.0",
		Capabilities: &protov1.WorkerCapabilities{
			MaxSessions: 10,
		},
	}
	server.RegisterWorker(context.Background(), regReq)

	// Get worker and set old heartbeat
	worker := registry.GetWorker("stale-worker")
	worker.mu.Lock()
	worker.LastHeartbeat = time.Now().Add(-10 * time.Minute) // 10 minutes ago
	worker.mu.Unlock()

	// Start cleanup loop with short interval
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go server.StartCleanupLoop(ctx, 100*time.Millisecond)

	// Wait for cleanup to run
	time.Sleep(200 * time.Millisecond)

	// Worker should be removed
	worker = registry.GetWorker("stale-worker")
	if worker != nil {
		t.Error("Stale worker should have been cleaned up")
	}

	// Cancel context and verify cleanup loop stops
	cancel()
	time.Sleep(50 * time.Millisecond)
}

func TestRegisterWithServer(t *testing.T) {
	registry := NewWorkerRegistry()
	sessionMgr := NewSessionManager(registry)
	server := NewCoordinatorServer(registry, sessionMgr, slog.Default())

	grpcServer := grpc.NewServer()
	server.RegisterWithServer(grpcServer)

	// If no panic, the registration succeeded
	// We can't easily test more without starting the server
}

func TestHeartbeat_WithCapacityInfo(t *testing.T) {
	registry := NewWorkerRegistry()
	sessionMgr := NewSessionManager(registry)
	server := NewCoordinatorServer(registry, sessionMgr, slog.Default())

	// Register worker
	regReq := &protov1.RegisterRequest{
		WorkerId: "test-worker",
		Version:  "1.0.0",
		Capabilities: &protov1.WorkerCapabilities{
			MaxSessions: 10,
		},
	}
	regResp, _ := server.RegisterWorker(context.Background(), regReq)

	// Send heartbeat with detailed capacity
	hbReq := &protov1.HeartbeatRequest{
		WorkerId:  "test-worker",
		SessionId: regResp.SessionId,
		Status: &protov1.WorkerStatus{
			State:       protov1.WorkerStatus_STATE_BUSY,
			ActiveTasks: 5,
		},
		Capacity: &protov1.SessionCapacity{
			TotalSessions:     10,
			ActiveSessions:    5,
			AvailableSessions: 5,
		},
	}

	hbResp, err := server.Heartbeat(context.Background(), hbReq)
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	if !hbResp.ContinueServing {
		t.Error("Expected ContinueServing=true")
	}

	// Verify capacity was updated in registry
	worker := registry.GetWorker("test-worker")
	worker.mu.RLock()
	defer worker.mu.RUnlock()

	if worker.Capacity == nil {
		t.Fatal("Worker capacity should be set")
	}

	if worker.Capacity.ActiveSessions != 5 {
		t.Errorf("Expected 5 active sessions, got %d", worker.Capacity.ActiveSessions)
	}

	if worker.Status == nil {
		t.Fatal("Worker status should be set")
	}

	if worker.Status.State != protov1.WorkerStatus_STATE_BUSY {
		t.Errorf("Expected BUSY state, got %v", worker.Status.State)
	}
}
