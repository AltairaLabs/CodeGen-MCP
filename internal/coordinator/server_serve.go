package coordinator

import (
	"log/slog"

	"github.com/mark3labs/mcp-go/server"
)

// This file contains server startup methods that are untestable in unit tests
// as they start blocking servers. These should be tested via integration tests.

// Serve starts the MCP server with stdio transport
func (ms *MCPServer) Serve() error {
	return server.ServeStdio(ms.server)
}

// ServeWithLogger starts the MCP server with stdio transport and custom logger
func (ms *MCPServer) ServeWithLogger(logger *slog.Logger) error {
	logger.Info("Starting MCP server with stdio transport")
	return ms.Serve()
}

// ServeHTTP starts the MCP server with HTTP/SSE transport on the specified address
func (ms *MCPServer) ServeHTTP(addr string) error {
	sseServer := server.NewSSEServer(ms.server,
		server.WithBaseURL("http://"+addr),
		server.WithStaticBasePath("/mcp"),
	)
	return sseServer.Start(addr)
}

// ServeHTTPWithLogger starts the MCP server with HTTP/SSE transport and custom logger
func (ms *MCPServer) ServeHTTPWithLogger(addr string, logger *slog.Logger) error {
	logger.Info("Starting MCP server with HTTP/SSE transport", "address", addr, "base_path", "/mcp")
	return ms.ServeHTTP(addr)
}
