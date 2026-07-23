package sdk

import (
	"context"
	"time"

	"github.com/KPO-Tech/seshat/internal/providers"
	"github.com/KPO-Tech/seshat/internal/sandbox"
	bashTool "github.com/KPO-Tech/seshat/internal/tools/bash"
	"github.com/KPO-Tech/seshat/pkg/runtimepath"
)

// SandboxKind selects an OS-level sandbox backend for bash tool execution.
type SandboxKind = sandbox.EnvironmentKind

// Sandbox backend kind values for ClientConfig.SandboxKind.
const (
	// SandboxKindLocal is the zero value — Landlock on Linux, unconfined
	// elsewhere unless RequireSandbox is also set. Today's existing behavior.
	SandboxKindLocal = sandbox.EnvironmentLocal
	// SandboxKindDocker routes bash execution through a persistent Docker
	// container (internal/sandbox.DockerExecutor) — real filesystem/process
	// isolation plus enforced CPU/memory caps and optional network isolation.
	// Falls back to SandboxKindLocal with a logged warning if Docker isn't
	// reachable when the client is constructed.
	SandboxKindDocker = sandbox.EnvironmentDocker
)

// SandboxDockerConfig configures the Docker sandbox backend used when
// ClientConfig.SandboxKind is SandboxKindDocker. See
// sandbox.DockerExecutorConfig for field-by-field documentation.
type SandboxDockerConfig = sandbox.DockerExecutorConfig

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

// ImageGenerationConfig controls the generate_image built-in tool.
type ImageGenerationConfig struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
	BaseURL  string `json:"base_url,omitempty"`
}

// TextToSpeechConfig controls the text_to_speech built-in tool.
type TextToSpeechConfig struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Voice    string `json:"voice,omitempty"`
	Format   string `json:"format,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
	BaseURL  string `json:"base_url,omitempty"`
}

// SpeechToTextConfig controls the speech_to_text built-in tool.
type SpeechToTextConfig struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Language string `json:"language,omitempty"`
	APIKey   string `json:"api_key,omitempty"`
	BaseURL  string `json:"base_url,omitempty"`
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
	// OnSessionTitled is called after the engine auto-generates a title for a
	// session (once, after the first completed turn). The id is the session ID
	// and title is the short, AI-generated label. Use this to refresh the UI.
	OnSessionTitled func(id SessionID, title string) `json:"-"`

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
	EnableHooks            bool `json:"enable_hooks"`
	EnableMonitoring       bool `json:"enable_monitoring"`
	DisableTitleGeneration bool `json:"disable_title_generation"`

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

	// Automation daemon connection (seshat-automation).
	// When set, the schedule_job / list_jobs / update_job / delete_job / pause_job /
	// resume_job / run_job_now tools become functional.
	AutomationServiceURL string `json:"automation_service_url,omitempty"`
	AutomationAPIKey     string `json:"-"`

	// WebSearchKeys provides per-execution web search provider keys.
	// When set, the web_search tool uses these keys instead of reading from the
	// process environment, preventing key leakage across concurrent sessions.
	// Keys are keyed by provider name: "tavily", "exa", "jina", "langsearch".
	WebSearchKeys map[string]string `json:"-"`

	// Optional capability-specific providers for multimodal built-in tools.
	ImageGeneration *ImageGenerationConfig `json:"image_generation,omitempty"`
	TextToSpeech    *TextToSpeechConfig    `json:"text_to_speech,omitempty"`
	SpeechToText    *SpeechToTextConfig    `json:"speech_to_text,omitempty"`

	// RequireSandbox makes the bash tool refuse to run commands when no
	// OS-level sandbox (currently: Landlock, Linux-only) is available on the
	// host, instead of silently falling back to unconfined execution. Leave
	// false for desktop use where refusing to run bash entirely is worse UX
	// than a visible unsandboxed-execution warning; set true for multi-tenant
	// server deployments where unconfined execution must never happen.
	RequireSandbox bool `json:"require_sandbox"`

	// SandboxKind selects the OS-level sandbox backend for bash tool
	// execution. Empty (SandboxKindLocal) preserves existing behavior.
	// SandboxKindDocker routes execution through a persistent Docker
	// container — see SandboxDocker.
	SandboxKind SandboxKind `json:"sandbox_kind,omitempty"`
	// SandboxDocker configures the Docker sandbox backend when SandboxKind
	// is SandboxKindDocker. Zero value uses sane, security-conscious
	// defaults (see sandbox.DefaultDockerConfig).
	SandboxDocker SandboxDockerConfig `json:"-"`
}

// PreToolHookConfig is a single shell hook that runs before a tool call.
type PreToolHookConfig struct {
	Matcher string // regex against tool name; empty = match all
	Command string // shell command to execute
	Timeout int    // timeout in seconds (default 30)
}

// SandboxAvailable reports whether an OS-level sandbox (currently: Landlock,
// Linux-only) is available on this host to confine bash tool execution.
// Host applications use this to surface the unconfined-execution tradeoff to
// the end user (e.g. in a settings UI) rather than leaving it as a
// backend-log-only warning.
func SandboxAvailable() bool {
	return bashTool.SandboxAvailable()
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
