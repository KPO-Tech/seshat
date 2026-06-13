package task

import (
	"context"
	"fmt"
	"strings"
	"time"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// TaskDetails represents detailed information about a background task.
type TaskDetails struct {
	ID        string `json:"id"`
	Command   string `json:"command"`
	Status    string `json:"status"`
	StartTime int64  `json:"startTime"`
	EndTime   *int64 `json:"endTime,omitempty"`
	ExitCode  *int   `json:"exitCode,omitempty"`
}

// TaskGetTool implements the TaskGet tool for getting task details.
type TaskGetTool struct{}

// NewTaskGetTool creates a new TaskGet tool.
func NewTaskGetTool() *TaskGetTool {
	return &TaskGetTool{}
}

// Definition returns the tool definition.
func (t *TaskGetTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameTaskGet,
		DisplayName: "TaskGet",
		SearchHint:  SearchHintTaskGet,
		Description: ToolDescriptionTaskGet,
		Category:    "task",
		Metadata:    taskSurfaceMetadata(),
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id": map[string]any{
					"type":        "string",
					"description": "The ID of the task to retrieve",
				},
				"taskType": map[string]any{
					"type":        "string",
					"enum":        []string{"todo", "background"},
					"description": "Optional task kind override. Defaults to todo first, then background fallback.",
				},
			},
			"required": []string{"task_id"},
		}),
	}
}

// Call executes the TaskGet tool.
func (t *TaskGetTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	parsed := input.Parsed
	if parsed == nil {
		parsed = make(map[string]any)
	}

	taskID, ok := parsed["task_id"].(string)
	if !ok || taskID == "" {
		return tool.CallResult{Error: fmt.Errorf("task_id is required")}, nil
	}

	taskType := resolveTaskKind(parsed, "")
	sessionID := resolveOptionalTaskSessionID(input)

	if taskType != "background" {
		if sessionID != "" {
			todoTask, err := GlobalTaskStore().GetTask(ctx, sessionID, taskID)
			if err == nil && todoTask != nil {
				details := todoTaskDetails(todoTask)
				return tool.CallResult{
					Data: map[string]any{
						"task": map[string]any{
							"type":        "todo",
							"id":          details.ID,
							"subject":     details.Subject,
							"description": details.Description,
							"status":      details.Status,
							"activeForm":  details.ActiveForm,
							"owner":       details.Owner,
							"blocks":      details.Blocks,
							"blockedBy":   details.BlockedBy,
							"createdAt":   details.CreatedAt,
							"updatedAt":   details.UpdatedAt,
						},
					},
					Metadata: &tool.ResultMetadata{Additional: map[string]any{
						"task_get": taskGetRenderMetadata{TaskType: "todo", Todo: details},
					}},
				}, nil
			}
		} else if taskType == "todo" {
			return tool.CallResult{Error: fmt.Errorf("session ID is required for todo task lookup")}, nil
		}
	}

	if taskType == "todo" {
		return tool.CallResult{Data: map[string]any{"task": nil}}, nil
	}

	runtime, err := requireRuntime()
	if err != nil {
		return tool.CallResult{Data: map[string]any{"task": nil}}, nil
	}
	backgroundTask, err := runtime.GetTask(ctx, taskID)
	if err != nil {
		return tool.CallResult{Data: map[string]any{"task": nil}}, nil
	}
	details := runtimeTaskDetails(backgroundTask)
	return tool.CallResult{
		Data: map[string]any{
			"task": map[string]any{
				"type":      "background",
				"id":        details.ID,
				"command":   details.Command,
				"status":    details.Status,
				"startTime": details.StartTime,
				"endTime":   details.EndTime,
				"exitCode":  details.ExitCode,
			},
		},
		Metadata: &tool.ResultMetadata{Additional: map[string]any{
			"task_get": taskGetRenderMetadata{TaskType: "background", Background: details},
		}},
	}, nil
}

func todoTaskDetails(task *Task) *TaskGetTodoDetails {
	if task == nil {
		return nil
	}
	return &TaskGetTodoDetails{
		ID:          task.ID,
		Subject:     task.Subject,
		Description: task.Description,
		Status:      task.Status,
		ActiveForm:  task.ActiveForm,
		Owner:       task.Owner,
		Blocks:      append([]string(nil), task.Blocks...),
		BlockedBy:   append([]string(nil), task.BlockedBy...),
		CreatedAt:   task.CreatedAt.Unix(),
		UpdatedAt:   task.UpdatedAt.Unix(),
	}
}

