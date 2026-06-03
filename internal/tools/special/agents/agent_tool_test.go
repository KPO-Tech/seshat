package agents

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	coreagent "github.com/EngineerProjects/nexus-engine/internal/agent"
	"github.com/EngineerProjects/nexus-engine/internal/engine"
	"github.com/EngineerProjects/nexus-engine/internal/execution"
	"github.com/EngineerProjects/nexus-engine/internal/permissions"
	"github.com/EngineerProjects/nexus-engine/internal/prompt"
	"github.com/EngineerProjects/nexus-engine/internal/providers"
	compact "github.com/EngineerProjects/nexus-engine/internal/runtime/memory"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/contract"
	registry "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// ---------------------------------------------------------------------------
// Unit tests — per-instance engine isolation
// ---------------------------------------------------------------------------

// Two AgentTool instances must hold independent engine references.
// Setting one must not affect the other.
func TestAgentTool_SetEngine_PerInstanceIsolation(t *testing.T) {
	a1 := NewAgentTool(nil)
	a2 := NewAgentTool(nil)

	// Simulate two SDK clients with their own engines (use non-nil *Engine pointers
	// via minimal constructor; nil fields are OK for this isolation test).
	eng1 := &engine.Engine{}
	eng2 := &engine.Engine{}

	a1.SetEngine(eng1)
	a2.SetEngine(eng2)

	if a1.engine != eng1 {
		t.Error("a1.engine should point to eng1")
	}
	if a2.engine != eng2 {
		t.Error("a2.engine should point to eng2")
	}
	if a1.engine == a2.engine {
		t.Error("a1 and a2 must NOT share the same engine reference")
	}
}

// SetEngine on one instance must not mutate the other instance.
func TestAgentTool_SetEngine_DoesNotAffectOtherInstance(t *testing.T) {
	a1 := NewAgentTool(nil)
	a2 := NewAgentTool(nil)

	eng := &engine.Engine{}
	a1.SetEngine(eng)

	if a2.engine != nil {
		t.Error("setting engine on a1 must not affect a2; a2.engine should remain nil")
	}
}

// When engine is nil, runAgent must return an error result rather than panic.
func TestAgentTool_runAgent_NilEngine_ReturnsError(t *testing.T) {
	a := NewAgentTool(nil)
	// Do NOT call SetEngine — engine is nil.

	result := a.runAgent(nil, "general", "do something", 1, nil, nil) //nolint:staticcheck
	if result == nil {
		t.Fatal("expected non-nil RunResult, got nil")
	}
	if result.Success {
		t.Error("expected Success=false when engine is nil")
	}
	if result.Error == "" {
		t.Error("expected non-empty Error when engine is nil")
	}
}

// When engine is nil, runForkAgent must return an error result rather than panic.
func TestAgentTool_runForkAgent_NilEngine_ReturnsError(t *testing.T) {
	a := NewAgentTool(nil)

	result := a.runForkAgent(nil, "general", "do something", 1, nil, nil, nil) //nolint:staticcheck
	if result == nil {
		t.Fatal("expected non-nil RunResult, got nil")
	}
	if result.Success {
		t.Error("expected Success=false when engine is nil")
	}
	if result.Error == "" {
		t.Error("expected non-empty Error when engine is nil")
	}
}

// AgentTool.Call must return a structured error when the engine is nil,
// not a panic, for the synchronous (non-background, non-fork) path.
func TestAgentTool_Call_NilEngine_ReturnsErrorResult(t *testing.T) {
	a := NewAgentTool(nil) // no engine set

	// Build a valid-looking raw input for a known agent type.
	raw, _ := json.Marshal(map[string]any{
		"type":   "general",
		"task":   "do something",
		"prompt": "do something",
	})

	result, err := a.Call(nil, tool.CallInput{Raw: string(raw)}, nil) //nolint:staticcheck
	if err != nil {
		t.Fatalf("Call must not return a Go error for missing engine, got: %v", err)
	}
	// The content should contain an error message.
	if result.Content == "" {
		t.Fatal("expected non-empty Content in error result")
	}
	if data, ok := result.Data.(map[string]any); !ok || data["error"] == nil {
		t.Errorf("expected error field in result.Data, got %#v", result.Data)
	}
}

func TestExtractForkMessagesFromToolContext(t *testing.T) {
	parent := []types.Message{
		types.UserMessage("msg-1", "parent context"),
	}
	callInput := tool.CallInput{
		ToolContext: &tool.ToolUseContext{
			Metadata: map[string]any{
				"transcript_messages": parent,
			},
		},
	}

	got := extractForkMessagesFromInput(callInput, nil)
	if len(got) != 1 {
		t.Fatalf("expected one inherited message, got %d", len(got))
	}
	if text := got[0].Content[0].(types.TextContent).Text; text != "parent context" {
		t.Fatalf("expected inherited text %q, got %q", "parent context", text)
	}
}

