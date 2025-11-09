package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/worker/language"
	"github.com/AltairaLabs/codegen-mcp/internal/worker/language/python"
)

const (
	defaultMaxRecentTasks   = 10
	defaultWorkspaceDirPerm = 0755
)

// SessionPool manages multiple isolated sessions on a single worker
type SessionPool struct {
	mu              sync.RWMutex
	sessions        map[string]*WorkerSession
	maxSessions     int32
	baseWorkspace   string
	workerID        string
	providerFactory language.Factory
}

// WorkerSession represents an isolated session with its own workspace
type WorkerSession struct {
	SessionID        string
	WorkspaceID      string
	UserID           string
	WorkspacePath    string // Isolated workspace directory
	State            protov1.SessionState
	Config           *protov1.SessionConfig
	Provider         language.Provider // Language-specific provider
	CreatedAt        time.Time
	LastActivity     time.Time
	ActiveTasks      int32
	ResourceUsage    *protov1.ResourceUsage
	InstalledPkgs    []string // Track installed Python packages
	RecentTasks      []string // Track recent task IDs (max 10)
	LastCheckpointID string   // Last successful checkpoint ID
	ArtifactIDs      []string // Artifact IDs created in this session
	mu               sync.RWMutex
	maxRecentTasks   int
}

// NewSessionPool creates a new session pool with default Python provider
func NewSessionPool(workerID string, maxSessions int32, baseWorkspace string) *SessionPool {
	// Create default registry with Python provider
	registry := language.NewRegistry()
	registry.Register("python", func() language.Provider {
		return python.NewProvider()
	})

	return &SessionPool{
		sessions:        make(map[string]*WorkerSession),
		maxSessions:     maxSessions,
		baseWorkspace:   baseWorkspace,
		workerID:        workerID,
		providerFactory: registry,
	}
}

// NewSessionPoolWithFactory creates a new session pool with a custom provider factory.
// Used for testing with mock providers.
func NewSessionPoolWithFactory(
	workerID string,
	maxSessions int32,
	baseWorkspace string,
	factory language.Factory,
) *SessionPool {
	return &SessionPool{
		sessions:        make(map[string]*WorkerSession),
		maxSessions:     maxSessions,
		baseWorkspace:   baseWorkspace,
		workerID:        workerID,
		providerFactory: factory,
	}
}

// CreateSessionWithID creates a new session with a specific session ID (used for coordinator-assigned sessions)
func (sp *SessionPool) CreateSessionWithID(ctx context.Context, sessionID, workspaceID, userID string, envVars map[string]string) error {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	// Check capacity
	//nolint:gosec // G115: Session count is bounded by maxSessions config, no overflow risk
	if int32(len(sp.sessions)) >= sp.maxSessions {
		return fmt.Errorf("session pool at capacity: %d/%d", len(sp.sessions), sp.maxSessions)
	}

	// Check if session already exists
	if _, exists := sp.sessions[sessionID]; exists {
		return fmt.Errorf("session already exists: %s", sessionID)
	}

	// Create isolated workspace directory
	workspacePath := filepath.Join(sp.baseWorkspace, sessionID)
	//nolint:lll // NOSONAR comment explains security considerations
	if err := os.MkdirAll(workspacePath, defaultWorkspaceDirPerm); err != nil { // NOSONAR - workspace directories need 0755 for proper file operations
		return fmt.Errorf("failed to create workspace: %w", err)
	}

	// Create session config
	config := &protov1.SessionConfig{
		EnvVars: envVars,
	}

	// Initialize session
	session := &WorkerSession{
		SessionID:        sessionID,
		WorkspaceID:      workspaceID,
		UserID:           userID,
		WorkspacePath:    workspacePath,
		State:            protov1.SessionState_SESSION_STATE_INITIALIZING,
		Config:           config,
		CreatedAt:        time.Now(),
		LastActivity:     time.Now(),
		ActiveTasks:      0,
		ResourceUsage:    &protov1.ResourceUsage{},
		InstalledPkgs:    make([]string, 0),
		RecentTasks:      make([]string, 0),
		LastCheckpointID: "",
		ArtifactIDs:      make([]string, 0),
		maxRecentTasks:   defaultMaxRecentTasks,
	}

	// Apply session configuration
	if err := sp.initializeSession(ctx, session); err != nil {
		_ = os.RemoveAll(workspacePath) // Best effort cleanup on error
		return fmt.Errorf("failed to initialize session: %w", err)
	}

	session.State = protov1.SessionState_SESSION_STATE_READY
	sp.sessions[sessionID] = session

	return nil
}

