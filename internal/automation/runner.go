package automation

import (
	"context"
	"fmt"
	"strings"

	"github.com/EngineerProjects/seshat/internal/providers"
	"github.com/EngineerProjects/seshat/internal/rag"
	engineconfig "github.com/EngineerProjects/seshat/pkg/config"
	"github.com/EngineerProjects/seshat/pkg/sdk"
)

// RunnerConfig is the base template used to build an SDK client for each
// workflow execution.
type RunnerConfig struct {
	Model          sdk.ModelIdentifier
	ProviderConfig *providers.Config
	MaxTokens      int
	// WebSearchKeys provides per-owner web search provider keys.
	// When set, the web_search tool uses these keys instead of reading from the
	// process environment — required for safe multi-tenant execution.
	WebSearchKeys map[string]string
	// RAGService enables the rag_search/rag_ingest tools for this
	// execution when set. Callers embedding automation in a multi-tenant
	// host (e.g. seshat-ai/seshat-server) are expected to build one scoped
	// to the right organization/corpus namespace per execution, the same
	// way WebSearchKeys is resolved per owner rather than read from a
	// single process-wide config.
	RAGService *rag.Service
	// DoclingURL enables the read_document_url tool when set — fetches
	// and converts a remote document (PDF, webpage, ...) to markdown via a
	// running docling-serve instance. Unlike WebSearchKeys/RAGService this
	// isn't a secret or per-tenant value, so it's fine to read straight
	// from RunnerConfig rather than resolved per execution.
	DoclingURL string
}

// ExecuteConfig holds per-execution overrides applied on top of RunnerConfig.
type ExecuteConfig struct {
	// StreamFn receives each text delta in real time. May be nil.
	StreamFn func(string)
	// ModelOverride specifies a different model for this execution only.
	// Format: "provider:model". Empty means use RunnerConfig.Model.
	ModelOverride string
	// SystemPrompt replaces the entire Seshat default system prompt.
	// Empty means use the default.
	SystemPrompt string
}

// Runner creates a fresh SDK client for each workflow execution.
// It holds no mutable state, making it safe for concurrent use.
type Runner struct {
	cfg RunnerConfig
}

// NewRunner builds a Runner from cfg.
func NewRunner(cfg RunnerConfig) (*Runner, error) {
	return &Runner{cfg: cfg}, nil
}

// Execute runs w against a fresh SDK client and session.
// A new client is created for every call, which ensures ExecuteConfig
// overrides (model, system prompt) are fully isolated between executions.
func (r *Runner) Execute(ctx context.Context, w Workflow, ec ExecuteConfig) error {
	model := r.cfg.Model
	if strings.TrimSpace(ec.ModelOverride) != "" {
		model = engineconfig.ParseModelIdentifier(ec.ModelOverride)
		if !engineconfig.HasExplicitProviderPrefix(ec.ModelOverride) {
			if p := engineconfig.DetectProviderFromModel(ec.ModelOverride); p != "" {
				model.Provider = p
			}
		}
	}

	clientCfg := &sdk.ClientConfig{
		APIKey:                 r.cfg.ProviderConfig.APIKey,
		Model:                  model,
		PermissionMode:         sdk.PermissionModeBypass,
		MaxTokens:              r.cfg.MaxTokens,
		AutoCompact:            false,
		PersistSessions:        false,
		DisableTitleGeneration: true,
		EnableMemory:           false,
		EnableHooks:            false,
		EnableMonitoring:       false,
		ProviderConfig:         r.cfg.ProviderConfig,
		WebSearchKeys:          r.cfg.WebSearchKeys,
		RAGService:             r.cfg.RAGService,
		DoclingURL:             r.cfg.DoclingURL,
	}

	if ec.StreamFn != nil {
		streamFn := ec.StreamFn
		clientCfg.ResponseChunkFn = func(chunk sdk.ResponseChunk) {
			if chunk.Delta != "" {
				streamFn(chunk.Delta)
			}
		}
	}

	if ec.SystemPrompt != "" {
		sp := ec.SystemPrompt
		clientCfg.PromptConfig = &sdk.PromptConfig{SystemPrompt: &sp}
	}

	client, err := sdk.NewClient(clientCfg)
	if err != nil {
		return fmt.Errorf("automation runner: %w", err)
	}
	defer client.Close()

	session, err := client.CreateSession(ctx)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	return w.Run(ctx, session)
}

// Close is a no-op — Runner holds no long-lived resources.
func (r *Runner) Close() {}
