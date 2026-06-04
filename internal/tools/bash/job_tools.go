package bash

import (
	"context"
	"fmt"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/tools/contract"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/tools/schema"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

var _ contract.Tool = (*JobOutputTool)(nil)
var _ contract.Tool = (*JobKillTool)(nil)

// ─── job_output ──────────────────────────────────────────────────────────────

// JobOutputTool returns buffered stdout/stderr from a background job.
// Inspired by crush's async bash job pattern.
type JobOutputTool struct{}

func NewJobOutputTool() *JobOutputTool { return &JobOutputTool{} }

func (t *JobOutputTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        "job_output",
		DisplayName: "Get Job Output",
		Description: "Read the latest buffered stdout/stderr from a background bash job. Returns the output since the last call plus the current job status.",
		Category:    "filesystem",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"required": []string{"job_id"},
			"properties": map[string]any{
				"job_id": map[string]any{
					"type":        "string",
					"description": "Background task ID returned by bash when run_in_background=true.",
				},
				"timeout_ms": map[string]any{
					"type":        "integer",
					"description": "Wait up to this many milliseconds for the job to produce output (default 0 = return immediately).",
					"default":     0,
				},
			},
		}),
	}
}

func (t *JobOutputTool) Call(
	ctx context.Context,
	input tool.CallInput,
	_ types.CanUseToolFn,
) (tool.CallResult, error) {
	jobID, _ := input.Parsed["job_id"].(string)
	if jobID == "" {
		return tool.CallResult{Content: "error: job_id is required"}, nil
	}

	mgr := GlobalTaskManager()
	if mgr == nil {
		return tool.CallResult{Content: "error: no background task manager available"}, nil
	}

	task := mgr.GetTask(jobID)
	if task == nil {
		return tool.CallResult{Content: fmt.Sprintf("error: job %q not found", jobID)}, nil
	}

	reader, readerErr := NewTaskOutputReader(jobID)
	if readerErr != nil {
		return tool.CallResult{Content: fmt.Sprintf("error creating reader: %v", readerErr)}, nil
	}

	out, err := reader.ReadOutput()
	if err != nil {
		return tool.CallResult{Content: fmt.Sprintf("error reading output: %v", err)}, nil
	}

	status := taskStatusString(task.Status)
	exitInfo := ""
	if task.Status == TaskStatusCompleted || task.Status == TaskStatusKilled || task.Status == TaskStatusTimeout {
		exitInfo = fmt.Sprintf("\nexit_code: %d", task.ExitCode)
	}

	result := fmt.Sprintf("job_id: %s\nstatus: %s%s", jobID, status, exitInfo)
	if out != "" {
		result += "\n\noutput:\n" + out
	} else {
		result += "\n\n(no new output)"
	}

	return tool.CallResult{Content: result}, nil
}

func taskStatusString(s TaskStatus) string {
	switch s {
	case TaskStatusRunning:
		return "running"
	case TaskStatusBackgrounded:
		return "backgrounded"
	case TaskStatusCompleted:
		return "completed"
	case TaskStatusKilled:
		return "killed"
	case TaskStatusTimeout:
		return "timeout"
	default:
		return "unknown"
	}
}

// ─── job_kill ────────────────────────────────────────────────────────────────

// JobKillTool kills a running background job and returns its final output.
type JobKillTool struct{}

func NewJobKillTool() *JobKillTool { return &JobKillTool{} }

func (t *JobKillTool) Definition() tool.Definition {
	return tool.Definition{
		Name:        "job_kill",
		DisplayName: "Kill Job",
		Description: "Kill a running background bash job. Returns any buffered output before termination.",
		Category:    "filesystem",
		InputSchema: schema.FromMap(map[string]any{
			"type": "object",
			"required": []string{"job_id"},
			"properties": map[string]any{
				"job_id": map[string]any{
					"type":        "string",
					"description": "Background task ID to kill.",
				},
			},
		}),
	}
}

func (t *JobKillTool) Call(
	ctx context.Context,
	input tool.CallInput,
	_ types.CanUseToolFn,
) (tool.CallResult, error) {
	jobID, _ := input.Parsed["job_id"].(string)
	if jobID == "" {
		return tool.CallResult{Content: "error: job_id is required"}, nil
	}

	mgr := GlobalTaskManager()
	if mgr == nil {
		return tool.CallResult{Content: "error: no background task manager available"}, nil
	}

	task := mgr.GetTask(jobID)
	if task == nil {
		return tool.CallResult{Content: fmt.Sprintf("error: job %q not found", jobID)}, nil
	}

	// Grab any remaining output before killing.
	reader, readerErr := NewTaskOutputReader(jobID)
	if readerErr != nil {
		return tool.CallResult{Content: fmt.Sprintf("error creating reader: %v", readerErr)}, nil
	}
	out, _ := reader.ReadOutput()

	if err := mgr.KillTask(jobID); err != nil {
		return tool.CallResult{Content: fmt.Sprintf("error killing job %q: %v", jobID, err)}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("job_id: %s\nstatus: killed", jobID))
	if out != "" {
		sb.WriteString("\n\nfinal output:\n")
		sb.WriteString(out)
	}
	return tool.CallResult{Content: sb.String()}, nil
}

// Satisfy the contract.Tool interface — these tools don't need backfilling.
func (t *JobOutputTool) BackfillInput(_ context.Context, in map[string]any) map[string]any {
	return in
}
func (t *JobKillTool) BackfillInput(_ context.Context, in map[string]any) map[string]any {
	return in
}

func (t *JobOutputTool) CheckPermissions(_ context.Context, _ map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(nil)
}
func (t *JobOutputTool) PreparePermissionMatcher(_ context.Context, _ map[string]any) (func(string) bool, error) {
	return nil, nil
}
func (t *JobOutputTool) Description(_ context.Context) (string, error) {
	return "Read buffered output from a background bash job.", nil
}

func (t *JobKillTool) CheckPermissions(_ context.Context, _ map[string]any, _ tool.ToolUseContext) types.PermissionResult {
	return types.Passthrough(nil)
}
func (t *JobKillTool) PreparePermissionMatcher(_ context.Context, _ map[string]any) (func(string) bool, error) {
	return nil, nil
}
func (t *JobKillTool) Description(_ context.Context) (string, error) {
	return "Kill a running background bash job.", nil
}

func (t *JobOutputTool) FormatResult(data any) string { return fmt.Sprintf("%v", data) }
func (t *JobKillTool) FormatResult(data any) string   { return fmt.Sprintf("%v", data) }

func (t *JobOutputTool) IsConcurrencySafe(_ map[string]any) bool { return true }
func (t *JobOutputTool) IsReadOnly(_ map[string]any) bool        { return true }
func (t *JobOutputTool) IsEnabled() bool                         { return true }
func (t *JobOutputTool) ValidateInput(_ context.Context, in map[string]any) (map[string]any, error) {
	return in, nil
}
func (t *JobOutputTool) RequiresUserInteraction() bool          { return false }
func (t *JobOutputTool) ExecutesInPlanMode(_ map[string]any) bool { return false }

func (t *JobKillTool) IsConcurrencySafe(_ map[string]any) bool { return false }
func (t *JobKillTool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *JobKillTool) IsEnabled() bool                         { return true }
func (t *JobKillTool) ValidateInput(_ context.Context, in map[string]any) (map[string]any, error) {
	return in, nil
}
func (t *JobKillTool) RequiresUserInteraction() bool          { return false }
func (t *JobKillTool) ExecutesInPlanMode(_ map[string]any) bool { return false }
