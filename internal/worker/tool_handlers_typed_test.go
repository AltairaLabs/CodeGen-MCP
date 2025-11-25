package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/worker/language"
	langmock "github.com/AltairaLabs/codegen-mcp/internal/worker/language/mock"
)

func TestHandleFsWriteTyped(t *testing.T) {
	tempDir := t.TempDir()
	sessionPool := NewSessionPool("test-worker", 10, tempDir)
	executor := NewTaskExecutor(sessionPool)

	ctx := context.Background()
	sessionID := "test-write-session"
	workspacePath := filepath.Join(tempDir, sessionID)

	err := sessionPool.CreateSessionWithID(ctx, sessionID, workspacePath, "user1", map[string]string{}, map[string]string{})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer sessionPool.DestroySession(ctx, sessionID, false)

	session, err := sessionPool.GetSession(sessionID)
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	tests := []struct {
		name        string
		req         *protov1.FsWriteRequest
		wantErr     bool
		errContains string
	}{
		{
			name: "successful write",
			req: &protov1.FsWriteRequest{
				Path:     "test.txt",
				Contents: "Hello, World!",
				Metadata: map[string]string{"test": "value"},
			},
			wantErr: false,
		},
		{
			name: "write to subdirectory",
			req: &protov1.FsWriteRequest{
				Path:     "subdir/nested/file.txt",
				Contents: "nested content",
			},
			wantErr: false,
		},
		{
			name: "write empty file",
			req: &protov1.FsWriteRequest{
				Path:     "empty.txt",
				Contents: "",
			},
			wantErr: false,
		},
		{
			name: "invalid path with parent directory",
			req: &protov1.FsWriteRequest{
				Path:     "../outside.txt",
				Contents: "malicious",
			},
			wantErr:     true,
			errContains: "invalid path",
		},
		{
			name: "absolute path",
			req: &protov1.FsWriteRequest{
				Path:     "/etc/passwd",
				Contents: "hack",
			},
			wantErr:     true,
			errContains: "invalid path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := executor.handleFsWriteTyped(ctx, session, tt.req)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result.Status != protov1.TaskResult_STATUS_SUCCESS {
				t.Errorf("Expected SUCCESS status, got %v", result.Status)
			}

			// Verify file was written
			if !tt.wantErr {
				fullPath := filepath.Join(workspacePath, tt.req.Path)
				content, err := os.ReadFile(fullPath)
				if err != nil {
					t.Errorf("Failed to read written file: %v", err)
				}
				if string(content) != tt.req.Contents {
					t.Errorf("File content mismatch: got %q, want %q", string(content), tt.req.Contents)
				}
			}

			// Verify response
			if result.TypedResponse == nil {
				t.Fatal("Expected typed response")
			}
			writeResp := result.TypedResponse.GetFsWrite()
			if writeResp == nil {
				t.Fatal("Expected FsWrite response")
			}
			if writeResp.Path != tt.req.Path {
				t.Errorf("Response path mismatch: got %q, want %q", writeResp.Path, tt.req.Path)
			}
			if writeResp.BytesWritten != int64(len(tt.req.Contents)) {
				t.Errorf("Bytes written mismatch: got %d, want %d", writeResp.BytesWritten, len(tt.req.Contents))
			}
		})
	}
}

