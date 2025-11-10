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

// serverConfig holds all configuration for background services
type serverConfig struct {
	ctx            context.Context
	cancel         context.CancelFunc
	logger         *slog.Logger
	taskQueue      *coordinator.TaskQueue
	mcpServer      *coordinator.MCPServer
	coordServer    *coordinator.CoordinatorServer
	sessionManager *coordinator.SessionManager
	grpcServer     *grpc.Server
	grpcPort       string
	httpPort       string
}

func startBackgroundServices(cfg serverConfig) {
	cfg.taskQueue.Start()
	cfg.logger.Info("Task queue background workers started")

	startGRPCServer(cfg)
	startMCPServer(cfg)
	startSessionCleanup(cfg)
	cfg.coordServer.StartCleanupLoop(cfg.ctx, workerCleanupInterval)
}

func startGRPCServer(cfg serverConfig) {
	go func() {
		cfg.logger.Info("Starting gRPC server for workers", "port", cfg.grpcPort)
		listenConfig := net.ListenConfig{}
		lis, err := listenConfig.Listen(cfg.ctx, "tcp", fmt.Sprintf(":%s", cfg.grpcPort))
		if err != nil {
			cfg.logger.Error("Failed to listen on gRPC port", "port", cfg.grpcPort, "error", err)
			cfg.cancel()
			return
		}
		if err := cfg.grpcServer.Serve(lis); err != nil {
			cfg.logger.Error("gRPC server error", "error", err)
			cfg.cancel()
		}
	}()
}

func startMCPServer(cfg serverConfig) {
	go func() {
		if *httpMode {
			cfg.logger.Info("Starting MCP server with HTTP/SSE transport", "port", cfg.httpPort)
			if err := cfg.mcpServer.ServeHTTPWithLogger(":"+cfg.httpPort, cfg.logger); err != nil {
				cfg.logger.Error("MCP server error", "error", err)
				cfg.cancel()
			}
		} else {
			cfg.logger.Info("Starting MCP server on stdio")
			if err := cfg.mcpServer.Serve(); err != nil {
				cfg.logger.Error("MCP server error", "error", err)
				cfg.cancel()
			}
		}
	}()
}

func startSessionCleanup(cfg serverConfig) {
	go func() {
		ticker := time.NewTicker(cleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				deleted := cfg.sessionManager.CleanupStale(defaultSessionMaxAge)
				if deleted > 0 {
					cfg.logger.Info("Cleaned up stale sessions", "count", deleted)
				}
			case <-cfg.ctx.Done():
				return
			}
		}
	}()
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

	startBackgroundServices(serverConfig{
		ctx:            ctx,
		cancel:         cancel,
		logger:         logger,
		taskQueue:      taskQueue,
		mcpServer:      mcpServer,
		coordServer:    coordServer,
		sessionManager: sessionManager,
		grpcServer:     grpcServer,
		grpcPort:       grpcPort,
		httpPort:       httpPort,
	})

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
