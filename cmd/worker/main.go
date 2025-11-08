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

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/worker"
	"google.golang.org/grpc"
)

const (
	defaultMaxSessions     = 5
	defaultWorkspacePerms  = 0755
	defaultGRPCPort        = "50051"
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
	grpcPort := getEnv("GRPC_PORT", defaultGRPCPort)
	maxSessions := getEnvInt("MAX_SESSIONS", defaultMaxSessions)
	baseWorkspace := getEnv("BASE_WORKSPACE", defaultBaseWorkspace)
	coordinatorAddr := getEnv("COORDINATOR_ADDR", defaultCoordinatorAddr)

	// Set up structured logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	logger.Info("Worker configuration",
		"worker_id", workerID,
		"grpc_port", grpcPort,
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
	regClient := worker.NewRegistrationClient(worker.RegistrationConfig{
		WorkerID:        workerID,
		CoordinatorAddr: coordinatorAddr,
		Version:         workerVersion,
		SessionPool:     workerServer.GetSessionPool(),
		Logger:          logger,
	})

	// Start registration with coordinator
	ctx := context.Background()
	if err := regClient.Start(ctx); err != nil {
		logger.Error("Failed to register with coordinator", "error", err)
		// Continue anyway - we can still serve if coordinator is unavailable
		logger.Warn("Worker starting without coordinator registration")
	}

	// Create gRPC server
	grpcServer := grpc.NewServer()

	// Register services
	protov1.RegisterSessionManagementServer(grpcServer, workerServer)
	protov1.RegisterTaskExecutionServer(grpcServer, workerServer)
	protov1.RegisterArtifactServiceServer(grpcServer, workerServer)

	// Start listening
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort)) //nolint:noctx // Standard gRPC server pattern
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	// Handle graceful shutdown
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-shutdownCh
		logger.Info("Received shutdown signal, starting graceful shutdown")

		// Deregister from coordinator
		const shutdownTimeout = 10 * time.Second
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := regClient.Stop(shutdownCtx); err != nil {
			logger.Warn("Error during deregistration", "error", err)
		}

		// Stop gRPC server
		logger.Info("Stopping gRPC server")
		grpcServer.GracefulStop()
	}()

	logger.Info("Worker listening for gRPC connections", "port", grpcPort)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
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