func TestHandleRunPythonTyped(t *testing.T) {
	tempDir := t.TempDir()
	sessionPool := NewSessionPool("test-worker", 10, tempDir)
	executor := NewTaskExecutor(sessionPool)

	ctx := context.Background()
	sessionID := "test-python-session"
	workspacePath := filepath.Join(tempDir, sessionID)

	err := sessionPool.CreateSessionWithID(ctx, sessionID, workspacePath, "user1", map[string]string{}, map[string]string{})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer sessionPool.DestroySession(ctx, sessionID, false)

	session, err := sessionPool.GetSession(sessionID)
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	// Set up mock provider
	mockProvider := langmock.NewProvider("python")
	session.Provider = mockProvider

	tests := []struct {
		name        string
		req         *protov1.RunPythonRequest
		setupMock   func()
		wantErr     bool
		errContains string
		wantStatus  protov1.TaskResult_Status
		checkResult func(*testing.T, *protov1.TaskResult)
	}{
		{
			name: "successful code execution",
			req: &protov1.RunPythonRequest{
				Source: &protov1.RunPythonRequest_Code{
					Code: "print('Hello')",
				},
				Metadata: map[string]string{"test": "value"},
			},
			setupMock: func() {
				mockProvider.QueueExecuteResult(&language.ExecuteResult{
					Stdout:   "Hello\n",
					Stderr:   "",
					ExitCode: 0,
				}, nil)
			},
			wantErr:    false,
			wantStatus: protov1.TaskResult_STATUS_SUCCESS,
			checkResult: func(t *testing.T, result *protov1.TaskResult) {
				resp := result.TypedResponse.GetRunPython()
				if resp.Stdout != "Hello\n" {
					t.Errorf("Expected stdout 'Hello\\n', got %q", resp.Stdout)
				}
				if resp.ExitCode != 0 {
					t.Errorf("Expected exit code 0, got %d", resp.ExitCode)
				}
			},
		},
		{
			name: "code with error",
			req: &protov1.RunPythonRequest{
				Source: &protov1.RunPythonRequest_Code{
					Code: "invalid python",
				},
			},
			setupMock: func() {
				mockProvider.QueueExecuteResult(&language.ExecuteResult{
					Stdout:   "",
					Stderr:   "SyntaxError: invalid syntax",
					ExitCode: 1,
				}, nil)
			},
			wantErr:    false,
			wantStatus: protov1.TaskResult_STATUS_FAILURE,
			checkResult: func(t *testing.T, result *protov1.TaskResult) {
				resp := result.TypedResponse.GetRunPython()
				if resp.ExitCode != 1 {
					t.Errorf("Expected exit code 1, got %d", resp.ExitCode)
				}
				if resp.Stderr == "" {
					t.Error("Expected stderr output")
				}
			},
		},
		{
			name: "execute from file",
			req: &protov1.RunPythonRequest{
				Source: &protov1.RunPythonRequest_File{
					File: "script.py",
				},
			},
			setupMock: func() {
				// Create a test file
				scriptPath := filepath.Join(workspacePath, "script.py")
				err := os.WriteFile(scriptPath, []byte("print('from file')"), 0644)
				if err != nil {
					t.Fatalf("Failed to create test script: %v", err)
				}
				mockProvider.QueueExecuteResult(&language.ExecuteResult{
					Stdout:   "from file\n",
					Stderr:   "",
					ExitCode: 0,
				}, nil)
			},
			wantErr:    false,
			wantStatus: protov1.TaskResult_STATUS_SUCCESS,
			checkResult: func(t *testing.T, result *protov1.TaskResult) {
				resp := result.TypedResponse.GetRunPython()
				if resp.Stdout != "from file\n" {
					t.Errorf("Expected stdout 'from file\\n', got %q", resp.Stdout)
				}
			},
		},
		{
			name: "file not found",
			req: &protov1.RunPythonRequest{
				Source: &protov1.RunPythonRequest_File{
					File: "nonexistent.py",
				},
			},
			setupMock:   func() {},
			wantErr:     true,
			errContains: "failed to read file",
		},
		{
			name: "no source provided",
			req: &protov1.RunPythonRequest{
				Source: nil,
			},
			setupMock:   func() {},
			wantErr:     true,
			errContains: "missing required source",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			result, err := executor.handleRunPythonTyped(ctx, session, tt.req, nil)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result.Status != tt.wantStatus {
				t.Errorf("Expected status %v, got %v", tt.wantStatus, result.Status)
			}

			if result.TypedResponse == nil {
				t.Fatal("Expected typed response")
			}

			if tt.checkResult != nil {
				tt.checkResult(t, result)
			}
		})
	}
}

