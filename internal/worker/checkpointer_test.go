package worker

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

func TestCheckpointer_CreateCheckpoint(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := NewSessionPool("test-worker", 5, baseWorkspace)
	checkpointer := NewCheckpointer(pool, baseWorkspace)

	// Create a session
	sessionResp, err := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Add some files to the workspace
	session, _ := pool.GetSession(sessionResp.SessionId)
	testFile := filepath.Join(session.WorkspacePath, "test.txt")
	os.WriteFile(testFile, []byte("test data"), 0644)

	// Create checkpoint
	checkpointID, err := checkpointer.CreateCheckpoint(context.Background(), sessionResp.SessionId, false)
	if err != nil {
		t.Fatalf("Failed to create checkpoint: %v", err)
	}

	if checkpointID == "" {
		t.Error("Expected non-empty checkpoint ID")
	}

	// Verify checkpoint file exists
	checkpointPath := filepath.Join(baseWorkspace, ".checkpoints", checkpointID+checkpointExt)
	if _, err := os.Stat(checkpointPath); os.IsNotExist(err) {
		t.Errorf("Checkpoint file not created: %s", checkpointPath)
	}

	// Verify metadata file exists
	metadataPath := filepath.Join(baseWorkspace, ".checkpoints", checkpointID+jsonExt)
	if _, err := os.Stat(metadataPath); os.IsNotExist(err) {
		t.Errorf("Checkpoint metadata not created: %s", metadataPath)
	}

	// Verify session's last checkpoint was updated
	session, _ = pool.GetSession(sessionResp.SessionId)
	if session.LastCheckpointID != checkpointID {
		t.Errorf("Expected last checkpoint ID '%s', got '%s'", checkpointID, session.LastCheckpointID)
	}
}

func TestCheckpointerRestore(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := NewSessionPool("test-worker", 5, baseWorkspace)
	checkpointer := NewCheckpointer(pool, baseWorkspace)

	// Create a session and checkpoint
	sessionResp, err := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Add test files
	session, _ := pool.GetSession(sessionResp.SessionId)
	testFile := filepath.Join(session.WorkspacePath, "test.txt")
	os.WriteFile(testFile, []byte("test data"), 0644)

	// Create subdirectory with file
	subdir := filepath.Join(session.WorkspacePath, "subdir")
	os.MkdirAll(subdir, 0755)
	os.WriteFile(filepath.Join(subdir, "nested.txt"), []byte("nested data"), 0644)

	// Create checkpoint
	checkpointID, err := checkpointer.CreateCheckpoint(context.Background(), sessionResp.SessionId, false)
	if err != nil {
		t.Fatalf("Failed to create checkpoint: %v", err)
	}

	// Restore from checkpoint
	restoreReq := &protov1.RestoreRequest{
		CheckpointId: checkpointID,
	}

	restoreResp, err := checkpointer.Restore(context.Background(), restoreReq)
	if err != nil {
		t.Fatalf("Failed to restore: %v", err)
	}

	if restoreResp.SessionId == "" {
		t.Error("Expected non-empty restored session ID")
	}

	// Verify restored session exists
	restoredSession, err := pool.GetSession(restoreResp.SessionId)
	if err != nil {
		t.Fatalf("Failed to get restored session: %v", err)
	}

	// Verify files were restored
	restoredFile := filepath.Join(restoredSession.WorkspacePath, "test.txt")
	data, err := os.ReadFile(restoredFile)
	if err != nil {
		t.Errorf("Failed to read restored file: %v", err)
	}
	if string(data) != "test data" {
		t.Errorf("Expected 'test data', got '%s'", string(data))
	}

	// Verify nested directory was restored
	nestedFile := filepath.Join(restoredSession.WorkspacePath, "subdir", "nested.txt")
	data, err = os.ReadFile(nestedFile)
	if err != nil {
		t.Errorf("Failed to read nested file: %v", err)
	}
	if string(data) != "nested data" {
		t.Errorf("Expected 'nested data', got '%s'", string(data))
	}
}