func runtimeTaskDetails(task *RuntimeTask) *TaskDetails {
	if task == nil {
		return nil
	}
	details := &TaskDetails{
		ID:        task.ID,
		Command:   task.Command,
		Status:    string(task.Status),
		StartTime: task.CreatedAt.Unix(),
	}
	if task.StartedAt != nil {
		details.StartTime = task.StartedAt.Unix()
	}
	if task.CompletedAt != nil {
		endTimeUnix := task.CompletedAt.Unix()
		details.EndTime = &endTimeUnix
	}
	if task.ExitCode != nil {
		details.ExitCode = task.ExitCode
	}
	return details
}

// Description returns a human-readable description.
func (t *TaskGetTool) Description(ctx context.Context) (string, error) {
	return ToolDescriptionTaskGet, nil
}

// ValidateInput validates the input.
func (t *TaskGetTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}

// CheckPermissions checks permissions.
func (t *TaskGetTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	taskID, ok := input["task_id"].(string)
	if !ok || taskID == "" {
		return types.Deny("task_id is required")
	}
	taskType := resolveTaskKind(input, "")
	if taskType != "background" && toolCtx.SessionID != "" {
		if _, err := GlobalTaskStore().GetTask(ctx, string(toolCtx.SessionID), taskID); err == nil {
			return types.PermissionResult{Behavior: types.PermissionBehaviorAllow}
		}
	}
	if taskType == "todo" {
		return types.Deny("todo task not found")
	}
	return types.PermissionResult{Behavior: types.PermissionBehaviorAllow}
}

// IsConcurrencySafe returns whether the tool is concurrency safe.
func (t *TaskGetTool) IsConcurrencySafe(input map[string]any) bool {
	return true
}

// IsReadOnly returns whether the tool is read-only.
func (t *TaskGetTool) IsReadOnly(input map[string]any) bool {
	return true
}

// IsEnabled returns whether the tool is enabled.
func (t *TaskGetTool) IsEnabled() bool {
	return true
}

// FormatResult formats the result.
func (t *TaskGetTool) FormatResult(data any) string {
	if data == nil {
		return "Task not found"
	}
	m, ok := data.(map[string]any)
	if !ok {
		return fmt.Sprintf("%v", data)
	}
	rawTask, ok := m["task"].(map[string]any)
	if !ok || rawTask == nil {
		return "Task not found"
	}
	taskType, _ := rawTask["type"].(string)
	if taskType == "todo" {
		subject, _ := rawTask["subject"].(string)
		status, _ := rawTask["status"].(string)
		description, _ := rawTask["description"].(string)
		parts := []string{fmt.Sprintf("Task: %s", subject), fmt.Sprintf("Status: %s", status)}
		if owner, _ := rawTask["owner"].(string); owner != "" {
			parts = append(parts, fmt.Sprintf("Owner: %s", owner))
		}
		if activeForm, _ := rawTask["activeForm"].(string); activeForm != "" {
			parts = append(parts, fmt.Sprintf("Active: %s", activeForm))
		}
		if strings.TrimSpace(description) != "" {
			parts = append(parts, "", description)
		}
		return JoinLines(parts)
	}
	id, _ := rawTask["id"].(string)
	cmd, _ := rawTask["command"].(string)
	status, _ := rawTask["status"].(string)
	startTime, _ := rawTask["startTime"].(int64)
	lines := []string{
		fmt.Sprintf("Task: %s", id),
		fmt.Sprintf("Status: %s", status),
		fmt.Sprintf("Command: %s", cmd),
		fmt.Sprintf("Started: %s", time.Unix(startTime, 0).Format(time.RFC3339)),
	}
	if endTime, ok := rawTask["endTime"].(int64); ok && endTime > 0 {
		lines = append(lines, fmt.Sprintf("Ended: %s", time.Unix(endTime, 0).Format(time.RFC3339)))
	}
	if exitCode, ok := rawTask["exitCode"].(int); ok {
		lines = append(lines, fmt.Sprintf("Exit Code: %d", exitCode))
	}
	return JoinLines(lines)
}

// BackfillInput backfills input.
func (t *TaskGetTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return input
}