func TestHandleRunPythonTyped_NoProvider(t *testing.T) {
	tempDir := t.TempDir()
	sessionPool := NewSessionPool("test-worker", 10, tempDir)
	executor := NewTaskExecutor(sessionPool)

	ctx := context.Background()
	sessionID := "test-noprovider-session"
	workspacePath := filepath.Join(tempDir, sessionID)

	err := sessionPool.CreateSessionWithID(ctx, sessionID, workspacePath, "user1", map[string]string{}, map[string]string{})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer sessionPool.DestroySession(ctx, sessionID, false)

	session, err := sessionPool.GetSession(sessionID)
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	// No provider set
	session.Provider = nil

	req := &protov1.RunPythonRequest{
		Source: &protov1.RunPythonRequest_Code{
			Code: "print('test')",
		},
	}

	result, err := executor.handleRunPythonTyped(ctx, session, req, nil)
	if err == nil {
		t.Error("Expected error when provider is nil")
	}
	if result == nil || result.Status != protov1.TaskResult_STATUS_FAILURE {
		t.Error("Expected FAILURE status when provider is nil")
	}
}

func TestHandlePkgInstallTyped(t *testing.T) {
	tempDir := t.TempDir()
	sessionPool := NewSessionPool("test-worker", 10, tempDir)
	executor := NewTaskExecutor(sessionPool)

	ctx := context.Background()
	sessionID := "test-pkg-session"
	workspacePath := filepath.Join(tempDir, sessionID)

	err := sessionPool.CreateSessionWithID(ctx, sessionID, workspacePath, "user1", map[string]string{}, map[string]string{})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer sessionPool.DestroySession(ctx, sessionID, false)

	session, err := sessionPool.GetSession(sessionID)
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	// Set up mock provider
	mockProvider := langmock.NewProvider("python")
	session.Provider = mockProvider

	tests := []struct {
		name        string
		req         *protov1.PkgInstallRequest
		setupMock   func()
		wantErr     bool
		errContains string
		wantStatus  protov1.TaskResult_Status
		checkResult func(*testing.T, *protov1.TaskResult)
	}{
		{
			name: "successful single package install",
			req: &protov1.PkgInstallRequest{
				Packages: []string{"requests"},
				Metadata: map[string]string{"test": "value"},
			},
			setupMock: func() {
				// Mock succeeds by default
			},
			wantErr:    false,
			wantStatus: protov1.TaskResult_STATUS_SUCCESS,
			checkResult: func(t *testing.T, result *protov1.TaskResult) {
				resp := result.TypedResponse.GetPkgInstall()
				if len(resp.Installed) != 1 {
					t.Errorf("Expected 1 installed package, got %d", len(resp.Installed))
				}
				if resp.Installed[0].Name != "requests" {
					t.Errorf("Expected package name 'requests', got %q", resp.Installed[0].Name)
				}
				if resp.ExitCode != 0 {
					t.Errorf("Expected exit code 0, got %d", resp.ExitCode)
				}
			},
		},
		{
			name: "successful multiple package install",
			req: &protov1.PkgInstallRequest{
				Packages: []string{"requests", "numpy", "pandas"},
			},
			setupMock: func() {
				// Mock succeeds by default
			},
			wantErr:    false,
			wantStatus: protov1.TaskResult_STATUS_SUCCESS,
			checkResult: func(t *testing.T, result *protov1.TaskResult) {
				resp := result.TypedResponse.GetPkgInstall()
				if len(resp.Installed) != 3 {
					t.Errorf("Expected 3 installed packages, got %d", len(resp.Installed))
				}
				if len(resp.Failed) != 0 {
					t.Errorf("Expected 0 failed packages, got %d", len(resp.Failed))
				}
			},
		},
		{
			name: "no packages specified",
			req: &protov1.PkgInstallRequest{
				Packages: []string{},
			},
			setupMock:   func() {},
			wantErr:     true,
			errContains: "no packages specified",
		},
		{
			name: "nil packages",
			req: &protov1.PkgInstallRequest{
				Packages: nil,
			},
			setupMock:   func() {},
			wantErr:     true,
			errContains: "no packages specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset installed packages
			session.mu.Lock()
			session.InstalledPkgs = []string{}
			session.mu.Unlock()

			tt.setupMock()

			result, err := executor.handlePkgInstallTyped(ctx, session, tt.req, nil)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result.Status != tt.wantStatus {
				t.Errorf("Expected status %v, got %v", tt.wantStatus, result.Status)
			}

			if result.TypedResponse == nil {
				t.Fatal("Expected typed response")
			}

			if tt.checkResult != nil {
				tt.checkResult(t, result)
			}

			// Verify session's installed packages list was updated on success
			if tt.wantStatus == protov1.TaskResult_STATUS_SUCCESS {
				session.mu.Lock()
				installedCount := len(session.InstalledPkgs)
				session.mu.Unlock()
				if installedCount != len(tt.req.Packages) {
					t.Errorf("Expected %d packages in session list, got %d", len(tt.req.Packages), installedCount)
				}
			}
		})
	}
}

