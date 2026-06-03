package monitor

import (
	"context"
	"fmt"
	"os"
	"time"

	bashTool "github.com/EngineerProjects/nexus-engine/internal/tools/bash"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
	"github.com/EngineerProjects/nexus-engine/pkg/runtimepath"
)

// ToolName is the name of the tool
const ToolName = "monitor"

// SearchHint is the search hint for the tool
const SearchHint = "stream shell output as notifications"

const Description = `
Execute a shell command in the background and stream its stdout line-by-line as notifications.
Each polling interval (~1s), new output lines are delivered to you.

Use this for:
- Monitoring logs in real-time
- Watching build output
- Observing long-running processes

For one-shot "wait until done" commands, prefer Bash tool with run_in_background instead.

This tool is similar to Bash with run_in_background, but specifically designed for
continuous monitoring with streaming notifications.
`

// MonitorTimeout is the timeout for monitor commands
const MonitorTimeout = 30 * time.Minute

// Tool implements the Monitor tool for streaming shell command output
type Tool struct {
	// workingDir is the current working directory
	workingDir string
	// taskManager manages background tasks
	taskManager *bashTool.BackgroundTaskManager
}

// NewMonitorTool creates a new monitor tool
func NewMonitorTool(workingDir string) *Tool {
	return &Tool{
		workingDir:  workingDir,
		taskManager: bashTool.NewBackgroundTaskManager(""),
	}
}

// Definition returns the tool definition
func (t *Tool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolName,
		DisplayName: "Monitor",
		SearchHint:  SearchHint,
		Description: Description,
		Category:    "process",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The shell command to run and monitor",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Clear, concise description of what this command does in active voice",
				},
			},
			"required": []string{"command"},
		}),
		Metadata: map[string]any{"surface_profiles": []string{"mono_run"}},
	}
}

// Call executes the monitor tool
func (t *Tool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	parsed := input.Parsed
	if parsed == nil {
		parsed = make(map[string]any)
	}

	command, ok := parsed["command"].(string)
	if !ok || command == "" {
		return tool.CallResult{
			Error: fmt.Errorf("command is required"),
		}, nil
	}

	// Check permissions - Monitor uses Bash permissions since it runs shell commands
	if permissionCheck != nil {
		result := permissionCheck(ctx, types.ToolPermissionRequest{
			ToolName:  "bash",
			ToolInput: map[string]any{"command": command},
		})

		if result.Behavior != types.PermissionBehaviorAllow {
			return tool.CallResult{
				Error: fmt.Errorf("permission denied: %s", result.Reason),
			}, nil
		}
	}

	// Start background task
	task, err := t.taskManager.StartBackgroundTask(
		ctx,
		command,
		t.workingDir,
		os.Environ(),
		"bash",
	)
	if err != nil {
		return tool.CallResult{
			Error: fmt.Errorf("failed to start monitor: %w", err),
		}, nil
	}

	// Prepare output
	output := map[string]any{
		"taskId":     task.ID,
		"outputFile": task.Output.Path,
		"command":    command,
		"status":     "running",
	}

	return tool.CallResult{
		Data: output,
	}, nil
}

// Description returns a human-readable description
func (t *Tool) Description(ctx context.Context) (string, error) {
	return Description, nil
}

// ValidateInput validates the input
func (t *Tool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}

// CheckPermissions checks permissions
func (t *Tool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	command, ok := input["command"].(string)
	if !ok || command == "" {
		return types.Deny("command is required")
	}

	// Monitor delegates to Bash permissions since it runs shell commands
	return types.PermissionResult{Behavior: types.PermissionBehaviorAllow}
}

// IsConcurrencySafe returns whether the tool is concurrency safe
func (t *Tool) IsConcurrencySafe(input map[string]any) bool {
	return true
}

// IsReadOnly returns whether the tool is read-only
func (t *Tool) IsReadOnly(input map[string]any) bool {
	return false
}

// IsEnabled returns whether the tool is enabled
func (t *Tool) IsEnabled() bool {
	return true
}

// FormatResult formats the result
func (t *Tool) FormatResult(data any) string {
	if data == nil {
		return "Monitor task started"
	}

	if m, ok := data.(map[string]any); ok {
		taskID, _ := m["taskId"].(string)
		outputFile, _ := m["outputFile"].(string)
		return fmt.Sprintf("Monitor task started with ID: %s. Output is being streamed to: %s", taskID, outputFile)
	}

	return fmt.Sprintf("%v", data)
}

// BackfillInput backfills input
func (t *Tool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return input
}

// GetTaskOutput reads output from a monitor task
func GetTaskOutput(taskID string) (string, error) {
	reader, err := bashTool.NewTaskOutputReader(taskID)
	if err != nil {
		return "", err
	}
	return reader.ReadOutput()
}

// GetTaskStatus gets the status of a monitor task
func GetTaskStatus(taskID string) (string, error) {
	task := bashTool.GlobalTaskManager().GetTask(taskID)
	if task == nil {
		return "", fmt.Errorf("task not found: %s", taskID)
	}

	switch task.GetStatus() {
	case bashTool.TaskStatusRunning:
		return "running", nil
	case bashTool.TaskStatusCompleted:
		return "completed", nil
	case bashTool.TaskStatusKilled:
		return "killed", nil
	case bashTool.TaskStatusTimeout:
		return "timeout", nil
	default:
		return "unknown", nil
	}
}

// KillTask kills a monitor task
func KillTask(taskID string) error {
	return bashTool.GlobalTaskManager().KillTask(taskID)
}

// DefaultTaskDir returns the default task output directory
func DefaultTaskDir() string {
	return runtimepath.BashTasksDir("")
}
