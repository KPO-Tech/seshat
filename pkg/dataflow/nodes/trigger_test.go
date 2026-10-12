package nodes

import (
	"context"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/pkg/dataflow"
)

func TestScheduleTriggerDescriptionIsATrigger(t *testing.T) {
	desc := NewScheduleTrigger().Description()
	if desc.Type != "schedule_trigger" {
		t.Fatalf("expected type schedule_trigger, got %q", desc.Type)
	}
	if !desc.IsTrigger {
		t.Fatal("expected IsTrigger to be true")
	}
	if desc.Category != "Trigger" {
		t.Fatalf("expected category Trigger, got %q", desc.Category)
	}
}

func TestScheduleTriggerValidateParametersCron(t *testing.T) {
	n := NewScheduleTrigger()
	if err := n.ValidateParameters(map[string]any{"mode": "cron", "cronExpr": "0 0 * * *"}); err != nil {
		t.Fatalf("expected valid cron to pass, got %v", err)
	}
	if err := n.ValidateParameters(map[string]any{"mode": "cron"}); err == nil {
		t.Fatal("expected error for missing cronExpr")
	}
	// Full cron syntax validation happens server-side (see this node's
	// ValidateParameters doc comment for why) - a non-empty but malformed
	// expression is accepted here by design.
}

func TestScheduleTriggerValidateParametersInterval(t *testing.T) {
	n := NewScheduleTrigger()
	if err := n.ValidateParameters(map[string]any{"mode": "interval", "intervalSeconds": 60}); err != nil {
		t.Fatalf("expected valid interval to pass, got %v", err)
	}
	if err := n.ValidateParameters(map[string]any{"mode": "interval", "intervalSeconds": 0}); err == nil {
		t.Fatal("expected error for non-positive intervalSeconds")
	}
}

func TestScheduleTriggerValidateParametersOnce(t *testing.T) {
	n := NewScheduleTrigger()
	if err := n.ValidateParameters(map[string]any{"mode": "once", "runAt": "2026-01-01T09:00:00Z"}); err != nil {
		t.Fatalf("expected valid runAt to pass, got %v", err)
	}
	if err := n.ValidateParameters(map[string]any{"mode": "once", "runAt": "not a date"}); err == nil {
		t.Fatal("expected error for malformed runAt")
	}
}

func TestScheduleTriggerValidateParametersRejectsUnknownMode(t *testing.T) {
	n := NewScheduleTrigger()
	if err := n.ValidateParameters(map[string]any{"mode": "weekly"}); err == nil {
		t.Fatal("expected error for unknown mode")
	}
}

func TestScheduleTriggerExecutePassesInputThrough(t *testing.T) {
	n := NewScheduleTrigger()
	out, err := n.Execute(context.Background(), nil, []dataflow.Item{{"x": 1}}, map[string]any{"mode": "cron", "cronExpr": "0 0 * * *"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(out.Ports["main"]) != 1 {
		t.Fatalf("expected input passed through, got %#v", out.Ports)
	}
}
