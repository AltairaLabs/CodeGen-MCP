package config

// Tool defines the available tools in the coordinator
const (
	// ToolEcho is the echo tool name
	ToolEcho = "echo"
	// ToolFsRead is the filesystem read tool name
	ToolFsRead = "fs.read"
	// ToolFsWrite is the filesystem write tool name
	ToolFsWrite = "fs.write"
	// ToolFsList is the filesystem list tool name
	ToolFsList = "fs.list"
	// ToolRunPython is the python execution tool name
	ToolRunPython = "run.python"
	// ToolPkgInstall is the package installation tool name
	ToolPkgInstall = "pkg.install"
	// ToolGetTaskResult is the task result retrieval tool name
	ToolGetTaskResult = "task.get_result"
	// ToolGetTaskStatus is the task status retrieval tool name
	ToolGetTaskStatus = "task.get_status"
)

// AllTools returns a slice of all available tool names
func AllTools() []string {
	return []string{
		ToolEcho,
		ToolFsRead,
		ToolFsWrite,
		ToolFsList,
		ToolRunPython,
		ToolPkgInstall,
		ToolGetTaskResult,
		ToolGetTaskStatus,
	}
}
