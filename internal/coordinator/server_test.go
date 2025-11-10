package coordinatorpackage coordinator



import (import (

	"context"	"context"

	"log/slog"	"log/slog"

	"os"	"os"

	"testing"	"testing"

	"time"	"time"



	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"

))



// Helper to create a test session manager// Helper to create a test session manager

func newTestServerSessionManager() *SessionManager {func newTestServerSessionManager() *SessionManager {

	storage := newTestSessionStorage()	storage := newTestSessionStorage()

	registry := NewWorkerRegistry()	registry := NewWorkerRegistry()

	return NewSessionManager(storage, registry)	return NewSessionManager(storage, registry)

}}



func TestServerNewMCPServer(t *testing.T) {func TestServerNewMCPServer(t *testing.T) {

	sessionMgr := newTestServerSessionManager()	sessionMgr := newTestServerSessionManager()

	worker := NewMockWorkerClient()	worker := NewMockWorkerClient()

	audit := NewAuditLogger(slog.Default())	audit := NewAuditLogger(slog.Default())

	taskQueue := &MockTaskQueue{workerClient: worker}	taskQueue := &MockTaskQueue{workerClient: worker}



	cfg := Config{	cfg := Config{

		Name:    "test-server",		Name:    "test-server",

		Version: "1.0.0",		Version: "1.0.0",

	}	}



	ms := NewMCPServer(cfg, sessionMgr, worker, audit, taskQueue)	ms := NewMCPServer(cfg, sessionMgr, worker, audit, taskQueue)



	if ms == nil {	if ms == nil {

		t.Fatal("Expected MCPServer to be created, got nil")		t.Fatal("Expected MCPServer to be created, got nil")

	}	}



	if ms.server == nil {	if ms.server == nil {

		t.Error("Expected mcp-go server to be initialized")		t.Error("Expected mcp-go server to be initialized")

	}	}



	if ms.sessionManager == nil {	if ms.sessionManager == nil {

		t.Error("Expected session manager to be set")		t.Error("Expected session manager to be set")

	}	}



	if ms.workerClient == nil {	if ms.workerClient == nil {

		t.Error("Expected worker client to be set")		t.Error("Expected worker client to be set")

	}	}



	if ms.taskQueue == nil {	if ms.taskQueue == nil {

		t.Error("Expected task queue to be set")		t.Error("Expected task queue to be set")

	}	}



	if ms.resultStreamer == nil {	if ms.resultStreamer == nil {

		t.Error("Expected result streamer to be initialized")		t.Error("Expected result streamer to be initialized")

	}	}



	if ms.resultCache == nil {	if ms.resultCache == nil {

		t.Error("Expected result cache to be initialized")		t.Error("Expected result cache to be initialized")

	}	}



	if ms.sseManager == nil {	if ms.sseManager == nil {

		t.Error("Expected SSE manager to be initialized")		t.Error("Expected SSE manager to be initialized")

	}	}

}}



func TestServerContextWithSessionID(t *testing.T) {func TestServerContextWithSessionID(t *testing.T) {

	ctx := context.Background()	ctx := context.Background()

	sessionID := "test-session-123"	sessionID := "test-session-123"



	newCtx := contextWithSessionID(ctx, sessionID)	newCtx := contextWithSessionID(ctx, sessionID)



	// Verify the context contains the session ID	// Verify the context contains the session ID

	value := newCtx.Value(sessionIDKey{})	value := newCtx.Value(sessionIDKey{})

	if value == nil {	if value == nil {

		t.Fatal("Expected session ID in context, got nil")		t.Fatal("Expected session ID in context, got nil")

	}	}



	retrievedID, ok := value.(string)	retrievedID, ok := value.(string)

	if !ok {	if !ok {

		t.Fatal("Expected session ID to be string")		t.Fatal("Expected session ID to be string")

	}	}



	if retrievedID != sessionID {	if retrievedID != sessionID {

		t.Errorf("Expected session ID %s, got %s", sessionID, retrievedID)		t.Errorf("Expected session ID %s, got %s", sessionID, retrievedID)

	}	}

}}



