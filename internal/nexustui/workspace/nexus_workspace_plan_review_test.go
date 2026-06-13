package workspace

import (
	"context"
	"testing"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/pubsub"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/session"
	tasktool "github.com/EngineerProjects/nexus-engine/internal/tools/task"
	"github.com/EngineerProjects/nexus-engine/pkg/sdk"
)

func TestOnRuntimeEventPublishesPlanReviewSubmission(t *testing.T) {
	w := NewNexusWorkspace(nil, "", "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := w.planBroker.Subscribe(ctx)

	w.OnRuntimeEvent(sdk.RuntimeEvent{
		Type:      sdk.RuntimeEventTypePlanSubmitted,
		SessionID: sdk.SessionID("session-1"),
		PlanEvent: &sdk.PlanRuntimeEvent{
			PlanID:   "plan-1",
			Slug:     "ship-submit-plan",
			Filename: "ship-submit-plan.md",
			Status:   "pending",
			Version:  2,
			Content:  "# Plan\n\n- step",
		},
	})

	select {
	case ev := <-events:
		if ev.Type != pubsub.CreatedEvent {
			t.Fatalf("expected created event, got %q", ev.Type)
		}
		if ev.Payload.SessionID != "session-1" {
			t.Fatalf("expected session-1, got %q", ev.Payload.SessionID)
		}
		if ev.Payload.PlanID != "plan-1" {
			t.Fatalf("expected plan-1, got %q", ev.Payload.PlanID)
		}
		if ev.Payload.Content != "# Plan\n\n- step" {
			t.Fatalf("unexpected content: %q", ev.Payload.Content)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for plan review event")
	}
}

func TestOnRuntimeEventUpdatesLiveExecutionMode(t *testing.T) {
	w := NewNexusWorkspace(nil, "", "")
	w.sessionsMu.Lock()
	w.sessStore["session-1"] = session.Session{ID: "session-1", Title: "Test"}
	w.sessionsMu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := w.sessBroker.Subscribe(ctx)

	w.OnRuntimeEvent(sdk.RuntimeEvent{
		Type:          sdk.RuntimeEventTypeExecutionModeChanged,
		SessionID:     sdk.SessionID("session-1"),
		ExecutionMode: string(sdk.ExecutionModePlan),
	})

	if got := w.liveExecutionMode("session-1"); got != string(sdk.ExecutionModePlan) {
		t.Fatalf("expected live mode %q, got %q", sdk.ExecutionModePlan, got)
	}

	select {
	case ev := <-updates:
		if ev.Type != pubsub.UpdatedEvent {
			t.Fatalf("expected updated event, got %q", ev.Type)
		}
		if ev.Payload.ID != "session-1" {
			t.Fatalf("expected session-1, got %q", ev.Payload.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for session refresh")
	}
}

func TestOnRuntimeEventSyncsSessionTodosFromTaskStore(t *testing.T) {
	path := t.TempDir() + "/tasks.sqlite"
	if err := tasktool.InitializeGlobalTaskStore(path); err != nil {
		t.Fatalf("initialize task store: %v", err)
	}
	ctx := context.Background()
	_, err := tasktool.GlobalTaskStore().CreateTask(ctx, "session-1", "Implement sidebar tasks", "Render a task details panel", "Implementing sidebar tasks", nil)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	w := NewNexusWorkspace(nil, "", "")
	w.sessionsMu.Lock()
	w.sessStore["session-1"] = session.Session{ID: "session-1", Title: "Test"}
	w.sessionsMu.Unlock()

	subCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updates := w.sessBroker.Subscribe(subCtx)

	w.OnRuntimeEvent(sdk.RuntimeEvent{
		Type:      sdk.RuntimeEventTypeTaskChanged,
		SessionID: sdk.SessionID("session-1"),
		TaskEvent: &sdk.TaskRuntimeEvent{Action: "create"},
	})

	select {
	case ev := <-updates:
		if len(ev.Payload.Todos) != 1 {
			t.Fatalf("expected 1 todo, got %d", len(ev.Payload.Todos))
		}
		if ev.Payload.Todos[0].Content != "Implement sidebar tasks" {
			t.Fatalf("unexpected todo content: %q", ev.Payload.Todos[0].Content)
		}
		if ev.Payload.Todos[0].Description != "Render a task details panel" {
			t.Fatalf("unexpected todo description: %q", ev.Payload.Todos[0].Description)
		}
		if ev.Payload.Todos[0].ID == "" {
			t.Fatal("expected todo ID to be populated")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for task session refresh")
	}
}
