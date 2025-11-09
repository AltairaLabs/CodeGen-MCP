package worker

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
)

func TestHandleEcho(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	// Create test session
	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})
	session, _ := pool.GetSession(sessionResp.SessionId)

	req := &protov1.TaskRequest{
		TaskId:   "test-task",
		ToolName: "echo",
		Arguments: map[string]string{
			"message": "Hello World",
		},
	}

	result, err := executor.handleEcho(context.Background(), session, req)
	if err != nil {
		t.Fatalf("handleEcho failed: %v", err)
	}

	if result.Status != protov1.TaskResult_STATUS_SUCCESS {
		t.Errorf("Expected SUCCESS status, got %v", result.Status)
	}

	if result.Outputs["message"] != "Hello World" {
		t.Errorf("Expected message 'Hello World', got '%s'", result.Outputs["message"])
	}
}

func TestHandleFsWrite(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})
	session, _ := pool.GetSession(sessionResp.SessionId)

	req := &protov1.TaskRequest{
		TaskId:   "test-task",
		ToolName: "fs.write",
		Arguments: map[string]string{
			"path":     "test.txt",
			"contents": "test content",
		},
	}

	result, err := executor.handleFsWrite(context.Background(), session, req)
	if err != nil {
		t.Fatalf("handleFsWrite failed: %v", err)
	}

	if result.Status != protov1.TaskResult_STATUS_SUCCESS {
		t.Errorf("Expected SUCCESS status, got %v", result.Status)
	}

	// Verify file was created
	filePath := filepath.Join(session.WorkspacePath, "test.txt")
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(content) != "test content" {
		t.Errorf("Expected content 'test content', got '%s'", string(content))
	}
}

func TestHandleFsRead(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})
	session, _ := pool.GetSession(sessionResp.SessionId)

	// Create a test file
	filePath := filepath.Join(session.WorkspacePath, "test.txt")
	os.WriteFile(filePath, []byte("test content"), 0644)

	req := &protov1.TaskRequest{
		TaskId:   "test-task",
		ToolName: "fs.read",
		Arguments: map[string]string{
			"path": "test.txt",
		},
	}

	result, err := executor.handleFsRead(context.Background(), session, req)
	if err != nil {
		t.Fatalf("handleFsRead failed: %v", err)
	}

	if result.Status != protov1.TaskResult_STATUS_SUCCESS {
		t.Errorf("Expected SUCCESS status, got %v", result.Status)
	}

	if result.Outputs["contents"] != "test content" {
		t.Errorf("Expected contents 'test content', got '%s'", result.Outputs["contents"])
	}
}

func TestHandleFsList(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})
	session, _ := pool.GetSession(sessionResp.SessionId)

	// Create some test files
	os.WriteFile(filepath.Join(session.WorkspacePath, "file1.txt"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(session.WorkspacePath, "file2.txt"), []byte("test"), 0644)
	os.Mkdir(filepath.Join(session.WorkspacePath, "subdir"), 0755)

	req := &protov1.TaskRequest{
		TaskId:   "test-task",
		ToolName: "fs.list",
		Arguments: map[string]string{
			"path": ".",
		},
	}

	result, err := executor.handleFsList(context.Background(), session, req)
	if err != nil {
		t.Fatalf("handleFsList failed: %v", err)
	}

	if result.Status != protov1.TaskResult_STATUS_SUCCESS {
		t.Errorf("Expected SUCCESS status, got %v", result.Status)
	}

	// Check files output contains expected entries
	files := result.Outputs["files"]
	if files == "" {
		t.Error("Expected non-empty files list")
	}
}

func TestValidateWorkspacePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"Valid relative path", "src/main.py", false},
		{"Valid nested path", "src/lib/utils.py", false},
		{"Current directory", ".", false},
		{"Hidden file", ".gitignore", false},
		{"Absolute path rejected", "/etc/passwd", true},
		{"Parent traversal rejected", "../etc/passwd", true},
		{"Nested traversal rejected", "src/../../etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWorkspacePath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateWorkspacePath() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHandleFsWrite_PathValidation(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})
	session, _ := pool.GetSession(sessionResp.SessionId)

	// Try to write to an absolute path (should fail)
	req := &protov1.TaskRequest{
		TaskId:   "test-task",
		ToolName: "fs.write",
		Arguments: map[string]string{
			"path":     "/etc/passwd",
			"contents": "malicious",
		},
	}

	_, err := executor.handleFsWrite(context.Background(), session, req)
	if err == nil {
		t.Error("Expected error when writing to absolute path")
	}

	// Try path traversal (should fail)
	req.Arguments["path"] = "../../../etc/passwd"
	_, err = executor.handleFsWrite(context.Background(), session, req)
	if err == nil {
		t.Error("Expected error when using path traversal")
	}
}