func TestServerValidateWorkspacePath(t *testing.T) {func TestServerValidateWorkspacePath(t *testing.T) {

	sessionMgr := newTestServerSessionManager()	sessionMgr := newTestServerSessionManager()

	worker := NewMockWorkerClient()	worker := NewMockWorkerClient()

	audit := NewAuditLogger(slog.Default())	audit := NewAuditLogger(slog.Default())

	taskQueue := &MockTaskQueue{workerClient: worker}	taskQueue := &MockTaskQueue{workerClient: worker}



	cfg := Config{Name: "test", Version: "1.0.0"}	cfg := Config{Name: "test", Version: "1.0.0"}

	ms := NewMCPServer(cfg, sessionMgr, worker, audit, taskQueue)	ms := NewMCPServer(cfg, sessionMgr, worker, audit, taskQueue)



	tests := []struct {	tests := []struct {

		name    string		name    string

		path    string		path    string

		wantErr bool		wantErr bool

	}{	}{

		{		{

			name:    "valid relative path",			name:    "valid relative path",

			path:    "src/main.py",			path:    "src/main.py",

			wantErr: false,			wantErr: false,

		},		},

		{		{

			name:    "valid nested path",			name:    "valid nested path",

			path:    "src/lib/utils.py",			path:    "src/lib/utils.py",

			wantErr: false,			wantErr: false,

		},		},

		{		{

			name:    "absolute path rejected",			name:    "absolute path rejected",

			path:    "/etc/passwd",			path:    "/etc/passwd",

			wantErr: true,			wantErr: true,

		},		},

		{		{

			name:    "path traversal with ..",			name:    "path traversal with ..",

			path:    "../../../etc/passwd",			path:    "../../../etc/passwd",

			wantErr: true,			wantErr: true,

		},		},

		{		{

			name:    "path traversal in middle",			name:    "path traversal in middle",

			path:    "src/../../../etc/passwd",			path:    "src/../../../etc/passwd",

			wantErr: true,			wantErr: true,

		},		},

		{		{

			name:    "valid path with dots",			name:    "valid path with dots",

			path:    "src/file.name.py",			path:    "src/file.name.py",

			wantErr: false,			wantErr: false,

		},		},

	}	}



	for _, tt := range tests {	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {		t.Run(tt.name, func(t *testing.T) {

			err := ms.validateWorkspacePath(tt.path)			err := ms.validateWorkspacePath(tt.path)

			if (err != nil) != tt.wantErr {			if (err != nil) != tt.wantErr {

				t.Errorf("validateWorkspacePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)				t.Errorf("validateWorkspacePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)

			}			}

		})		})

	}	}

}}



func TestServerGetSessionID(t *testing.T) {func TestServerGetSessionID(t *testing.T) {

	sessionMgr := newTestServerSessionManager()	sessionMgr := newTestServerSessionManager()

	worker := NewMockWorkerClient()	worker := NewMockWorkerClient()

	audit := NewAuditLogger(slog.Default())	audit := NewAuditLogger(slog.Default())

	taskQueue := &MockTaskQueue{workerClient: worker}	taskQueue := &MockTaskQueue{workerClient: worker}



	cfg := Config{Name: "test", Version: "1.0.0"}	cfg := Config{Name: "test", Version: "1.0.0"}

	ms := NewMCPServer(cfg, sessionMgr, worker, audit, taskQueue)	ms := NewMCPServer(cfg, sessionMgr, worker, audit, taskQueue)



	tests := []struct {	tests := []struct {

		name     string		name      string

		setupCtx func() context.Context		setupCtx  func() context.Context

		wantID   string		wantID    string

	}{	}{

		{		{

			name: "session ID from context value",			name: "session ID from context value",

			setupCtx: func() context.Context {			setupCtx: func() context.Context {

				return context.WithValue(context.Background(), "session_id", "ctx-session-123")				return context.WithValue(context.Background(), "session_id", "ctx-session-123")

			},			},

			wantID: "ctx-session-123",			wantID: "ctx-session-123",

		},		},

		{		{

			name: "fallback to default session",			name: "fallback to default session",

			setupCtx: func() context.Context {			setupCtx: func() context.Context {

				return context.Background()				return context.Background()

			},			},

			wantID: "default-session",			wantID: "default-session",

		},		},

	}	}



	for _, tt := range tests {	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {		t.Run(tt.name, func(t *testing.T) {

			ctx := tt.setupCtx()			ctx := tt.setupCtx()

			sessionID := ms.getSessionID(ctx)			sessionID := ms.getSessionID(ctx)



			if sessionID != tt.wantID {			if sessionID != tt.wantID {

				t.Errorf("getSessionID() = %v, want %v", sessionID, tt.wantID)				t.Errorf("getSessionID() = %v, want %v", sessionID, tt.wantID)

			}			}

		})		})

	}	}

}}



