package sdk

import (
	"context"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/providers"
	"github.com/EngineerProjects/nexus-engine/pkg/runtimepath"
)

// CredentialResolver resolves the API key for an LLM provider at client
// creation time. The provider argument matches types.APIProvider values
// (e.g. "anthropic", "openai", "codex").
//
// CLI / headless mode: not needed — the default env-var → FileStore
// resolution in internal/providers/auth.go applies automatically.
//
// Server mode: implement this interface to inject per-user credentials
// from a database instead of relying on a single global API key.
type CredentialResolver interface {
	ResolveAPIKey(ctx context.Context, provider string) (string, error)
}

// ClientConfig represents the client configuration.
type ClientConfig struct {
	APIKey         string          `json:"api_key"`
	Model          ModelIdentifier `json:"model"`
	PermissionMode PermissionMode  `json:"permission_mode"`
	MaxTurns       int             `json:"max_turns"`
	AutoCompact    bool            `json:"auto_compact"`

	// Session persistence
	PersistSessions   bool           `json:"persist_sessions"`
	SessionStorageDir string         `json:"session_storage_dir"`
	SessionSQLitePath string         `json:"session_sqlite_path"`
	SessionBackend    SessionBackend `json:"-"`
	SessionStore      SessionStore   `json:"-"`

	// MCP
	MCPServers []MCPServerConfig `json:"mcp_servers"`

	// Callbacks
	PromptFn        PromptFn            `json:"-"`
	ProgressFn      func(ToolProgress)  `json:"-"`
	ResponseChunkFn func(ResponseChunk) `json:"-"`
	RuntimeEventFn  func(RuntimeEvent)  `json:"-"`

	// Filesystem
	WorkingDir string `json:"working_dir"`

	// Browser
	BrowserRemoteControlURL string `json:"browser_remote_control_url"`
	BrowserExecutablePath   string `json:"browser_executable_path"`

	// Storage
	StorageConfig       *StorageConfig `json:"-"`
	ArtifactStore       ArtifactStore  `json:"-"`
	StorageGCEnabled    bool           `json:"storage_gc_enabled"`
	StorageGCInterval   time.Duration  `json:"storage_gc_interval"`
	StorageGCLimit      int            `json:"storage_gc_limit"`
	StorageGCNamespaces []string       `json:"storage_gc_namespaces"`

	// CredentialResolver, if set, is called at client creation to resolve the
	// API key for the configured provider. Takes precedence over APIKey.
	// Use in server mode to inject per-user credentials from a database.
	CredentialResolver CredentialResolver `json:"-"`

	// Provider override
	ProviderConfig *providers.Config `json:"-"`

	// Monitoring
	Monitoring *MonitoringSystem `json:"-"`

	// Interactive prompting
	EnablePromptReaderFallback bool `json:"enable_prompt_reader_fallback"`

	// Memory
	EnableMemory   bool `json:"enable_memory"`
	MemoryFailFast bool `json:"memory_fail_fast"`

	// Feature flags
	EnableHooks      bool `json:"enable_hooks"`
	EnableMonitoring bool `json:"enable_monitoring"`

	// Model parameters
	MaxTokens               int `json:"max_tokens"`
	MaxIterations           int `json:"max_iterations"`
	TurnTokenBudget         int `json:"turn_token_budget"`
	BudgetContinuationLimit int `json:"budget_continuation_limit"`
	ContinuationNudgeLimit  int `json:"continuation_nudge_limit"`
	MaxConsecutiveDenials   int `json:"max_consecutive_denials"`

	// System prompt
	SystemPromptTemplate string        `json:"system_prompt_template"`
	PromptConfig         *PromptConfig `json:"-"`

	// Stop hooks
	StopHooks []StopHook `json:"-"`

	// PreToolHooks are shell-based hooks that fire before each tool execution.
	// Each entry is a HookConfig: {Matcher, Command, Timeout}.
	// Compatible with crush's pre_tool_use hook format:
	//   - exit 0 + no JSON → allow (no-op)
	//   - exit 0 + {"decision":"allow"} → allow + skip permission prompt
	//   - exit 0 + {"updatedInput":"..."} → rewrite tool input
	//   - exit 2 + stderr → deny with reason
	//   - exit 49 → halt entire turn
	// Set via ClientConfig.PreToolHooks or loaded automatically from
	// config.Hooks["pre_tool_use"] by newClient().
	PreToolHooks []PreToolHookConfig `json:"-"`

	// RAG / Plan / Memory
	RAGService     *RAGService    `json:"-"`
	PlanStore      PlanStore      `json:"-"`
	LongTermMemory LongTermMemory `json:"-"`

	// Document conversion
	DoclingURL string `json:"docling_url,omitempty"`
}

// PreToolHookConfig is a single shell hook that runs before a tool call.
type PreToolHookConfig struct {
	Matcher string // regex against tool name; empty = match all
	Command string // shell command to execute
	Timeout int    // timeout in seconds (default 30)
}

// DefaultClientConfig returns default client configuration.
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		Model: ModelIdentifier{
			Provider: APIProviderAnthropic,
			Model:    "claude-3-5-sonnet-20241022",
		},
		PermissionMode:    PermissionModeOnRequest,
		MaxTurns:          100,
		AutoCompact:       true,
		PersistSessions:   true,
		SessionStorageDir: runtimepath.SessionStoreDir(""),
		MaxTokens:         8192,
		EnableMemory:      true,
		EnableHooks:       true,
		EnableMonitoring:  true,
		StorageGCEnabled:  true,
		StorageGCInterval: time.Hour,
		StorageGCLimit:    512,
		// Session-scoped artifacts are cleaned up via DeleteSessionDir on session
		// deletion, not by periodic GC. Global namespaces with expiring content
		// would go here if added in the future.
		StorageGCNamespaces: []string{},
	}
}