func TestHandleFsWrite_MissingArguments(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})
	session, _ := pool.GetSession(sessionResp.SessionId)

	// Missing path
	req := &protov1.TaskRequest{
		TaskId:   "test-task",
		ToolName: "fs.write",
		Arguments: map[string]string{
			"contents": "test",
		},
	}

	_, err := executor.handleFsWrite(context.Background(), session, req)
	if err == nil {
		t.Error("Expected error when path is missing")
	}

	// Missing contents
	req.Arguments = map[string]string{
		"path": "test.txt",
	}

	_, err = executor.handleFsWrite(context.Background(), session, req)
	if err == nil {
		t.Error("Expected error when contents is missing")
	}
}

func TestHandleRunPython(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})
	session, _ := pool.GetSession(sessionResp.SessionId)

	req := &protov1.TaskRequest{
		TaskId:    "test-python",
		ToolName:  "run.python",
		Arguments: map[string]string{"code": "print('hello')"},
	}

	stream := &mockTaskStream{}
	_, err := executor.handleRunPython(context.Background(), session, req, stream)
	if err != nil {
		t.Logf("Python execution error (expected if python not in PATH): %v", err)
	}
}

func TestHandlePkgInstall(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})
	session, _ := pool.GetSession(sessionResp.SessionId)

	t.Run("missing_requirements", func(t *testing.T) {
		req := &protov1.TaskRequest{
			TaskId:    "test-pkg",
			ToolName:  "pkg.install",
			Arguments: map[string]string{},
		}
		stream := &mockTaskStream{}
		_, err := executor.handlePkgInstall(context.Background(), session, req, stream)
		if err == nil {
			t.Error("Expected error for missing requirements")
		}
	})

	t.Run("with_requirements", func(t *testing.T) {
		req := &protov1.TaskRequest{
			TaskId:    "test-pkg",
			ToolName:  "pkg.install",
			Arguments: map[string]string{"requirements": "requests"},
		}
		stream := &mockTaskStream{}
		_, err := executor.handlePkgInstall(context.Background(), session, req, stream)
		if err != nil {
			t.Logf("Package install error (expected if pip not available): %v", err)
		}
	})

	t.Run("multiple_packages", func(t *testing.T) {
		req := &protov1.TaskRequest{
			TaskId:    "test-pkg-multi",
			ToolName:  "pkg.install",
			Arguments: map[string]string{"requirements": "requests\nflask"},
		}
		stream := &mockTaskStream{}
		_, err := executor.handlePkgInstall(context.Background(), session, req, stream)
		if err != nil {
			t.Logf("Package install error (expected if pip not available): %v", err)
		}
	})

	t.Run("empty_packages", func(t *testing.T) {
		req := &protov1.TaskRequest{
			TaskId:    "test-pkg-empty",
			ToolName:  "pkg.install",
			Arguments: map[string]string{"requirements": ""},
		}
		stream := &mockTaskStream{}
		result, err := executor.handlePkgInstall(context.Background(), session, req, stream)
		if err != nil {
			t.Logf("Expected result with error, got: %v", err)
		}
		if result != nil && result.Status != protov1.TaskResult_STATUS_FAILURE {
			t.Error("Expected failure status for empty package list")
		}
	})

	t.Run("comments_in_requirements", func(t *testing.T) {
		req := &protov1.TaskRequest{
			TaskId:    "test-pkg-comments",
			ToolName:  "pkg.install",
			Arguments: map[string]string{"requirements": "# Comment\nrequests\n# Another comment"},
		}
		stream := &mockTaskStream{}
		_, err := executor.handlePkgInstall(context.Background(), session, req, stream)
		if err != nil {
			t.Logf("Package install error (expected if pip not available): %v", err)
		}
	})
}

func TestHandleEcho_DefaultMessage(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})
	session, _ := pool.GetSession(sessionResp.SessionId)

	req := &protov1.TaskRequest{
		TaskId:    "test-task",
		ToolName:  "echo",
		Arguments: map[string]string{},
	}

	result, err := executor.handleEcho(context.Background(), session, req)
	if err != nil {
		t.Fatalf("handleEcho failed: %v", err)
	}

	if result.Outputs["message"] != "Hello from worker!" {
		t.Errorf("Expected default message, got '%s'", result.Outputs["message"])
	}
}

