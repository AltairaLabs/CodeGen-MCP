package cache

import (
	"context"
	"testing"
	"time"
)

// mockTaskResult implements TaskResultInterface for testing
type mockTaskResult struct {
	success  bool
	output   string
	error    string
	exitCode int
	duration time.Duration
}

func (m *mockTaskResult) GetSuccess() bool           { return m.success }
func (m *mockTaskResult) GetOutput() string          { return m.output }
func (m *mockTaskResult) GetError() string           { return m.error }
func (m *mockTaskResult) GetExitCode() int           { return m.exitCode }
func (m *mockTaskResult) GetDuration() time.Duration { return m.duration }

func TestNewResultCache(t *testing.T) {
	cache := NewResultCache(5 * time.Minute)
	if cache == nil {
		t.Fatal("Expected non-nil cache")
	}
	if cache.results == nil {
		t.Error("Expected results map to be initialized")
	}
	if cache.ttl != 5*time.Minute {
		t.Errorf("Expected TTL of 5 minutes, got %v", cache.ttl)
	}
}

func TestResultCacheStoreAndGet(t *testing.T) {
	cache := NewResultCache(5 * time.Minute)
	ctx := context.Background()

	taskID := "test-task-123"
	result := &mockTaskResult{
		success: true,
		output:  "test output",
	} // Store result
	err := cache.Store(ctx, taskID, result)
	if err != nil {
		t.Fatalf("Failed to store result: %v", err)
	}

	// Get result
	retrieved, err := cache.Get(ctx, taskID)
	if err != nil {
		t.Fatalf("Failed to get result: %v", err)
	}
	if retrieved == nil {
		t.Fatal("Expected non-nil result")
	}
	if retrieved.GetOutput() != result.GetOutput() {
		t.Errorf("Expected output %s, got %s", result.GetOutput(), retrieved.GetOutput())
	}
}

func TestResultCacheGetNonExistent(t *testing.T) {
	cache := NewResultCache(5 * time.Minute)
	ctx := context.Background()

	result, err := cache.Get(ctx, "non-existent-task")
	if err == nil {
		t.Error("Expected error for non-existent task")
	}
	if result != nil {
		t.Error("Expected nil result for non-existent task")
	}
}

func TestResultCacheDelete(t *testing.T) {
	cache := NewResultCache(5 * time.Minute)
	ctx := context.Background()

	taskID := "test-task-delete"
	result := &mockTaskResult{
		output: "test",
	}

	// Store and verify
	_ = cache.Store(ctx, taskID, result)
	if _, err := cache.Get(ctx, taskID); err != nil {
		t.Fatal("Result should exist before delete")
	}

	// Delete
	cache.Delete(ctx, taskID)

	// Verify deleted
	if _, err := cache.Get(ctx, taskID); err == nil {
		t.Error("Result should not exist after delete")
	}
}

func TestResultCacheExpiration(t *testing.T) {
	// Use very short TTL for testing
	cache := NewResultCache(100 * time.Millisecond)
	ctx := context.Background()

	taskID := "test-task-expire"
	result := &mockTaskResult{
		output: "expires soon",
	}

	// Store result
	_ = cache.Store(ctx, taskID, result)

	// Verify it exists
	if _, err := cache.Get(ctx, taskID); err != nil {
		t.Fatal("Result should exist immediately after store")
	}

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Verify it's expired
	if _, err := cache.Get(ctx, taskID); err == nil {
		t.Error("Result should be expired after TTL")
	}
}

func TestResultCacheCleanup(t *testing.T) {
	cache := NewResultCache(50 * time.Millisecond)
	ctx := context.Background()

	// Add multiple results
	for i := 0; i < 5; i++ {
		taskID := "task-" + string(rune('0'+i))
		result := &mockTaskResult{output: "test"}
		_ = cache.Store(ctx, taskID, result)
	}

	// Verify all are stored
	if cache.Size() != 5 {
		t.Errorf("Expected 5 cached results, got %d", cache.Size())
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Trigger cleanup
	cache.cleanup()

	// Verify cleanup removed expired results
	if cache.Size() != 0 {
		t.Errorf("Expected 0 cached results after cleanup, got %d", cache.Size())
	}
}

func TestResultCacheSize(t *testing.T) {
	cache := NewResultCache(5 * time.Minute)
	ctx := context.Background()

	if cache.Size() != 0 {
		t.Error("Expected empty cache initially")
	}

	// Add results
	for i := 0; i < 3; i++ {
		taskID := "task-" + string(rune('0'+i))
		result := &mockTaskResult{output: "test"}
		_ = cache.Store(ctx, taskID, result)
	}

	if cache.Size() != 3 {
		t.Errorf("Expected 3 cached results, got %d", cache.Size())
	}
}

func TestResultCacheClear(t *testing.T) {
	cache := NewResultCache(5 * time.Minute)
	ctx := context.Background()

	// Add results
	for i := 0; i < 5; i++ {
		taskID := "task-" + string(rune('0'+i))
		result := &mockTaskResult{output: "test"}
		_ = cache.Store(ctx, taskID, result)
	}

	if cache.Size() != 5 {
		t.Fatalf("Expected 5 cached results before clear, got %d", cache.Size())
	}

	// Clear
	cache.Clear()

	// Verify cleared
	if cache.Size() != 0 {
		t.Errorf("Expected 0 cached results after clear, got %d", cache.Size())
	}
}

func TestResultCacheClose(t *testing.T) {
	cache := NewResultCache(5 * time.Minute)

	// Close should not panic
	cache.Close()

	// Verify done channel is closed
	select {
	case <-cache.done:
		// Expected - channel closed
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected done channel to be closed")
	}
}

func TestResultCacheConcurrent(t *testing.T) {
	cache := NewResultCache(5 * time.Minute)
	ctx := context.Background()

	// Test concurrent Store operations
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			taskID := "task-" + string(rune('0'+id))
			result := &mockTaskResult{output: "test"}
			_ = cache.Store(ctx, taskID, result)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all results are stored
	if cache.Size() != 10 {
		t.Errorf("Expected 10 cached results, got %d", cache.Size())
	}
}

func TestCachedResult_GetResult(t *testing.T) {
	mockResult := &mockTaskResult{
		success: true,
		output:  "test output",
	}

	cachedResult := &CachedResult{
		Result:    mockResult,
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}

	result := cachedResult.GetResult()
	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if result.GetOutput() != "test output" {
		t.Errorf("Expected output 'test output', got %s", result.GetOutput())
	}
}

func TestCachedResult_GetCachedAt(t *testing.T) {
	now := time.Now()
	cachedResult := &CachedResult{
		Result:    &mockTaskResult{},
		CachedAt:  now,
		ExpiresAt: now.Add(5 * time.Minute),
	}

	cachedAt := cachedResult.GetCachedAt()
	if !cachedAt.Equal(now) {
		t.Errorf("Expected CachedAt to be %v, got %v", now, cachedAt)
	}
}

func TestCachedResult_GetExpiresAt(t *testing.T) {
	now := time.Now()
	expiresAt := now.Add(5 * time.Minute)
	cachedResult := &CachedResult{
		Result:    &mockTaskResult{},
		CachedAt:  now,
		ExpiresAt: expiresAt,
	}

	gotExpiresAt := cachedResult.GetExpiresAt()
	if !gotExpiresAt.Equal(expiresAt) {
		t.Errorf("Expected ExpiresAt to be %v, got %v", expiresAt, gotExpiresAt)
	}
}
