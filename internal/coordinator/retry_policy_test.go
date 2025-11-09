package coordinator

import (
	"errors"
	"testing"
	"time"
)

func TestDefaultRetryPolicy(t *testing.T) {
	policy := DefaultRetryPolicy()

	if policy.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries=3, got %d", policy.MaxRetries)
	}
	if policy.InitialDelay != 1*time.Second {
		t.Errorf("Expected InitialDelay=1s, got %v", policy.InitialDelay)
	}
	if policy.MaxDelay != 30*time.Second {
		t.Errorf("Expected MaxDelay=30s, got %v", policy.MaxDelay)
	}
	if policy.BackoffMultiplier != 2.0 {
		t.Errorf("Expected BackoffMultiplier=2.0, got %f", policy.BackoffMultiplier)
	}
}

func TestNetworkErrorRetryPolicy(t *testing.T) {
	policy := NetworkErrorRetryPolicy()

	if policy.MaxRetries != 5 {
		t.Errorf("Expected MaxRetries=5, got %d", policy.MaxRetries)
	}
	if policy.InitialDelay != 500*time.Millisecond {
		t.Errorf("Expected InitialDelay=500ms, got %v", policy.InitialDelay)
	}
	if policy.MaxDelay != 60*time.Second {
		t.Errorf("Expected MaxDelay=60s, got %v", policy.MaxDelay)
	}
	if policy.BackoffMultiplier != 2.0 {
		t.Errorf("Expected BackoffMultiplier=2.0, got %f", policy.BackoffMultiplier)
	}
}

func TestNoRetryPolicy(t *testing.T) {
	policy := NoRetryPolicy()

	if policy.MaxRetries != 0 {
		t.Errorf("Expected MaxRetries=0, got %d", policy.MaxRetries)
	}
}

func TestCalculateNextRetryTime(t *testing.T) {
	policy := RetryPolicy{
		MaxRetries:        3,
		InitialDelay:      1 * time.Second,
		MaxDelay:          30 * time.Second,
		BackoffMultiplier: 2.0,
	}

	tests := []struct {
		name       string
		retryCount int
		expectZero bool
		minDelay   time.Duration
		maxDelay   time.Duration
	}{
		{
			name:       "First retry",
			retryCount: 0,
			minDelay:   750 * time.Millisecond,  // 1s - 25% jitter
			maxDelay:   1250 * time.Millisecond, // 1s + 25% jitter
		},
		{
			name:       "Second retry",
			retryCount: 1,
			minDelay:   1500 * time.Millisecond, // 2s - 25% jitter
			maxDelay:   2500 * time.Millisecond, // 2s + 25% jitter
		},
		{
			name:       "Third retry",
			retryCount: 2,
			minDelay:   3000 * time.Millisecond, // 4s - 25% jitter
			maxDelay:   5000 * time.Millisecond, // 4s + 25% jitter
		},
		{
			name:       "Exceeds max retries",
			retryCount: 3,
			expectZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Now()
			nextRetry := policy.CalculateNextRetryTime(tt.retryCount)

			if tt.expectZero {
				if !nextRetry.IsZero() {
					t.Errorf("Expected zero time, got %v", nextRetry)
				}
				return
			}

			delay := nextRetry.Sub(now)
			if delay < tt.minDelay {
				t.Errorf("Delay too short: got %v, expected at least %v", delay, tt.minDelay)
			}
			if delay > tt.maxDelay {
				t.Errorf("Delay too long: got %v, expected at most %v", delay, tt.maxDelay)
			}
		})
	}
}

func TestCalculateNextRetryTimeRespectsMaxDelay(t *testing.T) {
	policy := RetryPolicy{
		MaxRetries:        10,
		InitialDelay:      1 * time.Second,
		MaxDelay:          5 * time.Second,
		BackoffMultiplier: 2.0,
	}

	// After 3 retries: 1s * 2^3 = 8s, which exceeds MaxDelay of 5s
	now := time.Now()
	nextRetry := policy.CalculateNextRetryTime(3)
	delay := nextRetry.Sub(now)

	// Should be capped at MaxDelay (5s) with jitter (±25%)
	minExpected := 3750 * time.Millisecond // 5s - 25%
	maxExpected := 6250 * time.Millisecond // 5s + 25%

	if delay < minExpected || delay > maxExpected {
		t.Errorf("Delay not capped correctly: got %v, expected between %v and %v",
			delay, minExpected, maxExpected)
	}
}

func TestIsRetryableError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		// Retryable errors
		{
			name:      "Connection refused",
			err:       errors.New("connection refused"),
			retryable: true,
		},
		{
			name:      "Dial tcp timeout",
			err:       errors.New("dial tcp: i/o timeout"),
			retryable: true,
		},
		{
			name:      "Context deadline exceeded",
			err:       errors.New("context deadline exceeded"),
			retryable: true,
		},
		{
			name:      "EOF error",
			err:       errors.New("EOF"),
			retryable: true,
		},
		{
			name:      "Unavailable error",
			err:       errors.New("service unavailable"),
			retryable: true,
		},
		{
			name:      "Connection reset",
			err:       errors.New("connection reset by peer"),
			retryable: true,
		},

		// Non-retryable errors
		{
			name:      "Validation error",
			err:       errors.New("validation failed: invalid input"),
			retryable: false,
		},
		{
			name:      "Permission denied",
			err:       errors.New("permission denied"),
			retryable: false,
		},
		{
			name:      "Not found",
			err:       errors.New("not found"),
			retryable: false,
		},
		{
			name:      "Invalid argument",
			err:       errors.New("invalid argument"),
			retryable: false,
		},
		{
			name:      "Nil error",
			err:       nil,
			retryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryableError(tt.err)
			if result != tt.retryable {
				t.Errorf("IsRetryableError(%v) = %v, expected %v",
					tt.err, result, tt.retryable)
			}
		})
	}
}
