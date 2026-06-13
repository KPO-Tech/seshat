package task

import (
	"context"
	"testing"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

func TestTaskCreateEmitsRuntimeEvent(t *testing.T) {
	path := t.TempDir() + "/tasks.sqlite"
	if err := InitializeGlobalTaskStore(path); err != nil {
		t.Fatalf("initialize task store: %v", err)
	}
	var emitted types.RuntimeEvent
	ctx := context.WithValue(context.Background(), types.RuntimeEventEmitterKey, func(ev types.RuntimeEvent) {
		emitted = ev
	})
	toolImpl := NewTaskCreateTool()
	result, err := toolImpl.Call(ctx, tool.CallInput{
		SessionID: types.SessionID("session-1"),
		Parsed: map[string]any{
			"subject":     "Implement task sidebar",
			"description": "Persist and display tasks",
		},
		ToolContext: &tool.ToolUseContext{SessionID: types.SessionID("session-1")},
	}, nil)
	if err != nil {
		t.Fatalf("call returned error: %v", err)
	}
	if result.IsError() {
		t.Fatalf("expected success result, got %q", result.GetContent())
	}
	if emitted.Type != types.RuntimeEventTypeTaskChanged {
		t.Fatalf("expected task.changed, got %q", emitted.Type)
	}
	if emitted.SessionID != types.SessionID("session-1") {
		t.Fatalf("expected session-1, got %q", emitted.SessionID)
	}
	if emitted.TaskEvent == nil || emitted.TaskEvent.Action != "create" {
		t.Fatalf("unexpected task event payload: %#v", emitted.TaskEvent)
	}
}

func TestTaskUpdateEmitsDeleteRuntimeEvent(t *testing.T) {
	path := t.TempDir() + "/tasks.sqlite"
	if err := InitializeGlobalTaskStore(path); err != nil {
		t.Fatalf("initialize task store: %v", err)
	}
	ctx := context.Background()
	created, err := GlobalTaskStore().CreateTask(ctx, "session-1", "Remove legacy todo", "", "", nil)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	var emitted types.RuntimeEvent
	emitCtx := context.WithValue(context.Background(), types.RuntimeEventEmitterKey, func(ev types.RuntimeEvent) {
		emitted = ev
	})
	toolImpl := NewTaskUpdateTool()
	result, err := toolImpl.Call(emitCtx, tool.CallInput{
		SessionID: types.SessionID("session-1"),
		Parsed: map[string]any{
			"taskId": created.ID,
			"status": TaskStatusDeleted,
		},
		ToolContext: &tool.ToolUseContext{SessionID: types.SessionID("session-1")},
	}, nil)
	if err != nil {
		t.Fatalf("call returned error: %v", err)
	}
	if result.IsError() {
		t.Fatalf("expected success result, got %q", result.GetContent())
	}
	if emitted.Type != types.RuntimeEventTypeTaskChanged {
		t.Fatalf("expected task.changed, got %q", emitted.Type)
	}
	if emitted.TaskEvent == nil || emitted.TaskEvent.Action != "delete" {
		t.Fatalf("unexpected task event payload: %#v", emitted.TaskEvent)
	}
}

func TestTaskListAddsRenderMetadata(t *testing.T) {
	path := t.TempDir() + "/tasks.sqlite"
	if err := InitializeGlobalTaskStore(path); err != nil {
		t.Fatalf("initialize task store: %v", err)
	}
	ctx := context.Background()
	if _, err := GlobalTaskStore().CreateTask(ctx, "session-1", "Implement sidebar tasks", "", "Implementing sidebar tasks", nil); err != nil {
		t.Fatalf("create task: %v", err)
	}
	toolImpl := NewTaskListTool()
	result, err := toolImpl.Call(ctx, tool.CallInput{
		SessionID: types.SessionID("session-1"),
		Parsed: map[string]any{
			"listType": "todo",
			"status":   "all",
		},
		ToolContext: &tool.ToolUseContext{SessionID: types.SessionID("session-1")},
	}, nil)
	if err != nil {
		t.Fatalf("call returned error: %v", err)
	}
	if result.Metadata == nil || result.Metadata.Additional == nil {
		t.Fatal("expected result metadata")
	}
	if _, ok := result.Metadata.Additional["task_list"]; !ok {
		t.Fatal("expected task_list metadata payload")
	}
}
