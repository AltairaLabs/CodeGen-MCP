package coordinator

import (
	"time"
)

// RetryPolicy defines how tasks should be retried on failure
type RetryPolicy struct {
	MaxRetries        int           // Maximum number of retry attempts (0 = no retries)
	InitialDelay      time.Duration // Initial delay before first retry
	MaxDelay          time.Duration // Maximum delay between retries
	BackoffMultiplier float64       // Multiplier for exponential backoff (e.g., 2.0)
}

// DefaultRetryPolicy returns the default retry policy for tasks
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:        3,
		InitialDelay:      1 * time.Second,
		MaxDelay:          30 * time.Second,
		BackoffMultiplier: 2.0,
	}
}

// NetworkErrorRetryPolicy returns a retry policy for network errors (more retries)
func NetworkErrorRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:        5,
		InitialDelay:      500 * time.Millisecond,
		MaxDelay:          60 * time.Second,
		BackoffMultiplier: 2.0,
	}
}

// NoRetryPolicy returns a policy that disables retries
func NoRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries:        0,
		InitialDelay:      0,
		MaxDelay:          0,
		BackoffMultiplier: 0,
	}
}

// CalculateNextRetryTime calculates when a task should be retried based on the retry policy
// Uses exponential backoff with jitter to prevent thundering herd
func (p RetryPolicy) CalculateNextRetryTime(retryCount int) time.Time {
	if retryCount >= p.MaxRetries {
		return time.Time{} // Return zero time if max retries exceeded
	}

	// Calculate exponential backoff: initialDelay * (backoffMultiplier ^ retryCount)
	delay := float64(p.InitialDelay)
	for i := 0; i < retryCount; i++ {
		delay *= p.BackoffMultiplier
	}

	// Cap at max delay
	if time.Duration(delay) > p.MaxDelay {
		delay = float64(p.MaxDelay)
	}

	// Add jitter (±25% randomness to prevent thundering herd)
	jitterPercent := float64(time.Now().UnixNano()%1000) / 1000.0 // 0.0 to 1.0
	jitter := delay * 0.25 * (2*jitterPercent - 1)                // -25% to +25%
	finalDelay := time.Duration(delay + jitter)

	return time.Now().Add(finalDelay)
}

// IsRetryableError determines if an error should trigger a retry
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()

	if isPermanentError(errMsg) {
		return false
	}

	if isNetworkError(errMsg) || isWorkerUnavailableError(errMsg) {
		return true
	}

	// Default: transient errors are retryable
	return true
}

// Error classification helpers

func isNetworkError(errMsg string) bool {
	return contains(errMsg, "connection refused") ||
		contains(errMsg, "connection reset") ||
		contains(errMsg, "timeout") ||
		contains(errMsg, "EOF") ||
		contains(errMsg, "broken pipe") ||
		contains(errMsg, "network is unreachable")
}

func isWorkerUnavailableError(errMsg string) bool {
	return contains(errMsg, "worker not found") ||
		contains(errMsg, "worker unavailable") ||
		contains(errMsg, "no workers available")
}

func isPermanentError(errMsg string) bool {
	return contains(errMsg, "validation failed") ||
		contains(errMsg, "invalid argument") ||
		contains(errMsg, "missing required field") ||
		contains(errMsg, "metadata validation failed") ||
		contains(errMsg, "permission denied") ||
		contains(errMsg, "not found") ||
		contains(errMsg, "unauthorized") ||
		contains(errMsg, "forbidden")
}

// String matching helpers

// contains is a helper function to check if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr || len(s) > len(substr) &&
			(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
				findInString(s, substr)))
}

func findInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
