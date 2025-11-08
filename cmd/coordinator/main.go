package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator"
)

const (
	defaultSessionMaxAge = 30 * time.Minute
	cleanupInterval      = 5 * time.Minute
)

var (
	version = flag.Bool("version", false, "Print version and exit")
	debug   = flag.Bool("debug", false, "Enable debug logging")
)

func main() {
	flag.Parse()

	if *version {
		fmt.Println("CodeGen MCP Coordinator v0.1.0")
		os.Exit(0)
	}

	// Setup structured logging
	logLevel := slog.LevelInfo
	if *debug {
		logLevel = slog.LevelDebug
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	logger.Info("Starting CodeGen MCP Coordinator",
		"version", "0.1.0",
		"debug", *debug,
	)

	// Initialize components
	sessionManager := coordinator.NewSessionManager()
	workerClient := coordinator.NewMockWorkerClient()
	auditLogger := coordinator.NewAuditLogger(logger)

	// Configure MCP server
	cfg := coordinator.Config{
		Name:    "codegen-mcp-coordinator",
		Version: "0.1.0",
	}

	mcpServer := coordinator.NewMCPServer(cfg, sessionManager, workerClient, auditLogger)

	logger.Info("MCP Server initialized",
		"name", cfg.Name,
		"version", cfg.Version,
	)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine
	go func() {
		logger.Info("Starting MCP server on stdio")
		if err := mcpServer.Serve(); err != nil {
			logger.Error("Server error", "error", err)
			cancel()
		}
	}()

	// Start session cleanup goroutine
	go func() {
		ticker := time.NewTicker(cleanupInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				deleted := sessionManager.CleanupStale(defaultSessionMaxAge)
				if deleted > 0 {
					logger.Info("Cleaned up stale sessions", "count", deleted)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for shutdown signal
	select {
	case <-sigChan:
		logger.Info("Received shutdown signal")
	case <-ctx.Done():
		logger.Info("Context canceled")
	}

	logger.Info("Shutting down gracefully")

	// Allow sessions to drain
	if count := sessionManager.SessionCount(); count > 0 {
		logger.Info("Waiting for active sessions", "count", count)
	}

	logger.Info("Coordinator shutdown complete")
}
