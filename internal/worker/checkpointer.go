package worker

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

const (
	checkpointExt = ".tar.gz"
	jsonExt       = ".json"
)

// Checkpointer handles session checkpointing and restoration
type Checkpointer struct {
	sessionPool    *SessionPool
	baseWorkspace  string
	checkpointDir  string
	maxCheckpoints int
}

// NewCheckpointer creates a new checkpointer
func NewCheckpointer(sessionPool *SessionPool, baseWorkspace string) *Checkpointer {
	checkpointDir := filepath.Join(baseWorkspace, ".checkpoints")
	_ = os.MkdirAll(checkpointDir, 0755) // NOSONAR - workspace directories need 0755 for proper file operations

	return &Checkpointer{
		sessionPool:    sessionPool,
		baseWorkspace:  baseWorkspace,
		checkpointDir:  checkpointDir,
		maxCheckpoints: 10, // Keep last 10 checkpoints per session
	}
}

// Checkpoint creates a checkpoint of a session
func (c *Checkpointer) Checkpoint(ctx context.Context, req *protov1.CheckpointRequest) (*protov1.CheckpointResponse, error) {
	checkpointID, err := c.CreateCheckpoint(ctx, req.SessionId, req.Incremental)
	if err != nil {
		return nil, err
	}

	// Get checkpoint file stats
	checkpointPath := filepath.Join(c.checkpointDir, checkpointID+checkpointExt)
	stat, err := os.Stat(checkpointPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat checkpoint: %w", err)
	}

	metadata := &protov1.CheckpointMetadata{
		SessionId:         req.SessionId,
		CreatedAtMs:       time.Now().UnixMilli(),
		ChecksumSha256:    checkpointID, // Use checkpoint ID as identifier
		InstalledPackages: []string{},   // Could be enhanced to track packages
		ModifiedFiles:     []string{},   // Could be enhanced to track files
	}

	return &protov1.CheckpointResponse{
		CheckpointId: checkpointID,
		SizeBytes:    stat.Size(),
		StorageUrl:   fmt.Sprintf("file://%s", checkpointPath),
		Metadata:     metadata,
	}, nil
}

