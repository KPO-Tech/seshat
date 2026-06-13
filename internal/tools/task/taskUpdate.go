package task

import (
	"context"
	"fmt"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// TaskUpdateTool implements the TaskUpdate tool for updating tasks
type TaskUpdateTool struct{}

// NewTaskUpdateTool creates a new TaskUpdate tool
func NewTaskUpdateTool() *TaskUpdateTool {
	return &TaskUpdateTool{}
}

// Definition returns the tool definition
func (t *TaskUpdateTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        ToolNameTaskUpdate,
		DisplayName: "TaskUpdate",
		SearchHint:  SearchHintTaskUpdate,
		Description: ToolDescriptionTaskUpdate,
		Category:    "task",
		Metadata:    taskSurfaceMetadata(),
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"taskId": map[string]any{
					"type":        "string",
					"description": "The ID of the task to update",
				},
				"subject": map[string]any{
					"type":        "string",
					"description": "New subject for the task",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "New description for the task",
				},
				"activeForm": map[string]any{
					"type":        "string",
					"description": "Present continuous form shown in spinner when in_progress",
				},
				"status": map[string]any{
					"type":        "string",
					"enum":        []string{"pending", "in_progress", "completed", "deleted"},
					"description": "New status for the task",
				},
				"owner": map[string]any{
					"type":        "string",
					"description": "New owner for the task",
				},
				"addBlocks": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Task IDs that this task blocks",
				},
				"addBlockedBy": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Task IDs that block this task",
				},
				"metadata": map[string]any{
					"type":        "object",
					"description": "Metadata keys to merge (set to null to delete)",
				},
			},
			"required": []string{"taskId"},
		}),
	}
}

// Call executes the TaskUpdate tool
func (t *TaskUpdateTool) Call(ctx context.Context, input tool.CallInput, permissionCheck types.CanUseToolFn) (tool.CallResult, error) {
	parsed := input.Parsed
	if parsed == nil {
		parsed = make(map[string]any)
	}

	taskID, ok := parsed["taskId"].(string)
	if !ok || taskID == "" {
		return tool.CallResult{
			Error: fmt.Errorf("taskId is required"),
		}, nil
	}

	sessionID, err := resolveTaskSessionID(input)
	if err != nil {
		return tool.CallResult{Error: err}, nil
	}

	// Check if task exists
	existingTask, err := GlobalTaskStore().GetTask(ctx, sessionID, taskID)
	if err != nil {
		output := map[string]any{
			"success":       false,
			"taskId":        taskID,
			"updatedFields": []string{},
			"error":         "Task not found",
		}
		return tool.CallResult{
			Data: output,
		}, nil
	}

	updatedFields := make([]string, 0)
	updates := make(map[string]any)

	// Update subject if provided
	if subject, ok := parsed["subject"].(string); ok && subject != "" && subject != existingTask.Subject {
		updates["subject"] = subject
		updatedFields = append(updatedFields, "subject")
	}

	// Update description if provided
	if description, ok := parsed["description"].(string); ok && description != existingTask.Description {
		updates["description"] = description
		updatedFields = append(updatedFields, "description")
	}

	// Update activeForm if provided
	if activeForm, ok := parsed["activeForm"].(string); ok && activeForm != existingTask.ActiveForm {
		updates["activeForm"] = activeForm
		updatedFields = append(updatedFields, "activeForm")
	}

	// Update status if provided
	var oldStatus string
	var newStatus string
	if status, ok := parsed["status"].(string); ok && status != existingTask.Status {
		// Handle deletion
		if status == TaskStatusDeleted {
			err := GlobalTaskStore().DeleteTask(ctx, sessionID, taskID)
			if err != nil {
				output := map[string]any{
					"success":       false,
					"taskId":        taskID,
					"updatedFields": []string{},
					"error":         "Failed to delete task",
				}
				return tool.CallResult{
					Data: output,
				}, nil
			}
			emitTaskRuntimeEvent(ctx, sessionID, "delete", &Task{ID: taskID, SessionID: sessionID, Subject: existingTask.Subject, Status: TaskStatusDeleted})
			output := map[string]any{
				"success":       true,
				"taskId":        taskID,
				"updatedFields": []string{"deleted"},
				"statusChange": map[string]string{
					"from": existingTask.Status,
					"to":   TaskStatusDeleted,
				},
			}
			return tool.CallResult{
				Data: output,
			}, nil
		}
		oldStatus = existingTask.Status
		newStatus = status
		updates["status"] = status
		updatedFields = append(updatedFields, "status")
	}

	// Update owner if provided
	if owner, ok := parsed["owner"].(string); ok && owner != existingTask.Owner {
		updates["owner"] = owner
		updatedFields = append(updatedFields, "owner")
	}

	// Update metadata if provided
	if metadata, ok := parsed["metadata"].(map[string]any); ok {
		updates["metadata"] = metadata
		updatedFields = append(updatedFields, "metadata")
	}

	// Handle addBlocks
	if addBlocks, ok := parsed["addBlocks"].([]any); ok && len(addBlocks) > 0 {
		for _, blockID := range addBlocks {
			if blockIDStr, ok := blockID.(string); ok {
				GlobalTaskStore().BlockTask(ctx, sessionID, taskID, blockIDStr) //nolint:errcheck
			}
		}
		updatedFields = append(updatedFields, "blocks")
	}

	// Handle addBlockedBy
	if addBlockedBy, ok := parsed["addBlockedBy"].([]any); ok && len(addBlockedBy) > 0 {
		for _, blockerID := range addBlockedBy {
			if blockerIDStr, ok := blockerID.(string); ok {
				GlobalTaskStore().BlockTask(ctx, sessionID, blockerIDStr, taskID) //nolint:errcheck
			}
		}
		updatedFields = append(updatedFields, "blockedBy")
	}

	// Apply updates if any
	if len(updates) > 0 {
		_, err := GlobalTaskStore().UpdateTask(ctx, sessionID, taskID, updates)
		if err != nil {
			return tool.CallResult{
				Error: fmt.Errorf("failed to update task: %w", err),
			}, nil
		}
	}

	updatedTask, err := GlobalTaskStore().GetTask(ctx, sessionID, taskID)
	if err == nil {
		emitTaskRuntimeEvent(ctx, sessionID, "update", updatedTask)
	}

	// Build response
	output := map[string]any{
		"success":       true,
		"taskId":        taskID,
		"updatedFields": updatedFields,
	}

	if oldStatus != "" && newStatus != "" {
		output["statusChange"] = map[string]string{
			"from": oldStatus,
			"to":   newStatus,
		}
	}

	return tool.CallResult{
		Data: output,
	}, nil
}

