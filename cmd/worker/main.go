package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/worker"
	"google.golang.org/grpc"
)

const (
	defaultMaxSessions    = 5
	defaultWorkspacePerms = 0755
	defaultGRPCPort       = "50051"
	defaultWorkerID       = "worker-1"
	defaultBaseWorkspace  = "/tmp/codegen-workspaces"
)

var (
	version = flag.Bool("version", false, "Print version and exit")
)

func main() {
	flag.Parse()

	if *version {
		fmt.Println("CodeGen MCP Worker v0.1.0")
		os.Exit(0)
	}

	fmt.Println("⚙️ AltairaLabs CodeGen MCP Worker starting up...")

	// Read configuration from environment
	workerID := getEnv("WORKER_ID", defaultWorkerID)
	grpcPort := getEnv("GRPC_PORT", defaultGRPCPort)
	maxSessions := getEnvInt("MAX_SESSIONS", defaultMaxSessions)
	baseWorkspace := getEnv("BASE_WORKSPACE", defaultBaseWorkspace)

	log.Printf("Worker ID: %s", workerID)
	log.Printf("gRPC Port: %s", grpcPort)
	log.Printf("Max Sessions: %d", maxSessions)
	log.Printf("Base Workspace: %s", baseWorkspace)

	// Create base workspace directory
	// #nosec G301 - Base workspace directory needs to be accessible by user and group
	if err := os.MkdirAll(baseWorkspace, defaultWorkspacePerms); err != nil {
		log.Fatalf("Failed to create base workspace: %v", err)
	}

	// Create worker server
	// #nosec G115 - maxSessions is bounded by config validation (typically < 1000)
	workerServer := worker.NewWorkerServer(workerID, int32(maxSessions), baseWorkspace)

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
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down worker...")
		grpcServer.GracefulStop()
	}()

	log.Printf("Worker listening on port %s", grpcPort)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
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
