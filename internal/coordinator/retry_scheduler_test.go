package coordinator

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"
)

// MockTaskRetryStorage implements TaskRetryStorage for testing
type MockTaskRetryStorage struct {
	mu                 sync.Mutex
	tasksReadyForRetry []*QueuedTask
	requeuedTasks      []string
	updatedTasks       map[string]time.Time
	getTasksError      error
	requeueErrorFunc   func(taskID string) error
	updateError        error
	getTasksCallCount  int
	requeueCallCount   int
	updateCallCount    int
}

func (m *MockTaskRetryStorage) GetTasksReadyForRetry(ctx context.Context, limit int) ([]*QueuedTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getTasksCallCount++

	if m.getTasksError != nil {
		return nil, m.getTasksError
	}

	if limit < len(m.tasksReadyForRetry) {
		return m.tasksReadyForRetry[:limit], nil
	}
	return m.tasksReadyForRetry, nil
}

func (m *MockTaskRetryStorage) RequeueTaskForRetry(ctx context.Context, taskID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requeueCallCount++

	if m.requeueErrorFunc != nil {
		if err := m.requeueErrorFunc(taskID); err != nil {
			return err
		}
	}

	m.requeuedTasks = append(m.requeuedTasks, taskID)
	return nil
}

func (m *MockTaskRetryStorage) UpdateTaskForRetry(ctx context.Context, taskID string, nextRetryAt time.Time, errorMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateCallCount++

	if m.updateError != nil {
		return m.updateError
	}

	if m.updatedTasks == nil {
		m.updatedTasks = make(map[string]time.Time)
	}
	m.updatedTasks[taskID] = nextRetryAt
	return nil
}

func (m *MockTaskRetryStorage) GetRequeuedTasks() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.requeuedTasks...)
}

func (m *MockTaskRetryStorage) GetCallCounts() (getTasks, requeue, update int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getTasksCallCount, m.requeueCallCount, m.updateCallCount
}

