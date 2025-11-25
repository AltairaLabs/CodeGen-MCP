package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ArtifactService manages artifact storage and retrieval using local filesystem
type ArtifactService struct {
	protov1.UnimplementedArtifactServiceServer
	artifactDir string // Base directory for artifact storage
	sessionPool *SessionPool
}

// NewArtifactService creates a new artifact service
func NewArtifactService(artifactDir string, sessionPool *SessionPool) (*ArtifactService, error) {
	// Create artifact directory if it doesn't exist
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create artifact directory: %w", err)
	}

	return &ArtifactService{
		artifactDir: artifactDir,
		sessionPool: sessionPool,
	}, nil
}

// GetUploadURL generates a local file path for artifact upload
// For MVP, this returns a local file path instead of S3 presigned URL
func (as *ArtifactService) GetUploadURL(ctx context.Context, req *protov1.UploadRequest) (*protov1.UploadResponse, error) {
	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}
	if req.ArtifactName == "" {
		return nil, status.Error(codes.InvalidArgument, "artifact_name is required")
	}

	// Generate unique artifact ID
	artifactID := fmt.Sprintf("%s-%d-%s",
		req.SessionId,
		time.Now().UnixNano(),
		sanitizeFilename(req.ArtifactName),
	)

	// Create session-specific artifact directory
	sessionArtifactDir := filepath.Join(as.artifactDir, req.SessionId)
	if err := os.MkdirAll(sessionArtifactDir, 0755); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create session artifact directory: %v", err)
	}

	// Generate local file path (acts as "upload URL" for MVP)
	artifactPath := filepath.Join(sessionArtifactDir, sanitizeFilename(req.ArtifactName))

	// For MVP, return file:// URL scheme
	uploadURL := "file://" + artifactPath

	// Set expiration to 1 hour from now
	expiresAt := time.Now().Add(1 * time.Hour).UnixMilli()

	return &protov1.UploadResponse{
		UploadUrl:   uploadURL,
		ArtifactId:  artifactID,
		ExpiresAtMs: expiresAt,
	}, nil
}

// RecordArtifact records artifact metadata after upload
func (as *ArtifactService) RecordArtifact(ctx context.Context, req *protov1.ArtifactMetadata) (*protov1.ArtifactResponse, error) {
	if req.ArtifactId == "" {
		return nil, status.Error(codes.InvalidArgument, "artifact_id is required")
	}
	if req.TaskId == "" {
		return nil, status.Error(codes.InvalidArgument, "task_id is required")
	}

	// Extract session ID from artifact ID (format: sessionID-timestamp-filename)
	sessionID := extractSessionIDFromArtifactID(req.ArtifactId)
	if sessionID == "" {
		return nil, status.Error(codes.InvalidArgument, "invalid artifact_id format")
	}

	// Add artifact to session tracking
	if as.sessionPool != nil {
		if err := as.sessionPool.AddArtifact(sessionID, req.ArtifactId); err != nil {
			// Log error but don't fail the request
			fmt.Printf("Warning: failed to track artifact in session: %v\n", err)
		}
	}

	// Generate download URL (for MVP, same as artifact path)
	artifactPath := filepath.Join(as.artifactDir, sessionID, sanitizeFilename(req.Name))
	downloadURL := "file://" + artifactPath

	return &protov1.ArtifactResponse{
		Recorded:    true,
		DownloadUrl: downloadURL,
	}, nil
}

// GetArtifact retrieves artifact contents by ID (helper method for MCP tool)
func (as *ArtifactService) GetArtifact(ctx context.Context, artifactID string) ([]byte, error) {
	sessionID := extractSessionIDFromArtifactID(artifactID)
	if sessionID == "" {
		return nil, fmt.Errorf("invalid artifact_id format")
	}

	// Get session to find artifact
	session, err := as.sessionPool.GetSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	// Check if artifact exists in session
	found := false
	for _, aid := range session.ArtifactIDs {
		if aid == artifactID {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("artifact not found in session")
	}

	// Extract filename from artifact ID
	filename := extractFilenameFromArtifactID(artifactID)
	artifactPath := filepath.Join(as.artifactDir, sessionID, filename)

	// Read artifact file
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read artifact: %w", err)
	}

	return data, nil
}

// ListArtifacts lists all artifacts for a session (helper method)
func (as *ArtifactService) ListArtifacts(ctx context.Context, sessionID string) ([]string, error) {
	session, err := as.sessionPool.GetSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	return session.ArtifactIDs, nil
}

// CalculateChecksum computes SHA256 checksum of a file
func (as *ArtifactService) CalculateChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

// Helper functions

func sanitizeFilename(name string) string {
	// Basic sanitization - remove path separators
	return filepath.Base(name)
}

func extractSessionIDFromArtifactID(artifactID string) string {
	// Artifact ID format: sessionID-timestamp-filename
	// Session IDs are like "session-1234567890"
	// Full artifact ID: "session-1234567890-9876543210-file.txt"
	// We need to find the second-to-last hyphen that separates sessionID from timestamp

	// Find all hyphen positions
	hyphens := make([]int, 0)
	for i := 0; i < len(artifactID); i++ {
		if artifactID[i] == '-' {
			hyphens = append(hyphens, i)
		}
	}

	// Need at least 3 hyphens: one in session ID, one before timestamp, one before filename
	if len(hyphens) < 3 {
		return ""
	}

	// The session ID ends at the second-to-last hyphen
	// session-1234567890-[timestamp]-[filename]
	//                  ^-- This is second-to-last hyphen
	sessionEndIdx := hyphens[len(hyphens)-2]
	return artifactID[:sessionEndIdx]
}

func extractFilenameFromArtifactID(artifactID string) string {
	// Artifact ID format: sessionID-timestamp-filename
	// Find the last hyphen to extract filename
	lastHyphen := -1
	for i := len(artifactID) - 1; i >= 0; i-- {
		if artifactID[i] == '-' {
			lastHyphen = i
			break
		}
	}

	if lastHyphen == -1 || lastHyphen == len(artifactID)-1 {
		return ""
	}

	return artifactID[lastHyphen+1:]
}