func TestHandlePkgInstallTyped_NoProvider(t *testing.T) {
	tempDir := t.TempDir()
	sessionPool := NewSessionPool("test-worker", 10, tempDir)
	executor := NewTaskExecutor(sessionPool)

	ctx := context.Background()
	sessionID := "test-noprovider-pkg-session"
	workspacePath := filepath.Join(tempDir, sessionID)

	err := sessionPool.CreateSessionWithID(ctx, sessionID, workspacePath, "user1", map[string]string{}, map[string]string{})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer sessionPool.DestroySession(ctx, sessionID, false)

	session, err := sessionPool.GetSession(sessionID)
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	// No provider set
	session.Provider = nil

	req := &protov1.PkgInstallRequest{
		Packages: []string{"requests"},
	}

	result, err := executor.handlePkgInstallTyped(ctx, session, req, nil)
	if err == nil {
		t.Error("Expected error when provider is nil")
	}
	if result == nil || result.Status != protov1.TaskResult_STATUS_FAILURE {
		t.Error("Expected FAILURE status when provider is nil")
	}
}

func TestHandleFsReadTyped(t *testing.T) {
	tempDir := t.TempDir()
	sessionPool := NewSessionPool("test-worker", 10, tempDir)
	executor := NewTaskExecutor(sessionPool)

	ctx := context.Background()
	sessionID := "test-read-session"
	workspacePath := filepath.Join(tempDir, sessionID)

	err := sessionPool.CreateSessionWithID(ctx, sessionID, workspacePath, "user1", map[string]string{}, map[string]string{})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer sessionPool.DestroySession(ctx, sessionID, false)

	session, err := sessionPool.GetSession(sessionID)
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	// Create a test file
	testFilePath := filepath.Join(workspacePath, "test.txt")
	testContent := "Hello, World!"
	err = os.WriteFile(testFilePath, []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name        string
		req         *protov1.FsReadRequest
		wantErr     bool
		errContains string
	}{
		{
			name: "successful read",
			req: &protov1.FsReadRequest{
				Path:     "test.txt",
				Metadata: map[string]string{"test": "value"},
			},
			wantErr: false,
		},
		{
			name: "file not found",
			req: &protov1.FsReadRequest{
				Path: "nonexistent.txt",
			},
			wantErr:     true,
			errContains: "failed to read file",
		},
		{
			name: "invalid path",
			req: &protov1.FsReadRequest{
				Path: "../outside.txt",
			},
			wantErr:     true,
			errContains: "invalid path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := executor.handleFsReadTyped(ctx, session, tt.req)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Error %q does not contain %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result.Status != protov1.TaskResult_STATUS_SUCCESS {
				t.Errorf("Expected SUCCESS status, got %v", result.Status)
			}

			// Verify response
			if result.TypedResponse == nil {
				t.Fatal("Expected typed response")
			}
			readResp := result.TypedResponse.GetFsRead()
			if readResp == nil {
				t.Fatal("Expected FsRead response")
			}
			if readResp.Contents != testContent {
				t.Errorf("Content mismatch: got %q, want %q", readResp.Contents, testContent)
			}
			if readResp.SizeBytes != int64(len(testContent)) {
				t.Errorf("Size mismatch: got %d, want %d", readResp.SizeBytes, len(testContent))
			}
		})
	}
}

