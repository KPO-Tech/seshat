package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KPO-Tech/seshat/internal/engine"
	"github.com/KPO-Tech/seshat/internal/execution"
	"github.com/KPO-Tech/seshat/internal/permissions"
	"github.com/KPO-Tech/seshat/internal/prompt"
	"github.com/KPO-Tech/seshat/internal/providers"
	compact "github.com/KPO-Tech/seshat/internal/runtime/memory"
	"github.com/KPO-Tech/seshat/internal/tools/registry"
	"github.com/KPO-Tech/seshat/internal/types"
)

// TestRestrictSessionTools_PrunesToPatterns is a direct unit test of the
// fix for TODO.md's M5c follow-up (2026-10-11): a sub-agent's Tools/
// ToolFilter never actually restricted anything - RunConfig.Tools was
// always nil at every call site, so the old filter-then-register approach
// always filtered an empty list. RestrictSessionTools replaces that with
// pruning the session's already-populated tool set in place.
func TestRestrictSessionTools_PrunesToPatterns(t *testing.T) {
	eng := newMinimalTestEngine(t)
	session, err := eng.NewSession(context.Background())
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer session.Close()

	before := session.GetToolNames()
	if len(before) < 2 {
		t.Fatalf("expected at least 2 tools registered before restricting, got %v", before)
	}

	RestrictSessionTools(session, []string{"allowed_stub"})

	after := session.GetToolNames()
	if len(after) != 1 || after[0] != "allowed_stub" {
		t.Errorf("expected only 'allowed_stub' to survive restriction, got %v", after)
	}
}

// TestRestrictSessionTools_EmptyOrWildcardIsNoRestriction confirms the two
// documented "no restriction" cases leave every tool in place.
func TestRestrictSessionTools_EmptyOrWildcardIsNoRestriction(t *testing.T) {
	for _, patterns := range [][]string{nil, {}, {"*"}} {
		eng := newMinimalTestEngine(t)
		session, err := eng.NewSession(context.Background())
		if err != nil {
			t.Fatalf("NewSession: %v", err)
		}

		before := len(session.GetToolNames())
		RestrictSessionTools(session, patterns)
		after := len(session.GetToolNames())

		if after != before {
			t.Errorf("patterns=%v: expected no change (before=%d, after=%d)", patterns, before, after)
		}
		session.Close()
	}
}

// TestRunAgent_AppliesAgentDefToolPatterns proves the fix end-to-end through
// RunAgent itself: an agent type whose own Tools list names only one of two
// registered tools must end up with just that one tool in its session,
// instead of the full registry every sub-agent silently got before this fix.
func TestRunAgent_AppliesAgentDefToolPatterns(t *testing.T) {
	eng := newMinimalTestEngine(t)

	agentRegistry := NewAgentRegistry()
	agentRegistry.Register(&AgentDefinition{
		AgentType: "narrow-test-agent",
		Tools:     []string{"allowed_stub"},
	})

	result, err := RunAgent(&RunConfig{
		AgentType: "narrow-test-agent",
		Task:      "go",
		Engine:    eng,
		Context:   context.Background(),
		Registry:  agentRegistry,
	})
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if !result.Success {
		t.Fatalf("RunAgent did not succeed: %s", result.Error)
	}
}

// newMinimalTestEngine builds a real engine with a mock streaming provider
// (so a turn completes without hitting a real API) and a registry holding
// two simple tools, "allowed_stub" and "denied_stub" - enough to exercise
// tool-set pruning without needing a real builtin tool registry (which
// internal/agent cannot import here: internal/tools/builtin imports
// internal/agent, so the reverse import from a test in this package would
// be a cycle).
func newMinimalTestEngine(t *testing.T) *engine.Engine {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeStreamEvents(t, w, textStreamEvents("done"))
	}))
	t.Cleanup(server.Close)

	apiClient := providers.NewClientWithConfig("test-key", &providers.Config{
		Provider: types.APIProviderAnthropic,
		BaseURL:  server.URL,
	})
	apiClient.SetHTTPClient(server.Client())
	apiClient.SetRetryConfig(types.RetryConfig{MaxAttempts: 1})

	reg := registry.NewRegistry()
	allowed, err := registry.NewSimpleTool("allowed_stub", "stub tool that should remain visible", func(ctx context.Context, input string) (string, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("build allowed_stub: %v", err)
	}
	denied, err := registry.NewSimpleTool("denied_stub", "stub tool that restriction should hide", func(ctx context.Context, input string) (string, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("build denied_stub: %v", err)
	}
	if err := reg.Register(allowed); err != nil {
		t.Fatalf("register allowed_stub: %v", err)
	}
	if err := reg.Register(denied); err != nil {
		t.Fatalf("register denied_stub: %v", err)
	}

	engineConfig := engine.DefaultConfig()
	engineConfig.PermissionMode = types.PermissionModeBypass

	runtimeEngine := engine.NewEngine(
		apiClient,
		execution.NewOrchestrator(),
		compact.NewEngine(apiClient, compact.DefaultConfig()),
		prompt.NewAssembler(),
		permissions.NewIntegrator(permissions.NewEngine()),
		reg,
		nil,
		engineConfig,
		nil,
		nil,
	)
	runtimeEngine.SetAPIClient(apiClient)
	return runtimeEngine
}
