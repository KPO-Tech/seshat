package task

import (
	"context"
	"strings"
	"testing"

	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

func batchInput(parsed map[string]any) tool.CallInput {
	return tool.CallInput{
		SessionID:   types.SessionID("session-1"),
		Parsed:      parsed,
		ToolContext: &tool.ToolUseContext{SessionID: types.SessionID("session-1")},
	}
}

func initBatchStore(t *testing.T) {
	t.Helper()
	path := t.TempDir() + "/tasks.sqlite"
	if err := InitializeGlobalTaskStore(path); err != nil {
		t.Fatalf("initialize task store: %v", err)
	}
	t.Cleanup(func() { _ = CloseGlobalTaskStore(path) })
}

func TestTaskCreateBatchCreatesEveryTaskInOrder(t *testing.T) {
	initBatchStore(t)
	result, err := NewTaskCreateTool().Call(context.Background(), batchInput(map[string]any{
		"tasks": []any{
			map[string]any{"subject": "Write the plan", "description": "d1"},
			map[string]any{"subject": "Do the research", "description": "d2"},
			map[string]any{"subject": "Write the report", "description": "d3"},
		},
	}), nil)
	if err != nil || result.IsError() {
		t.Fatalf("batch create failed: %v %q", err, result.GetContent())
	}
	stored, err := GlobalTaskStore().ListTasks(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(stored) != 3 {
		t.Fatalf("want 3 tasks, got %d", len(stored))
	}
	if out := NewTaskCreateTool().FormatResult(result.Data); !strings.Contains(out, "3 tasks created") {
		t.Errorf("unexpected format: %q", out)
	}
}

func TestTaskCreateBatchChecksEveryItemBeforeCreatingAny(t *testing.T) {
	initBatchStore(t)
	result, _ := NewTaskCreateTool().Call(context.Background(), batchInput(map[string]any{
		"tasks": []any{
			map[string]any{"subject": "Fine", "description": "d"},
			map[string]any{"description": "no subject"},
		},
	}), nil)
	if !result.IsError() {
		t.Fatal("a task without a subject must fail the call")
	}
	stored, _ := GlobalTaskStore().ListTasks(context.Background(), "session-1")
	if len(stored) != 0 {
		t.Fatalf("nothing may be created when one item is invalid, got %d", len(stored))
	}
}

func TestTaskCreateSingleFormStillWorks(t *testing.T) {
	initBatchStore(t)
	result, err := NewTaskCreateTool().Call(context.Background(), batchInput(map[string]any{
		"subject": "One task", "description": "d",
	}), nil)
	if err != nil || result.IsError() {
		t.Fatalf("single create failed: %v %q", err, result.GetContent())
	}
	if out := NewTaskCreateTool().FormatResult(result.Data); !strings.Contains(out, "created successfully: One task") {
		t.Errorf("unexpected format: %q", out)
	}
}

func TestTaskUpdateBatchCompletesOneAndStartsTheNext(t *testing.T) {
	initBatchStore(t)
	ctx := context.Background()
	a, _ := GlobalTaskStore().CreateTask(ctx, "session-1", "first", "", "", nil)
	b, _ := GlobalTaskStore().CreateTask(ctx, "session-1", "second", "", "", nil)

	result, err := NewTaskUpdateTool().Call(ctx, batchInput(map[string]any{
		"updates": []any{
			map[string]any{"taskId": a.ID, "status": TaskStatusCompleted},
			map[string]any{"taskId": b.ID, "status": TaskStatusInProgress},
		},
	}), nil)
	if err != nil || result.IsError() {
		t.Fatalf("batch update failed: %v %q", err, result.GetContent())
	}
	gotA, _ := GlobalTaskStore().GetTask(ctx, "session-1", a.ID)
	gotB, _ := GlobalTaskStore().GetTask(ctx, "session-1", b.ID)
	if gotA.Status != TaskStatusCompleted || gotB.Status != TaskStatusInProgress {
		t.Fatalf("want completed/in_progress, got %s/%s", gotA.Status, gotB.Status)
	}
}

func TestTaskUpdateBatchReportsAMissingTaskAndKeepsGoing(t *testing.T) {
	initBatchStore(t)
	ctx := context.Background()
	b, _ := GlobalTaskStore().CreateTask(ctx, "session-1", "real", "", "", nil)

	result, err := NewTaskUpdateTool().Call(ctx, batchInput(map[string]any{
		"updates": []any{
			map[string]any{"taskId": "does-not-exist", "status": TaskStatusCompleted},
			map[string]any{"taskId": b.ID, "status": TaskStatusCompleted},
		},
	}), nil)
	if err != nil || result.IsError() {
		t.Fatalf("batch update failed: %v %q", err, result.GetContent())
	}
	gotB, _ := GlobalTaskStore().GetTask(ctx, "session-1", b.ID)
	if gotB.Status != TaskStatusCompleted {
		t.Fatalf("the second update must still apply, got %s", gotB.Status)
	}
	out := NewTaskUpdateTool().FormatResult(result.Data)
	if !strings.Contains(out, "not updated") || !strings.Contains(out, "Updated task #"+b.ID) {
		t.Errorf("both outcomes must be reported, got %q", out)
	}
}

func TestTaskUpdateSingleFormStillRequiresATaskID(t *testing.T) {
	initBatchStore(t)
	result, _ := NewTaskUpdateTool().Call(context.Background(), batchInput(map[string]any{"status": TaskStatusCompleted}), nil)
	if !result.IsError() {
		t.Fatal("an update with neither taskId nor updates must fail")
	}
}

func TestTaskUpdatePermissionsAcceptTheBatchForm(t *testing.T) {
	tl := NewTaskUpdateTool()
	ctx := context.Background()
	batch := map[string]any{"updates": []any{map[string]any{"taskId": "1", "status": TaskStatusCompleted}}}
	if res := tl.CheckPermissions(ctx, batch, tool.ToolUseContext{}); res.IsDenied() {
		t.Fatal("a batch update must not be denied for lacking a top-level taskId")
	}
	if res := tl.CheckPermissions(ctx, map[string]any{}, tool.ToolUseContext{}); !res.IsDenied() {
		t.Fatal("an update with neither taskId nor updates must be denied")
	}
}
