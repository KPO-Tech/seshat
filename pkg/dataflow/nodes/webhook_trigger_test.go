package nodes

import (
	"context"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/pkg/dataflow"
)

func TestWebhookTriggerDescriptionIsATrigger(t *testing.T) {
	desc := NewWebhookTrigger().Description()
	if desc.Type != "webhook_trigger" {
		t.Fatalf("expected type webhook_trigger, got %q", desc.Type)
	}
	if !desc.IsTrigger {
		t.Fatal("expected IsTrigger to be true")
	}
	if desc.Category != "Trigger" {
		t.Fatalf("expected category Trigger, got %q", desc.Category)
	}
}

func TestWebhookTriggerValidateParametersAcceptsKnownMethods(t *testing.T) {
	n := NewWebhookTrigger()
	for _, m := range []string{"GET", "post", "Put", "PATCH", "delete"} {
		if err := n.ValidateParameters(map[string]any{"method": m}); err != nil {
			t.Fatalf("expected method %q to pass, got %v", m, err)
		}
	}
}

func TestWebhookTriggerValidateParametersAllowsEmptyMethod(t *testing.T) {
	n := NewWebhookTrigger()
	if err := n.ValidateParameters(map[string]any{}); err != nil {
		t.Fatalf("expected empty method to pass (server defaults it), got %v", err)
	}
}

func TestWebhookTriggerValidateParametersRejectsUnknownMethod(t *testing.T) {
	n := NewWebhookTrigger()
	if err := n.ValidateParameters(map[string]any{"method": "TRACE"}); err == nil {
		t.Fatal("expected error for unsupported method")
	}
}

func TestWebhookTriggerValidateParametersAcceptsKnownResponseModes(t *testing.T) {
	n := NewWebhookTrigger()
	for _, mode := range []string{"", WebhookResponseModeImmediate, WebhookResponseModeWhenFinished} {
		if err := n.ValidateParameters(map[string]any{"responseMode": mode}); err != nil {
			t.Fatalf("expected responseMode %q to pass, got %v", mode, err)
		}
	}
}

func TestWebhookTriggerValidateParametersRejectsUnknownResponseMode(t *testing.T) {
	n := NewWebhookTrigger()
	if err := n.ValidateParameters(map[string]any{"responseMode": "streaming"}); err == nil {
		t.Fatal("expected error for an unsupported responseMode")
	}
}

func TestWebhookTriggerExecutePassesInputThrough(t *testing.T) {
	n := NewWebhookTrigger()
	out, err := n.Execute(context.Background(), nil, []dataflow.Item{{"x": 1}}, map[string]any{"method": "POST"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(out.Ports["main"]) != 1 {
		t.Fatalf("expected input passed through, got %#v", out.Ports)
	}
}