func TestRunForkedAgentIncludesInheritedMessages(t *testing.T) {
	var capturedPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&capturedPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "msg_test",
			"type":  "message",
			"role":  "assistant",
			"model": "claude-3-5-sonnet-20241022",
			"content": []map[string]any{
				{"type": "text", "text": "done"},
			},
			"stop_reason": "end_turn",
			"usage": map[string]any{
				"input_tokens":  12,
				"output_tokens": 4,
			},
		})
	}))
	defer server.Close()

	apiClient := providers.NewClientWithConfig("test-key", &providers.Config{
		Provider: types.APIProviderAnthropic,
		BaseURL:  server.URL,
	})
	apiClient.SetHTTPClient(server.Client())
	apiClient.SetRetryConfig(types.RetryConfig{MaxAttempts: 1})

	engineConfig := engine.DefaultConfig()
	engineConfig.PermissionMode = types.PermissionModeBypass

	runtimeEngine := engine.NewEngine(
		apiClient,
		execution.NewOrchestrator(),
		compact.NewEngine(apiClient, compact.DefaultConfig()),
		prompt.NewAssembler(),
		permissions.NewIntegrator(permissions.NewEngine()),
		registry.NewRegistry(),
		nil,
		engineConfig,
		nil,
		nil,
	)
	runtimeEngine.SetAPIClient(apiClient)

	result, err := coreagent.RunForkedAgent(&coreagent.RunConfig{
		AgentType:        coreagent.AgentTypeGeneralPurpose,
		Task:             "child task",
		Engine:           runtimeEngine,
		Context:          context.Background(),
		MaxTurns:         1,
		ForkFromMessages: []types.Message{types.UserMessage("parent-1", "parent context")},
	})
	if err != nil {
		t.Fatalf("run forked agent: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error %q", result.Error)
	}

	rawMessages, ok := capturedPayload["messages"].([]any)
	if !ok {
		t.Fatalf("expected provider payload messages, got %#v", capturedPayload["messages"])
	}
	payloadText := stringifyProviderMessages(rawMessages)
	if !strings.Contains(payloadText, "parent context") {
		t.Fatalf("expected inherited parent context in payload, got %q", payloadText)
	}
	if !strings.Contains(payloadText, "child task") {
		t.Fatalf("expected child task in payload, got %q", payloadText)
	}
}

func stringifyProviderMessages(rawMessages []any) string {
	parts := make([]string, 0, len(rawMessages))
	for _, rawMessage := range rawMessages {
		messageMap, ok := rawMessage.(map[string]any)
		if !ok {
			continue
		}
		content, _ := messageMap["content"].([]any)
		for _, rawBlock := range content {
			blockMap, ok := rawBlock.(map[string]any)
			if !ok {
				continue
			}
			if text, _ := blockMap["text"].(string); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

// ---------------------------------------------------------------------------
// Resource limits — spawn depth guard (P requisite for Level 3)
// ---------------------------------------------------------------------------

// subAgentDepth and withSubAgentDepth must round-trip correctly.
func TestSubAgentDepth_RoundTrip(t *testing.T) {
	ctx := context.Background()
	if d := subAgentDepth(ctx); d != 0 {
		t.Errorf("fresh context should have depth 0, got %d", d)
	}
	ctx1 := withSubAgentDepth(ctx)
	if d := subAgentDepth(ctx1); d != 1 {
		t.Errorf("after one withSubAgentDepth, expected depth 1, got %d", d)
	}
	ctx2 := withSubAgentDepth(ctx1)
	if d := subAgentDepth(ctx2); d != 2 {
		t.Errorf("after two withSubAgentDepth, expected depth 2, got %d", d)
	}
}

// runAgent must reject spawn when depth >= MaxSubAgentDepth.
func TestRunAgent_DepthGuard_Rejects(t *testing.T) {
	agentTool := NewAgentTool(nil)

	// Build a context already at the maximum depth.
	ctx := context.Background()
	for i := 0; i < coreagent.MaxSubAgentDepth; i++ {
		ctx = withSubAgentDepth(ctx)
	}

	result := agentTool.runAgent(ctx, coreagent.AgentTypeGeneralPurpose, "test task", 0, nil, nil)
	if result.Success {
		t.Error("expected spawn to be rejected at max depth, but it succeeded")
	}
	if result.Error == "" {
		t.Error("expected a non-empty error message explaining the depth rejection")
	}
	if !strings.Contains(result.Error, "depth") {
		t.Errorf("expected error to mention 'depth', got: %s", result.Error)
	}
}

// runAgent must not reject spawn when depth < MaxSubAgentDepth (engine is nil
// here so it fails at the engine check, not the depth check — that's fine:
// the depth guard does not fire).
func TestRunAgent_DepthGuard_AllowsWithinLimit(t *testing.T) {
	agentTool := NewAgentTool(nil)
	ctx := context.Background()
	// Depth 0 — well within limit.
	result := agentTool.runAgent(ctx, coreagent.AgentTypeGeneralPurpose, "test task", 0, nil, nil)
	// Engine is nil so execution fails, but the error must be about the engine,
	// not about depth.
	if strings.Contains(result.Error, "depth") {
		t.Errorf("depth guard fired unexpectedly at depth 0: %s", result.Error)
	}
}

// runForkAgent must also enforce the depth guard.
func TestRunForkAgent_DepthGuard_Rejects(t *testing.T) {
	agentTool := NewAgentTool(nil)
	ctx := context.Background()
	for i := 0; i < coreagent.MaxSubAgentDepth; i++ {
		ctx = withSubAgentDepth(ctx)
	}
	result := agentTool.runForkAgent(ctx, coreagent.AgentTypeGeneralPurpose, "test", 0, nil, nil, nil)
	if result.Success {
		t.Error("expected fork spawn to be rejected at max depth")
	}
	if !strings.Contains(result.Error, "depth") {
		t.Errorf("expected error to mention 'depth', got: %s", result.Error)
	}
}
