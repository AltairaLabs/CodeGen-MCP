package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

func TestNewArtifactService(t *testing.T) {
	tmpDir := t.TempDir()
	artifactDir := filepath.Join(tmpDir, "artifacts")

	pool := NewSessionPool("worker-test", 5, tmpDir)

	svc, err := NewArtifactService(artifactDir, pool)
	if err != nil {
		t.Fatalf("Failed to create artifact service: %v", err)
	}

	if svc.artifactDir != artifactDir {
		t.Errorf("Expected artifactDir %s, got %s", artifactDir, svc.artifactDir)
	}

	// Check that directory was created
	if _, err := os.Stat(artifactDir); os.IsNotExist(err) {
		t.Errorf("Artifact directory was not created")
	}
}

func TestGetUploadURL(t *testing.T) {
	tmpDir := t.TempDir()
	artifactDir := filepath.Join(tmpDir, "artifacts")

	pool := NewSessionPool("worker-test", 5, tmpDir)

	svc, err := NewArtifactService(artifactDir, pool)
	if err != nil {
		t.Fatalf("Failed to create artifact service: %v", err)
	}

	tests := []struct {
		name        string
		req         *protov1.UploadRequest
		expectError bool
	}{
		{
			name: "valid request",
			req: &protov1.UploadRequest{
				SessionId:    "session-123",
				TaskId:       "task-456",
				ArtifactName: "output.zip",
				ContentType:  "application/zip",
				SizeBytes:    1024,
			},
			expectError: false,
		},
		{
			name: "missing session_id",
			req: &protov1.UploadRequest{
				ArtifactName: "output.zip",
			},
			expectError: true,
		},
		{
			name: "missing artifact_name",
			req: &protov1.UploadRequest{
				SessionId: "session-123",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := svc.GetUploadURL(context.Background(), tt.req)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if resp.UploadUrl == "" {
				t.Errorf("Expected upload_url to be set")
			}
			if resp.ArtifactId == "" {
				t.Errorf("Expected artifact_id to be set")
			}
			if resp.ExpiresAtMs == 0 {
				t.Errorf("Expected expires_at_ms to be set")
			}

			// Verify URL starts with file://
			if len(resp.UploadUrl) < 7 || resp.UploadUrl[:7] != "file://" {
				t.Errorf("Expected upload_url to start with file://, got %s", resp.UploadUrl)
			}

			// Verify session directory was created
			sessionDir := filepath.Join(artifactDir, tt.req.SessionId)
			if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
				t.Errorf("Session artifact directory was not created")
			}
		})
	}
}

func TestRecordArtifact(t *testing.T) {
	tmpDir := t.TempDir()
	artifactDir := filepath.Join(tmpDir, "artifacts")

	pool := NewSessionPool("worker-test", 5, tmpDir)

	// Create a session first
	sessionResp, err := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkerId: "worker-test",
		UserId:   "test-client",
	})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	svc, err := NewArtifactService(artifactDir, pool)
	if err != nil {
		t.Fatalf("Failed to create artifact service: %v", err)
	}

	// Generate artifact ID using session
	artifactID := sessionResp.SessionId + "-1234567890-test.txt"

	tests := []struct {
		name        string
		req         *protov1.ArtifactMetadata
		expectError bool
	}{
		{
			name: "valid artifact",
			req: &protov1.ArtifactMetadata{
				ArtifactId:     artifactID,
				TaskId:         "task-123",
				Name:           "test.txt",
				ContentType:    "text/plain",
				SizeBytes:      100,
				ChecksumSha256: "abc123",
			},
			expectError: false,
		},
		{
			name: "missing artifact_id",
			req: &protov1.ArtifactMetadata{
				TaskId: "task-123",
				Name:   "test.txt",
			},
			expectError: true,
		},
		{
			name: "missing task_id",
			req: &protov1.ArtifactMetadata{
				ArtifactId: artifactID,
				Name:       "test.txt",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := svc.RecordArtifact(context.Background(), tt.req)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if !resp.Recorded {
				t.Errorf("Expected recorded to be true")
			}
			if resp.DownloadUrl == "" {
				t.Errorf("Expected download_url to be set")
			}
		})
	}
}

