package engine

import (
	"strings"

	"github.com/KPO-Tech/seshat/internal/hooks"
	"github.com/KPO-Tech/seshat/internal/prompt"
	"github.com/KPO-Tech/seshat/internal/providers"
	"github.com/KPO-Tech/seshat/internal/types"
)

// Fork returns a new Engine configured for a specific agent persona. It shares
// read-only resources (apiClient, orchestrator, compactor, promptAssembler,
// permissionIntegrator, toolRegistry, sessionStore, monitoring, browserManager)
// with the parent but creates an independent Loop so concurrent agent sessions
// never race on shared mutable loop state.
//
// systemPrompt is injected as the session's system-prompt template verbatim.
// modelStr, when non-empty, must be in "provider:model" format and overrides
// the parent model; pass an empty string to keep the parent model.
func (e *Engine) Fork(systemPrompt, modelStr string) *Engine {
	cfg := *e.config
	cfg.SystemPromptTemplate = systemPrompt
	if modelStr != "" {
		cfg.Model = forkParseModel(modelStr, e.config.Model)
	}

	var providerConfig *providers.Config
	if e.apiClient != nil {
		providerConfig = e.apiClient.Config()
	}

	hookRegistry := hooks.NewRegistry()
	hookExecutor := hooks.NewExecutor(hookRegistry)
	loop := NewLoop(
		e.apiClient,
		e.orchestrator,
		e.compactor,
		e.promptAssembler,
		e.permissionIntegrator,
		hookExecutor,
		loopConfigFromConfig(&cfg),
		e.monitoring,
		providerConfig,
	)

	return &Engine{
		apiClient:            e.apiClient,
		orchestrator:         e.orchestrator,
		compactor:            e.compactor,
		promptAssembler:      e.promptAssembler,
		promptBuilder:        prompt.NewBuilder(e.promptAssembler, prompt.DefaultBuilderConfig()),
		loop:                 loop,
		permissionIntegrator: e.permissionIntegrator,
		toolRegistry:         e.toolRegistry,
		sessionStore:         e.sessionStore,
		config:               &cfg,
		hookRegistry:         hookRegistry,
		hookExecutor:         hookExecutor,
		monitoring:           e.monitoring,
		browserManager:       e.browserManager,
		// memoryService intentionally omitted: agent sessions do not share
		// the interactive user's memory store.
	}
}

// forkParseModel parses a "provider:model" string into a ModelIdentifier,
// falling back to the provided default when the string is empty or has no
// recognisable provider prefix.
func forkParseModel(raw string, fallback types.ModelIdentifier) types.ModelIdentifier {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	provider, model, ok := strings.Cut(raw, ":")
	if !ok || strings.TrimSpace(model) == "" {
		return types.ModelIdentifier{Provider: fallback.Provider, Model: raw}
	}
	return types.ModelIdentifier{
		Provider: types.APIProvider(strings.TrimSpace(provider)),
		Model:    strings.TrimSpace(model),
	}
}
