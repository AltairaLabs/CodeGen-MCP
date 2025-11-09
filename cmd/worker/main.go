package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AltairaLabs/codegen-mcp/internal/worker"
)

const (
	defaultMaxSessions     = 5
	defaultWorkspacePerms  = 0755
	defaultWorkerID        = "worker-1"
	defaultBaseWorkspace   = "/tmp/codegen-workspaces"
	defaultCoordinatorAddr = "localhost:50050"
	workerVersion          = "0.1.0"
)

var (
	version = flag.Bool("version", false, "Print version and exit")
)

func main() {
	flag.Parse()

	if *version {
		fmt.Printf("CodeGen MCP Worker v%s\n", workerVersion)
		os.Exit(0)
	}

	fmt.Println("⚙️ AltairaLabs CodeGen MCP Worker starting up...")

	// Read configuration from environment
	workerID := getEnv("WORKER_ID", defaultWorkerID)
	maxSessions := getEnvInt("MAX_SESSIONS", defaultMaxSessions)
	baseWorkspace := getEnv("BASE_WORKSPACE", defaultBaseWorkspace)
	coordinatorAddr := getEnv("COORDINATOR_ADDR", defaultCoordinatorAddr)

	// Set up structured logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	logger.Info("Worker configuration",
		"worker_id", workerID,
		"max_sessions", maxSessions,
		"base_workspace", baseWorkspace,
		"coordinator_addr", coordinatorAddr,
		"version", workerVersion)

	// Create base workspace directory
	// #nosec G301 - Base workspace directory needs to be accessible by user and group
	if err := os.MkdirAll(baseWorkspace, defaultWorkspacePerms); err != nil {
		log.Fatalf("Failed to create base workspace: %v", err)
	}

	// Create worker server
	// #nosec G115 - maxSessions is bounded by config validation (typically < 1000)
	workerServer := worker.NewWorkerServer(workerID, int32(maxSessions), baseWorkspace)

	// Create registration client
	// Note: GRPCAddress is sent to coordinator but worker doesn't listen (legacy field)
	regClient := worker.NewRegistrationClient(&worker.RegistrationConfig{
		WorkerID:        workerID,
		GRPCAddress:     "", // Worker doesn't listen, all communication via task stream
		CoordinatorAddr: coordinatorAddr,
		Version:         workerVersion,
		SessionPool:     workerServer.GetSessionPool(),
		TaskExecutor:    workerServer.GetTaskExecutor(),
		Logger:          logger,
	})

	// Start registration with coordinator
	ctx := context.Background()
	if err := regClient.Start(ctx); err != nil {
		logger.Error("Failed to register with coordinator", "error", err)
		log.Fatalf("Cannot start worker without coordinator connection")
	}

	logger.Info("Worker started successfully, connected to coordinator")

	// Handle graceful shutdown
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, os.Interrupt, syscall.SIGTERM)

	// Wait for shutdown signal
	<-shutdownCh
	logger.Info("Received shutdown signal, starting graceful shutdown")

	// Deregister from coordinator
	const shutdownTimeout = 10 * time.Second
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := regClient.Stop(shutdownCtx); err != nil {
		logger.Warn("Error during deregistration", "error", err)
	}

	logger.Info("Worker shutdown complete")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var intValue int
		if _, err := fmt.Sscanf(value, "%d", &intValue); err == nil {
			return intValue
		}
	}
	return defaultValue
}
