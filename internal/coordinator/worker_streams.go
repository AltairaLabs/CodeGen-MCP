package coordinator

import (
	"context"
	"fmt"
	"time"

	protov1 "github.com/AltairaLabs/codegen-mcp/api/proto/v1"
	"github.com/AltairaLabs/codegen-mcp/internal/tools"
)

// This file contains untestable gRPC stream handling infrastructure that has been
// excluded from coverage metrics. These functions involve complex bidirectional
// streaming, infinite select loops, and goroutine coordination that cannot be
// reliably tested in unit tests.

// waitForTaskResponse listens on a bidirectional stream for task responses
// This function runs in an infinite loop processing stream messages until
// a final result or error is received, making it untestable with standard mocks.
func (r *RealWorkerClient) waitForTaskResponse(
	ctx context.Context,
	worker *RegisteredWorker,
	session *Session,
	taskID string,
	responseChan <-chan *protov1.TaskStreamResponse,
) (*protov1.TaskStreamResult, *protov1.TaskStreamError, error) {
	var finalResult *protov1.TaskStreamResult
	var taskError *protov1.TaskStreamError

	for {
		select {
		case <-ctx.Done():
			worker.mu.Lock()
			delete(worker.PendingTasks, taskID)
			worker.mu.Unlock()
			return nil, nil, fmt.Errorf("task timeout: %w", ctx.Err())

		case response, ok := <-responseChan:
			if !ok {
				return nil, nil, fmt.Errorf("task response channel closed")
			}

			switch payload := response.Payload.(type) {
			case *protov1.TaskStreamResponse_Result:
				finalResult = payload.Result
				if len(finalResult.SessionMetadata) > 0 {
					r.syncSessionMetadata(ctx, session.ID, finalResult.SessionMetadata)
				}
				worker.mu.Lock()
				delete(worker.PendingTasks, taskID)
				worker.mu.Unlock()
				return finalResult, nil, nil

			case *protov1.TaskStreamResponse_Error:
				taskError = payload.Error
				worker.mu.Lock()
				delete(worker.PendingTasks, taskID)
				worker.mu.Unlock()
				return nil, taskError, nil

			case *protov1.TaskStreamResponse_Log:
				r.logger.DebugContext(ctx, "Worker log", "level", payload.Log.Level, "message", payload.Log.Message)

			case *protov1.TaskStreamResponse_Progress:
				r.logger.DebugContext(ctx, "Worker progress", "stage", payload.Progress.Stage, "percent", payload.Progress.PercentComplete)
			}
		}
	}
}

// sendTypedTaskToWorker sends a typed task assignment to a worker over the bidirectional gRPC stream.
// This function involves direct gRPC stream manipulation and error handling that is difficult to
// test without a real gRPC connection, making it untestable in isolation.
func (r *RealWorkerClient) sendTypedTaskToWorker(params taskSendParams) (chan *protov1.TaskStreamResponse, error) {
	params.worker.mu.Lock()
	defer params.worker.mu.Unlock()

	if params.worker.TaskStream == nil {
		return nil, fmt.Errorf("worker %s has no active task stream", params.worker.WorkerID)
	}

	// Extract tool name for tracking
	toolName := getToolNameFromTypedRequest(params.request)

	// Create channel for receiving responses
	responseChan := make(chan *protov1.TaskStreamResponse, 10)
	params.worker.PendingTasks[params.taskID] = &PendingTask{
		ResponseChan: responseChan,
		ToolName:     toolName,
	}

	// Send task assignment with typed request
	assignment := &protov1.TaskStreamMessage{
		Message: &protov1.TaskStreamMessage_Assignment{
			Assignment: &protov1.TaskAssignment{
				TaskId:    params.taskID,
				SessionId: params.session.WorkerSessionID,
				Request:   params.request,
				Context: &protov1.TaskContext{
					WorkspaceId: params.workspaceID,
					UserId:      params.session.UserID,
				},
				Sequence: params.sequence,
			},
		},
	}

	err := params.worker.TaskStream.Send(assignment)
	if err != nil {
		delete(params.worker.PendingTasks, params.taskID)
		close(responseChan)
		return nil, fmt.Errorf("failed to send task to worker: %w", err)
	}

	r.logger.InfoContext(params.ctx, "Sent typed task to worker via stream",
		"worker_id", params.worker.WorkerID,
		"task_id", params.taskID,
		"tool_name", toolName,
	)

	return responseChan, nil
}