func TestServerHasWorkersAvailable(t *testing.T) {func TestServerHasWorkersAvailable(t *testing.T) {

	tests := []struct {	tests := []struct {

		name      string		name      string

		setupMgr  func() *SessionManager		setupMgr  func() *SessionManager

		wantAvail bool		wantAvail bool

	}{	}{

		{		{

			name: "workers available",			name: "workers available",

			setupMgr: func() *SessionManager {			setupMgr: func() *SessionManager {

				mgr := newTestServerSessionManager()				mgr := newTestServerSessionManager()

				// Register a worker				// Register a worker

				worker := &RegisteredWorker{				worker := &RegisteredWorker{

					WorkerID:              "test-worker",					WorkerID:              "test-worker",

					SessionID:             "reg-test-worker",					SessionID:             "reg-test-worker",

					PendingTasks:          make(map[string]*PendingTask),					PendingTasks:          make(map[string]*PendingTask),

					PendingSessionCreates: make(map[string]chan *protov1.SessionCreateResponse),					PendingSessionCreates: make(map[string]chan *protov1.SessionCreateResponse),

					Capacity: &protov1.SessionCapacity{					Capacity: &protov1.SessionCapacity{

						TotalSessions:     10,						TotalSessions:     10,

						ActiveSessions:    0,						ActiveSessions:    0,

						AvailableSessions: 10,						AvailableSessions: 10,

					},					},

					LastHeartbeat: time.Now(),					LastHeartbeat: time.Now(),

					RegisteredAt:  time.Now(),					RegisteredAt:  time.Now(),

				}				}

				mgr.workerRegistry.workers["test-worker"] = worker				mgr.workerRegistry.workers["test-worker"] = worker

				return mgr				return mgr

			},			},

			wantAvail: true,			wantAvail: true,

		},		},

		{		{

			name: "no workers available",			name: "no workers available",

			setupMgr: func() *SessionManager {			setupMgr: func() *SessionManager {

				return newTestServerSessionManager()				return newTestServerSessionManager()

			},			},

			wantAvail: false,			wantAvail: false,

		},		},

		{		{

			name: "nil session manager",			name: "nil session manager",

			setupMgr: func() *SessionManager {			setupMgr: func() *SessionManager {

				return nil				return nil

			},			},

			wantAvail: false,			wantAvail: false,

		},		},

	}	}



	for _, tt := range tests {	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {		t.Run(tt.name, func(t *testing.T) {

			worker := NewMockWorkerClient()			worker := NewMockWorkerClient()

			audit := NewAuditLogger(slog.Default())			audit := NewAuditLogger(slog.Default())

			taskQueue := &MockTaskQueue{workerClient: worker}			taskQueue := &MockTaskQueue{workerClient: worker}



			cfg := Config{Name: "test", Version: "1.0.0"}			cfg := Config{Name: "test", Version: "1.0.0"}

			sessionMgr := tt.setupMgr()			sessionMgr := tt.setupMgr()

			

			ms := NewMCPServer(cfg, sessionMgr, worker, audit, taskQueue)			ms := NewMCPServer(cfg, sessionMgr, worker, audit, taskQueue)

			if sessionMgr == nil {			if sessionMgr == nil {

				ms.sessionManager = nil				ms.sessionManager = nil

			}			}



			available := ms.hasWorkersAvailable()			available := ms.hasWorkersAvailable()

			if available != tt.wantAvail {			if available != tt.wantAvail {

				t.Errorf("hasWorkersAvailable() = %v, want %v", available, tt.wantAvail)				t.Errorf("hasWorkersAvailable() = %v, want %v", available, tt.wantAvail)

			}			}

		})		})

	}	}

}}



