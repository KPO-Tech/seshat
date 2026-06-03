package bash

import (
	"context"
	"fmt"
	"time"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// WriteStdinTool writes characters to a running background task's stdin.
// After writing it waits wait_ms milliseconds, then returns any new output
// the task has produced — useful for driving interactive REPLs.
type WriteStdinTool struct{}

func NewWriteStdinTool() *WriteStdinTool { return &WriteStdinTool{} }

const (
	writeStdinDefaultWaitMs = 500
	writeStdinMaxWaitMs     = 30_000
)

func (t *WriteStdinTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        "write_stdin",
		DisplayName: "Write to Stdin",
		Description: "Write text to a running background task's stdin. Use this to drive interactive programs (REPLs, debuggers, prompts). Returns any new output produced after writing.",
		Category:    "filesystem",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id": map[string]any{
					"type":        "string",
					"description": "Background task ID returned by the bash tool when run_in_background=true.",
				},
				"input": map[string]any{
					"type":        "string",
					"description": "Text to write to stdin.",
				},
				"append_newline": map[string]any{
					"type":        "boolean",
					"description": "Append a newline after input (default true). Set false when piping raw bytes.",
				},
				"wait_ms": map[string]any{
					"type":        "number",
					"description": fmt.Sprintf("Milliseconds to wait for output after writing (default %d, max %d).", writeStdinDefaultWaitMs, writeStdinMaxWaitMs),
				},
			},
			"required": []string{"task_id", "input"},
		}),
		IsReadOnly:         false,
		IsConcurrencySafe:  false,
		IsDestructive:      false,
		RequiresPermission: false,
	}
}

func (t *WriteStdinTool) Call(
	ctx context.Context,
	input tool.CallInput,
	_ types.CanUseToolFn,
) (tool.CallResult, error) {
	taskID, ok := input.Parsed["task_id"].(string)
	if !ok || taskID == "" {
		return tool.NewErrorResult(fmt.Errorf("task_id is required")), nil
	}
	text, ok := input.Parsed["input"].(string)
	if !ok {
		return tool.NewErrorResult(fmt.Errorf("input is required")), nil
	}

	appendNewline := true
	if v, ok := input.Parsed["append_newline"].(bool); ok {
		appendNewline = v
	}

	waitMs := writeStdinDefaultWaitMs
	if v, ok := input.Parsed["wait_ms"].(float64); ok && v > 0 {
		waitMs = int(v)
		if waitMs > writeStdinMaxWaitMs {
			waitMs = writeStdinMaxWaitMs
		}
	}

	mgr := GlobalTaskManager()
	if mgr == nil {
		return tool.NewErrorResult(fmt.Errorf("no background task manager available — start a background bash command first")), nil
	}

	// Capture output position before writing so we can return only new output.
	reader, err := NewTaskOutputReaderFrom(mgr, taskID)
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("task %q: %w", taskID, err)), nil
	}
	// Advance reader to current end-of-output.
	if _, err := reader.ReadOutput(); err != nil {
		return tool.NewErrorResult(fmt.Errorf("read task output: %w", err)), nil
	}

	// Write to stdin.
	payload := text
	if appendNewline {
		payload += "\n"
	}
	if err := mgr.WriteStdin(taskID, payload); err != nil {
		return tool.NewErrorResult(fmt.Errorf("write_stdin: %w", err)), nil
	}

	// Wait for output.
	select {
	case <-ctx.Done():
		return tool.NewErrorResult(fmt.Errorf("cancelled while waiting for output")), nil
	case <-time.After(time.Duration(waitMs) * time.Millisecond):
	}

	// Read new output.
	newOutput, err := reader.ReadOutput()
	if err != nil {
		return tool.NewErrorResult(fmt.Errorf("read new output: %w", err)), nil
	}

	task := mgr.GetTask(taskID)
	status := "running"
	if task != nil {
		switch task.GetStatus() {
		case TaskStatusCompleted:
			status = fmt.Sprintf("completed (exit %d)", task.GetExitCode())
		case TaskStatusKilled:
			status = "killed"
		case TaskStatusTimeout:
			status = "timed out"
		}
	}

	if newOutput == "" {
		return tool.NewTextResult(fmt.Sprintf("[task %s: %s — no new output]", taskID, status)), nil
	}
	return tool.NewTextResult(newOutput), nil
}

// ─── tool.Tool interface ──────────────────────────────────────────────────────

func (t *WriteStdinTool) IsEnabled() bool                         { return true }
func (t *WriteStdinTool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *WriteStdinTool) IsConcurrencySafe(_ map[string]any) bool { return false }
func (t *WriteStdinTool) FormatResult(data any) string            { return fmt.Sprintf("%v", data) }
func (t *WriteStdinTool) BackfillInput(_ context.Context, in map[string]any) map[string]any {
	return in
}
func (t *WriteStdinTool) ValidateInput(_ context.Context, in map[string]any) (map[string]any, error) {
	if id, _ := in["task_id"].(string); id == "" {
		return nil, fmt.Errorf("task_id is required")
	}
	return in, nil
}
func (t *WriteStdinTool) CheckPermissions(_ context.Context, _ map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(nil)
}
func (t *WriteStdinTool) PreparePermissionMatcher(_ context.Context, _ map[string]any) (func(string) bool, error) {
	return nil, nil
}
func (t *WriteStdinTool) Description(_ context.Context) (string, error) {
	return "Write text to a running background task's stdin and return new output.", nil
}
