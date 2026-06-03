package task

import (
	"context"
	"fmt"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// TaskCreateTool implements the TaskCreate tool for creating tasks
type TaskCreateTool struct{}

// NewTaskCreateTool creates a new TaskCreate tool
func NewTaskCreateTool() *TaskCreateTool {
	return &TaskCreateTool{}
}

// Definition returns the tool definition
func (t *TaskCreateTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameTaskCreate,
		DisplayName: "TaskCreate",
		SearchHint:  SearchHintTaskCreate,
		Description: ToolDescriptionTaskCreate,
		Category:    "task",
		Metadata:    taskSurfaceMetadata(),
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"subject": map[string]any{
					"type":        "string",
					"description": "A brief title for the task",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "What needs to be done",
				},
				"activeForm": map[string]any{
					"type":        "string",
					"description": "Present continuous form shown in spinner when in_progress (e.g., 'Running tests')",
				},
				"metadata": map[string]any{
					"type":        "object",
					"description": "Arbitrary metadata to attach to the task",
				},
			},
			"required": []string{"subject", "description"},
		}),
	}
}

// Call executes the TaskCreate tool
func (t *TaskCreateTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	parsed := input.Parsed
	if parsed == nil {
		parsed = make(map[string]any)
	}

	subject, ok := parsed["subject"].(string)
	if !ok || subject == "" {
		return tool.CallResult{
			Error: fmt.Errorf("subject is required"),
		}, nil
	}

	description, ok := parsed["description"].(string)
	if !ok {
		description = ""
	}

	activeForm := ""
	if af, ok := parsed["activeForm"].(string); ok {
		activeForm = af
	}

	var metadata map[string]any
	if m, ok := parsed["metadata"].(map[string]any); ok {
		metadata = m
	}

	task, err := GlobalTaskStore().CreateTask(subject, description, activeForm, metadata)
	if err != nil {
		return tool.CallResult{
			Error: fmt.Errorf("failed to create task: %w", err),
		}, nil
	}

	output := map[string]any{
		"task": map[string]any{
			"id":      task.ID,
			"subject": task.Subject,
		},
	}

	return tool.CallResult{
		Data: output,
	}, nil
}

// Description returns a human-readable description
func (t *TaskCreateTool) Description(ctx context.Context) (string, error) {
	return ToolDescriptionTaskCreate, nil
}

// ValidateInput validates the input
func (t *TaskCreateTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}

// CheckPermissions checks permissions (always allowed)
func (t *TaskCreateTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	return types.PermissionResult{Behavior: types.PermissionBehaviorAllow}
}

// IsConcurrencySafe returns whether the tool is concurrency safe
func (t *TaskCreateTool) IsConcurrencySafe(input map[string]any) bool {
	return true
}

// IsReadOnly returns whether the tool is read-only
func (t *TaskCreateTool) IsReadOnly(input map[string]any) bool {
	return false
}

// IsEnabled returns whether the tool is enabled
func (t *TaskCreateTool) IsEnabled() bool {
	return true
}

// FormatResult formats the result
func (t *TaskCreateTool) FormatResult(data any) string {
	if data == nil {
		return "Task created"
	}

	if m, ok := data.(map[string]any); ok {
		task, hasTask := m["task"].(map[string]any)
		if hasTask && task != nil {
			id, _ := task["id"].(string)
			subject, _ := task["subject"].(string)
			return fmt.Sprintf("Task #%s created successfully: %s", id, subject)
		}
	}

	return fmt.Sprintf("%v", data)
}

// BackfillInput backfills input
func (t *TaskCreateTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return input
}
