package automation

import (
	"context"
	"fmt"
	"strings"

	"github.com/KPO-Tech/seshat/internal/documentreader"
	"github.com/KPO-Tech/seshat/internal/providers"
	"github.com/KPO-Tech/seshat/internal/rag"
	engineconfig "github.com/KPO-Tech/seshat/pkg/config"
	"github.com/KPO-Tech/seshat/pkg/dataflow"
	"github.com/KPO-Tech/seshat/pkg/sdk"
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
	// DocumentReaderURL enables document conversion tools when set. Unlike
	// WebSearchKeys/RAGService this isn't a secret or per-tenant value, so it's
	// fine to read straight from RunnerConfig rather than resolved per execution.
	DocumentReaderURL string
	// DocumentConverter, when set, is used instead of building a service client
	// from DocumentReaderURL. Takes precedence over DocumentReaderURL.
	DocumentConverter documentreader.Converter
	// ArtifactStore backs the SDK client's file I/O (e.g. artifacts written
	// by the bash/file tools) for this execution when set. Like RAGService,
	// callers embedding automation in a multi-tenant host are expected to
	// resolve/build one per execution scoped to the right tenant, rather
	// than relying on ClientConfig's own per-process default (local disk) -
	// see sdk.ClientConfig.ArtifactStore's doc comment for the fallback
	// behavior when left nil.
	ArtifactStore sdk.ArtifactStore
	// MCPServers registers external MCP servers' tools for this execution,
	// same as sdk.ClientConfig.MCPServers - unlike WebSearchKeys/RAGService/
	// ArtifactStore this was never threaded through here at all, so a
	// multi-tenant host embedding automation (seshat-ai/seshat-server) had
	// no way to give a cloud-targeted job access to org- or per-employee-
	// scoped MCP tools (Slack/Outlook/Teams/Gmail via a bridged Bearer
	// token, internal Jira/GitHub servers, ...) the way an interactive
	// session already can. Callers resolving a per-employee bridged token
	// are expected to do so per execution, the same way RAGService is
	// resolved per organization rather than read from a single process-wide
	// config.
	MCPServers []sdk.MCPServerConfig
	// RequireSandbox makes the bash tool refuse to run unconfined instead
	// of silently degrading — see sdk.ClientConfig.RequireSandbox /
	// bash.ToolConfig.RequireSandbox for the underlying mechanism. Leave
	// false for a single-user embedding; set true for multi-tenant server
	// deployments (e.g. seshat-ai/seshat-server's cloud-targeted job
	// execution), where an LLM-issued shell command must never run
	// unconfined on the shared host.
	RequireSandbox bool
	// NodeRegistry, when set, enables Job.Graph execution — a job with a
	// non-nil Graph fails clearly (not silently) if this is nil. Left
	// unset by default: registering pkg/dataflow/nodes/database's node
	// types pulls in Postgres/MySQL/MongoDB/Redis/Elasticsearch client
	// code, so which node types are available is a deployment choice made
	// by whoever builds this Registry (seshat-backend/seshat-server's
	// bootstrap), not something this package opts every embedder into by
	// default.
	NodeRegistry *dataflow.Registry
	// Secrets resolves references used by dataflow nodes that need
	// credentials (e.g. a database DSN) — same per-execution-scoped-by-
	// caller pattern as RAGService/WebSearchKeys above. nil is valid for a
	// Graph that uses no credential-needing node type.
	Secrets dataflow.SecretResolver
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

	clientCfg := r.buildClientConfig(model)

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

// buildClientConfig builds the sdk.ClientConfig for one execution against
// the given (possibly per-call-overridden) model — pulled out of Execute as
// a pure function so RunnerConfig's field-by-field propagation into
// ClientConfig (RequireSandbox in particular) is unit-testable without a
// real LLM call.
func (r *Runner) buildClientConfig(model sdk.ModelIdentifier) *sdk.ClientConfig {
	return &sdk.ClientConfig{
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
		DocumentReaderURL:      r.cfg.DocumentReaderURL,
		DocumentConverter:      r.cfg.DocumentConverter,
		ArtifactStore:          r.cfg.ArtifactStore,
		MCPServers:             r.cfg.MCPServers,
		RequireSandbox:         r.cfg.RequireSandbox,
	}
}

// Close is a no-op — Runner holds no long-lived resources.
func (r *Runner) Close() {}
