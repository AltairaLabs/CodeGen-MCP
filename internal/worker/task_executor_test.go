package worker

import (
	"context"
	"testing"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"google.golang.org/grpc/metadata"
)

const (
	testUserID   = "test-user"
	testTaskID   = "task-1"
	testActiveID = "active-task"
	testStatusID = "status-task"
)

type mockTaskStream struct {
	responses []*protov1.TaskResponse
}

func (m *mockTaskStream) Send(resp *protov1.TaskResponse) error {
	m.responses = append(m.responses, resp)
	return nil
}

func (m *mockTaskStream) SetHeader(metadata.MD) error  { return nil }
func (m *mockTaskStream) SendHeader(metadata.MD) error { return nil }
func (m *mockTaskStream) SetTrailer(metadata.MD)       {} // No-op for mock
func (m *mockTaskStream) Context() context.Context     { return context.Background() }
func (m *mockTaskStream) SendMsg(interface{}) error    { return nil }
func (m *mockTaskStream) RecvMsg(interface{}) error    { return nil }

func TestTaskExecutorExecute(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool(testWorkerID, 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      testUserID,
	})

	req := &protov1.TaskRequest{
		TaskId:    testTaskID,
		SessionId: sessionResp.SessionId,
		TypedRequest: &protov1.ToolRequest{
			Request: &protov1.ToolRequest_Echo{
				Echo: &protov1.EchoRequest{
					Message: "hello",
				},
			},
		},
	}

	stream := &mockTaskStream{}
	err := executor.Execute(context.Background(), req, stream)
	if err != nil {
		t.Fatalf("Failed to execute task: %v", err)
	}

	if len(stream.responses) == 0 {
		t.Error("Expected at least one response in stream")
	}

	// Check that task was added to history
	session, _ := pool.GetSession(sessionResp.SessionId)
	found := false
	for _, taskID := range session.RecentTasks {
		if taskID == testTaskID {
			found = true
			break
		}
	}
	if !found {
		t.Error("Task not added to session history")
	}
}

func TestTaskExecutorNewTaskExecutor(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	if executor == nil {
		t.Fatal("Expected non-nil executor")
	}

	if executor.sessionPool != pool {
		t.Error("Expected executor to have correct session pool")
	}

	if executor.activeTasks == nil {
		t.Error("Expected activeTasks map to be initialized")
	}
}

func TestTaskExecutorExecuteInvalidSession(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	req := &protov1.TaskRequest{
		TaskId:    "task-1",
		SessionId: "non-existent-session",
		TypedRequest: &protov1.ToolRequest{
			Request: &protov1.ToolRequest_Echo{
				Echo: &protov1.EchoRequest{
					Message: "hello",
				},
			},
		},
	}

	stream := &mockTaskStream{}
	err := executor.Execute(context.Background(), req, stream)

	if err == nil {
		t.Error("Expected error for invalid session")
	}
}

func TestTaskExecutorActiveTasks(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	if executor.activeTasks == nil {
		t.Fatal("Expected activeTasks to be initialized")
	}

	if len(executor.activeTasks) != 0 {
		t.Errorf("Expected 0 active tasks, got %d", len(executor.activeTasks))
	}
}

func TestTaskExecutorCancelNonExistent(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})

	// Cancel a non-existent task
	resp, err := executor.Cancel(context.Background(), &protov1.CancelRequest{
		TaskId:    "task-cancel",
		SessionId: sessionResp.SessionId,
	})

	if err != nil {
		t.Fatalf("Failed to cancel task: %v", err)
	}

	if resp.Cancelled {
		t.Error("Expected cancel to return false for non-existent task")
	}

	if resp.State != "not_found" {
		t.Errorf("Expected state 'not_found', got '%s'", resp.State)
	}
}

func TestTaskExecutorMutex(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	// Test concurrent access to activeTasks map
	done := make(chan bool)

	go func() {
		executor.mu.Lock()
		// Verify we can access the map safely
		_ = len(executor.activeTasks)
		executor.mu.Unlock()
		done <- true
	}()

	<-done
	t.Log("Mutex access successful")
}