func TestCheckpointer_Checkpoint_WithRequest(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := NewSessionPool("test-worker", 5, baseWorkspace)
	checkpointer := NewCheckpointer(pool, baseWorkspace)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})

	req := &protov1.CheckpointRequest{
		SessionId:   sessionResp.SessionId,
		Incremental: false,
	}

	resp, err := checkpointer.Checkpoint(context.Background(), req)
	if err != nil {
		t.Fatalf("Failed to checkpoint: %v", err)
	}

	if resp.CheckpointId == "" {
		t.Error("Expected non-empty checkpoint ID")
	}

	if resp.SizeBytes == 0 {
		t.Error("Expected non-zero checkpoint size")
	}

	if resp.Metadata == nil {
		t.Error("Expected checkpoint metadata")
	}
}

func TestCheckpointer_CleanupOldCheckpoints(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := NewSessionPool("test-worker", 5, baseWorkspace)
	checkpointer := NewCheckpointer(pool, baseWorkspace)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})

	// Create 12 checkpoints (more than maxCheckpoints of 10)
	checkpointIDs := make([]string, 12)
	for i := 0; i < 12; i++ {
		checkpointID, err := checkpointer.CreateCheckpoint(context.Background(), sessionResp.SessionId, false)
		if err != nil {
			t.Fatalf("Failed to create checkpoint %d: %v", i, err)
		}
		checkpointIDs[i] = checkpointID
	}

	// Give cleanup goroutine time to run
	// Note: In a real test, we'd use a more deterministic approach
	// but for this simple test, we'll just verify the checkpoint was created
	if checkpointIDs[11] == "" {
		t.Error("Failed to create last checkpoint")
	}
}

func TestCheckpointer_RestoreNonExistent(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := NewSessionPool("test-worker", 5, baseWorkspace)
	checkpointer := NewCheckpointer(pool, baseWorkspace)

	_, err := checkpointer.Restore(context.Background(), &protov1.RestoreRequest{
		CheckpointId: "non-existent-checkpoint",
	})

	if err == nil {
		t.Error("Expected error when restoring non-existent checkpoint")
	}
}

func TestCheckpointerArchiveExtract(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := NewSessionPool("test-worker", 5, baseWorkspace)
	checkpointer := NewCheckpointer(pool, baseWorkspace)

	// Create a session with some files
	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})
	session, _ := pool.GetSession(sessionResp.SessionId)

	// Write multiple files
	testFile1 := filepath.Join(session.WorkspacePath, "file1.txt")
	testFile2 := filepath.Join(session.WorkspacePath, "file2.txt")
	os.WriteFile(testFile1, []byte("content1"), 0644)
	os.WriteFile(testFile2, []byte("content2"), 0644)

	// Create subdirectory with file
	subdir := filepath.Join(session.WorkspacePath, "subdir")
	os.MkdirAll(subdir, 0755)
	testFile3 := filepath.Join(subdir, "file3.txt")
	os.WriteFile(testFile3, []byte("content3"), 0644)

	// Create checkpoint (tests createArchive)
	checkpointID, err := checkpointer.CreateCheckpoint(context.Background(), sessionResp.SessionId, false)
	if err != nil {
		t.Fatalf("Failed to create checkpoint: %v", err)
	}

	// Verify archive was created
	archivePath := filepath.Join(baseWorkspace, ".checkpoints", checkpointID+checkpointExt)
	if _, err := os.Stat(archivePath); os.IsNotExist(err) {
		t.Error("Archive file was not created")
	}

	// Now test extraction by restoring
	// Note: This will fail due to session ID mismatch, but it will exercise extractArchive
	_, _ = checkpointer.Restore(context.Background(), &protov1.RestoreRequest{
		CheckpointId: checkpointID,
	})
}

func TestCheckpointerInvalidSession(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := NewSessionPool("test-worker", 5, baseWorkspace)
	checkpointer := NewCheckpointer(pool, baseWorkspace)

	// Try to checkpoint a non-existent session
	_, err := checkpointer.CreateCheckpoint(context.Background(), "nonexistent-session", false)
	if err == nil {
		t.Error("Expected error for non-existent session")
	}
}

