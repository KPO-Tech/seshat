package permissions

import (
	"context"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// TestResolverEmitsToolPermissionRequiredBeforePrompting guards H6 in
// TODO.md: tool.permission_required was defined and re-exported but nothing
// ever constructed it, so a host that only listens on the runtime-event
// channel (not PromptFn itself) never learned a tool call needed approval.
func TestResolverEmitsToolPermissionRequiredBeforePrompting(t *testing.T) {
	t.Setenv("SESHAT_RUNTIME_ROOT", t.TempDir())
	engine := NewEngine()
	if err := engine.AddRule(PermissionRule{
		Value:    PermissionRuleValue{ToolName: "bash", RuleContent: "echo *"},
		Behavior: types.PermissionBehaviorAsk,
		Priority: 100,
		Reason:   "echo commands require approval in this test",
		Source:   types.PermissionSourceStatic,
	}); err != nil {
		t.Fatalf("failed to add permission rule: %v", err)
	}

	integrator := NewIntegrator(engine)
	var events []types.RuntimeEvent
	promptCalled := false
	integrator.SetPromptFn(func(ctx context.Context, request types.PromptRequest) (types.PromptResponse, error) {
		promptCalled = true
		if len(events) == 0 {
			t.Fatal("expected tool.permission_required to be emitted before the prompt function runs")
		}
		return types.PromptResponse{Value: true}, nil
	})

	ctx := context.WithValue(context.Background(), types.RuntimeEventEmitterKey, func(e types.RuntimeEvent) {
		events = append(events, e)
	})

	resolver := integrator.Resolver("session-1", "turn-1", types.PermissionModeOnRequest)
	result := resolver.ResolvePermission(ctx, types.GlobalToolPermissionRequest(
		"bash",
		map[string]any{"command": "echo hi"},
		"tool-1",
		"session-1",
		"turn-1",
		types.PermissionModeOnRequest,
		"",
		nil,
	))

	if !promptCalled {
		t.Fatal("expected prompt function to be called")
	}
	if !result.IsAllowed() {
		t.Fatalf("expected prompt approval to allow tool use, got %#v", result)
	}

	if len(events) != 1 {
		t.Fatalf("expected exactly 1 runtime event, got %d: %#v", len(events), events)
	}
	event := events[0]
	if event.Type != types.RuntimeEventTypeToolPermissionRequired {
		t.Fatalf("expected event type %q, got %q", types.RuntimeEventTypeToolPermissionRequired, event.Type)
	}
	if event.SessionID != types.SessionID("session-1") {
		t.Errorf("expected session_id=session-1, got %q", event.SessionID)
	}
	if event.PermissionRequest == nil || event.PermissionRequest.ToolName != "bash" {
		t.Fatalf("expected PermissionRequest.ToolName=bash, got %#v", event.PermissionRequest)
	}
}

// TestResolverNoRuntimeEventWithoutEmitter confirms the new emission does
// not panic or otherwise misbehave when no emitter is in context (the
// common case today, since nothing wires one into the permission path yet).
func TestResolverNoRuntimeEventWithoutEmitter(t *testing.T) {
	t.Setenv("SESHAT_RUNTIME_ROOT", t.TempDir())
	engine := NewEngine()
	if err := engine.AddRule(PermissionRule{
		Value:    PermissionRuleValue{ToolName: "bash", RuleContent: "echo *"},
		Behavior: types.PermissionBehaviorAsk,
		Priority: 100,
		Reason:   "echo commands require approval in this test",
		Source:   types.PermissionSourceStatic,
	}); err != nil {
		t.Fatalf("failed to add permission rule: %v", err)
	}

	integrator := NewIntegrator(engine)
	integrator.SetPromptFn(func(ctx context.Context, request types.PromptRequest) (types.PromptResponse, error) {
		return types.PromptResponse{Value: true}, nil
	})

	resolver := integrator.Resolver("session-1", "turn-1", types.PermissionModeOnRequest)
	result := resolver.ResolvePermission(context.Background(), types.GlobalToolPermissionRequest(
		"bash",
		map[string]any{"command": "echo hi"},
		"tool-1",
		"session-1",
		"turn-1",
		types.PermissionModeOnRequest,
		"",
		nil,
	))
	if !result.IsAllowed() {
		t.Fatalf("expected prompt approval to allow tool use, got %#v", result)
	}
}
