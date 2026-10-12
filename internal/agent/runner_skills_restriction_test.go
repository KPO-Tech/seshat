package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/internal/engine"
	"github.com/KPO-Tech/seshat-runtime/internal/execution"
	"github.com/KPO-Tech/seshat-runtime/internal/permissions"
	"github.com/KPO-Tech/seshat-runtime/internal/prompt"
	"github.com/KPO-Tech/seshat-runtime/internal/providers"
	compact "github.com/KPO-Tech/seshat-runtime/internal/runtime/memory"
	"github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
	skilltool "github.com/KPO-Tech/seshat-runtime/internal/tools/system/skills"
	"github.com/KPO-Tech/seshat-runtime/internal/types"
)

// writeStreamEvents writes a sequence of Anthropic Messages SSE events - the
// engine's loop always requests with Stream: true (LoopConfig.EnableStreaming
// defaults to true and loop.go forces it back to true if it's ever false), so
// a mock provider must speak SSE, not plain JSON, or its response parses as
// an empty stream and the turn silently ends after one call.
func writeStreamEvents(t *testing.T, w http.ResponseWriter, events []map[string]any) {
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

// toolUseStreamEvents is the minimal SSE event sequence for a response that
// calls one tool with a complete (non-chunked) JSON input.
func toolUseStreamEvents(toolID, toolName string, inputJSON string) []map[string]any {
	return []map[string]any{
		{
			"type": "content_block_start",
			"content_block": map[string]any{
				"type":  "tool_use",
				"id":    toolID,
				"name":  toolName,
				"input": map[string]any{},
			},
		},
		{
			"type": "content_block_delta",
			"delta": map[string]any{
				"type":         "input_json_delta",
				"partial_json": inputJSON,
			},
		},
		{"type": "content_block_stop"},
		{
			"type":  "message_delta",
			"delta": map[string]any{"stop_reason": "tool_use"},
			"usage": map[string]any{"input_tokens": 10, "output_tokens": 5},
		},
		{"type": "message_stop"},
	}
}

// textStreamEvents is the minimal SSE event sequence for a final plain-text
// completion.
func textStreamEvents(text string) []map[string]any {
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

func registerAllowedAndBlockedTestSkills(t *testing.T) {
	t.Cleanup(skilltool.ClearBundledSkills)
	skilltool.RegisterBundledSkill(skilltool.BundledSkillDefinition{
		Name:          "allowed-skill",
		Description:   "the one the restriction should let through",
		UserInvocable: true,
		GetPromptForCommand: func(args string, ctx context.Context) ([]skilltool.ContentBlock, error) {
			return []skilltool.ContentBlock{{Type: "text", Text: "ran allowed-skill"}}, nil
		},
	})
	skilltool.RegisterBundledSkill(skilltool.BundledSkillDefinition{
		Name:          "blocked-skill",
		Description:   "the one the restriction should hide",
		UserInvocable: true,
		GetPromptForCommand: func(args string, ctx context.Context) ([]skilltool.ContentBlock, error) {
			return []skilltool.ContentBlock{{Type: "text", Text: "ran blocked-skill"}}, nil
		},
	})
}

// newSkillRestrictionTestEngine builds a real engine whose registry holds
// one shared skill tool (mirroring internal/tools/builtin's real
// registration), backed by a mock Anthropic Messages endpoint that calls
// "skill" with skillArg on the first request, then returns a plain "done" on
// the second (after the tool result is sent back). onSecondRequest, when
// non-nil, is called with the second request's decoded body so the test can
// inspect the tool_result that was actually sent to the model.
func newSkillRestrictionTestEngine(t *testing.T, skillArg string, onSecondRequest func(body map[string]any)) (*engine.Engine, *int) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "text/event-stream")
		if requests == 1 {
			inputJSON, _ := json.Marshal(map[string]string{"skill": skillArg})
			writeStreamEvents(t, w, toolUseStreamEvents("toolu_1", "skill", string(inputJSON)))
			return
		}
		if onSecondRequest != nil {
			onSecondRequest(body)
		}
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
	if err := reg.Register(skilltool.NewSkillTool(nil)); err != nil {
		t.Fatalf("register shared skill tool: %v", err)
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
	return runtimeEngine, &requests
}

func TestRunAgent_SkillsRestriction_AllowedSkillSucceeds(t *testing.T) {
	registerAllowedAndBlockedTestSkills(t)

	var secondBody map[string]any
	runtimeEngine, requests := newSkillRestrictionTestEngine(t, "allowed-skill", func(body map[string]any) {
		secondBody = body
	})

	agentRegistry := NewAgentRegistry()
	agentRegistry.Register(&AgentDefinition{
		AgentType: "restricted-test-allowed",
		Skills:    []string{"allowed-skill"},
	})

	result, err := RunAgent(&RunConfig{
		AgentType: "restricted-test-allowed",
		Task:      "use your allowed skill",
		Engine:    runtimeEngine,
		Context:   context.Background(),
		MaxTurns:  1,
		Registry:  agentRegistry,
	})
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error %q", result.Error)
	}
	if *requests != 2 {
		t.Fatalf("expected exactly 2 model calls (tool_use then final), got %d", *requests)
	}
	if secondBody == nil {
		t.Fatalf("expected to capture the follow-up request")
	}
	payload, _ := json.Marshal(secondBody)
	if !strings.Contains(string(payload), "ran allowed-skill") {
		t.Fatalf("expected the allowed skill's output in the tool result sent back to the model, got: %s", payload)
	}
}

func TestRunAgent_SkillsRestriction_BlockedSkillFailsLookup(t *testing.T) {
	registerAllowedAndBlockedTestSkills(t)

	var secondBody map[string]any
	runtimeEngine, requests := newSkillRestrictionTestEngine(t, "blocked-skill", func(body map[string]any) {
		secondBody = body
	})

	agentRegistry := NewAgentRegistry()
	agentRegistry.Register(&AgentDefinition{
		AgentType: "restricted-test-blocked",
		Skills:    []string{"allowed-skill"},
	})

	result, err := RunAgent(&RunConfig{
		AgentType: "restricted-test-blocked",
		Task:      "try to use a skill outside your allow-list",
		Engine:    runtimeEngine,
		Context:   context.Background(),
		MaxTurns:  1,
		Registry:  agentRegistry,
	})
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected the turn to still complete (the tool returns an error result, it does not fail the turn), got error %q", result.Error)
	}
	if *requests != 2 {
		t.Fatalf("expected exactly 2 model calls (tool_use then final), got %d", *requests)
	}
	if secondBody == nil {
		t.Fatalf("expected to capture the follow-up request carrying the tool result")
	}
	payload, _ := json.Marshal(secondBody)
	if strings.Contains(string(payload), "ran blocked-skill") {
		t.Fatalf("blocked skill must not have executed, but its output appears in the tool result: %s", payload)
	}
	if !strings.Contains(string(payload), "not found") {
		t.Fatalf("expected a not-found error in the tool result sent back to the model, got: %s", payload)
	}
}