func TestHandleArtifactGetTyped(t *testing.T) {
	tempDir := t.TempDir()
	sessionPool := NewSessionPool("test-worker", 10, tempDir)
	executor := NewTaskExecutor(sessionPool)

	ctx := context.Background()
	sessionID := "test-artifact-session"
	workspacePath := filepath.Join(tempDir, sessionID)

	err := sessionPool.CreateSessionWithID(ctx, sessionID, workspacePath, "user1", map[string]string{}, map[string]string{})
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	defer sessionPool.DestroySession(ctx, sessionID, false)

	session, err := sessionPool.GetSession(sessionID)
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	// Create artifacts directory
	artifactsDir := filepath.Join(workspacePath, "artifacts")
	if err := os.MkdirAll(artifactsDir, 0755); err != nil {
		t.Fatalf("Failed to create artifacts directory: %v", err)
	}

	// Create a test artifact file
	testContent := []byte("test artifact content")
	testFilename := "testfile.txt"
	testPath := filepath.Join(artifactsDir, testFilename)
	if err := os.WriteFile(testPath, testContent, 0644); err != nil {
		t.Fatalf("Failed to write test artifact: %v", err)
	}

	// Add artifact to session with proper format: sessionID-timestamp-filename
	artifactID := sessionID + "-12345-" + testFilename
	session.ArtifactIDs = append(session.ArtifactIDs, artifactID)

	tests := []struct {
		name        string
		req         *protov1.ArtifactGetRequest
		wantErr     bool
		errContains string
		checkData   bool
	}{
		{
			name: "successful get",
			req: &protov1.ArtifactGetRequest{
				ArtifactId: artifactID,
				Metadata:   map[string]string{"test": "value"},
			},
			wantErr:   false,
			checkData: true,
		},
		{
			name: "artifact not in session",
			req: &protov1.ArtifactGetRequest{
				ArtifactId: "unknown-artifact",
			},
			wantErr:     true,
			errContains: "artifact not found in session",
		},
		{
			name: "invalid artifact ID format",
			req: &protov1.ArtifactGetRequest{
				ArtifactId: "invalid",
			},
			wantErr:     true,
			errContains: "artifact not found in session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := executor.handleArtifactGetTyped(ctx, session, tt.req)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("Error %q does not contain %q", err.Error(), tt.errContains)
				}
				if result.Status != protov1.TaskResult_STATUS_FAILURE {
					t.Errorf("Expected STATUS_FAILURE, got %v", result.Status)
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if result.Status != protov1.TaskResult_STATUS_SUCCESS {
				t.Errorf("Expected STATUS_SUCCESS, got %v", result.Status)
			}

			// Verify typed response
			artifactResp := result.TypedResponse.GetArtifactGet()
			if artifactResp == nil {
				t.Fatal("Expected ArtifactGetResponse, got nil")
			}

			if tt.checkData {
				if string(artifactResp.Data) != string(testContent) {
					t.Errorf("Data mismatch: got %q, want %q", artifactResp.Data, testContent)
				}
				if artifactResp.SizeBytes != int64(len(testContent)) {
					t.Errorf("Size mismatch: got %d, want %d", artifactResp.SizeBytes, len(testContent))
				}
				if artifactResp.ArtifactId != artifactID {
					t.Errorf("ArtifactID mismatch: got %q, want %q", artifactResp.ArtifactId, artifactID)
				}
			}
		})
	}
}

func TestValidateWorkspacePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid relative path", "file.txt", false},
		{"valid subdirectory", "subdir/file.txt", false},
		{"valid deep path", "a/b/c/d/file.txt", false},
		{"current directory", ".", false},
		{"parent directory", "..", true},
		{"parent in path", "../file.txt", true},
		{"absolute path", "/etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWorkspacePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateWorkspacePath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findInString(s, substr)))
}

func findInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
