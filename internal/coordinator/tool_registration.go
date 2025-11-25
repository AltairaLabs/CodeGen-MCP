package coordinator

import (
	"context"
	"fmt"

	"github.com/AltairaLabs/codegen-mcp/internal/coordinator/config"
	"github.com/mark3labs/mcp-go/mcp"
)

// Legacy constants for backward compatibility
// These should eventually be replaced with direct config package usage
const (
	// Tool names - use config.Tool* constants instead
	toolEcho          = config.ToolEcho
	toolFsRead        = config.ToolFsRead
	toolFsWrite       = config.ToolFsWrite
	toolFsList        = config.ToolFsList
	toolRunPython     = config.ToolRunPython
	toolPkgInstall    = config.ToolPkgInstall
	toolGetTaskResult = config.ToolGetTaskResult
	toolGetTaskStatus = config.ToolGetTaskStatus
)

// registerTools registers all MCP tools with handlers via the tool registry
func (ms *MCPServer) registerTools() {
	// helper to register tool using registry
	add := func(tool mcp.Tool, name string) {
		if ms.toolRegistry != nil {
			if h, err := ms.toolRegistry.GetHandler(name); err == nil {
				// wrap handler func to the expected signature
				ms.server.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
					return h(ctx, req)
				})
				return
			}
		}
		// If no registry handler found, panic as all tools should be in registry
		panic(fmt.Sprintf("Tool %s not found in registry", name))
	}

	// Echo tool - simple test tool
	echoTool := mcp.NewTool(toolEcho,
		mcp.WithDescription("Echo a message back (test tool)"),
		mcp.WithString("message",
			mcp.Required(),
			mcp.Description("Message to echo"),
		),
	)
	add(echoTool, toolEcho)

	// fs.read tool - read file from workspace
	fsReadTool := mcp.NewTool(toolFsRead,
		mcp.WithDescription("Read a file from the workspace"),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to file to read"),
		),
	)
	add(fsReadTool, toolFsRead)

	// fs.write tool - write file to workspace
	fsWriteTool := mcp.NewTool(toolFsWrite,
		mcp.WithDescription("Write a file to the workspace"),
		mcp.WithString("path",
			mcp.Required(),
			mcp.Description("Path to file to write"),
		),
		mcp.WithString("content",
			mcp.Required(),
			mcp.Description("Content to write to file"),
		),
	)
	add(fsWriteTool, toolFsWrite)

	// fs.list tool - list files in workspace
	fsListTool := mcp.NewTool(toolFsList,
		mcp.WithDescription("List files and directories in the workspace"),
		mcp.WithString("path",
			mcp.Description("Path to list (defaults to workspace root)"),
		),
	)
	add(fsListTool, toolFsList)

	// run.python tool - execute Python code
	runPythonTool := mcp.NewTool(toolRunPython,
		mcp.WithDescription("Execute Python code or run a Python file"),
		mcp.WithString("code",
			mcp.Description("Python code to execute"),
		),
		mcp.WithString("file",
			mcp.Description("Path to Python file to run"),
		),
	)
	add(runPythonTool, toolRunPython)

	// pkg.install tool - install Python packages
	pkgInstallTool := mcp.NewTool(toolPkgInstall,
		mcp.WithDescription("Install Python packages"),
		mcp.WithString("packages",
			mcp.Required(),
			mcp.Description("Space-separated list of packages to install"),
		),
	)
	add(pkgInstallTool, toolPkgInstall)

	// get.task.result tool - get result of async task
	getTaskResultTool := mcp.NewTool(toolGetTaskResult,
		mcp.WithDescription("Get the result of a task"),
		mcp.WithString("task_id",
			mcp.Required(),
			mcp.Description("Task ID to get result for"),
		),
	)
	add(getTaskResultTool, toolGetTaskResult)

	// get.task.status tool - get status of async task
	getTaskStatusTool := mcp.NewTool(toolGetTaskStatus,
		mcp.WithDescription("Get the status of a task"),
		mcp.WithString("task_id",
			mcp.Required(),
			mcp.Description("Task ID to get status for"),
		),
	)
	add(getTaskStatusTool, toolGetTaskStatus)

	// artifact.get tool - retrieve artifact
	artifactGetTool := mcp.NewTool(config.ToolArtifactGet,
		mcp.WithDescription("Retrieve a generated artifact by ID"),
		mcp.WithString("artifact_id",
			mcp.Required(),
			mcp.Description("Artifact identifier to retrieve"),
		),
	)
	add(artifactGetTool, config.ToolArtifactGet)
}