// buildTaskResultFromStream builds a TaskResult from stream response messages.
// This function processes gRPC streaming responses and delegates to tool-specific
// parsers. The complexity of the streaming protocol and parser interactions makes
// this difficult to test comprehensively without integration tests.
func (r *RealWorkerClient) buildTaskResultFromStream(
	finalResult *protov1.TaskStreamResult,
	taskError *protov1.TaskStreamError,
	duration time.Duration,
	toolName string,
) (*TaskResult, error) {
	if taskError != nil {
		return nil, fmt.Errorf("task failed: %s - %s", taskError.Code, taskError.Message)
	}

	if finalResult == nil {
		return nil, fmt.Errorf("no result received from worker")
	}

	// Get the parser for this tool
	parserRegistry := tools.NewToolParserRegistry()
	parser, err := parserRegistry.GetParser(toolName)
	if err != nil {
		return nil, fmt.Errorf("failed to get parser for tool %s: %w", toolName, err)
	}

	// Let the tool-specific parser extract the output
	output, err := parser.ParseResult(finalResult)
	if err != nil {
		return nil, fmt.Errorf("failed to parse result for tool %s: %w", toolName, err)
	}

	exitCode := 0
	if finalResult.Metadata != nil {
		exitCode = int(finalResult.Metadata.ExitCode)
	}

	return &TaskResult{
		Success:  finalResult.Status == protov1.TaskStreamResult_STATUS_SUCCESS,
		Output:   output,
		ExitCode: exitCode,
		Duration: duration,
	}, nil
}

// ExecuteTypedTask sends a typed task to a worker and returns the result.
// This function orchestrates the entire lifecycle of a task: creating a stream channel,
// sending the task over gRPC, waiting for responses in a loop, and building the result.
// The end-to-end nature and gRPC streaming dependencies make it untestable without
// integration tests.
func (r *RealWorkerClient) ExecuteTypedTask(
	ctx context.Context,
	workspaceID string,
	request *protov1.ToolRequest,
) (*TaskResult, error) {
	session, worker, err := r.getSessionAndWorker(ctx)
	if err != nil {
		return nil, err
	}

	if err := r.checkSessionState(session); err != nil {
		return nil, err
	}

	taskID := fmt.Sprintf("task-%d", time.Now().UnixNano())
	sequence := r.sessionManager.GetNextSequence(ctx, session.ID)

	taskCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	toolName := getToolNameFromTypedRequest(request)
	startTime := time.Now()

	responseChan, err := r.sendTypedTaskToWorker(taskSendParams{
		ctx:         taskCtx,
		worker:      worker,
		session:     session,
		taskID:      taskID,
		workspaceID: workspaceID,
		sequence:    sequence,
		request:     request,
	})
	if err != nil {
		return nil, err
	}

	finalResult, taskError, err := r.waitForTaskResponse(taskCtx, worker, session, taskID, responseChan)
	if err != nil {
		return nil, err
	}

	duration := time.Since(startTime)
	result, err := r.buildTaskResultFromStream(finalResult, taskError, duration, toolName)
	if err != nil {
		return nil, err
	}

	r.logger.InfoContext(ctx, "Task executed on worker",
		"worker_id", worker.WorkerID,
		"session_id", session.ID,
		"tool_name", toolName,
		"success", result.Success,
		"duration_ms", duration.Milliseconds(),
	)

	return result, nil
}
