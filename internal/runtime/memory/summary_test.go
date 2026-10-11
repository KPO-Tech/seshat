package memory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KPO-Tech/seshat/internal/providers"
	"github.com/KPO-Tech/seshat/internal/types"
)

// fakeChatCompletionServer returns an httptest server that answers any
// request with a minimal OpenAI-shaped chat completion, recording the model
// name the client actually sent.
func fakeChatCompletionServer(t *testing.T, gotModel *string, reply string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		*gotModel = body.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp-1","choices":[{"message":{"role":"assistant","content":"` + reply + `"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(server.Close)
	return server
}

// TestGenerateSummary_NonAnthropicProvider guards H1: a session running
// entirely on a non-Anthropic provider, with no Anthropic key configured at
// all, must still be able to generate a compaction summary - using its own
// provider and model, not a hardcoded claude-3-5-haiku call.
func TestGenerateSummary_NonAnthropicProvider(t *testing.T) {
	var gotModel string
	server := fakeChatCompletionServer(t, &gotModel, "summary text")

	client := providers.NewClientWithConfig("k", &providers.Config{
		Provider: types.APIProviderOpenAI,
		BaseURL:  server.URL,
	})
	client.SetHTTPClient(server.Client())

	engine := NewEngine(client, DefaultConfig())
	sessionModel := types.ModelIdentifier{Provider: types.APIProviderOpenAI, Model: "gpt-4o"}

	summary, err := engine.generateSummary(
		context.Background(),
		[]types.Message{types.UserMessage("m1", "hello")},
		sessionModel,
		types.SessionID("s1"),
		types.TurnID("t1"),
	)
	if err != nil {
		t.Fatalf("generateSummary on a non-Anthropic provider should succeed, got error: %v", err)
	}
	if summary != "summary text" {
		t.Errorf("summary = %q, want %q", summary, "summary text")
	}
	if gotModel != "gpt-4o" {
		t.Errorf("server received model %q, want the session's own model %q (not a hardcoded Anthropic one)", gotModel, "gpt-4o")
	}
}

// TestGenerateSummary_ExplicitOverrideWins confirms an explicitly configured
// SummaryModel still takes precedence over the session model.
func TestGenerateSummary_ExplicitOverrideWins(t *testing.T) {
	var gotModel string
	server := fakeChatCompletionServer(t, &gotModel, "ok")

	client := providers.NewClientWithConfig("k", &providers.Config{
		Provider: types.APIProviderOpenAI,
		BaseURL:  server.URL,
	})
	client.SetHTTPClient(server.Client())

	cfg := DefaultConfig()
	cfg.SummaryModel = types.ModelIdentifier{Provider: types.APIProviderOpenAI, Model: "gpt-4o-mini"}
	engine := NewEngine(client, cfg)

	sessionModel := types.ModelIdentifier{Provider: types.APIProviderOpenAI, Model: "gpt-4o"}
	_, err := engine.generateSummary(
		context.Background(),
		[]types.Message{types.UserMessage("m1", "hi")},
		sessionModel,
		types.SessionID("s1"),
		types.TurnID("t1"),
	)
	if err != nil {
		t.Fatalf("generateSummary: %v", err)
	}
	if gotModel != "gpt-4o-mini" {
		t.Errorf("server received model %q, want the configured override %q", gotModel, "gpt-4o-mini")
	}
}
