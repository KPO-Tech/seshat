package bash

import (
	"context"
	"fmt"
	"os"
	"time"

	tool "github.com/KPO-Tech/seshat/internal/tools/registry"
	"github.com/KPO-Tech/seshat/internal/tools/schema"
	"github.com/KPO-Tech/seshat/internal/types"
	"github.com/KPO-Tech/seshat/pkg/runtimepath"
)

const monitorToolName = "monitor"
const monitorSearchHint = "stream shell output as notifications"

const monitorDescription = `
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

const MonitorTimeout = 30 * time.Minute

// MonitorTool streams a shell command's stdout as live notifications.
type MonitorTool struct {
	workingDir  string
	taskManager *BackgroundTaskManager
}

// NewMonitorTool creates the monitor tool.
func NewMonitorTool(workingDir string) *MonitorTool {
	return &MonitorTool{
		workingDir:  workingDir,
		taskManager: NewBackgroundTaskManager(""),
	}
}

func (t *MonitorTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        monitorToolName,
		DisplayName: "Monitor",
		SearchHint:  monitorSearchHint,
		Description: monitorDescription,
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

func (t *MonitorTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	parsed := input.Parsed
	if parsed == nil {
		parsed = make(map[string]any)
	}
	command, ok := parsed["command"].(string)
	if !ok || command == "" {
		return tool.CallResult{Error: fmt.Errorf("command is required")}, nil
	}
	if permissionCheck != nil {
		result := permissionCheck(ctx, types.ToolPermissionRequest{
			ToolName:  "bash",
			ToolInput: map[string]any{"command": command},
		})
		if result.Behavior != types.PermissionBehaviorAllow {
			return tool.CallResult{Error: fmt.Errorf("permission denied: %s", result.Reason)}, nil
		}
	}
	task, err := t.taskManager.StartBackgroundTask(ctx, command, t.workingDir, os.Environ(), "bash")
	if err != nil {
		return tool.CallResult{Error: fmt.Errorf("failed to start monitor: %w", err)}, nil
	}
	output := map[string]any{
		"taskId":     task.ID,
		"outputFile": task.Output.Path,
		"command":    command,
		"status":     "running",
	}
	return tool.CallResult{Data: output}, nil
}

func (t *MonitorTool) Description(_ context.Context) (string, error) { return monitorDescription, nil }
func (t *MonitorTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}
func (t *MonitorTool) CheckPermissions(_ context.Context, input map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	if cmd, ok := input["command"].(string); !ok || cmd == "" {
		return types.Deny("command is required")
	}
	return types.PermissionResult{Behavior: types.PermissionBehaviorAllow}
}
func (t *MonitorTool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *MonitorTool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *MonitorTool) IsEnabled() bool                         { return true }
func (t *MonitorTool) FormatResult(data any) string {
	if m, ok := data.(map[string]any); ok {
		taskID, _ := m["taskId"].(string)
		outputFile, _ := m["outputFile"].(string)
		return fmt.Sprintf("Monitor task started with ID: %s. Output is being streamed to: %s", taskID, outputFile)
	}
	return fmt.Sprintf("%v", data)
}
func (t *MonitorTool) BackfillInput(_ context.Context, input map[string]any) map[string]any {
	return input
}

// GetTaskOutput reads output from a monitor task.
func GetTaskOutput(taskID string) (string, error) {
	reader, err := NewTaskOutputReader(taskID)
	if err != nil {
		return "", err
	}
	return reader.ReadOutput()
}

// GetTaskStatus returns the current status string for a monitor task.
func GetTaskStatus(taskID string) (string, error) {
	task := GlobalTaskManager().GetTask(taskID)
	if task == nil {
		return "", fmt.Errorf("task not found: %s", taskID)
	}
	switch task.GetStatus() {
	case TaskStatusRunning:
		return "running", nil
	case TaskStatusCompleted:
		return "completed", nil
	case TaskStatusKilled:
		return "killed", nil
	case TaskStatusTimeout:
		return "timeout", nil
	default:
		return "unknown", nil
	}
}

// KillTask kills a monitor task by ID.
func KillTask(taskID string) error {
	return GlobalTaskManager().KillTask(taskID)
}

// DefaultTaskDir returns the default task output directory.
func DefaultTaskDir() string {
	return runtimepath.BashTasksDir("")
}
