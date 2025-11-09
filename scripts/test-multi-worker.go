package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	pb "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	coordinatorAddr = "localhost:50051"
	numSessions     = 6  // Create 6 sessions (3 per worker with max_sessions=5)
	numConcurrent   = 10 // Run 10 concurrent tasks
)

type TestResult struct {
	SessionID string
	WorkerID  string
	TaskNum   int
	Success   bool
	Duration  time.Duration
	Error     error
}

func main() {
	log.Println("🧪 Multi-Worker & Multi-Session Test")
	log.Println("====================================")

	// Connect to coordinator
	conn, err := grpc.Dial(coordinatorAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to coordinator: %v", err)
	}
	defer conn.Close()

	sessionClient := pb.NewSessionManagementClient(conn)
	taskClient := pb.NewTaskExecutionClient(conn)
	ctx := context.Background()

	// Phase 1: Create multiple sessions
	log.Printf("\n📋 Phase 1: Creating %d sessions...", numSessions)
	sessions := createMultipleSessions(ctx, sessionClient, numSessions)
	log.Printf("✅ Created %d sessions", len(sessions))

	// Print session distribution
	printSessionDistribution(sessions)

	// Phase 2: Test concurrent task execution
	log.Printf("\n📋 Phase 2: Testing %d concurrent tasks across sessions...", numConcurrent)
	results := executeConcurrentTasks(ctx, taskClient, sessions, numConcurrent)

	// Phase 3: Analyze results
	log.Println("\n📋 Phase 3: Analyzing results...")
	analyzeResults(results)

	// Phase 4: Test Python venv isolation
	log.Println("\n📋 Phase 4: Testing Python venv isolation...")
	testPythonIsolation(ctx, taskClient, sessions)

	log.Println("\n🎉 Multi-Worker Test Complete!")
}

func createMultipleSessions(ctx context.Context, client pb.SessionManagementClient, count int) []*pb.SessionInfo {
	var sessions []*pb.SessionInfo
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			req := &pb.CreateSessionRequest{
				ClientId: fmt.Sprintf("test-client-%d", idx),
				Config: &pb.SessionConfig{
					Language: "python",
				},
			}

			resp, err := client.CreateSession(ctx, req)
			if err != nil {
				log.Printf("❌ Failed to create session %d: %v", idx, err)
				return
			}

			mu.Lock()
			sessions = append(sessions, resp.Session)
			mu.Unlock()

			log.Printf("  ✓ Session %d: %s (Worker: %s)", idx, resp.Session.SessionId, resp.Session.WorkerId)
		}(i)
	}

	wg.Wait()
	return sessions
}

func printSessionDistribution(sessions []*pb.SessionInfo) {
	distribution := make(map[string]int)
	for _, s := range sessions {
		distribution[s.WorkerId]++
	}

	log.Println("\n📊 Session Distribution:")
	for workerID, count := range distribution {
		log.Printf("  Worker %s: %d sessions", workerID, count)
	}
}

func executeConcurrentTasks(ctx context.Context, client pb.TaskExecutionClient, sessions []*pb.SessionInfo, count int) []TestResult {
	var results []TestResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(taskNum int) {
			defer wg.Done()

			// Round-robin across sessions
			session := sessions[taskNum%len(sessions)]

			start := time.Now()
			result := TestResult{
				SessionID: session.SessionId,
				WorkerID:  session.WorkerId,
				TaskNum:   taskNum,
			}

			// Execute a simple file write + read task
			req := &pb.ExecuteTaskRequest{
				SessionId: session.SessionId,
				Tool:      "fs.write",
				Arguments: map[string]string{
					"path":    fmt.Sprintf("test-file-%d.txt", taskNum),
					"content": fmt.Sprintf("Task %d executed at %v", taskNum, time.Now()),
				},
			}

			stream, err := client.ExecuteTask(ctx, req)
			if err != nil {
				result.Error = err
				result.Duration = time.Since(start)
				mu.Lock()
				results = append(results, result)
				mu.Unlock()
				return
			}

			// Process stream
			for {
				resp, err := stream.Recv()
				if err == io.EOF {
					break
				}
				if err != nil {
					result.Error = err
					break
				}
				if resp.Status == pb.TaskStatus_TASK_COMPLETED {
					result.Success = true
					break
				}
				if resp.Status == pb.TaskStatus_TASK_FAILED {
					result.Error = fmt.Errorf("task failed: %s", resp.Error)
					break
				}
			}

			result.Duration = time.Since(start)
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(i)
	}

	wg.Wait()
	return results
}

