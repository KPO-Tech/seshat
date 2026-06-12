package workspace

import (
	"context"
	"testing"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/nexustui/pubsub"
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
