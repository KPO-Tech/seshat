package task

import (
	"context"
	"testing"
)

func TestSQLiteTaskStorePersistsTasksBySession(t *testing.T) {
	path := t.TempDir() + "/tasks.sqlite"
	store, err := NewSQLiteTaskStore(path)
	if err != nil {
		t.Fatalf("new sqlite task store: %v", err)
	}

	ctx := context.Background()
	first, err := store.CreateTask(ctx, "session-1", "First task", "Do the first thing", "Doing the first thing", map[string]any{"step": 1})
	if err != nil {
		t.Fatalf("create first task: %v", err)
	}
	if _, err := store.CreateTask(ctx, "session-1", "Second task", "Do the second thing", "Doing the second thing", nil); err != nil {
		t.Fatalf("create second task: %v", err)
	}
	other, err := store.CreateTask(ctx, "session-2", "Other task", "Other session", "Working elsewhere", nil)
	if err != nil {
		t.Fatalf("create other session task: %v", err)
	}
	if first.Position != 1 {
		t.Fatalf("expected first position 1, got %d", first.Position)
	}
	if other.Position != 1 {
		t.Fatalf("expected other session first position 1, got %d", other.Position)
	}
	updated, err := store.UpdateTask(ctx, "session-1", first.ID, map[string]any{"status": TaskStatusInProgress})
	if err != nil {
		t.Fatalf("update task: %v", err)
	}
	if updated.Status != TaskStatusInProgress {
		t.Fatalf("expected updated status %q, got %q", TaskStatusInProgress, updated.Status)
	}

	reloaded, err := NewSQLiteTaskStore(path)
	if err != nil {
		t.Fatalf("reload sqlite task store: %v", err)
	}
	gotSession1, err := reloaded.ListTasks(ctx, "session-1")
	if err != nil {
		t.Fatalf("list session-1 tasks: %v", err)
	}
	if len(gotSession1) != 2 {
		t.Fatalf("expected 2 tasks for session-1, got %d", len(gotSession1))
	}
	if gotSession1[0].ID != first.ID {
		t.Fatalf("expected first task %q, got %q", first.ID, gotSession1[0].ID)
	}
	if gotSession1[0].Status != TaskStatusInProgress {
		t.Fatalf("expected persisted status %q, got %q", TaskStatusInProgress, gotSession1[0].Status)
	}
	gotSession2, err := reloaded.ListTasks(ctx, "session-2")
	if err != nil {
		t.Fatalf("list session-2 tasks: %v", err)
	}
	if len(gotSession2) != 1 || gotSession2[0].ID != other.ID {
		t.Fatalf("unexpected session-2 tasks: %#v", gotSession2)
	}
}

func TestSQLiteTaskStorePersistsBlockRelationships(t *testing.T) {
	path := t.TempDir() + "/tasks.sqlite"
	store, err := NewSQLiteTaskStore(path)
	if err != nil {
		t.Fatalf("new sqlite task store: %v", err)
	}
	ctx := context.Background()
	first, err := store.CreateTask(ctx, "session-1", "First", "", "", nil)
	if err != nil {
		t.Fatalf("create first task: %v", err)
	}
	second, err := store.CreateTask(ctx, "session-1", "Second", "", "", nil)
	if err != nil {
		t.Fatalf("create second task: %v", err)
	}
	if err := store.BlockTask(ctx, "session-1", first.ID, second.ID); err != nil {
		t.Fatalf("block task: %v", err)
	}
	reloaded, err := NewSQLiteTaskStore(path)
	if err != nil {
		t.Fatalf("reload sqlite task store: %v", err)
	}
	firstReloaded, err := reloaded.GetTask(ctx, "session-1", first.ID)
	if err != nil {
		t.Fatalf("get first task: %v", err)
	}
	secondReloaded, err := reloaded.GetTask(ctx, "session-1", second.ID)
	if err != nil {
		t.Fatalf("get second task: %v", err)
	}
	if len(firstReloaded.Blocks) != 1 || firstReloaded.Blocks[0] != second.ID {
		t.Fatalf("unexpected blocks: %#v", firstReloaded.Blocks)
	}
	if len(secondReloaded.BlockedBy) != 1 || secondReloaded.BlockedBy[0] != first.ID {
		t.Fatalf("unexpected blockedBy: %#v", secondReloaded.BlockedBy)
	}
}
