package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	coreagent "github.com/KPO-Tech/seshat-runtime/internal/agent"
	"github.com/KPO-Tech/seshat-runtime/internal/engine"
	"github.com/KPO-Tech/seshat-runtime/internal/execution"
	"github.com/KPO-Tech/seshat-runtime/internal/permissions"
	"github.com/KPO-Tech/seshat-runtime/internal/prompt"
	"github.com/KPO-Tech/seshat-runtime/internal/providers"
	compact "github.com/KPO-Tech/seshat-runtime/internal/runtime/memory"
	"github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// writeSSEEvents writes a sequence of Anthropic Messages SSE events. The
// engine's loop always requests with Stream: true, so a mock provider must
// speak SSE - a plain JSON body parses as an empty stream and the turn
// silently ends after one call (see internal/agent/runner_skills_restriction_test.go
// for the full explanation and a tool_use variant of this same mock).
func writeSSEEvents(t *testing.T, w http.ResponseWriter, events []map[string]any) {
	t.Helper()
	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal stream event: %v", err)
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
			t.Fatalf("write stream event: %v", err)
		}
	}
}

func textOnlySSEEvents(text string) []map[string]any {
	return []map[string]any{
		{
			"type":          "content_block_start",
			"content_block": map[string]any{"type": "text", "text": ""},
		},
		{
			"type":  "content_block_delta",
			"delta": map[string]any{"type": "text_delta", "text": text},
		},
		{"type": "content_block_stop"},
		{
			"type":  "message_delta",
			"delta": map[string]any{"stop_reason": "end_turn"},
			"usage": map[string]any{"input_tokens": 10, "output_tokens": 5},
		},
		{"type": "message_stop"},
	}
}

func newSkillForkTestEngine(t *testing.T, responseText string) *engine.Engine {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSEEvents(t, w, textOnlySSEEvents(responseText))
	}))
	t.Cleanup(server.Close)

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
	return runtimeEngine
}

func TestSkillForkRunner_RunForkedSkill_ReturnsSubAgentOutput(t *testing.T) {
	runtimeEngine := newSkillForkTestEngine(t, "forked skill completed successfully")
	runner := NewSkillForkRunner(runtimeEngine)

	output, err := runner.RunForkedSkill(context.Background(), "my-skill", "do the thing", []string{"read_file"}, "", types.PermissionModeOnRequest)
	if err != nil {
		t.Fatalf("RunForkedSkill: %v", err)
	}
	if !strings.Contains(output, "forked skill completed successfully") {
		t.Fatalf("expected the sub-agent's output, got: %q", output)
	}
}

func TestSkillForkRunner_RunForkedSkill_NilEngine_ReturnsError(t *testing.T) {
	runner := NewSkillForkRunner(nil)

	_, err := runner.RunForkedSkill(context.Background(), "my-skill", "do the thing", nil, "", types.PermissionModeOnRequest)
	if err == nil {
		t.Fatal("expected an error when no engine is configured")
	}
}

func TestSkillForkRunner_RunForkedSkill_RespectsDepthGuard(t *testing.T) {
	runtimeEngine := newSkillForkTestEngine(t, "should never be reached")
	runner := NewSkillForkRunner(runtimeEngine)

	// Build a context already at the maximum depth, same pattern as
	// TestRunAgent_DepthGuard_Rejects in agent_tool_test.go.
	ctx := context.Background()
	for i := 0; i < coreagent.MaxSubAgentDepth; i++ {
		ctx = withSubAgentDepth(ctx)
	}

	_, err := runner.RunForkedSkill(ctx, "my-skill", "do the thing", nil, "", types.PermissionModeOnRequest)
	if err == nil {
		t.Fatal("expected a depth-guard rejection at or past the max sub-agent depth")
	}
	if !strings.Contains(err.Error(), "depth") {
		t.Fatalf("expected error to mention 'depth', got: %v", err)
	}
}
