package task

import (
	"context"
	"testing"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

func TestTaskListDefaultsToTodoWhenSessionTasksExist(t *testing.T) {
	path := t.TempDir() + "/tasks.sqlite"
	if err := InitializeGlobalTaskStore(path); err != nil {
		t.Fatalf("initialize task store: %v", err)
	}
	ctx := context.Background()
	if _, err := GlobalTaskStore().CreateTask(ctx, "session-1", "Implement sidebar tasks", "Persist and render tasks", "Implementing sidebar tasks", nil); err != nil {
		t.Fatalf("create task: %v", err)
	}
	toolImpl := NewTaskListTool()
	result, err := toolImpl.Call(ctx, tool.CallInput{
		SessionID: types.SessionID("session-1"),
		Parsed:    map[string]any{},
		ToolContext: &tool.ToolUseContext{
			SessionID: types.SessionID("session-1"),
		},
	}, nil)
	if err != nil {
		t.Fatalf("call returned error: %v", err)
	}
	meta, ok := result.Metadata.Additional["task_list"].(taskListRenderMetadata)
	if !ok {
		t.Fatalf("expected task_list metadata, got %#v", result.Metadata.Additional["task_list"])
	}
	if meta.ListType != "todo" {
		t.Fatalf("expected default listType todo, got %q", meta.ListType)
	}
	if meta.StatusFilter != "all" {
		t.Fatalf("expected default status all, got %q", meta.StatusFilter)
	}
	if meta.Count != 1 || len(meta.TodoTasks) != 1 {
		t.Fatalf("unexpected todo metadata: %#v", meta)
	}
}

func TestTaskGetReturnsTodoTaskByDefault(t *testing.T) {
	path := t.TempDir() + "/tasks.sqlite"
	if err := InitializeGlobalTaskStore(path); err != nil {
		t.Fatalf("initialize task store: %v", err)
	}
	ctx := context.Background()
	created, err := GlobalTaskStore().CreateTask(ctx, "session-1", "Implement task details", "Render a detailed task pane", "Rendering task details", nil)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := GlobalTaskStore().UpdateTask(ctx, "session-1", created.ID, map[string]any{"status": TaskStatusInProgress, "owner": "UI Designer"}); err != nil {
		t.Fatalf("update task: %v", err)
	}
	toolImpl := NewTaskGetTool()
	result, err := toolImpl.Call(ctx, tool.CallInput{
		SessionID: types.SessionID("session-1"),
		Parsed: map[string]any{
			"task_id": created.ID,
		},
		ToolContext: &tool.ToolUseContext{SessionID: types.SessionID("session-1")},
	}, nil)
	if err != nil {
		t.Fatalf("call returned error: %v", err)
	}
	meta, ok := result.Metadata.Additional["task_get"].(taskGetRenderMetadata)
	if !ok {
		t.Fatalf("expected task_get metadata, got %#v", result.Metadata.Additional["task_get"])
	}
	if meta.TaskType != "todo" || meta.Todo == nil {
		t.Fatalf("expected todo task metadata, got %#v", meta)
	}
	if meta.Todo.Subject != "Implement task details" {
		t.Fatalf("unexpected subject: %#v", meta.Todo)
	}
	if meta.Todo.Status != TaskStatusInProgress {
		t.Fatalf("unexpected status: %#v", meta.Todo)
	}
}

func TestTaskStopDeletesTodoTaskByDefault(t *testing.T) {
	path := t.TempDir() + "/tasks.sqlite"
	if err := InitializeGlobalTaskStore(path); err != nil {
		t.Fatalf("initialize task store: %v", err)
	}
	ctx := context.Background()
	created, err := GlobalTaskStore().CreateTask(ctx, "session-1", "Drop obsolete task", "This task should be stopped", "Dropping obsolete task", nil)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	toolImpl := NewTaskStopTool()
	result, err := toolImpl.Call(ctx, tool.CallInput{
		SessionID: types.SessionID("session-1"),
		Parsed: map[string]any{
			"task_id": created.ID,
		},
		ToolContext: &tool.ToolUseContext{SessionID: types.SessionID("session-1")},
	}, nil)
	if err != nil {
		t.Fatalf("call returned error: %v", err)
	}
	if result.IsError() {
		t.Fatalf("expected success result, got %q", result.GetContent())
	}
	if _, err := GlobalTaskStore().GetTask(ctx, "session-1", created.ID); err == nil {
		t.Fatal("expected task to be deleted from store")
	}
	meta, ok := result.Metadata.Additional["task_stop"].(taskStopRenderMetadata)
	if !ok {
		t.Fatalf("expected task_stop metadata, got %#v", result.Metadata.Additional["task_stop"])
	}
	if meta.TaskType != "todo" || meta.Todo == nil {
		t.Fatalf("expected todo stop metadata, got %#v", meta)
	}
}
