package task

import (
	"context"
	"fmt"
	"strings"

	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	"github.com/KPO-Tech/seshat-runtime/internal/tools/schema"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
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
				"tasks": map[string]any{
					"type":        "array",
					"description": "Create several tasks in one call, in order. Each item has subject and description (activeForm and metadata are optional). Use this instead of one call per task.",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"subject":     map[string]any{"type": "string", "description": "A brief title for the task"},
							"description": map[string]any{"type": "string", "description": "What needs to be done"},
							"activeForm":  map[string]any{"type": "string", "description": "Present continuous form shown in spinner when in_progress"},
							"metadata":    map[string]any{"type": "object", "description": "Arbitrary metadata to attach to the task"},
						},
						"required": []string{"subject", "description"},
					},
				},
			},
		}),
	}
}

// Call executes the TaskCreate tool. It creates one task from the top-level
// fields, or several from "tasks"; every item is checked before any is created.
func (t *TaskCreateTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	parsed := input.Parsed
	if parsed == nil {
		parsed = make(map[string]any)
	}

	items, batch, err := taskCreateItems(parsed)
	if err != nil {
		return tool.CallResult{Error: err}, nil
	}

	sessionID, err := resolveTaskSessionID(input)
	if err != nil {
		return tool.CallResult{Error: err}, nil
	}

	created := make([]map[string]any, 0, len(items))
	for _, item := range items {
		task, err := GlobalTaskStore().CreateTask(ctx, sessionID, item.subject, item.description, item.activeForm, item.metadata)
		if err != nil {
			return tool.CallResult{
				Error: fmt.Errorf("failed to create task %q (%d of %d created before it): %w", item.subject, len(created), len(items), err),
			}, nil
		}
		emitTaskRuntimeEvent(ctx, sessionID, "create", task)
		created = append(created, map[string]any{"id": task.ID, "subject": task.Subject})
	}

	if batch {
		return tool.CallResult{Data: map[string]any{"tasks": created}}, nil
	}
	return tool.CallResult{Data: map[string]any{"task": created[0]}}, nil
}

// taskCreateItem is one task to create.
type taskCreateItem struct {
	subject, description, activeForm string
	metadata                         map[string]any
}

// taskCreateItems reads the tasks to create from the input: the "tasks" list
// when present, otherwise the single task in the top-level fields. batch says
// which form was used, so the result can mirror it.
func taskCreateItems(parsed map[string]any) (items []taskCreateItem, batch bool, err error) {
	if raw, ok := parsed["tasks"].([]any); ok && len(raw) > 0 {
		for i, entry := range raw {
			m, ok := entry.(map[string]any)
			if !ok {
				return nil, true, fmt.Errorf("tasks[%d] must be an object with subject and description", i)
			}
			item, err := parseTaskCreateItem(m)
			if err != nil {
				return nil, true, fmt.Errorf("tasks[%d]: %w", i, err)
			}
			items = append(items, item)
		}
		return items, true, nil
	}
	item, err := parseTaskCreateItem(parsed)
	if err != nil {
		return nil, false, fmt.Errorf("%w (or pass tasks to create several at once)", err)
	}
	return []taskCreateItem{item}, false, nil
}

func parseTaskCreateItem(m map[string]any) (taskCreateItem, error) {
	subject, _ := m["subject"].(string)
	if subject == "" {
		return taskCreateItem{}, fmt.Errorf("subject is required")
	}
	item := taskCreateItem{subject: subject}
	item.description, _ = m["description"].(string)
	item.activeForm, _ = m["activeForm"].(string)
	item.metadata, _ = m["metadata"].(map[string]any)
	return item, nil
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
		if tasks, ok := m["tasks"].([]map[string]any); ok {
			return formatCreatedTasks(tasks)
		}
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

func formatCreatedTasks(tasks []map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d tasks created:", len(tasks))
	for _, task := range tasks {
		id, _ := task["id"].(string)
		subject, _ := task["subject"].(string)
		fmt.Fprintf(&b, "\n#%s %s", id, subject)
	}
	return b.String()
}