func TestNewRetryScheduler(t *testing.T) {
	storage := &MockTaskRetryStorage{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	scheduler := NewRetryScheduler(storage, logger)

	if scheduler == nil {
		t.Fatal("Expected non-nil scheduler")
	}
	if scheduler.storage != storage {
		t.Error("Expected storage to be set")
	}
	if scheduler.logger != logger {
		t.Error("Expected logger to be set")
	}
}

func TestRetryScheduler_ProcessRetries_NoTasks(t *testing.T) {
	storage := &MockTaskRetryStorage{
		tasksReadyForRetry: []*QueuedTask{},
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scheduler := NewRetryScheduler(storage, logger)

	ctx := context.Background()
	err := scheduler.processRetries(ctx)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	requeued := storage.GetRequeuedTasks()
	if len(requeued) != 0 {
		t.Errorf("Expected 0 requeued tasks, got %d", len(requeued))
	}
}

func TestRetryScheduler_ProcessRetries_SingleTask(t *testing.T) {
	task1 := &QueuedTask{
		ID:         "task-1",
		SessionID:  "session-1",
		RetryCount: 1,
	}

	storage := &MockTaskRetryStorage{
		tasksReadyForRetry: []*QueuedTask{task1},
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scheduler := NewRetryScheduler(storage, logger)

	ctx := context.Background()
	err := scheduler.processRetries(ctx)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	requeued := storage.GetRequeuedTasks()
	if len(requeued) != 1 {
		t.Errorf("Expected 1 requeued task, got %d", len(requeued))
	}
	if requeued[0] != "task-1" {
		t.Errorf("Expected task-1 to be requeued, got %s", requeued[0])
	}
}

func TestRetryScheduler_ProcessRetries_MultipleTasks(t *testing.T) {
	tasks := []*QueuedTask{
		{ID: "task-1", SessionID: "session-1", RetryCount: 1},
		{ID: "task-2", SessionID: "session-1", RetryCount: 2},
		{ID: "task-3", SessionID: "session-2", RetryCount: 1},
	}

	storage := &MockTaskRetryStorage{
		tasksReadyForRetry: tasks,
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scheduler := NewRetryScheduler(storage, logger)

	ctx := context.Background()
	err := scheduler.processRetries(ctx)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	requeued := storage.GetRequeuedTasks()
	if len(requeued) != 3 {
		t.Errorf("Expected 3 requeued tasks, got %d", len(requeued))
	}

	requeuedMap := make(map[string]bool)
	for _, id := range requeued {
		requeuedMap[id] = true
	}

	for _, task := range tasks {
		if !requeuedMap[task.ID] {
			t.Errorf("Expected task %s to be requeued", task.ID)
		}
	}
}

func TestRetryScheduler_ProcessRetries_GetTasksError(t *testing.T) {
	expectedErr := errors.New("database connection failed")
	storage := &MockTaskRetryStorage{
		getTasksError: expectedErr,
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scheduler := NewRetryScheduler(storage, logger)

	ctx := context.Background()
	err := scheduler.processRetries(ctx)

	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}

	requeued := storage.GetRequeuedTasks()
	if len(requeued) != 0 {
		t.Errorf("Expected 0 requeued tasks on error, got %d", len(requeued))
	}
}

func TestRetryScheduler_ProcessRetries_RequeueError(t *testing.T) {
	tasks := []*QueuedTask{
		{ID: "task-1", SessionID: "session-1", RetryCount: 1},
		{ID: "task-2", SessionID: "session-1", RetryCount: 2},
	}

	storage := &MockTaskRetryStorage{
		tasksReadyForRetry: tasks,
		requeueErrorFunc: func(taskID string) error {
			return errors.New("requeue failed")
		},
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scheduler := NewRetryScheduler(storage, logger)

	ctx := context.Background()
	err := scheduler.processRetries(ctx)

	if err != nil {
		t.Errorf("Expected no error (errors are logged), got %v", err)
	}

	requeued := storage.GetRequeuedTasks()
	if len(requeued) != 0 {
		t.Errorf("Expected 0 requeued tasks due to errors, got %d", len(requeued))
	}
}

func TestRetryScheduler_ProcessRetries_PartialRequeueFailure(t *testing.T) {
	tasks := []*QueuedTask{
		{ID: "task-1", SessionID: "session-1", RetryCount: 1},
		{ID: "task-2", SessionID: "session-1", RetryCount: 2},
		{ID: "task-3", SessionID: "session-2", RetryCount: 1},
	}

	storage := &MockTaskRetryStorage{
		tasksReadyForRetry: tasks,
		requeueErrorFunc: func(taskID string) error {
			if taskID == "task-2" {
				return errors.New("temporary failure")
			}
			return nil
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scheduler := NewRetryScheduler(storage, logger)

	ctx := context.Background()
	err := scheduler.processRetries(ctx)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	requeued := storage.GetRequeuedTasks()
	if len(requeued) != 2 {
		t.Errorf("Expected 2 successfully requeued tasks, got %d", len(requeued))
	}

	requeuedMap := make(map[string]bool)
	for _, id := range requeued {
		requeuedMap[id] = true
	}
	if !requeuedMap["task-1"] {
		t.Error("Expected task-1 to be requeued")
	}
	if requeuedMap["task-2"] {
		t.Error("Expected task-2 NOT to be requeued (error)")
	}
	if !requeuedMap["task-3"] {
		t.Error("Expected task-3 to be requeued")
	}
}

func TestRetryScheduler_ProcessRetries_BatchLimit(t *testing.T) {
	var tasks []*QueuedTask
	for i := 0; i < 150; i++ {
		tasks = append(tasks, &QueuedTask{
			ID:         string(rune('A' + i)),
			SessionID:  "session-1",
			RetryCount: 1,
		})
	}

	storage := &MockTaskRetryStorage{
		tasksReadyForRetry: tasks,
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scheduler := NewRetryScheduler(storage, logger)

	ctx := context.Background()
	err := scheduler.processRetries(ctx)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	requeued := storage.GetRequeuedTasks()
	if len(requeued) != 100 {
		t.Errorf("Expected 100 requeued tasks (batch limit), got %d", len(requeued))
	}
}

func TestRetryScheduler_Start_StopsOnContextCancel(t *testing.T) {
	storage := &MockTaskRetryStorage{
		tasksReadyForRetry: []*QueuedTask{},
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scheduler := NewRetryScheduler(storage, logger)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		scheduler.Start(ctx)
		close(done)
	}()

	// Wait for at least one tick cycle (1 second + buffer)
	time.Sleep(1200 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Scheduler did not stop after context cancellation")
	}

	getTasks, _, _ := storage.GetCallCounts()
	if getTasks < 1 {
		t.Errorf("Expected at least 1 GetTasksReadyForRetry call, got %d", getTasks)
	}
}

func TestRetryScheduler_Start_ProcessesTasksPeriodically(t *testing.T) {
	tasks := []*QueuedTask{
		{ID: "task-1", SessionID: "session-1", RetryCount: 1},
	}

	storage := &MockTaskRetryStorage{
		tasksReadyForRetry: tasks,
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scheduler := NewRetryScheduler(storage, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		scheduler.Start(ctx)
		close(done)
	}()

	time.Sleep(2500 * time.Millisecond)
	cancel()
	<-done

	getTasks, requeue, _ := storage.GetCallCounts()
	if getTasks < 2 {
		t.Errorf("Expected at least 2 GetTasksReadyForRetry calls, got %d", getTasks)
	}
	if requeue < 2 {
		t.Errorf("Expected at least 2 requeue calls, got %d", requeue)
	}
}

func TestRetryScheduler_Start_HandlesProcessErrorsGracefully(t *testing.T) {
	storage := &MockTaskRetryStorage{
		getTasksError: errors.New("temporary database error"),
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scheduler := NewRetryScheduler(storage, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		scheduler.Start(ctx)
		close(done)
	}()

	time.Sleep(1500 * time.Millisecond)
	cancel()
	<-done

	getTasks, _, _ := storage.GetCallCounts()
	if getTasks < 1 {
		t.Errorf("Expected at least 1 GetTasksReadyForRetry call, got %d", getTasks)
	}
}

func TestRetryScheduler_ConcurrentProcessing(t *testing.T) {
	tasks := []*QueuedTask{
		{ID: "task-1", SessionID: "session-1", RetryCount: 1},
		{ID: "task-2", SessionID: "session-2", RetryCount: 1},
	}

	storage := &MockTaskRetryStorage{
		tasksReadyForRetry: tasks,
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	scheduler := NewRetryScheduler(storage, logger)

	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := scheduler.processRetries(ctx)
			if err != nil {
				t.Errorf("Unexpected error in concurrent processing: %v", err)
			}
		}()
	}

	wg.Wait()

	getTasks, requeue, _ := storage.GetCallCounts()
	if getTasks != 5 {
		t.Errorf("Expected 5 GetTasksReadyForRetry calls, got %d", getTasks)
	}
	if requeue != 10 {
		t.Errorf("Expected 10 requeue calls, got %d", requeue)
	}
}