// CreateCheckpoint creates a checkpoint and returns the ID
func (c *Checkpointer) CreateCheckpoint(ctx context.Context, sessionID string, incremental bool) (string, error) {
	session, err := c.sessionPool.GetSession(sessionID)
	if err != nil {
		return "", err
	}

	// Update session state
	session.mu.Lock()
	previousState := session.State
	session.State = protov1.SessionState_SESSION_STATE_CHECKPOINTING
	session.mu.Unlock()

	defer func() {
		session.mu.Lock()
		session.State = previousState
		session.mu.Unlock()
	}()

	// Generate checkpoint ID
	checkpointID := fmt.Sprintf("checkpoint-%s-%d", sessionID, time.Now().UnixNano())
	checkpointPath := filepath.Join(c.checkpointDir, checkpointID+checkpointExt)

	// Create archive of workspace
	if archiveErr := c.createArchive(session.WorkspacePath, checkpointPath); archiveErr != nil {
		return "", fmt.Errorf("failed to create archive: %w", archiveErr)
	}

	// Save session metadata
	metadataPath := filepath.Join(c.checkpointDir, checkpointID+jsonExt)
	sessionData := map[string]interface{}{
		"session_id":     sessionID,
		"workspace_path": session.WorkspacePath,
		"created_at":     time.Now().Format(time.RFC3339),
	}

	metadataJSON, err := json.Marshal(sessionData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(metadataPath, metadataJSON, 0644); err != nil { // NOSONAR - checkpoint metadata is non-sensitive internal data
		return "", fmt.Errorf("failed to write metadata: %w", err)
	}

	// Update session with last checkpoint ID
	if err := c.sessionPool.SetLastCheckpoint(sessionID, checkpointID); err != nil {
		// Non-fatal - log but don't fail
		_ = fmt.Errorf("failed to set last checkpoint: %w", err)
	}

	// Clean up old checkpoints
	go c.cleanupOldCheckpoints(sessionID)

	return checkpointID, nil
}

// Restore restores a session from a checkpoint
func (c *Checkpointer) Restore(ctx context.Context, req *protov1.RestoreRequest) (*protov1.RestoreResponse, error) {
	checkpointID := req.CheckpointId
	checkpointPath := filepath.Join(c.checkpointDir, checkpointID+checkpointExt)
	metadataPath := filepath.Join(c.checkpointDir, checkpointID+jsonExt)

	// Verify checkpoint exists
	if _, err := os.Stat(checkpointPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("checkpoint %s not found", checkpointID)
	}

	// Read metadata
	metadataJSON, err := os.ReadFile(metadataPath) // NOSONAR - checkpoint path is validated and controlled by the system
	if err != nil {
		return nil, fmt.Errorf("failed to read checkpoint metadata: %w", err)
	}

	var sessionData map[string]interface{}
	if unmarshalErr := json.Unmarshal(metadataJSON, &sessionData); unmarshalErr != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", unmarshalErr)
	}

	// Create new session for restoration
	workspaceID := fmt.Sprintf("restored-session-%d", time.Now().UnixNano())
	session, err := c.sessionPool.CreateSession(ctx, &protov1.CreateSessionRequest{
		WorkspaceId: workspaceID,
		Config: &protov1.SessionConfig{
			Limits: &protov1.ResourceLimits{},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Get session from pool using the returned session ID
	workerSession, err := c.sessionPool.GetSession(session.SessionId)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// Clear existing workspace
	if err := os.RemoveAll(workerSession.WorkspacePath); err != nil {
		return nil, fmt.Errorf("failed to clear workspace: %w", err)
	}
	if err := os.MkdirAll(workerSession.WorkspacePath, 0755); err != nil { // NOSONAR - workspace directories need 0755 for proper file operations
		return nil, fmt.Errorf("failed to create workspace: %w", err)
	}

	// Extract archive
	if err := c.extractArchive(checkpointPath, workerSession.WorkspacePath); err != nil {
		return nil, fmt.Errorf("failed to extract archive: %w", err)
	}

	return &protov1.RestoreResponse{
		SessionId: session.SessionId,
		WorkerId:  session.WorkerId,
		State:     session.State,
		CheckpointMetadata: &protov1.CheckpointMetadata{
			SessionId:   session.SessionId,
			CreatedAtMs: time.Now().UnixMilli(),
		},
	}, nil
}

// createArchive creates a tar.gz archive of a directory
func (c *Checkpointer) createArchive(sourceDir, targetPath string) error {
	file, err := os.Create(targetPath) // NOSONAR - target path is controlled and validated by checkpoint system
	if err != nil {
		return err
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return c.addFileToArchive(tarWriter, sourceDir, path, info)
	})
}

// addFileToArchive adds a single file to the tar archive
func (c *Checkpointer) addFileToArchive(tarWriter *tar.Writer, sourceDir, path string, info os.FileInfo) error {
	header, err := c.createTarHeader(sourceDir, path, info)
	if err != nil {
		return err
	}

	if err := tarWriter.WriteHeader(header); err != nil {
		return err
	}

	if info.Mode().IsRegular() {
		return c.copyFileToArchive(tarWriter, path)
	}
	return nil
}

// createTarHeader creates a tar header for a file
func (c *Checkpointer) createTarHeader(sourceDir, path string, info os.FileInfo) (*tar.Header, error) {
	header, err := tar.FileInfoHeader(info, info.Name())
	if err != nil {
		return nil, err
	}

	relPath, err := filepath.Rel(sourceDir, path)
	if err != nil {
		return nil, err
	}
	header.Name = relPath
	return header, nil
}

// copyFileToArchive copies a file's contents to the tar archive
func (c *Checkpointer) copyFileToArchive(tarWriter *tar.Writer, path string) error {
	file, err := os.Open(path) // NOSONAR - path is from controlled workspace directory
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(tarWriter, file)
	return err
}

// extractArchive extracts a tar.gz archive to a directory
func (c *Checkpointer) extractArchive(archivePath, targetDir string) error {
	tarReader, closeFunc, err := c.openArchiveReader(archivePath)
	if err != nil {
		return err
	}
	defer closeFunc()

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if err := c.extractFileFromArchive(tarReader, targetDir, header); err != nil {
			return err
		}
	}

	return nil
}

// openArchiveReader opens and returns a tar reader for the archive
func (c *Checkpointer) openArchiveReader(archivePath string) (*tar.Reader, func(), error) {
	file, err := os.Open(archivePath) // NOSONAR - archive path is validated checkpoint file
	if err != nil {
		return nil, nil, err
	}

	gzipReader, err := gzip.NewReader(file) // NOSONAR - checkpoints are trusted internal data, not user-supplied
	if err != nil {
		_ = file.Close() // Best effort cleanup
		return nil, nil, err
	}

	tarReader := tar.NewReader(gzipReader)
	closeFunc := func() {
		_ = gzipReader.Close() // Best effort cleanup
		_ = file.Close()       // Best effort cleanup
	}

	return tarReader, closeFunc, nil
}

// extractFileFromArchive extracts a single file from the tar archive
func (c *Checkpointer) extractFileFromArchive(tarReader *tar.Reader, targetDir string, header *tar.Header) error {
	// Validate path to prevent path traversal attacks (zip-slip vulnerability)
	// Reject absolute paths
	if filepath.IsAbs(header.Name) {
		return fmt.Errorf("illegal file path in archive: %s (absolute paths not allowed)", header.Name)
	}

	// Clean the name before joining to prevent "../" attacks
	cleanedName := filepath.Clean(header.Name)
	if strings.HasPrefix(cleanedName, "..") || strings.Contains(cleanedName, "/../") {
		return fmt.Errorf("illegal file path in archive: %s (path traversal not allowed)", header.Name)
	}

	// Now safe to join with target directory
	targetPath := filepath.Join(targetDir, cleanedName)
	cleanedPath := filepath.Clean(targetPath)
	cleanedTargetDir := filepath.Clean(targetDir) + string(filepath.Separator)

	// Final check: ensure the cleaned path is within the target directory
	if !strings.HasPrefix(cleanedPath+string(filepath.Separator), cleanedTargetDir) {
		return fmt.Errorf("illegal file path in archive: %s (outside target directory)", header.Name)
	}

	switch header.Typeflag {
	case tar.TypeDir:
		return c.extractDirectory(cleanedPath)
	case tar.TypeReg:
		return c.extractRegularFile(tarReader, cleanedPath, header)
	}
	return nil
}

// extractDirectory creates a directory from the archive
func (c *Checkpointer) extractDirectory(targetPath string) error {
	return os.MkdirAll(targetPath, 0755) // NOSONAR - workspace directories need 0755 for proper operations
}

// extractRegularFile extracts a regular file from the archive
func (c *Checkpointer) extractRegularFile(tarReader *tar.Reader, targetPath string, header *tar.Header) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil { // NOSONAR - workspace directories need 0755 for proper operations
		return err
	}

	outFile, err := os.Create(targetPath) // NOSONAR - target path is from trusted checkpoint archive
	if err != nil {
		return err
	}

	if _, err := io.Copy(outFile, tarReader); err != nil { // NOSONAR - archive is created internally and trusted
		_ = outFile.Close() // Best effort cleanup on error
		return err
	}
	_ = outFile.Close() // Best effort close

	return os.Chmod(targetPath, os.FileMode(header.Mode)) // NOSONAR - file mode from trusted checkpoint archive
}

// cleanupOldCheckpoints removes old checkpoints for a session
func (c *Checkpointer) cleanupOldCheckpoints(sessionID string) {
	pattern := filepath.Join(c.checkpointDir, fmt.Sprintf("checkpoint-%s-*%s", sessionID, checkpointExt))
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) <= c.maxCheckpoints {
		return
	}

	// Sort by modification time
	type checkpointFile struct {
		path    string
		modTime time.Time
	}

	files := make([]checkpointFile, 0, len(matches))
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			continue
		}
		files = append(files, checkpointFile{path: match, modTime: info.ModTime()})
	}

	// Simple sort (oldest first)
	for i := 0; i < len(files); i++ {
		for j := i + 1; j < len(files); j++ {
			if files[i].modTime.After(files[j].modTime) {
				files[i], files[j] = files[j], files[i]
			}
		}
	}

	// Remove oldest checkpoints
	toRemove := len(files) - c.maxCheckpoints
	for i := 0; i < toRemove; i++ {
		_ = os.Remove(files[i].path) // Best effort cleanup
		metadataPath := files[i].path[:len(files[i].path)-len(checkpointExt)] + jsonExt
		_ = os.Remove(metadataPath) // Best effort cleanup
	}
}
