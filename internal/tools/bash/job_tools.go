package bash

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/KPO-Tech/seshat-runtime/internal/tools/contract"
	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	"github.com/KPO-Tech/seshat-runtime/internal/tools/schema"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

const jobOutputMaxTimeoutMs = 60_000

// applyLineOffset slices content by line the same way read_file's offset
// does: positive = start at that 1-indexed line, negative = start that many
// lines before the end (tail), 0 = unchanged. Out-of-range values clamp
// rather than error - a job's buffered output is a live, growing thing, not
// a fixed file, so "give me what exists" is more useful than a hard failure.
func applyLineOffset(content string, offset int) string {
	if offset == 0 || content == "" {
		return content
	}
	// Buffered job output almost always ends in "\n" (the last command's
	// trailing newline). Splitting that directly would produce a spurious
	// empty final "line", throwing off both the line count used for
	// negative/tail offsets and the last real line returned.
	trailingNewline := strings.HasSuffix(content, "\n")
	trimmed := strings.TrimSuffix(content, "\n")
	lines := strings.Split(trimmed, "\n")
	n := len(lines)
	start := offset - 1
	if offset < 0 {
		start = n + offset
	}
	if start < 0 {
		start = 0
	}
	if start >= n {
		return ""
	}
	result := strings.Join(lines[start:], "\n")
	if trailingNewline {
		result += "\n"
	}
	return result
}

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
		Description: "Read buffered stdout/stderr from a background bash job, plus its current status. With timeout_ms set, waits for new output (returning early if the job looks idle at a prompt, or finishes) instead of returning immediately.",
		Category:    "filesystem",
		InputSchema: schema.FromMap(map[string]any{
			"type":     "object",
			"required": []string{"job_id"},
			"properties": map[string]any{
				"job_id": map[string]any{
					"type":        "string",
					"description": "Background task ID returned by bash when run_in_background=true.",
				},
				"timeout_ms": map[string]any{
					"type":        "integer",
					"description": fmt.Sprintf("Wait up to this many milliseconds for the job to produce output (default 0 = return immediately, max %d). Returns early if the job looks like it's idle at an interactive prompt, or finishes.", jobOutputMaxTimeoutMs),
					"default":     0,
				},
				"offset": map[string]any{
					"type":        "integer",
					"description": "1-indexed line to start the returned output from. Negative counts back from the end (e.g. -50 = last 50 lines) - useful for tailing a chatty job's log without re-reading everything. Omit/0 for the full buffered output.",
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

	timeoutMs := 0
	if v, ok := input.Parsed["timeout_ms"].(float64); ok && v > 0 {
		timeoutMs = int(v)
		if timeoutMs > jobOutputMaxTimeoutMs {
			timeoutMs = jobOutputMaxTimeoutMs
		}
	}

	var out string
	var finalStatus TaskStatus
	if timeoutMs > 0 {
		out, finalStatus = pollForOutput(ctx, mgr, jobID, reader, time.Duration(timeoutMs)*time.Millisecond)
		if ctx.Err() != nil {
			return tool.CallResult{Content: "error: cancelled while waiting for output"}, nil
		}
	} else {
		var err error
		out, err = reader.ReadOutput()
		if err != nil {
			return tool.CallResult{Content: fmt.Sprintf("error reading output: %v", err)}, nil
		}
		finalStatus = task.GetStatus()
	}

	if offsetVal, ok := input.Parsed["offset"].(float64); ok {
		out = applyLineOffset(out, int(offsetVal))
	}

	status := taskStatusString(finalStatus)
	exitInfo := ""
	if finalStatus == TaskStatusCompleted || finalStatus == TaskStatusKilled || finalStatus == TaskStatusTimeout {
		if refreshed := mgr.GetTask(jobID); refreshed != nil {
			exitInfo = fmt.Sprintf("\nexit_code: %d", refreshed.GetExitCode())
		}
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
			"type":     "object",
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
func (t *JobOutputTool) RequiresUserInteraction() bool            { return false }
func (t *JobOutputTool) ExecutesInPlanMode(_ map[string]any) bool { return false }

func (t *JobKillTool) IsConcurrencySafe(_ map[string]any) bool { return false }
func (t *JobKillTool) IsReadOnly(_ map[string]any) bool        { return false }
func (t *JobKillTool) IsEnabled() bool                         { return true }
func (t *JobKillTool) ValidateInput(_ context.Context, in map[string]any) (map[string]any, error) {
	return in, nil
}
func (t *JobKillTool) RequiresUserInteraction() bool            { return false }
func (t *JobKillTool) ExecutesInPlanMode(_ map[string]any) bool { return false }
