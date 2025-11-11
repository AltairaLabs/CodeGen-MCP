package tools

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestRegisterAndGet(t *testing.T) {
	const (
		toolA = "test.tool"
		toolB = "other.tool"
	)

	called := false
	handler := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return mcp.NewToolResultText("ok"), nil
	}

	r := NewToolHandlerRegistry(map[string]ToolHandlerFunc{toolA: handler})

	h, err := r.GetHandler(toolA)
	if err != nil {
		t.Fatalf("expected handler, got error: %v", err)
	}

	var req mcp.CallToolRequest
	res, err := h(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if res == nil {
		t.Fatalf("expected non-nil result")
	}
	if !called {
		t.Fatalf("expected handler to be called")
	}

	// Register a new handler and ensure GetAllHandlers reflects it
	handler2 := func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("ok2"), nil
	}
	r.Register(toolB, handler2)

	all := r.GetAllHandlers()
	if len(all) != 2 {
		t.Fatalf("expected 2 handlers, got %d", len(all))
	}
	if _, ok := all[toolA]; !ok {
		t.Fatalf("%s missing from GetAllHandlers", toolA)
	}
	if _, ok := all[toolB]; !ok {
		t.Fatalf("%s missing from GetAllHandlers", toolB)
	}
}

func TestMissingHandler(t *testing.T) {
	r := NewToolHandlerRegistry(nil)
	if _, err := r.GetHandler("nope"); err == nil {
		t.Fatalf("expected error for missing handler")
	}
}