func analyzeResults(results []TestResult) {
	successful := 0
	failed := 0
	totalDuration := time.Duration(0)
	workerTasks := make(map[string]int)

	for _, r := range results {
		if r.Success {
			successful++
		} else {
			failed++
			log.Printf("  ❌ Task %d failed: %v", r.TaskNum, r.Error)
		}
		totalDuration += r.Duration
		workerTasks[r.WorkerID]++
	}

	avgDuration := totalDuration / time.Duration(len(results))

	log.Printf("\n📈 Results Summary:")
	log.Printf("  Total Tasks: %d", len(results))
	log.Printf("  Successful: %d", successful)
	log.Printf("  Failed: %d", failed)
	log.Printf("  Success Rate: %.1f%%", float64(successful)/float64(len(results))*100)
	log.Printf("  Avg Duration: %v", avgDuration)

	log.Println("\n📊 Task Distribution Across Workers:")
	for workerID, count := range workerTasks {
		log.Printf("  Worker %s: %d tasks", workerID, count)
	}
}

func testPythonIsolation(ctx context.Context, client pb.TaskExecutionClient, sessions []*pb.SessionInfo) {
	if len(sessions) < 2 {
		log.Println("  ⚠️  Need at least 2 sessions to test isolation")
		return
	}

	// Install different packages in different sessions
	testCases := []struct {
		session  *pb.SessionInfo
		package_ string
	}{
		{sessions[0], "requests"},
		{sessions[1], "flask"},
	}

	log.Println("\n  Installing packages in different sessions...")
	for _, tc := range testCases {
		req := &pb.ExecuteTaskRequest{
			SessionId: tc.session.SessionId,
			Tool:      "pkg.install",
			Arguments: map[string]string{
				"packages": tc.package_,
			},
		}

		stream, err := client.ExecuteTask(ctx, req)
		if err != nil {
			log.Printf("  ❌ Failed to install %s in session %s: %v", tc.package_, tc.session.SessionId, err)
			continue
		}

		// Wait for completion
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("  ❌ Stream error: %v", err)
				break
			}
			if resp.Status == pb.TaskStatus_TASK_COMPLETED {
				log.Printf("  ✓ Installed %s in session %s (Worker: %s)", tc.package_, tc.session.SessionId, tc.session.WorkerId)
				break
			}
			if resp.Status == pb.TaskStatus_TASK_FAILED {
				log.Printf("  ❌ Failed to install %s: %s", tc.package_, resp.Error)
				break
			}
		}
	}

	// Verify isolation - try to import packages in opposite sessions
	log.Println("\n  Verifying package isolation...")
	verifyTests := []struct {
		session     *pb.SessionInfo
		package_    string
		shouldExist bool
	}{
		{sessions[0], "requests", true},  // Should exist
		{sessions[0], "flask", false},    // Should NOT exist
		{sessions[1], "flask", true},     // Should exist
		{sessions[1], "requests", false}, // Should NOT exist
	}

	for _, vt := range verifyTests {
		code := fmt.Sprintf("import %s; print('%s available')", vt.package_, vt.package_)
		req := &pb.ExecuteTaskRequest{
			SessionId: vt.session.SessionId,
			Tool:      "run.python",
			Arguments: map[string]string{
				"code": code,
			},
		}

		stream, err := client.ExecuteTask(ctx, req)
		if err != nil {
			log.Printf("  ⚠️  Error checking %s in session %s: %v", vt.package_, vt.session.SessionId, err)
			continue
		}

		succeeded := false
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
			if resp.Status == pb.TaskStatus_TASK_COMPLETED {
				succeeded = true
				break
			}
			if resp.Status == pb.TaskStatus_TASK_FAILED {
				break
			}
		}

		if succeeded == vt.shouldExist {
			log.Printf("  ✅ Isolation verified: %s %s in session %s",
				vt.package_,
				map[bool]string{true: "available", false: "not available"}[vt.shouldExist],
				vt.session.SessionId)
		} else {
			log.Printf("  ❌ Isolation FAILED: %s should be %s in session %s",
				vt.package_,
				map[bool]string{true: "available", false: "not available"}[vt.shouldExist],
				vt.session.SessionId)
		}
	}
}
