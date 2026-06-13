package task

import (
	"context"
	"fmt"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// TaskStopTool implements the TaskStop tool for stopping tracked tasks.
type TaskStopTool struct{}

func NewTaskStopTool() *TaskStopTool { return &TaskStopTool{} }

func (t *TaskStopTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameTaskStop,
		DisplayName: "TaskStop",
		SearchHint:  SearchHintTaskStop,
		Description: ToolDescriptionTaskStop,
		Category:    "task",
		Metadata:    taskSurfaceMetadata(),
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id": map[string]any{
					"type":        "string",
					"description": "The ID of the task to stop",
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

func (t *TaskStopTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
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
				if err := GlobalTaskStore().DeleteTask(ctx, sessionID, taskID); err != nil {
					return tool.CallResult{Error: fmt.Errorf("failed to stop task: %w", err)}, nil
				}
				emitTaskRuntimeEvent(ctx, sessionID, "delete", &Task{ID: todoTask.ID, SessionID: sessionID, Subject: todoTask.Subject, Status: TaskStatusDeleted})
				messageText := fmt.Sprintf("Stopped task tracking: %s", todoTask.Subject)
				return tool.CallResult{Data: map[string]any{
					"message":   messageText,
					"task_id":   taskID,
					"task_type": "todo",
					"subject":   todoTask.Subject,
				}, Metadata: &tool.ResultMetadata{Additional: map[string]any{
					"task_stop": taskStopRenderMetadata{
						TaskID:   taskID,
						TaskType: "todo",
						Message:  messageText,
						Todo: &TaskStopTodoDetails{
							ID:             todoTask.ID,
							Subject:        todoTask.Subject,
							PreviousStatus: todoTask.Status,
						},
					},
				}}}, nil
			}
		} else if taskType == "todo" {
			return tool.CallResult{Error: fmt.Errorf("session ID is required for todo task stop")}, nil
		}
	}

	if taskType == "todo" {
		return tool.CallResult{Error: fmt.Errorf("no task found with ID: %s", taskID)}, nil
	}

	runtime, err := requireRuntime()
	if err != nil {
		return tool.CallResult{Error: fmt.Errorf("task runtime not available")}, nil
	}
	task, err := runtime.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return tool.CallResult{Error: fmt.Errorf("no task found with ID: %s", taskID)}, nil
	}
	if task.Status != RuntimeTaskStatusRunning && task.Status != RuntimeTaskStatusPending {
		return tool.CallResult{Error: fmt.Errorf("task %s is not running (status: %s)", taskID, task.Status)}, nil
	}
	if err := runtime.KillTask(ctx, taskID); err != nil {
		return tool.CallResult{Error: fmt.Errorf("failed to stop task: %w", err)}, nil
	}

	messageText := fmt.Sprintf("Successfully stopped task: %s", taskID)
	return tool.CallResult{Data: map[string]any{
		"message":   messageText,
		"task_id":   taskID,
		"task_type": string(task.Type),
		"command":   task.Command,
	}, Metadata: &tool.ResultMetadata{Additional: map[string]any{
		"task_stop": taskStopRenderMetadata{
			TaskID:   taskID,
			TaskType: string(task.Type),
			Command:  task.Command,
			Message:  messageText,
		},
	}}}, nil
}

func (t *TaskStopTool) Description(ctx context.Context) (string, error) {
	return ToolDescriptionTaskStop, nil
}
func (t *TaskStopTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}
func (t *TaskStopTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
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
	runtime, err := requireRuntime()
	if err != nil {
		return types.Deny("task runtime not available")
	}
	task, err := runtime.GetTask(ctx, taskID)
	if err != nil || task == nil {
		return types.Deny("task not found")
	}
	if task.Status != RuntimeTaskStatusRunning && task.Status != RuntimeTaskStatusPending {
		return types.Deny("task is not running")
	}
	return types.PermissionResult{Behavior: types.PermissionBehaviorAllow}
}
func (t *TaskStopTool) IsConcurrencySafe(input map[string]any) bool { return true }
func (t *TaskStopTool) IsReadOnly(input map[string]any) bool        { return false }
func (t *TaskStopTool) IsEnabled() bool                             { return true }
func (t *TaskStopTool) FormatResult(data any) string {
	if data == nil {
		return "Task stopped"
	}
	if m, ok := data.(map[string]any); ok {
		message, _ := m["message"].(string)
		taskID, _ := m["task_id"].(string)
		taskType, _ := m["task_type"].(string)
		subject, _ := m["subject"].(string)
		command, _ := m["command"].(string)
		suffix := taskID
		if taskType == "todo" && subject != "" {
			suffix = subject
		} else if command != "" {
			suffix = fmt.Sprintf("%s, command: %s", taskID, command)
		}
		return fmt.Sprintf("%s (%s)", message, suffix)
	}
	return fmt.Sprintf("%v", data)
}
func (t *TaskStopTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return input
}

// ListRunningTasks returns all running background tasks.
func ListRunningTasks(ctx context.Context) []map[string]any {
	runtime, err := requireRuntime()
	if err != nil {
		return nil
	}
	tasks, err := runtime.ListTasks(ctx)
	if err != nil {
		return nil
	}
	result := make([]map[string]any, 0)
	for _, task := range tasks {
		if task.Status == RuntimeTaskStatusRunning || task.Status == RuntimeTaskStatusPending {
			result = append(result, map[string]any{
				"task_id":   task.ID,
				"command":   task.Command,
				"status":    statusToString(task.Status),
				"startTime": task.CreatedAt,
			})
		}
	}
	return result
}
