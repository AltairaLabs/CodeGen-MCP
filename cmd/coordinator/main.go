package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator"
)

const (
	defaultSessionMaxAge  = 30 * time.Minute
	cleanupInterval       = 5 * time.Minute
	defaultGRPCPort       = "50050"
	workerCleanupInterval = 1 * time.Minute
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

	// Read gRPC port from environment
	grpcPort := os.Getenv("GRPC_PORT")
	if grpcPort == "" {
		grpcPort = defaultGRPCPort
	}

	logger.Info("Starting CodeGen MCP Coordinator",
		"version", "0.1.0",
		"debug", *debug,
		"grpc_port", grpcPort,
	)

	// Initialize components
	workerRegistry := coordinator.NewWorkerRegistry()
	sessionManager := coordinator.NewSessionManager(workerRegistry)
	workerClient := coordinator.NewRealWorkerClient(workerRegistry, sessionManager, logger)
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

	// Create coordinator gRPC server for worker lifecycle
	coordServer := coordinator.NewCoordinatorServer(workerRegistry, sessionManager, logger)
	grpcServer := grpc.NewServer()
	coordServer.RegisterWithServer(grpcServer)

	// Setup context for shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// Start listening for worker connections
	listenConfig := net.ListenConfig{}
	lis, err := listenConfig.Listen(ctx, "tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		cancel()
		log.Fatalf("Failed to listen on port %s: %v", grpcPort, err)
	}
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start gRPC server for workers
	go func() {
		logger.Info("Starting gRPC server for workers", "port", grpcPort)
		if err := grpcServer.Serve(lis); err != nil {
			logger.Error("gRPC server error", "error", err)
			cancel()
		}
	}()

	// Start MCP server in goroutine
	go func() {
		logger.Info("Starting MCP server on stdio")
		if err := mcpServer.Serve(); err != nil {
			logger.Error("MCP server error", "error", err)
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

	// Start worker cleanup goroutine
	go coordServer.StartCleanupLoop(ctx, workerCleanupInterval)

	// Wait for shutdown signal
	select {
	case <-sigChan:
		logger.Info("Received shutdown signal")
	case <-ctx.Done():
		logger.Info("Context canceled")
	}

	logger.Info("Shutting down gracefully")

	// Stop gRPC server
	logger.Info("Stopping gRPC server")
	grpcServer.GracefulStop()

	// Allow sessions to drain
	if count := sessionManager.SessionCount(); count > 0 {
		logger.Info("Waiting for active sessions", "count", count)
	}

	logger.Info("Coordinator shutdown complete")
}