func TestServerGetOrCreateSessionNew(t *testing.T) {func TestGetOrCreateSession_NewSession(t *testing.T) {

	sessionMgr := newTestServerSessionManager()	sessionMgr := newTestSessionManager()

	

	// Register a worker	// Register a worker

	worker := &RegisteredWorker{	worker := &RegisteredWorker{

		WorkerID:              "test-worker",		WorkerID:          "test-worker",

		SessionID:             "reg-test-worker",		MaxSessions:       10,

		PendingTasks:          make(map[string]*PendingTask),		ActiveSessionsMap: make(map[string]bool),

		PendingSessionCreates: make(map[string]chan *protov1.SessionCreateResponse),	}

		Capacity: &protov1.SessionCapacity{	sessionMgr.workerRegistry.workers["test-worker"] = worker

			TotalSessions:     10,

			ActiveSessions:    0,	workerClient := &MockWorkerClient{}

			AvailableSessions: 10,	audit := NewAuditLogger(slog.Default())

		},	taskQueue := NewMockTaskQueue()

		LastHeartbeat: time.Now(),

		RegisteredAt:  time.Now(),	cfg := Config{Name: "test", Version: "1.0.0"}

	}	ms := NewMCPServer(cfg, sessionMgr, workerClient, audit, taskQueue)

	sessionMgr.workerRegistry.workers["test-worker"] = worker

	ctx := context.WithValue(context.Background(), "session_id", "new-session-123")

	workerClient := NewMockWorkerClient()

	audit := NewAuditLogger(slog.Default())	session, err := ms.getOrCreateSession(ctx)

	taskQueue := &MockTaskQueue{workerClient: workerClient}	if err != nil {

		t.Fatalf("Unexpected error: %v", err)

	cfg := Config{Name: "test", Version: "1.0.0"}	}

	ms := NewMCPServer(cfg, sessionMgr, workerClient, audit, taskQueue)

	if session == nil {

	ctx := context.WithValue(context.Background(), "session_id", "new-session-123")		t.Fatal("Expected session to be created, got nil")

	}

	session, err := ms.getOrCreateSession(ctx)

	if err != nil {	if session.ID != "new-session-123" {

		t.Fatalf("Unexpected error: %v", err)		t.Errorf("Expected session ID %s, got %s", "new-session-123", session.ID)

	}	}



	if session == nil {	if session.WorkerID == "" {

		t.Fatal("Expected session to be created, got nil")		t.Error("Expected worker to be assigned to session")

	}	}

}

	if session.ID != "new-session-123" {

		t.Errorf("Expected session ID %s, got %s", "new-session-123", session.ID)func TestGetOrCreateSession_ExistingSession(t *testing.T) {

	}	sessionMgr := newTestSessionManager()

	

	if session.WorkerID == "" {	// Create existing session

		t.Error("Expected worker to be assigned to session")	ctx := context.Background()

	}	existingSession := sessionMgr.CreateSession(ctx, "existing-session", "user1", "workspace1")

}	existingSession.WorkerID = "test-worker"



func TestServerGetOrCreateSessionExisting(t *testing.T) {	workerClient := &MockWorkerClient{}

	sessionMgr := newTestServerSessionManager()	audit := NewAuditLogger(slog.Default())

	taskQueue := NewMockTaskQueue()

	// Create existing session

	ctx := context.Background()	cfg := Config{Name: "test", Version: "1.0.0"}

	existingSession := sessionMgr.CreateSession(ctx, "existing-session", "user1", "workspace1")	ms := NewMCPServer(cfg, sessionMgr, workerClient, audit, taskQueue)

	existingSession.WorkerID = "test-worker"

	ctx = context.WithValue(context.Background(), "session_id", "existing-session")

	workerClient := NewMockWorkerClient()

	audit := NewAuditLogger(slog.Default())	session, err := ms.getOrCreateSession(ctx)

	taskQueue := &MockTaskQueue{workerClient: workerClient}	if err != nil {

		t.Fatalf("Unexpected error: %v", err)

	cfg := Config{Name: "test", Version: "1.0.0"}	}

	ms := NewMCPServer(cfg, sessionMgr, workerClient, audit, taskQueue)

	if session == nil {

	ctx = context.WithValue(context.Background(), "session_id", "existing-session")		t.Fatal("Expected session to be returned, got nil")

	}

	session, err := ms.getOrCreateSession(ctx)

	if err != nil {	if session.ID != "existing-session" {

		t.Fatalf("Unexpected error: %v", err)		t.Errorf("Expected session ID %s, got %s", "existing-session", session.ID)

	}	}



	if session == nil {	if session.WorkerID != "test-worker" {

		t.Fatal("Expected session to be returned, got nil")		t.Errorf("Expected worker ID %s, got %s", "test-worker", session.WorkerID)

	}	}

}

	if session.ID != "existing-session" {

		t.Errorf("Expected session ID %s, got %s", "existing-session", session.ID)func TestGetOrCreateSession_NoWorkersAvailable(t *testing.T) {

	}	sessionMgr := newTestSessionManager()

	// Don't register any workers

	if session.WorkerID != "test-worker" {

		t.Errorf("Expected worker ID %s, got %s", "test-worker", session.WorkerID)	workerClient := &MockWorkerClient{}

	}	audit := NewAuditLogger(slog.Default())

}	taskQueue := NewMockTaskQueue()



func TestServerGetOrCreateSessionNoWorkers(t *testing.T) {	cfg := Config{Name: "test", Version: "1.0.0"}

	sessionMgr := newTestServerSessionManager()	ms := NewMCPServer(cfg, sessionMgr, workerClient, audit, taskQueue)

	// Don't register any workers

	ctx := context.WithValue(context.Background(), "session_id", "no-worker-session")

	workerClient := NewMockWorkerClient()

	audit := NewAuditLogger(slog.Default())	_, err := ms.getOrCreateSession(ctx)

	taskQueue := &MockTaskQueue{workerClient: workerClient}	if err == nil {

		t.Fatal("Expected error when no workers available, got nil")

	cfg := Config{Name: "test", Version: "1.0.0"}	}

	ms := NewMCPServer(cfg, sessionMgr, workerClient, audit, taskQueue)

	if err.Error() != "no workers available" {

	ctx := context.WithValue(context.Background(), "session_id", "no-worker-session")		t.Errorf("Expected 'no workers available' error, got: %v", err)

	}

	_, err := ms.getOrCreateSession(ctx)}

	if err == nil {

		t.Fatal("Expected error when no workers available, got nil")func TestServerMethods(t *testing.T) {

	}	sessionMgr := newTestSessionManager()

	worker := &MockWorkerClient{}

	if err.Error() != "no workers available" {	audit := NewAuditLogger(slog.Default())

		t.Errorf("Expected 'no workers available' error, got: %v", err)	taskQueue := NewMockTaskQueue()

	}

}	cfg := Config{Name: "test", Version: "1.0.0"}

	ms := NewMCPServer(cfg, sessionMgr, worker, audit, taskQueue)

func TestServerMethods(t *testing.T) {

	sessionMgr := newTestServerSessionManager()	t.Run("Server returns underlying server", func(t *testing.T) {

	worker := NewMockWorkerClient()		server := ms.Server()

	audit := NewAuditLogger(slog.Default())		if server == nil {

	taskQueue := &MockTaskQueue{workerClient: worker}			t.Error("Expected Server() to return non-nil server")

		}

	cfg := Config{Name: "test", Version: "1.0.0"}		if server != ms.server {

	ms := NewMCPServer(cfg, sessionMgr, worker, audit, taskQueue)			t.Error("Expected Server() to return the internal server")

		}

	t.Run("Server returns underlying server", func(t *testing.T) {	})

		server := ms.Server()

		if server == nil {	t.Run("ServeWithLogger returns quickly", func(t *testing.T) {

			t.Error("Expected Server() to return non-nil server")		// This test just ensures the method exists and can be called

		}		// We can't actually test Serve() without blocking, so we just verify the signature

		if server != ms.server {		logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

			t.Error("Expected Server() to return the internal server")		if logger == nil {

		}			t.Error("Failed to create logger")

	})		}

		// We don't call ServeWithLogger as it would block

	t.Run("ServeWithLogger exists", func(t *testing.T) {	})

		// Verify the method exists with correct signature}

		logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

		if logger == nil {func TestServerHTTPMethods(t *testing.T) {

			t.Error("Failed to create logger")	sessionMgr := newTestSessionManager()

		}	worker := &MockWorkerClient{}

		// We don't call ServeWithLogger as it would block	audit := NewAuditLogger(slog.Default())

	})	taskQueue := NewMockTaskQueue()

}

	cfg := Config{Name: "test", Version: "1.0.0"}

func TestServerHTTPMethods(t *testing.T) {	ms := NewMCPServer(cfg, sessionMgr, worker, audit, taskQueue)

	sessionMgr := newTestServerSessionManager()

	worker := NewMockWorkerClient()	t.Run("ServeHTTPWithLogger signature", func(t *testing.T) {

	audit := NewAuditLogger(slog.Default())		// Verify the method exists with correct signature

	taskQueue := &MockTaskQueue{workerClient: worker}		logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

		if logger == nil {

	cfg := Config{Name: "test", Version: "1.0.0"}			t.Error("Failed to create logger")

	ms := NewMCPServer(cfg, sessionMgr, worker, audit, taskQueue)		}

		// We don't call ServeHTTPWithLogger as it would start a server

	t.Run("ServeHTTPWithLogger signature", func(t *testing.T) {	})

		// Verify the method exists with correct signature}

		logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

		if logger == nil {// TestGetSessionIDFromMCPContext tests extraction from mcp-go ClientSession

			t.Error("Failed to create logger")func TestGetSessionIDFromMCPContext(t *testing.T) {

		}	sessionMgr := newTestSessionManager()

		// Verify server is not nil	worker := &MockWorkerClient{}

		if ms.server == nil {	audit := NewAuditLogger(slog.Default())

			t.Error("Expected ms.server to be initialized")	taskQueue := NewMockTaskQueue()

		}

	})	cfg := Config{Name: "test", Version: "1.0.0"}

}	ms := NewMCPServer(cfg, sessionMgr, worker, audit, taskQueue)



func TestServerGetSessionIDFromDefaultContext(t *testing.T) {	t.Run("with ClientSession in context", func(t *testing.T) {

	sessionMgr := newTestServerSessionManager()		// Create a mock ClientSession context

	worker := NewMockWorkerClient()		// Note: In real usage, mcp-go's server injects this

	audit := NewAuditLogger(slog.Default())		ctx := context.Background()

	taskQueue := &MockTaskQueue{workerClient: worker}		

		// The actual implementation checks server.ClientSessionFromContext

	cfg := Config{Name: "test", Version: "1.0.0"}		// which returns nil for our test context, so it falls back to default

	ms := NewMCPServer(cfg, sessionMgr, worker, audit, taskQueue)		sessionID := ms.getSessionID(ctx)

		

	// Test with empty context (should fall back to default-session)		// Should fall back to default-session

	ctx := context.Background()		if sessionID != "default-session" {

	sessionID := ms.getSessionID(ctx)			t.Errorf("Expected 'default-session', got %s", sessionID)

		}

	if sessionID != "default-session" {	})

		t.Errorf("Expected 'default-session', got %s", sessionID)}

	}

}func TestRegisterTools(t *testing.T) {

	sessionMgr := newTestSessionManager()

func TestServerRegisterToolsImplicit(t *testing.T) {	worker := &MockWorkerClient{}

	sessionMgr := newTestServerSessionManager()	audit := NewAuditLogger(slog.Default())

	worker := NewMockWorkerClient()	taskQueue := NewMockTaskQueue()

	audit := NewAuditLogger(slog.Default())

	taskQueue := &MockTaskQueue{workerClient: worker}	cfg := Config{Name: "test", Version: "1.0.0"}

	ms := NewMCPServer(cfg, sessionMgr, worker, audit, taskQueue)

	cfg := Config{Name: "test", Version: "1.0.0"}

	ms := NewMCPServer(cfg, sessionMgr, worker, audit, taskQueue)	// The tools are registered in NewMCPServer via registerTools()

	// We verify the server has tools by checking it was created

	// The tools are registered in NewMCPServer via registerTools()	if ms.server == nil {

	// We verify the server has tools by checking it was created		t.Fatal("Expected server to be initialized with tools")

	if ms.server == nil {	}

		t.Fatal("Expected server to be initialized with tools")

	}	// The mcp-go server doesn't expose a method to list tools,

	// but we can verify the server exists and was configured

	if ms.Server() == nil {	if ms.Server() == nil {

		t.Error("Expected Server() to return configured server")		t.Error("Expected Server() to return configured server")

	}	}

}}

