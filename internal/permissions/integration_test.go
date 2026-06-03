package permissions

import (
	"context"
	"testing"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// ─── mock classifier for e2e tests ───────────────────────────────────────────

type e2eClassifier struct {
	allowed bool
	reason  string
}

func (c *e2eClassifier) Classify(_ context.Context, _ string, _ map[string]any) (Classification, error) {
	return Classification{Allowed: c.allowed, Confidence: 0.95, Reason: c.reason}, nil
}

func TestResolverUsesPromptFnApproval(t *testing.T) {
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
	promptCalled := false
	integrator.SetPromptFn(func(ctx context.Context, request types.PromptRequest) (types.PromptResponse, error) {
		promptCalled = true
		if request.Type != types.PromptTypeConfirm {
			t.Fatalf("expected confirm prompt, got %q", request.Type)
		}
		if got := request.Metadata["tool_name"]; got != "bash" {
			t.Fatalf("expected tool metadata for bash, got %#v", got)
		}
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

	if !promptCalled {
		t.Fatal("expected prompt function to be called")
	}
	if !result.IsAllowed() {
		t.Fatalf("expected prompt approval to allow tool use, got %#v", result)
	}
	if result.DecisionReason == nil || result.DecisionReason.Source != "prompt" {
		t.Fatalf("expected prompt decision reason, got %#v", result.DecisionReason)
	}
	if got := result.UpdatedInput["command"]; got != "echo hi" {
		t.Fatalf("expected updated input to preserve command, got %#v", got)
	}
}

// TestIntegratorAutoModeClassifierAllows verifies the full path:
// engine + auto-mode + mock classifier → allow decision.
func TestIntegratorAutoModeClassifierAllows(t *testing.T) {
	engine := NewEngine()
	engine.SetClassifier(&e2eClassifier{allowed: true, reason: "safe operation"})

	integrator := NewIntegrator(engine)
	resolver := integrator.ResolverWithContext("s1", "t1", nil, nil)

	result := resolver.ResolvePermission(context.Background(), types.GlobalToolPermissionRequest(
		"bash",
		map[string]any{"command": "ls"},
		"tu-1",
		"s1",
		"t1",
		types.PermissionModeAuto,
		"",
		nil,
	))

	if !result.IsAllowed() {
		t.Fatalf("expected allow from classifier, got %+v (reason: %v)", result.Behavior, result.DecisionReason)
	}
}

// TestIntegratorAutoModeClassifierDenies verifies the full path:
// engine + auto-mode + mock classifier → deny decision.
func TestIntegratorAutoModeClassifierDenies(t *testing.T) {
	engine := NewEngine()
	engine.SetClassifier(&e2eClassifier{allowed: false, reason: "dangerous command"})

	integrator := NewIntegrator(engine)
	resolver := integrator.ResolverWithContext("s1", "t1", nil, nil)

	result := resolver.ResolvePermission(context.Background(), types.GlobalToolPermissionRequest(
		"bash",
		map[string]any{"command": "rm -rf /"},
		"tu-2",
		"s1",
		"t1",
		types.PermissionModeAuto,
		"",
		nil,
	))

	if !result.IsDenied() {
		t.Fatalf("expected deny from classifier, got %+v (reason: %v)", result.Behavior, result.DecisionReason)
	}
}

// TestIntegratorDenyRuleTakesPrecedenceOverAutoMode verifies that an explicit
// deny rule fires before the auto-mode classifier is consulted.
func TestIntegratorDenyRuleTakesPrecedenceOverAutoMode(t *testing.T) {
	engine := NewEngine()
	// Classifier would allow, but deny rule should win.
	engine.SetClassifier(&e2eClassifier{allowed: true, reason: "would allow"})
	if err := engine.AddRule(PermissionRule{
		Value:    PermissionRuleValue{ToolName: "bash", RuleContent: "rm *"},
		Behavior: types.PermissionBehaviorDeny,
		Priority: 1000,
		Reason:   "rm commands are always denied",
		Source:   types.PermissionSourceStatic,
	}); err != nil {
		t.Fatalf("failed to add rule: %v", err)
	}

	integrator := NewIntegrator(engine)
	resolver := integrator.ResolverWithContext("s1", "t1", nil, nil)

	result := resolver.ResolvePermission(context.Background(), types.GlobalToolPermissionRequest(
		"bash",
		map[string]any{"command": "rm foo"},
		"tu-3",
		"s1",
		"t1",
		types.PermissionModeAuto,
		"",
		nil,
	))

	if !result.IsDenied() {
		t.Fatalf("expected deny rule to fire, got %+v", result.Behavior)
	}
}

func TestResolverUsesPromptFnDenial(t *testing.T) {
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
		return types.PromptResponse{Value: false}, nil
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

	if !result.IsDenied() {
		t.Fatalf("expected prompt denial to deny tool use, got %#v", result)
	}
	if result.DecisionReason == nil || result.DecisionReason.Source != "prompt" {
		t.Fatalf("expected prompt decision reason, got %#v", result.DecisionReason)
	}
}
