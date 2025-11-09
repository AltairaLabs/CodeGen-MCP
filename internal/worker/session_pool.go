package worker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

const (
	defaultMaxRecentTasks   = 10
	defaultWorkspaceDirPerm = 0755
)

// SessionPool manages multiple isolated sessions on a single worker
type SessionPool struct {
	mu            sync.RWMutex
	sessions      map[string]*WorkerSession
	maxSessions   int32
	baseWorkspace string
	workerID      string
}

// WorkerSession represents an isolated session with its own workspace
type WorkerSession struct {
	SessionID        string
	WorkspaceID      string
	UserID           string
	WorkspacePath    string // Isolated workspace directory
	State            protov1.SessionState
	Config           *protov1.SessionConfig
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

// NewSessionPool creates a new session pool
func NewSessionPool(workerID string, maxSessions int32, baseWorkspace string) *SessionPool {
	return &SessionPool{
		sessions:      make(map[string]*WorkerSession),
		maxSessions:   maxSessions,
		baseWorkspace: baseWorkspace,
		workerID:      workerID,
	}
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

	// Always initialize Python venv for session isolation
	// This ensures each session has its own Python environment regardless of configured language
	if err := sp.initializePythonVenv(ctx, session); err != nil {
		return fmt.Errorf("failed to initialize Python venv: %w", err)
	}

	// Apply environment variables from session config
	// Note: These will be used when executing commands in the session
	// They are stored in session.Config.EnvVars and applied at execution time

	return nil
}

// initializePythonVenv creates a Python virtual environment for the session
func (sp *SessionPool) initializePythonVenv(ctx context.Context, session *WorkerSession) error {
	venvPath := filepath.Join(session.WorkspacePath, ".venv")

	// Check if python3 is available
	_, err := exec.LookPath("python3")
	if err != nil {
		return fmt.Errorf("python3 not found in PATH: %w", err)
	}

	// Create venv
	//nolint:gosec // G204: python3 is system binary, venvPath is in isolated session workspace
	cmd := exec.CommandContext(ctx, "python3", "-m", "venv", venvPath)
	cmd.Dir = session.WorkspacePath

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to create venv: %w (output: %s)", err, string(output))
	}

	// Upgrade pip in the venv
	pipPath := filepath.Join(venvPath, "bin", "pip")
	//nolint:gosec // G204: pip binary is in session's isolated venv directory
	cmd = exec.CommandContext(ctx, pipPath, "install", "--upgrade", "pip")
	cmd.Dir = session.WorkspacePath

	if output, err := cmd.CombinedOutput(); err != nil {
		// Non-fatal - log but don't fail
		_ = fmt.Errorf("failed to upgrade pip: %w (output: %s)", err, string(output))
	}

	return nil
}

// generateSessionID generates a unique session identifier
func generateSessionID() string {
	return fmt.Sprintf("session-%d", time.Now().UnixNano())
}
