package retry

import (
	"errors"
	"testing"
	"time"
)

func TestDefaultPolicy(t *testing.T) {
	policy := DefaultPolicy()

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

func TestNetworkErrorPolicy(t *testing.T) {
	policy := NetworkErrorPolicy()

	if policy.MaxRetries != 5 {
		t.Errorf("Expected MaxRetries=5, got %d", policy.MaxRetries)
	}
	if policy.InitialDelay != 500*time.Millisecond {
		t.Errorf("Expected InitialDelay=500ms, got %v", policy.InitialDelay)
	}
	if policy.MaxDelay != 10*time.Second {
		t.Errorf("Expected MaxDelay=10s, got %v", policy.MaxDelay)
	}
	if policy.BackoffMultiplier != 2.0 {
		t.Errorf("Expected BackoffMultiplier=2.0, got %f", policy.BackoffMultiplier)
	}
}

func TestQuickRetryPolicy(t *testing.T) {
	policy := QuickRetryPolicy()

	if policy.MaxRetries != 2 {
		t.Errorf("Expected MaxRetries=2, got %d", policy.MaxRetries)
	}
	if policy.InitialDelay != 100*time.Millisecond {
		t.Errorf("Expected InitialDelay=100ms, got %v", policy.InitialDelay)
	}
	if policy.MaxDelay != 1*time.Second {
		t.Errorf("Expected MaxDelay=1s, got %v", policy.MaxDelay)
	}
	if policy.BackoffMultiplier != 1.5 {
		t.Errorf("Expected BackoffMultiplier=1.5, got %f", policy.BackoffMultiplier)
	}
}

func TestPolicyCalculateDelay(t *testing.T) {
	policy := Policy{
		MaxRetries:        3,
		InitialDelay:      1 * time.Second,
		MaxDelay:          10 * time.Second,
		BackoffMultiplier: 2.0,
	}

	tests := []struct {
		retryCount int
		expected   time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 10 * time.Second}, // Capped at MaxDelay
	}

	for _, test := range tests {
		actual := policy.CalculateDelay(test.retryCount)
		if actual != test.expected {
			t.Errorf("For retryCount %d, expected delay %v, got %v",
				test.retryCount, test.expected, actual)
		}
	}
}

func TestPolicyShouldRetry(t *testing.T) {
	policy := Policy{MaxRetries: 3}

	tests := []struct {
		retryCount int
		expected   bool
	}{
		{0, true},
		{1, true},
		{2, true},
		{3, false},
		{4, false},
	}

	for _, test := range tests {
		actual := policy.ShouldRetry(test.retryCount)
		if actual != test.expected {
			t.Errorf("For retryCount %d, expected %t, got %t",
				test.retryCount, test.expected, actual)
		}
	}
}

func TestIsRetriableError(t *testing.T) {
	tests := []struct {
		err      error
		expected bool
	}{
		{nil, false},
		{errors.New("connection refused"), true},
		{errors.New("timeout"), true},
		{errors.New("network is unreachable"), true},
		{errors.New("temporary failure"), true},
		{errors.New("service unavailable"), true},
		{errors.New("too many requests"), true},
		{errors.New("context deadline exceeded"), true},
		{errors.New("not found"), false},
		{errors.New("invalid input"), false},
		{errors.New("permission denied"), false},
	}

	for _, test := range tests {
		actual := IsRetriableError(test.err)
		if actual != test.expected {
			errMsg := "nil"
			if test.err != nil {
				errMsg = test.err.Error()
			}
			t.Errorf("For error %q, expected %t, got %t",
				errMsg, test.expected, actual)
		}
	}
}

func TestPolicyValidate(t *testing.T) {
	tests := []struct {
		name    string
		policy  Policy
		wantErr bool
	}{
		{
			name: "valid policy",
			policy: Policy{
				MaxRetries:        3,
				InitialDelay:      1 * time.Second,
				MaxDelay:          30 * time.Second,
				BackoffMultiplier: 2.0,
			},
			wantErr: false,
		},
		{
			name: "negative max retries",
			policy: Policy{
				MaxRetries:        -1,
				InitialDelay:      1 * time.Second,
				MaxDelay:          30 * time.Second,
				BackoffMultiplier: 2.0,
			},
			wantErr: true,
		},
		{
			name: "zero initial delay",
			policy: Policy{
				MaxRetries:        3,
				InitialDelay:      0,
				MaxDelay:          30 * time.Second,
				BackoffMultiplier: 2.0,
			},
			wantErr: true,
		},
		{
			name: "zero max delay",
			policy: Policy{
				MaxRetries:        3,
				InitialDelay:      1 * time.Second,
				MaxDelay:          0,
				BackoffMultiplier: 2.0,
			},
			wantErr: true,
		},
		{
			name: "zero backoff multiplier",
			policy: Policy{
				MaxRetries:        3,
				InitialDelay:      1 * time.Second,
				MaxDelay:          30 * time.Second,
				BackoffMultiplier: 0,
			},
			wantErr: true,
		},
		{
			name: "initial delay greater than max delay",
			policy: Policy{
				MaxRetries:        3,
				InitialDelay:      30 * time.Second,
				MaxDelay:          10 * time.Second,
				BackoffMultiplier: 2.0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Policy.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}