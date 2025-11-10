package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator"
	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/storage/memory"
)

const (
	defaultSessionMaxAge  = 30 * time.Minute
	cleanupInterval       = 5 * time.Minute
	defaultGRPCPort       = "50050"
	workerCleanupInterval = 1 * time.Minute
)

var (
	version  = flag.Bool("version", false, "Print version and exit")
	debug    = flag.Bool("debug", false, "Enable debug logging")
	httpMode = flag.Bool("http", false, "Enable HTTP/SSE transport instead of stdio")
)

func setupLogger() *slog.Logger {
	logLevel := slog.LevelInfo
	if *debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)
	return logger
}

func getPortConfig() (grpcPort, httpPort string) {
	grpcPort = os.Getenv("GRPC_PORT")
	if grpcPort == "" {
		grpcPort = defaultGRPCPort
	}
	httpPort = os.Getenv("HTTP_PORT")
	if httpPort == "" {
		httpPort = "8080"
	}
	return grpcPort, httpPort
}

func initializeComponents(logger *slog.Logger) (
	*coordinator.TaskQueue,
	*coordinator.MCPServer,
	*coordinator.CoordinatorServer,
	*coordinator.SessionManager,
) {
	var sessionStateStorage coordinator.SessionStateStorage = memory.NewInMemorySessionStateStorage()
	taskQueueStorage := memory.NewInMemoryTaskQueueStorage(10000, 100)
	workerRegistry := coordinator.NewWorkerRegistry()
	sessionManager := coordinator.NewSessionManager(sessionStateStorage, workerRegistry)
	workerClient := coordinator.NewRealWorkerClient(workerRegistry, sessionManager, logger)
	auditLogger := coordinator.NewAuditLogger(logger)

	retryPolicy := coordinator.DefaultRetryPolicy()
	taskDispatcher := coordinator.NewTaskDispatcher(taskQueueStorage, workerClient, &retryPolicy)
	taskQueue := coordinator.NewTaskQueue(
		taskQueueStorage,
		sessionManager,
		workerClient,
		taskDispatcher,
		logger,
		coordinator.DefaultTaskQueueConfig(),
	)

	cfg := coordinator.Config{
		Name:    "codegen-mcp-coordinator",
		Version: "0.1.0",
	}
	mcpServer := coordinator.NewMCPServer(cfg, sessionManager, workerClient, auditLogger, taskQueue)
	coordServer := coordinator.NewCoordinatorServer(workerRegistry, sessionManager, logger)

	return taskQueue, mcpServer, coordServer, sessionManager
}

func startBackgroundServices(
	ctx context.Context,
	cancel context.CancelFunc,
	logger *slog.Logger,
	taskQueue *coordinator.TaskQueue,
	mcpServer *coordinator.MCPServer,
	coordServer *coordinator.CoordinatorServer,
	sessionManager *coordinator.SessionManager,
	grpcServer *grpc.Server,
	grpcPort, httpPort string,
) {
	taskQueue.Start()
	logger.Info("Task queue background workers started")

	// Start gRPC server
	go func() {
		logger.Info("Starting gRPC server for workers", "port", grpcPort)
		listenConfig := net.ListenConfig{}
		lis, err := listenConfig.Listen(ctx, "tcp", fmt.Sprintf(":%s", grpcPort))
		if err != nil {
			logger.Error("Failed to listen on gRPC port", "port", grpcPort, "error", err)
			cancel()
			return
		}
		if err := grpcServer.Serve(lis); err != nil {
			logger.Error("gRPC server error", "error", err)
			cancel()
		}
	}()

	// Start MCP server
	go func() {
		if *httpMode {
			logger.Info("Starting MCP server with HTTP/SSE transport", "port", httpPort)
			if err := mcpServer.ServeHTTPWithLogger(":"+httpPort, logger); err != nil {
				logger.Error("MCP server error", "error", err)
				cancel()
			}
		} else {
			logger.Info("Starting MCP server on stdio")
			if err := mcpServer.Serve(); err != nil {
				logger.Error("MCP server error", "error", err)
				cancel()
			}
		}
	}()

	// Start session cleanup
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

	// Start worker cleanup
	go coordServer.StartCleanupLoop(ctx, workerCleanupInterval)
}

func main() {
	flag.Parse()

	if *version {
		fmt.Println("CodeGen MCP Coordinator v0.1.0")
		os.Exit(0)
	}

	logger := setupLogger()
	grpcPort, httpPort := getPortConfig()

	logger.Info("Starting CodeGen MCP Coordinator",
		"version", "0.1.0",
		"debug", *debug,
		"grpc_port", grpcPort,
		"http_mode", *httpMode,
		"http_port", httpPort,
	)

	taskQueue, mcpServer, coordServer, sessionManager := initializeComponents(logger)

	logger.Info("MCP Server initialized", "name", "codegen-mcp-coordinator", "version", "0.1.0")

	grpcServer := grpc.NewServer()
	coordServer.RegisterWithServer(grpcServer)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startBackgroundServices(ctx, cancel, logger, taskQueue, mcpServer, coordServer, sessionManager, grpcServer, grpcPort, httpPort)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigChan:
		logger.Info("Received shutdown signal")
	case <-ctx.Done():
		logger.Info("Context canceled")
	}

	logger.Info("Shutting down gracefully")
	cancel()

	// Stop task queue
	logger.Info("Stopping task queue")
	taskQueue.Stop()

	// Stop gRPC server with timeout
	logger.Info("Stopping gRPC server")

	// For long-running bidirectional streams, GracefulStop will wait forever
	// Use a short timeout and force stop if needed
	const shutdownTimeout = 2 * time.Second
	shutdownComplete := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(shutdownComplete)
	}()

	// Wait for graceful shutdown with timeout
	select {
	case <-shutdownComplete:
		logger.Info("gRPC server stopped gracefully")
	case <-time.After(shutdownTimeout):
		logger.Warn("Graceful shutdown timeout, forcing stop")
		grpcServer.Stop()
		<-shutdownComplete // Wait for GracefulStop to return after Stop()
	}

	logger.Info("Coordinator shutdown complete")
}
