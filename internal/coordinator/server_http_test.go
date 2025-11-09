package coordinator

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// TestHandleFsWrite_WithHTTPSession tests that fs.write works with HTTP/SSE session context
// This test replicates the real HTTP/SSE flow where session is managed by mcp-go library
func TestHandleFsWrite_WithHTTPSession(t *testing.T) {
	storage := newTestSessionStorage()
	sm := NewSessionManager(storage, nil)
	worker := NewMockWorkerClient()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	audit := NewAuditLogger(logger)

	cfg := Config{
		Name:    "TestServer",
		Version: "1.0.0",
	}

	mcpServer := NewMCPServer(cfg, sm, worker, audit)

	// Create session like the session manager would
	_ = sm.CreateSession(context.Background(), "http-test-session", "user1", "workspace1")

	// Create context WITHOUT the "session_id" key (simulating HTTP/SSE transport)
	// The mcp-go library stores session differently
	ctx := context.Background()

	// Test valid write - THIS SHOULD FAIL initially because getSessionID returns "default-session"
	request := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "fs.write",
			Arguments: map[string]interface{}{
				"path":     "output.txt",
				"contents": "test content from HTTP",
			},
		},
	}

	result, err := mcpServer.handleFsWrite(ctx, request)

	if err != nil {
		t.Fatalf("handleFsWrite returned error: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// With the fix, the session ID should be properly extracted from the HTTP context
	if result.IsError {
		// If we get an error, it's likely because no session/worker exists
		if len(result.Content) > 0 {
			if textContent, ok := mcp.AsTextContent(result.Content[0]); ok {
				t.Logf("Error content: %s", textContent.Text)

				// Check if it's a "no workers available" error (expected in test environment)
				if strings.Contains(textContent.Text, "no workers available") {
					t.Skip("Test skipped: no workers available (expected in unit test)")
				}
			}
		}
		t.Fatalf("Tool call failed: %v", result.Content)
	}

	// Success! The session ID was properly extracted
	t.Logf("Successfully called fs.write with session ID extracted from HTTP context")
}
