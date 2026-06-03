package task

import (
	"context"
	"fmt"

	bashTool "github.com/EngineerProjects/nexus-engine/internal/tools/bash"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// TaskInfo represents information about a background task
type TaskInfo struct {
	ID        string `json:"id"`
	Command   string `json:"command"`
	Status    string `json:"status"`
	StartTime int64  `json:"startTime"`
	EndTime   *int64 `json:"endTime,omitempty"`
	ExitCode  int    `json:"exitCode,omitempty"`
}

// TaskListTool implements the TaskList tool for listing background tasks
type TaskListTool struct{}

// NewTaskListTool creates a new TaskList tool
func NewTaskListTool() *TaskListTool {
	return &TaskListTool{}
}

// Definition returns the tool definition
func (t *TaskListTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameTaskList,
		DisplayName: "TaskList",
		SearchHint:  SearchHintTaskList,
		Description: ToolDescriptionTaskList,
		Category:    "process",
		Metadata:    taskSurfaceMetadata(),
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status": map[string]any{
					"type":        "string",
					"enum":        []string{"all", "running", "completed"},
					"description": "Filter tasks by status: 'all', 'running', or 'completed'. Default is 'running'.",
				},
				"listType": map[string]any{
					"type":        "string",
					"enum":        []string{"background", "todo", "all"},
					"description": "Type of tasks to list: 'background' (running processes), 'todo' (task list), or 'all'. Default is 'background'.",
				},
			},
		}),
	}
}

// Call executes the TaskList tool
func (t *TaskListTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	parsed := input.Parsed
	if parsed == nil {
		parsed = make(map[string]any)
	}

	// Get status filter (default: running)
	statusFilter := "running"
	if s, ok := parsed["status"].(string); ok {
		statusFilter = s
	}

	// Get list type - "background" (default), "todo", or "all"
	listType := "background"
	if lt, ok := parsed["listType"].(string); ok {
		listType = lt
	}

	result := make([]map[string]any, 0)

	// List background tasks
	if listType == "background" || listType == "all" {
		tasks := bashTool.GlobalTaskManager().ListTasks()

		for _, task := range tasks {
			status := task.GetStatus()
			taskStatus := ""

			switch status {
			case bashTool.TaskStatusRunning:
				taskStatus = "running"
			case bashTool.TaskStatusBackgrounded:
				taskStatus = "backgrounded"
			case bashTool.TaskStatusCompleted:
				taskStatus = "completed"
			case bashTool.TaskStatusKilled:
				taskStatus = "killed"
			case bashTool.TaskStatusTimeout:
				taskStatus = "timeout"
			default:
				taskStatus = "unknown"
			}

			// Apply filter
			if statusFilter == "running" && taskStatus != "running" && taskStatus != "backgrounded" {
				continue
			}
			if statusFilter == "completed" && taskStatus != "completed" && taskStatus != "killed" && taskStatus != "timeout" {
				continue
			}

			taskInfo := map[string]any{
				"id":        task.ID,
				"command":   task.Command,
				"status":    taskStatus,
				"startTime": task.StartTime.Unix(),
				"type":      "background",
			}

			// Add end time and exit code for completed tasks
			if taskStatus == "completed" || taskStatus == "killed" || taskStatus == "timeout" {
				endTime := task.GetEndTime()
				if endTime != nil {
					taskInfo["endTime"] = endTime.Unix()
				}
				taskInfo["exitCode"] = task.GetExitCode()
			}

			result = append(result, taskInfo)
		}
	}

	// List todo-style tasks
	if listType == "todo" || listType == "all" {
		todoTasks := GlobalTaskStore().ListTasks()

		for _, task := range todoTasks {
			// Apply filter
			if statusFilter == "running" && task.Status != TaskStatusInProgress {
				continue
			}
			if statusFilter == "completed" && task.Status != TaskStatusCompleted {
				continue
			}

			taskInfo := map[string]any{
				"id":          task.ID,
				"subject":     task.Subject,
				"description": task.Description,
				"status":      task.Status,
				"activeForm":  task.ActiveForm,
				"owner":       task.Owner,
				"blocks":      task.Blocks,
				"blockedBy":   task.BlockedBy,
				"createdAt":   task.CreatedAt.Unix(),
				"updatedAt":   task.UpdatedAt.Unix(),
				"type":        "todo",
			}

			result = append(result, taskInfo)
		}
	}

	output := map[string]any{
		"tasks": result,
		"count": len(result),
	}

	return tool.CallResult{
		Data: output,
	}, nil
}

// Description returns a human-readable description
func (t *TaskListTool) Description(ctx context.Context) (string, error) {
	return ToolDescriptionTaskList, nil
}

// ValidateInput validates the input
func (t *TaskListTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}

// CheckPermissions checks permissions (always allowed)
func (t *TaskListTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return types.PermissionResult{Behavior: types.PermissionBehaviorAllow}
}

// IsConcurrencySafe returns whether the tool is concurrency safe
func (t *TaskListTool) IsConcurrencySafe(input map[string]any) bool {
	return true
}

// IsReadOnly returns whether the tool is read-only
func (t *TaskListTool) IsReadOnly(input map[string]any) bool {
	return true
}

// IsEnabled returns whether the tool is enabled
func (t *TaskListTool) IsEnabled() bool {
	return true
}

// FormatResult formats the result
func (t *TaskListTool) FormatResult(data any) string {
	if data == nil {
		return "No tasks"
	}

	if m, ok := data.(map[string]any); ok {
		tasks, _ := m["tasks"].([]map[string]any)
		count, _ := m["count"].(int)

		if len(tasks) == 0 {
			return "No tasks found"
		}

		lines := make([]string, 0, len(tasks)+1)
		lines = append(lines, fmt.Sprintf("Found %d task(s):", count))

		for _, task := range tasks {
			taskType, _ := task["type"].(string)
			id, _ := task["id"].(string)
			status, _ := task["status"].(string)

			if taskType == "todo" {
				subject, _ := task["subject"].(string)
				owner, _ := task["owner"].(string)
				ownerStr := ""
				if owner != "" {
					ownerStr = fmt.Sprintf(" (owner: %s)", owner)
				}
				lines = append(lines, fmt.Sprintf("  [%s] #%s - %s%s", status, id, subject, ownerStr))
			} else {
				cmd, _ := task["command"].(string)
				// Truncate long commands
				displayCmd := cmd
				if len(displayCmd) > 50 {
					displayCmd = displayCmd[:47] + "..."
				}
				lines = append(lines, fmt.Sprintf("  [%s] %s - %s", status, id, displayCmd))
			}
		}

		return JoinLines(lines)
	}

	return fmt.Sprintf("%v", data)
}

// BackfillInput backfills input
func (t *TaskListTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return input
}