func TestGetArtifact(t *testing.T) {
	tmpDir := t.TempDir()
	artifactDir := filepath.Join(tmpDir, "artifacts")

	pool := NewSessionPool("worker-test", 5, tmpDir)

	// Create a session
	sessionResp, err := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkerId: "worker-test",
		UserId:   "test-client",
	})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	svc, err := NewArtifactService(artifactDir, pool)
	if err != nil {
		t.Fatalf("Failed to create artifact service: %v", err)
	}

	// Create a test artifact file
	sessionArtifactDir := filepath.Join(artifactDir, sessionResp.SessionId)
	if err := os.MkdirAll(sessionArtifactDir, 0755); err != nil {
		t.Fatalf("Failed to create session artifact dir: %v", err)
	}

	testContent := []byte("test artifact content")
	testFile := filepath.Join(sessionArtifactDir, "test.txt")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("Failed to write test artifact: %v", err)
	}

	// Add artifact to session
	artifactID := sessionResp.SessionId + "-1234567890-test.txt"
	if err := pool.AddArtifact(sessionResp.SessionId, artifactID); err != nil {
		t.Fatalf("Failed to add artifact to session: %v", err)
	}

	// Test GetArtifact
	data, err := svc.GetArtifact(context.Background(), artifactID)
	if err != nil {
		t.Fatalf("Failed to get artifact: %v", err)
	}

	if string(data) != string(testContent) {
		t.Errorf("Expected content %q, got %q", testContent, data)
	}
}

func TestListArtifacts(t *testing.T) {
	tmpDir := t.TempDir()
	artifactDir := filepath.Join(tmpDir, "artifacts")

	pool := NewSessionPool("worker-test", 5, tmpDir)

	// Create a session
	sessionResp, err := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkerId: "worker-test",
		UserId:   "test-client",
	})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	svc, err := NewArtifactService(artifactDir, pool)
	if err != nil {
		t.Fatalf("Failed to create artifact service: %v", err)
	}

	// Add multiple artifacts
	artifactIDs := []string{
		sessionResp.SessionId + "-1-file1.txt",
		sessionResp.SessionId + "-2-file2.txt",
		sessionResp.SessionId + "-3-file3.txt",
	}

	for _, aid := range artifactIDs {
		if err := pool.AddArtifact(sessionResp.SessionId, aid); err != nil {
			t.Fatalf("Failed to add artifact: %v", err)
		}
	}

	// List artifacts
	artifacts, err := svc.ListArtifacts(context.Background(), sessionResp.SessionId)
	if err != nil {
		t.Fatalf("Failed to list artifacts: %v", err)
	}

	if len(artifacts) != len(artifactIDs) {
		t.Errorf("Expected %d artifacts, got %d", len(artifactIDs), len(artifacts))
	}

	// Check all artifacts are present
	for _, expected := range artifactIDs {
		found := false
		for _, actual := range artifacts {
			if actual == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected artifact %s not found in list", expected)
		}
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple.txt", "simple.txt"},
		{"../etc/passwd", "passwd"},
		{"/absolute/path/file.txt", "file.txt"},
		{"../../dangerous.sh", "dangerous.sh"},
		{"normal/path/file.zip", "file.zip"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeFilename(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestExtractSessionIDFromArtifactID(t *testing.T) {
	tests := []struct {
		artifactID string
		expected   string
	}{
		{"session-1234567890-9876543210-file.txt", "session-1234567890"},
		{"session-123-456-output.zip", "session-123"},
		{"abc-123-456-test", "abc-123"},
		{"invalid", ""},
		{"one-two", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.artifactID, func(t *testing.T) {
			result := extractSessionIDFromArtifactID(tt.artifactID)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestExtractFilenameFromArtifactID(t *testing.T) {
	tests := []struct {
		artifactID string
		expected   string
	}{
		{"session-1234567890-9876543210-file.txt", "file.txt"},
		{"session-123-456-output.zip", "output.zip"},
		{"abc-123-456-test", "test"},
		{"invalid", ""},
		{"one-two", "two"}, // Valid format with 1 hyphen extracts last part
	}

	for _, tt := range tests {
		t.Run(tt.artifactID, func(t *testing.T) {
			result := extractFilenameFromArtifactID(tt.artifactID)
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}