func TestTaskExecutorExecuteToolInSession(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})
	session, _ := pool.GetSession(sessionResp.SessionId)

	// Test fs.read
	req := &protov1.TaskRequest{
		TaskId:    "test-read",
		SessionId: sessionResp.SessionId,
		TypedRequest: &protov1.ToolRequest{
			Request: &protov1.ToolRequest_FsRead{
				FsRead: &protov1.FsReadRequest{
					Path: "nonexistent.txt",
				},
			},
		},
	}

	stream := &mockTaskStream{}
	_, err := executor.executeToolInSession(context.Background(), session, req, stream)
	if err == nil {
		t.Log("fs.read executed (may fail if file doesn't exist)")
	}

	// Test fs.list
	req2 := &protov1.TaskRequest{
		TaskId:    "test-list",
		SessionId: sessionResp.SessionId,
		TypedRequest: &protov1.ToolRequest{
			Request: &protov1.ToolRequest_FsList{
				FsList: &protov1.FsListRequest{
					Path: ".",
				},
			},
		},
	}

	_, err = executor.executeToolInSession(context.Background(), session, req2, stream)
	if err != nil {
		t.Logf("fs.list error: %v", err)
	}
}

func TestTaskExecutorGetStatusNonExistent(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})

	statusResp, err := executor.GetStatus(context.Background(), &protov1.StatusRequest{
		TaskId:    "non-existent-task",
		SessionId: sessionResp.SessionId,
	})

	if err != nil {
		t.Fatalf("GetStatus should not error for non-existent task: %v", err)
	}

	if statusResp.Status != protov1.TaskResult_STATUS_UNSPECIFIED {
		t.Errorf("Expected UNSPECIFIED status for non-existent task, got %v", statusResp.Status)
	}
}

func TestTaskExecutorCancelActiveTask(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})

	// Manually create an active task entry
	taskCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	task := &ActiveTask{
		TaskID:    "active-task",
		SessionID: sessionResp.SessionId,
		ToolName:  "echo",
		Status:    protov1.TaskResult_STATUS_UNSPECIFIED,
		Cancel:    cancel,
	}

	executor.mu.Lock()
	executor.activeTasks["active-task"] = task
	executor.mu.Unlock()

	// Cancel the active task
	resp, err := executor.Cancel(taskCtx, &protov1.CancelRequest{
		TaskId:    "active-task",
		SessionId: sessionResp.SessionId,
	})

	if err != nil {
		t.Fatalf("Failed to cancel active task: %v", err)
	}

	if !resp.Cancelled {
		t.Error("Expected cancel to return true for active task")
	}

	if resp.State != "canceled" {
		t.Errorf("Expected state 'canceled', got '%s'", resp.State)
	}

	// Verify task status was updated
	if task.Status != protov1.TaskResult_STATUS_CANCELLED {
		t.Errorf("Expected task status CANCELLED, got %v", task.Status)
	}
}

func TestTaskExecutorGetStatusActiveTask(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      "test-user",
	})

	// Manually create an active task entry
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	task := &ActiveTask{
		TaskID:    "status-task",
		SessionID: sessionResp.SessionId,
		ToolName:  "long-running",
		Status:    protov1.TaskResult_STATUS_SUCCESS,
		Cancel:    cancel,
	}

	executor.mu.Lock()
	executor.activeTasks["status-task"] = task
	executor.mu.Unlock()

	// Get status of active task
	statusResp, err := executor.GetStatus(ctx, &protov1.StatusRequest{
		TaskId:    "status-task",
		SessionId: sessionResp.SessionId,
	})

	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if statusResp.Status != protov1.TaskResult_STATUS_SUCCESS {
		t.Errorf("Expected SUCCESS status, got %v", statusResp.Status)
	}
}

func TestTaskExecutorArtifactGet(t *testing.T) {
	baseWorkspace := t.TempDir()
	pool := newTestSessionPool("test-worker", 5, baseWorkspace)
	executor := NewTaskExecutor(pool)

	sessionResp, _ := pool.CreateSession(context.Background(), &protov1.CreateSessionRequest{
		WorkspaceId: "test",
		UserId:      testUserID,
	})

	req := &protov1.TaskRequest{
		TaskId:    "artifact-task",
		SessionId: sessionResp.SessionId,
		TypedRequest: &protov1.ToolRequest{
			Request: &protov1.ToolRequest_ArtifactGet{
				ArtifactGet: &protov1.ArtifactGetRequest{
					ArtifactId: "test-artifact-123",
				},
			},
		},
	}

	stream := &mockTaskStream{}
	_ = executor.Execute(context.Background(), req, stream)

	// Should get at least one response (error response)
	if len(stream.responses) == 0 {
		t.Fatal("Expected at least one response")
	}

	// Verify we got an error response for non-existent artifact
	lastResp := stream.responses[len(stream.responses)-1]
	if lastResp.GetError() == nil {
		t.Error("Expected error response for non-existent artifact")
	}
}