// CreateSession creates a new isolated session
//
//nolint:lll // Protobuf types create inherently long function signatures
func (sp *SessionPool) CreateSession(ctx context.Context, req *protov1.CreateSessionRequest) (*protov1.CreateSessionResponse, error) {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	// Check capacity
	//nolint:gosec // G115: Session count is bounded by maxSessions config, no overflow risk
	if int32(len(sp.sessions)) >= sp.maxSessions {
		return nil, fmt.Errorf("session pool at capacity: %d/%d", len(sp.sessions), sp.maxSessions)
	}

	// Generate session ID
	sessionID := generateSessionID()

	// Create isolated workspace directory
	workspacePath := filepath.Join(sp.baseWorkspace, sessionID)
	//nolint:lll // NOSONAR comment explains security considerations
	if err := os.MkdirAll(workspacePath, defaultWorkspaceDirPerm); err != nil { // NOSONAR - workspace directories need 0755 for proper file operations
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}

	// Initialize session
	session := &WorkerSession{
		SessionID:        sessionID,
		WorkspaceID:      req.WorkspaceId,
		UserID:           req.UserId,
		WorkspacePath:    workspacePath,
		State:            protov1.SessionState_SESSION_STATE_INITIALIZING,
		Config:           req.Config,
		CreatedAt:        time.Now(),
		LastActivity:     time.Now(),
		ActiveTasks:      0,
		ResourceUsage:    &protov1.ResourceUsage{},
		InstalledPkgs:    make([]string, 0),
		RecentTasks:      make([]string, 0),
		LastCheckpointID: "",
		ArtifactIDs:      make([]string, 0),
		maxRecentTasks:   defaultMaxRecentTasks,
	}

	// Apply session configuration
	if err := sp.initializeSession(ctx, session); err != nil {
		_ = os.RemoveAll(workspacePath) // Best effort cleanup on error
		return nil, fmt.Errorf("failed to initialize session: %w", err)
	}

	session.State = protov1.SessionState_SESSION_STATE_READY
	sp.sessions[sessionID] = session

	return &protov1.CreateSessionResponse{
		SessionId:     sessionID,
		WorkerId:      sp.workerID,
		WorkspacePath: workspacePath,
		State:         session.State,
		CreatedAtMs:   session.CreatedAt.UnixMilli(),
	}, nil
}

