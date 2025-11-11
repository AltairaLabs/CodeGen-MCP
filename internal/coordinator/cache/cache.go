package cache

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	// ErrEmptyTaskID is returned when a task ID is empty
	ErrEmptyTaskID = "taskID cannot be empty"
)

// ResultCache caches task results for retrieval with TTL-based expiration
type ResultCache struct {
	results map[string]*CachedResult
	mu      sync.RWMutex
	ttl     time.Duration
	done    chan struct{} // Signal to stop cleanup goroutine
}

// CachedResult represents a cached task result with expiration metadata
type CachedResult struct {
	Result    TaskResultInterface
	CachedAt  time.Time
	ExpiresAt time.Time
}

// GetResult implements CachedResultInterface
func (cr *CachedResult) GetResult() TaskResultInterface {
	return cr.Result
}

// GetCachedAt implements CachedResultInterface
func (cr *CachedResult) GetCachedAt() time.Time {
	return cr.CachedAt
}

// GetExpiresAt implements CachedResultInterface
func (cr *CachedResult) GetExpiresAt() time.Time {
	return cr.ExpiresAt
}

// NewResultCache creates a new result cache with the specified TTL
// Starts a background cleanup goroutine that removes expired results
func NewResultCache(ttl time.Duration) *ResultCache {
	cache := &ResultCache{
		results: make(map[string]*CachedResult),
		ttl:     ttl,
		done:    make(chan struct{}),
	}

	// Start cleanup goroutine
	go cache.cleanupLoop()

	return cache
}

// Store caches a task result with the configured TTL
func (rc *ResultCache) Store(ctx context.Context, taskID string, result TaskResultInterface) error {
	if taskID == "" {
		return fmt.Errorf(ErrEmptyTaskID)
	}

	if result == nil {
		return fmt.Errorf("result cannot be nil")
	}

	rc.mu.Lock()
	defer rc.mu.Unlock()

	now := time.Now()
	rc.results[taskID] = &CachedResult{
		Result:    result,
		CachedAt:  now,
		ExpiresAt: now.Add(rc.ttl),
	}

	return nil
}

// Get gets a cached task result by task ID
// Returns the result and any error, returns error if not found or expired
func (rc *ResultCache) Get(ctx context.Context, taskID string) (TaskResultInterface, error) {
	if taskID == "" {
		return nil, fmt.Errorf(ErrEmptyTaskID)
	}

	rc.mu.RLock()
	defer rc.mu.RUnlock()

	cached, exists := rc.results[taskID]
	if !exists {
		return nil, fmt.Errorf("result not found: %s", taskID)
	}

	// Check if expired
	if time.Now().After(cached.ExpiresAt) {
		return nil, fmt.Errorf("result expired: %s", taskID)
	}

	return cached.Result, nil
}

// Delete removes a cached result by task ID
func (rc *ResultCache) Delete(ctx context.Context, taskID string) {
	if taskID == "" {
		return // silently ignore empty taskID for compatibility
	}

	rc.mu.Lock()
	defer rc.mu.Unlock()

	delete(rc.results, taskID)
}

// Size returns the current number of cached results
func (rc *ResultCache) Size() int {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return len(rc.results)
}

// Clear removes all cached results
func (rc *ResultCache) Clear() {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.results = make(map[string]*CachedResult)
}

// Close stops the cleanup goroutine
func (rc *ResultCache) Close() {
	close(rc.done)
}

// cleanupLoop periodically removes expired results
func (rc *ResultCache) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rc.cleanup()
		case <-rc.done:
			return
		}
	}
}

// cleanup removes expired results
func (rc *ResultCache) cleanup() {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	now := time.Now()
	for taskID, cached := range rc.results {
		if now.After(cached.ExpiresAt) {
			delete(rc.results, taskID)
		}
	}
}