func TestHandleFsRead_MissingPath(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})
	session, _ := pool.GetSession(sessionResp.SessionId)

	req := &protov1.TaskRequest{
		TaskId:    "test-task",
		ToolName:  "fs.read",
		Arguments: map[string]string{},
	}

	_, err := executor.handleFsRead(context.Background(), session, req)
	if err == nil {
		t.Error("Expected error when path is missing")
	}
}

func TestHandleFsRead_PathValidation(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})
	session, _ := pool.GetSession(sessionResp.SessionId)

	tests := []struct {
		name string
		path string
	}{
		{"absolute_path", "/etc/passwd"},
		{"parent_traversal", "../../../etc/passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &protov1.TaskRequest{
				TaskId:   "test-task",
				ToolName: "fs.read",
				Arguments: map[string]string{
					"path": tt.path,
				},
			}

			_, err := executor.handleFsRead(context.Background(), session, req)
			if err == nil {
				t.Errorf("Expected error for path: %s", tt.path)
			}
		})
	}
}

func TestHandleFsRead_FileNotFound(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})
	session, _ := pool.GetSession(sessionResp.SessionId)

	req := &protov1.TaskRequest{
		TaskId:   "test-task",
		ToolName: "fs.read",
		Arguments: map[string]string{
			"path": "nonexistent.txt",
		},
	}

	_, err := executor.handleFsRead(context.Background(), session, req)
	if err == nil {
		t.Error("Expected error when file doesn't exist")
	}
}

func TestHandleFsList_DefaultPath(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})
	session, _ := pool.GetSession(sessionResp.SessionId)

	// Create some test files
	os.WriteFile(filepath.Join(session.WorkspacePath, "file1.txt"), []byte("test"), 0644)

	req := &protov1.TaskRequest{
		TaskId:    "test-task",
		ToolName:  "fs.list",
		Arguments: map[string]string{},
	}

	result, err := executor.handleFsList(context.Background(), session, req)
	if err != nil {
		t.Fatalf("handleFsList failed: %v", err)
	}

	if result.Status != protov1.TaskResult_STATUS_SUCCESS {
		t.Errorf("Expected SUCCESS status, got %v", result.Status)
	}

	if result.Outputs["path"] != "." {
		t.Errorf("Expected path '.', got '%s'", result.Outputs["path"])
	}
}

func TestHandleFsList_PathValidation(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})
	session, _ := pool.GetSession(sessionResp.SessionId)

	tests := []struct {
		name string
		path string
	}{
		{"absolute_path", "/etc"},
		{"parent_traversal", "../../etc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &protov1.TaskRequest{
				TaskId:   "test-task",
				ToolName: "fs.list",
				Arguments: map[string]string{
					"path": tt.path,
				},
			}

			_, err := executor.handleFsList(context.Background(), session, req)
			if err == nil {
				t.Errorf("Expected error for path: %s", tt.path)
			}
		})
	}
}

func TestHandleFsList_NonExistentPath(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})
	session, _ := pool.GetSession(sessionResp.SessionId)

	req := &protov1.TaskRequest{
		TaskId:   "test-task",
		ToolName: "fs.list",
		Arguments: map[string]string{
			"path": "nonexistent",
		},
	}

	_, err := executor.handleFsList(context.Background(), session, req)
	if err == nil {
		t.Error("Expected error when directory doesn't exist")
	}
}

func TestHandleRunPython_MissingCode(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})
	session, _ := pool.GetSession(sessionResp.SessionId)

	req := &protov1.TaskRequest{
		TaskId:    "test-python",
		ToolName:  "run.python",
		Arguments: map[string]string{},
	}

	stream := &mockTaskStream{}
	_, err := executor.handleRunPython(context.Background(), session, req, stream)
	if err == nil {
		t.Error("Expected error when code is missing")
	}
}

func TestHandleFsWrite_CreateNestedDirectories(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})
	session, _ := pool.GetSession(sessionResp.SessionId)

	req := &protov1.TaskRequest{
		TaskId:   "test-task",
		ToolName: "fs.write",
		Arguments: map[string]string{
			"path":     "deep/nested/path/test.txt",
			"contents": "test content",
		},
	}

	result, err := executor.handleFsWrite(context.Background(), session, req)
	if err != nil {
		t.Fatalf("handleFsWrite failed: %v", err)
	}

	if result.Status != protov1.TaskResult_STATUS_SUCCESS {
		t.Errorf("Expected SUCCESS status, got %v", result.Status)
	}

	// Verify file was created
	filePath := filepath.Join(session.WorkspacePath, "deep/nested/path/test.txt")
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(content) != "test content" {
		t.Errorf("Expected content 'test content', got '%s'", string(content))
	}
}
