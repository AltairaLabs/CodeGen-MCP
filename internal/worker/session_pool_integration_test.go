// Integration tests for session pool that require Python venv initialization
// These tests are skipped in -short mode

package worker

import "testing"

// TestSessionPoolIntegration is a placeholder that will skip all session pool tests in short mode
func TestSessionPoolIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping session pool integration tests - they require Python venv initialization")
	}
}