// Description returns a human-readable description
func (t *TaskUpdateTool) Description(ctx context.Context) (string, error) {
	return ToolDescriptionTaskUpdate, nil
}

// ValidateInput validates the input
func (t *TaskUpdateTool) ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error) {
	return input, nil
}

// CheckPermissions checks permissions (always allowed)
func (t *TaskUpdateTool) CheckPermissions(ctx context.Context, input map[string]any, toolCtx tool.ToolUseContext) types.PermissionResult {
	taskID, ok := input["taskId"].(string)
	if !ok || taskID == "" {
		return types.Deny("taskId is required")
	}
	return types.PermissionResult{Behavior: types.PermissionBehaviorAllow}
}

// IsConcurrencySafe returns whether the tool is concurrency safe
func (t *TaskUpdateTool) IsConcurrencySafe(input map[string]any) bool {
	return true
}

// IsReadOnly returns whether the tool is read-only
func (t *TaskUpdateTool) IsReadOnly(input map[string]any) bool {
	return false
}

// IsEnabled returns whether the tool is enabled
func (t *TaskUpdateTool) IsEnabled() bool {
	return true
}

// FormatResult formats the result
func (t *TaskUpdateTool) FormatResult(data any) string {
	if data == nil {
		return "Task updated"
	}

	if m, ok := data.(map[string]any); ok {
		success, _ := m["success"].(bool)
		taskID, _ := m["taskId"].(string)
		updatedFields, _ := m["updatedFields"].([]string)

		if !success {
			errorMsg, _ := m["error"].(string)
			return fmt.Sprintf("Task #%s not found: %s", taskID, errorMsg)
		}

		return fmt.Sprintf("Updated task #%s %s", taskID, JoinFields(updatedFields))
	}

	return fmt.Sprintf("%v", data)
}

// BackfillInput backfills input
func (t *TaskUpdateTool) BackfillInput(ctx context.Context, input map[string]any) map[string]any {
	return input
}

// JoinFields joins a slice of strings with comma
func JoinFields(fields []string) string {
	result := ""
	for i, f := range fields {
		if i > 0 {
			result += ", "
		}
		result += f
	}
	return result
}
