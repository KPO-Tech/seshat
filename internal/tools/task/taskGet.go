package task

import (
	"context"
	"fmt"
	"time"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// TaskDetails represents detailed information about a background task
type TaskDetails struct {
	ID        string `json:"id"`
	Command   string `json:"command"`
	Status    string `json:"status"`
	StartTime int64  `json:"startTime"`
	EndTime   *int64 `json:"endTime,omitempty"`
	ExitCode  *int   `json:"exitCode,omitempty"`
}

// TaskGetTool implements the TaskGet tool for getting background task details
type TaskGetTool struct{}

// NewTaskGetTool creates a new TaskGet tool
func NewTaskGetTool() *TaskGetTool {
	return &TaskGetTool{}
}

// Definition returns the tool definition
func (t *TaskGetTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameTaskGet,
		DisplayName: "TaskGet",
		SearchHint:  SearchHintTaskGet,
		Description: ToolDescriptionTaskGet,
		Category:    "process",
		Metadata:    taskSurfaceMetadata(),
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id": map[string]any{
					"type":        "string",
					"description": "The ID of the background task to retrieve",
				},
			},
			"required": []string{"task_id"},
		}),
	}
}

// Call executes the TaskGet tool
func (t *TaskGetTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	parsed := input.Parsed
	if parsed == nil {
		parsed = make(map[string]any)
	}

	taskID, ok := parsed["task_id"].(string)
	if !ok || taskID == "" {
		return tool.CallResult{
			Error: fmt.Errorf("task_id is required"),
		}, nil
	}

	runtime, err := requireRuntime()
	if err != nil {
		return tool.CallResult{Data: map[string]any{"task": nil}}, nil
	}
	task, err := runtime.GetTask(ctx, taskID)
	if err != nil {
		return tool.CallResult{Data: map[string]any{"task": nil}}, nil
	}

	taskDetails := TaskDetails{
		ID:        task.ID,
		Command:   task.Command,
		Status:    string(task.Status),
		StartTime: task.CreatedAt.Unix(),
	}
	if task.StartedAt != nil {
		taskDetails.StartTime = task.StartedAt.Unix()
	}
	if task.CompletedAt != nil {
		endTimeUnix := task.CompletedAt.Unix()
		taskDetails.EndTime = &endTimeUnix
	}
	if task.ExitCode != nil {
		taskDetails.ExitCode = task.ExitCode
	}

	output := map[string]any{
		"task": taskDetails,
	}

	return tool.CallResult{
		Data: output,
	}, nil
}

// Description returns a human-readable description
func (t *TaskGetTool) Description(ctx context.Context) (string, error) {
	return ToolDescriptionTaskGet, nil
}

// ValidateInput validates the input
func (t *TaskGetTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}

// CheckPermissions checks permissions (always allowed)
func (t *TaskGetTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	taskID, ok := input["task_id"].(string)
	if !ok || taskID == "" {
		return types.Deny("task_id is required")
	}
	return types.PermissionResult{Behavior: types.PermissionBehaviorAllow}
}

// IsConcurrencySafe returns whether the tool is concurrency safe
func (t *TaskGetTool) IsConcurrencySafe(input map[string]any) bool {
	return true
}

// IsReadOnly returns whether the tool is read-only
func (t *TaskGetTool) IsReadOnly(input map[string]any) bool {
	return true
}

// IsEnabled returns whether the tool is enabled
func (t *TaskGetTool) IsEnabled() bool {
	return true
}

// FormatResult formats the result
func (t *TaskGetTool) FormatResult(data any) string {
	if data == nil {
		return "Task not found"
	}

	if m, ok := data.(map[string]any); ok {
		task, hasTask := m["task"].(map[string]any)
		if !hasTask || task == nil {
			return "Task not found"
		}

		id, _ := task["id"].(string)
		cmd, _ := task["command"].(string)
		status, _ := task["status"].(string)
		startTime, _ := task["startTime"].(int64)

		lines := []string{
			fmt.Sprintf("Task: %s", id),
			fmt.Sprintf("Status: %s", status),
			fmt.Sprintf("Command: %s", cmd),
			fmt.Sprintf("Started: %s", time.Unix(startTime, 0).Format(time.RFC3339)),
		}

		// Add end time if completed
		if endTime, ok := task["endTime"].(int64); ok && endTime > 0 {
			lines = append(lines, fmt.Sprintf("Ended: %s", time.Unix(endTime, 0).Format(time.RFC3339)))
		}

		// Add exit code if available
		if exitCode, ok := task["exitCode"].(int); ok {
			lines = append(lines, fmt.Sprintf("Exit Code: %d", exitCode))
		}

		return JoinLines(lines)
	}

	return fmt.Sprintf("%v", data)
}

// BackfillInput backfills input
func (t *TaskGetTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return input
}