// GetSession retrieves a session by ID
func (sp *SessionPool) GetSession(sessionID string) (*WorkerSession, error) {
	sp.mu.RLock()
	defer sp.mu.RUnlock()

	session, exists := sp.sessions[sessionID]
	if !exists {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	return session, nil
}

// DestroySession removes a session and cleans up resources
func (sp *SessionPool) DestroySession(ctx context.Context, sessionID string, saveCheckpoint bool) error {
	sp.mu.Lock()
	session, exists := sp.sessions[sessionID]
	if !exists {
		sp.mu.Unlock()
		return fmt.Errorf("session not found: %s", sessionID)
	}
	delete(sp.sessions, sessionID)
	sp.mu.Unlock()

	// Clean up language provider
	if session.Provider != nil {
		if err := session.Provider.Cleanup(ctx); err != nil {
			// Log error but continue with workspace cleanup
			_ = fmt.Errorf("failed to cleanup provider: %w", err)
		}
	}

	// Clean up workspace
	if err := os.RemoveAll(session.WorkspacePath); err != nil {
		return fmt.Errorf("failed to remove workspace: %w", err)
	}

	return nil
}

// GetCapacity returns current session capacity
func (sp *SessionPool) GetCapacity() *protov1.SessionCapacity {
	sp.mu.RLock()
	defer sp.mu.RUnlock()

	sessionInfos := make([]*protov1.SessionInfo, 0, len(sp.sessions))
	for _, session := range sp.sessions {
		session.mu.RLock()
		sessionInfos = append(sessionInfos, &protov1.SessionInfo{
			SessionId:      session.SessionID,
			WorkspaceId:    session.WorkspaceID,
			CreatedAtMs:    session.CreatedAt.UnixMilli(),
			LastActivityMs: session.LastActivity.UnixMilli(),
			State:          session.State,
			Usage:          session.ResourceUsage,
			ActiveTasks:    session.ActiveTasks,
		})
		session.mu.RUnlock()
	}

	//nolint:gosec // G115: Session count is bounded by maxSessions config, no overflow risk
	activeSessions := int32(len(sp.sessions))
	return &protov1.SessionCapacity{
		TotalSessions:     sp.maxSessions,
		ActiveSessions:    activeSessions,
		AvailableSessions: sp.maxSessions - activeSessions,
		Sessions:          sessionInfos,
	}
}

// UpdateSessionActivity updates the last activity time for a session
func (sp *SessionPool) UpdateSessionActivity(sessionID string) error {
	session, err := sp.GetSession(sessionID)
	if err != nil {
		return err
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	session.LastActivity = time.Now()
	return nil
}

// IncrementActiveTasks increments the active task count for a session
func (sp *SessionPool) IncrementActiveTasks(sessionID string) error {
	session, err := sp.GetSession(sessionID)
	if err != nil {
		return err
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	session.ActiveTasks++
	session.State = protov1.SessionState_SESSION_STATE_BUSY
	return nil
}

// DecrementActiveTasks decrements the active task count for a session
func (sp *SessionPool) DecrementActiveTasks(sessionID string) error {
	session, err := sp.GetSession(sessionID)
	if err != nil {
		return err
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	session.ActiveTasks--
	if session.ActiveTasks == 0 {
		session.State = protov1.SessionState_SESSION_STATE_IDLE
	}
	return nil
}

// AddTaskToHistory adds a task ID to the session's recent tasks
func (sp *SessionPool) AddTaskToHistory(sessionID, taskID string) error {
	session, err := sp.GetSession(sessionID)
	if err != nil {
		return err
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	// Add task to front of list
	session.RecentTasks = append([]string{taskID}, session.RecentTasks...)

	// Keep only last N tasks
	if len(session.RecentTasks) > session.maxRecentTasks {
		session.RecentTasks = session.RecentTasks[:session.maxRecentTasks]
	}

	return nil
}

// SetLastCheckpoint sets the last checkpoint ID for a session
func (sp *SessionPool) SetLastCheckpoint(sessionID, checkpointID string) error {
	session, err := sp.GetSession(sessionID)
	if err != nil {
		return err
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	session.LastCheckpointID = checkpointID
	return nil
}

// AddArtifact adds an artifact ID to the session
func (sp *SessionPool) AddArtifact(sessionID, artifactID string) error {
	session, err := sp.GetSession(sessionID)
	if err != nil {
		return err
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	session.ArtifactIDs = append(session.ArtifactIDs, artifactID)
	return nil
}

// initializeSession sets up a new session environment
func (sp *SessionPool) initializeSession(ctx context.Context, session *WorkerSession) error {
	// Create basic workspace structure
	dirs := []string{"src", "tests", "artifacts"}
	for _, dir := range dirs {
		dirPath := filepath.Join(session.WorkspacePath, dir)
		//nolint:gosec // G301: Workspace directories need 0755 for proper file operations
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Create language-specific provider
	lang := "python" // default language
	if session.Config != nil && session.Config.Language != "" {
		lang = session.Config.Language
	}

	provider, err := sp.providerFactory.CreateProvider(lang)
	if err != nil {
		return fmt.Errorf("failed to create language provider: %w", err)
	}
	session.Provider = provider

	// Initialize language environment
	initConfig := language.InitConfig{
		WorkspacePath: session.WorkspacePath,
		SessionID:     session.SessionID,
		EnvVars:       make(map[string]string),
	}

	if session.Config != nil && session.Config.EnvVars != nil {
		initConfig.EnvVars = session.Config.EnvVars
	}

	if err := provider.Initialize(ctx, initConfig); err != nil {
		return fmt.Errorf("failed to initialize language provider: %w", err)
	}

	return nil
}

// generateSessionID generates a unique session identifier
func generateSessionID() string {
	return fmt.Sprintf("session-%d", time.Now().UnixNano())
}
