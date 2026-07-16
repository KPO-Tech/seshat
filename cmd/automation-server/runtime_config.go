package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/KPO-Tech/seshat/internal/automation"
	"github.com/KPO-Tech/seshat/internal/providers"
	engineconfig "github.com/KPO-Tech/seshat/pkg/config"
)

// ─── Response types (mirror seshat-ai/internal/api/automation_runtime.go) ────

type runtimeProviderConfig struct {
	Name    string `json:"name"`
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	ModelID string `json:"model_id"`
}

type runtimeWebSearch struct {
	Enabled   bool              `json:"enabled"`
	Providers map[string]string `json:"providers"`
}

type runtimeAgentConfig struct {
	Slug         string   `json:"slug"`
	Model        string   `json:"model,omitempty"`
	SystemPrompt string   `json:"system_prompt,omitempty"`
	Tools        []string `json:"tools,omitempty"`
	MaxTurns     int      `json:"max_turns,omitempty"`
}

type runtimeConfig struct {
	Provider  runtimeProviderConfig `json:"provider"`
	WebSearch runtimeWebSearch      `json:"web_search"`
	// Agent is present only when ?agent_slug= was requested and resolved.
	Agent *runtimeAgentConfig `json:"agent,omitempty"`
}

// ─── Client ───────────────────────────────────────────────────────────────────

// RuntimeConfigClient fetches per-user execution config from seshat-ai.
type RuntimeConfigClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewRuntimeConfigClient returns nil when baseURL is empty.
func NewRuntimeConfigClient(baseURL, apiKey string) *RuntimeConfigClient {
	if strings.TrimSpace(baseURL) == "" {
		return nil
	}
	return &RuntimeConfigClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Fetch calls seshat-ai and returns the runtime config for ownerID.
// agentSlug is optional; when non-empty the named agent definition is resolved
// and included in the response under the "agent" key.
func (c *RuntimeConfigClient) Fetch(ctx context.Context, ownerID, agentSlug string) (*runtimeConfig, error) {
	u, err := url.Parse(c.baseURL + "/api/v1/automation/runtime-config")
	if err != nil {
		return nil, fmt.Errorf("build runtime-config URL: %w", err)
	}
	if agentSlug != "" {
		q := u.Query()
		q.Set("agent_slug", agentSlug)
		u.RawQuery = q.Encode()
	}
	rawURL := u.String()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("X-Seshat-User-ID", ownerID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("seshat-ai runtime-config: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("seshat-ai runtime-config returned %d: %s", resp.StatusCode, string(body))
	}

	var cfg runtimeConfig
	if err := json.Unmarshal(body, &cfg); err != nil {
		return nil, fmt.Errorf("parse runtime-config: %w", err)
	}
	return &cfg, nil
}

// ToRunnerConfig converts the fetched config into an automation.RunnerConfig and
// the resolved base AgentConfig (populated only when rc.Agent != nil).
// fallbackModel is used when seshat-ai returns no model override.
func (rc *runtimeConfig) ToRunnerConfig(fallbackModel string) (automation.RunnerConfig, automation.AgentConfig, error) {
	// Determine model: job-specified → seshat-ai provider default → daemon fallback
	modelRaw := fallbackModel
	if rc.Provider.ModelID != "" && modelRaw == "" {
		modelRaw = rc.Provider.Name + ":" + rc.Provider.ModelID
	}
	if modelRaw == "" {
		modelRaw = "anthropic:claude-sonnet-4-6"
	}

	model := engineconfig.ParseModelIdentifier(modelRaw)
	if !engineconfig.HasExplicitProviderPrefix(modelRaw) {
		if p := engineconfig.DetectProviderFromModel(modelRaw); p != "" {
			model.Provider = p
		}
	}

	// Build provider config from seshat-ai response.
	// Falls back to the existing default config for the provider (e.g. correct base URL).
	providerCfg := providers.GetProviderConfig(model.Provider)
	if providerCfg == nil {
		providerCfg = &providers.Config{Provider: model.Provider}
	}
	if rc.Provider.APIKey != "" {
		providerCfg.APIKey = rc.Provider.APIKey
	}
	if rc.Provider.BaseURL != "" {
		providerCfg.BaseURL = rc.Provider.BaseURL
	}

	// Build per-owner web search keys map. These are injected into the tool
	// at execution time via RunnerConfig.WebSearchKeys — no os.Setenv, so
	// concurrent jobs from different owners cannot leak each other's keys.
	var webSearchKeys map[string]string
	if rc.WebSearch.Enabled && len(rc.WebSearch.Providers) > 0 {
		webSearchKeys = make(map[string]string, len(rc.WebSearch.Providers))
		for provider, key := range rc.WebSearch.Providers {
			if key != "" {
				webSearchKeys[provider] = key
			}
		}
	}

	runnerCfg := automation.RunnerConfig{
		Model:          model,
		ProviderConfig: providerCfg,
		MaxTokens:      8192,
		WebSearchKeys:  webSearchKeys,
	}

	// Build base agent config from resolved named agent (if any).
	var agentCfg automation.AgentConfig
	if rc.Agent != nil {
		agentCfg = automation.AgentConfig{
			Slug:         rc.Agent.Slug,
			Model:        rc.Agent.Model,
			SystemPrompt: rc.Agent.SystemPrompt,
			Tools:        rc.Agent.Tools,
			MaxTurns:     rc.Agent.MaxTurns,
		}
	}

	return runnerCfg, agentCfg, nil
}