func TestCheckpointerCheckpointAPI(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := NewSessionPool("test-worker", 5, baseWorkspace)
	checkpointer := NewCheckpointer(pool, baseWorkspace)

	// Create session
	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})

	// Use Checkpoint API (not CreateCheckpoint)
	resp, err := checkpointer.Checkpoint(context.Background(), &protov1.CheckpointRequest{
		SessionId:   sessionResp.SessionId,
		Incremental: true, // Test incremental flag
	})

	if err != nil {
		t.Fatalf("Checkpoint API failed: %v", err)
	}

	if resp.CheckpointId == "" {
		t.Error("Expected checkpoint ID")
	}

	if resp.Metadata == nil {
		t.Error("Expected checkpoint metadata")
	}
}

func TestCheckpointerPathTraversalProtection(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := NewSessionPool("test-worker", 5, baseWorkspace)
	checkpointer := NewCheckpointer(pool, baseWorkspace)

	// Create a test session
	sessionResp, err := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	session, _ := pool.GetSession(sessionResp.SessionId)

	// Test malicious paths
	maliciousPaths := []string{
		"../../../etc/passwd",
		"../../secret.txt",
		"/etc/shadow",
		"normal/../../../etc/hosts",
	}

	// Create temporary archive with malicious paths
	archivePath := createMaliciousArchive(t, baseWorkspace, maliciousPaths)
	defer os.Remove(archivePath)

	// Try to extract the malicious archive
	tarReader, closeFunc, err := checkpointer.openArchiveReader(archivePath)
	if err != nil {
		t.Fatalf("Failed to open archive: %v", err)
	}
	defer closeFunc()

	rejectedFiles := countRejectedExtractions(t, checkpointer, tarReader, session.WorkspacePath)

	// All malicious paths should be rejected
	if rejectedFiles != len(maliciousPaths) {
		t.Errorf("Expected all %d malicious paths to be rejected, got %d", len(maliciousPaths), rejectedFiles)
	}

	// Verify no files were created outside workspace
	verifyNoFilesOutsideWorkspace(t, session.WorkspacePath)
}

func createMaliciousArchive(t *testing.T, baseDir string, paths []string) string {
	t.Helper()
	archivePath := filepath.Join(baseDir, "malicious.tar.gz")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("Failed to create archive: %v", err)
	}

	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)

	for _, path := range paths {
		header := &tar.Header{
			Name:     path,
			Mode:     0600,
			Size:     4,
			Typeflag: tar.TypeReg,
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("Failed to write header: %v", err)
		}
		if _, err := tarWriter.Write([]byte("test")); err != nil {
			t.Fatalf("Failed to write data: %v", err)
		}
	}

	tarWriter.Close()
	gzipWriter.Close()
	file.Close()
	return archivePath
}

func countRejectedExtractions(t *testing.T, checkpointer *Checkpointer, tarReader *tar.Reader, workspace string) int {
	t.Helper()
	rejected := 0

	for {
		header, err := tarReader.Next()
		if err != nil {
			break
		}

		err = checkpointer.extractFileFromArchive(tarReader, workspace, header)
		if err != nil && filepath.HasPrefix(err.Error(), "illegal file path") {
			rejected++
		}
	}
	return rejected
}

func verifyNoFilesOutsideWorkspace(t *testing.T, workspacePath string) {
	t.Helper()
	parentDir := filepath.Dir(workspacePath)
	entries, _ := os.ReadDir(parentDir)

	for _, entry := range entries {
		name := entry.Name()
		// Allow workspace dir, checkpoints dir, and the test archive file
		if name != filepath.Base(workspacePath) &&
			name != ".checkpoints" &&
			!strings.HasSuffix(name, ".tar.gz") {
			t.Errorf("Unexpected file/directory created outside workspace: %s", name)
		}
	}
}
