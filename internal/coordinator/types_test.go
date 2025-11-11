package coordinator

import (
	"errors"
	"testing"
	"time"
)

// Test utility functions

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		substr   string
		expected bool
	}{
		{"exact match", "timeout", "timeout", true},
		{"substring in middle", "connection timeout error", "timeout", true},
		{"substring at start", "timeout occurred", "timeout", true},
		{"substring at end", "connection timeout", "timeout", true},
		{"not found", "hello world", "timeout", false},
		{"empty substring", "hello", "", true},
		{"empty string", "", "timeout", false},
		{"case sensitive", "Timeout", "timeout", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.s, tt.substr)
			if result != tt.expected {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, result, tt.expected)
			}
		})
	}
}

func TestFindInString(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		substr   string
		expected bool
	}{
		{"found in middle", "hello world", "wor", true},
		{"found at start", "hello world", "hel", true},
		{"found at end", "hello world", "rld", true},
		{"not found", "hello world", "xyz", false},
		{"empty substring", "hello", "", true},
		{"substring longer than string", "hi", "hello", false},
		{"exact match", "test", "test", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findInString(tt.s, tt.substr)
			if result != tt.expected {
				t.Errorf("findInString(%q, %q) = %v, want %v", tt.s, tt.substr, result, tt.expected)
			}
		})
	}
}

func TestIsNetworkError(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		expected bool
	}{
		{"connection refused", "connection refused by server", true},
		{"connection reset", "connection reset by peer", true},
		{"timeout", "operation timeout", true},
		{"EOF", "unexpected EOF", true},
		{"broken pipe", "write: broken pipe", true},
		{"network unreachable", "network is unreachable", true},
		{"not network error", "invalid argument", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isNetworkError(tt.errMsg)
			if result != tt.expected {
				t.Errorf("isNetworkError(%q) = %v, want %v", tt.errMsg, result, tt.expected)
			}
		})
	}
}

func TestIsWorkerUnavailableError(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		expected bool
	}{
		{"worker not found", "worker not found: worker-123", true},
		{"worker unavailable", "worker unavailable due to maintenance", true},
		{"no workers available", "no workers available to handle request", true},
		{"not worker error", "connection timeout", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isWorkerUnavailableError(tt.errMsg)
			if result != tt.expected {
				t.Errorf("isWorkerUnavailableError(%q) = %v, want %v", tt.errMsg, result, tt.expected)
			}
		})
	}
}

func TestIsPermanentError(t *testing.T) {
	tests := []struct {
		name     string
		errMsg   string
		expected bool
	}{
		{"validation failed", "validation failed: missing field", true},
		{"invalid argument", "invalid argument provided", true},
		{"missing required field", "missing required field: name", true},
		{"metadata validation", "metadata validation failed", true},
		{"permission denied", "permission denied", true},
		{"not found", "resource not found", true},
		{"unauthorized", "unauthorized access", true},
		{"forbidden", "forbidden action", true},
		{"network error", "connection timeout", false},
		{"transient error", "temporary failure", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPermanentError(tt.errMsg)
			if result != tt.expected {
				t.Errorf("isPermanentError(%q) = %v, want %v", tt.errMsg, result, tt.expected)
			}
		})
	}
}

func TestDefaultRetryPolicy(t *testing.T) {
	policy := DefaultRetryPolicy()

	if policy.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries 3, got %d", policy.MaxRetries)
	}
	if policy.InitialDelay != time.Second {
		t.Errorf("Expected InitialDelay 1s, got %v", policy.InitialDelay)
	}
	if policy.MaxDelay != 30*time.Second {
		t.Errorf("Expected MaxDelay 30s, got %v", policy.MaxDelay)
	}
	if policy.BackoffMultiplier != 2.0 {
		t.Errorf("Expected BackoffMultiplier 2.0, got %f", policy.BackoffMultiplier)
	}
}

func TestNetworkErrorRetryPolicy(t *testing.T) {
	policy := NetworkErrorRetryPolicy()

	if policy.MaxRetries != 5 {
		t.Errorf("Expected MaxRetries 5, got %d", policy.MaxRetries)
	}
	if policy.InitialDelay != 500*time.Millisecond {
		t.Errorf("Expected InitialDelay 500ms, got %v", policy.InitialDelay)
	}
	if policy.MaxDelay != 60*time.Second {
		t.Errorf("Expected MaxDelay 60s, got %v", policy.MaxDelay)
	}
	if policy.BackoffMultiplier != 2.0 {
		t.Errorf("Expected BackoffMultiplier 2.0, got %f", policy.BackoffMultiplier)
	}
}

func TestNoRetryPolicy(t *testing.T) {
	policy := NoRetryPolicy()

	if policy.MaxRetries != 0 {
		t.Errorf("Expected MaxRetries 0, got %d", policy.MaxRetries)
	}
}

func TestCalculateNextRetryTime(t *testing.T) {
	policy := RetryPolicy{
		MaxRetries:        10,
		InitialDelay:      time.Second,
		MaxDelay:          10 * time.Second,
		BackoffMultiplier: 2.0,
	}

	// First retry
	next1 := policy.CalculateNextRetryTime(1)
	delay1 := next1.Sub(time.Now())
	if delay1 < 750*time.Millisecond || delay1 > 1500*time.Millisecond {
		t.Errorf("First retry delay should be around 1s with jitter, got %v", delay1)
	}

	// Second retry (should be ~2s with backoff)
	next2 := policy.CalculateNextRetryTime(2)
	delay2 := next2.Sub(time.Now())
	if delay2 < 1500*time.Millisecond || delay2 > 3*time.Second {
		t.Errorf("Second retry delay should be around 2s with jitter, got %v", delay2)
	}

	// Large retry count (should be capped at MaxDelay)
	next10 := policy.CalculateNextRetryTime(9)
	delay10 := next10.Sub(time.Now())
	maxWithJitter := policy.MaxDelay + policy.MaxDelay/4
	if delay10 > maxWithJitter {
		t.Errorf("Retry delay should be capped at MaxDelay (with jitter), got %v, max allowed %v", delay10, maxWithJitter)
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "connection refused",
			err:      errors.New("connection refused"),
			expected: true,
		},
		{
			name:     "deadline exceeded",
			err:      errors.New("context deadline exceeded"),
			expected: true,
		},
		{
			name:     "temporary failure",
			err:      errors.New("temporary failure"),
			expected: true,
		},
		{
			name:     "timeout",
			err:      errors.New("timeout waiting for response"),
			expected: true,
		},
		{
			name:     "non-retryable error",
			err:      errors.New("invalid argument"),
			expected: false,
		},
		{
			name:     "validation failed",
			err:      errors.New("validation failed: missing field"),
			expected: false,
		},
		{
			name:     "permission denied",
			err:      errors.New("permission denied"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryableError(tt.err)
			if result != tt.expected {
				t.Errorf("IsRetryableError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}
