# SDK Guide

## Installation

```go
import "github.com/EngineerProjects/nexus-engine/pkg/sdk"
```

---

## Basic usage

### Single-turn query

```go
client, err := sdk.NewClient(&sdk.ClientConfig{
    APIKey: os.Getenv("ANTHROPIC_API_KEY"),
    Model:  sdk.ParseModelIdentifier("anthropic:claude-sonnet-4-6"),
})
if err != nil {
    log.Fatal(err)
}
defer client.Close()

response, err := client.Ask(ctx, "Explain how goroutines work.")
if err != nil {
    log.Fatal(err)
}
fmt.Println(response.Content)
```

### Multi-turn session

```go
session, err := client.CreateSession(ctx)
if err != nil {
    log.Fatal(err)
}
defer session.Close()

r1, _ := session.SubmitMessage(ctx, "List the files in the current directory.")
fmt.Println(r1.Content)

r2, _ := session.SubmitMessage(ctx, "Now show me the content of main.go.")
fmt.Println(r2.Content)
```

### Resume a persisted session

```go
// Session IDs are returned in every SessionResponse
savedID := r1.SessionID

restored, err := client.LoadSession(ctx, sdk.SessionID(savedID))
// Continue the session from where it was left
```

### Inject a session backend

```go
client, err := sdk.NewClient(&sdk.ClientConfig{
    PersistSessions: false,
    SessionBackend:  sdk.NewMemorySessionBackend(),
})
if err != nil {
    log.Fatal(err)
}
```

### Inject artifact storage

```go
store, err := sdk.NewArtifactStoreFromConfig(sdk.StorageConfig{
    Provider:  sdk.StorageProviderLocal,
    LocalPath: "/tmp/nexus-artifacts",
})
if err != nil {
    log.Fatal(err)
}

client, err := sdk.NewClient(&sdk.ClientConfig{
    ArtifactStore: store,
})
if err != nil {
    log.Fatal(err)
}
```

---

## ClientConfig reference

```go
type ClientConfig struct {
    // ── Provider ──────────────────────────────────────────────────────────
    APIKey    string               // required for cloud providers
    Model     types.ModelIdentifier // "provider:model", e.g. "anthropic:claude-sonnet-4-6"
    MaxTokens int                  // max output tokens (default 8192)

    // ── Loop limits ───────────────────────────────────────────────────────
    MaxTurns      int // total turns per session (default 100)
    MaxIterations int // model→tool cycles per turn (default 10)

    // ── Context management ────────────────────────────────────────────────
    AutoCompact        bool // compact transcript when context window fills (default true)
    TurnTokenBudget    int  // if set, pause when budget exhausted
    BudgetContinuationLimit int // how many budget continuations per turn

    // ── Permissions ───────────────────────────────────────────────────────
    PermissionMode sdk.PermissionMode // bypass | auto | acceptEdits | onRequest | never

    // ── Persistence ───────────────────────────────────────────────────────
    PersistSessions   bool             // enable default filesystem persistence
    SessionSQLitePath string           // enable SQLite-backed session persistence
    SessionBackend    sdk.SessionBackend // inject a custom persistence backend
    SessionStore      sdk.SessionStore   // inject a fully-built session store
    WorkingDir        string // root for file tool operations

    // ── Features ──────────────────────────────────────────────────────────
    EnableMemory   bool // long-term memory (project/user/cross-session)
    EnableHooks    bool // lifecycle hooks
    EnableMonitoring bool

    // ── MCP ───────────────────────────────────────────────────────────────
    MCPServers []sdk.MCPServerConfig // MCP servers to connect on startup

    // ── Storage / artifacts ──────────────────────────────────────────────
    StorageConfig *sdk.StorageConfig // per-client artifact storage config
    ArtifactStore sdk.ArtifactStore  // inject a fully-built artifact store

    // ── Callbacks ─────────────────────────────────────────────────────────
    ResponseChunkFn func(sdk.ResponseChunk) // called for each streaming chunk
    ProgressFn      func(sdk.ToolProgress)  // called for tool execution events
    PromptFn        func(ctx, prompt) (string, error) // user input callback

    // ── Prompt customization ──────────────────────────────────────────────
    SystemPromptTemplate string       // replace default system prompt
    PromptConfig         *PromptConfig // stage overlays, tool hints, append text
}
```

---

## PromptConfig

```go
type PromptConfig struct {
    // Replace the entire system prompt (overrides all defaults)
    SystemPrompt *string

    // Text appended after all assembled sections
    AppendSystemPrompt *string

    // Active execution stage (injects a stage-specific overlay section)
    Stage ExecutionStage  // "tool_call" | "tool_result" | "continuation" | "plan"

    // Per-stage custom overlay text (overrides built-in templates)
    StageOverrides map[ExecutionStage]string

    // Per-tool extra guidance appended to the tool's description
    ToolHints map[string]string
}
```

Example — restrict bash to read-only and activate plan stage:

```go
cfg := &sdk.ClientConfig{
    APIKey: key,
    Model:  sdk.ParseModelIdentifier("anthropic:claude-sonnet-4-6"),
    PromptConfig: &sdk.PromptConfig{
        Stage: sdk.StagePlan,
        ToolHints: map[string]string{
            "bash": "Only run read-only commands. Do not modify any files.",
        },
    },
}
```

---

## Streaming

### ResponseChunkFn callback

```go
client, _ := sdk.NewClient(&sdk.ClientConfig{
    APIKey: key,
    ResponseChunkFn: func(chunk sdk.ResponseChunk) {
        if chunk.Type == sdk.ResponseChunkTypeContentBlockDelta {
            fmt.Print(chunk.Delta)
        }
    },
})
```

### EventQueue (per-session)

```go
session, _ := client.CreateSession(ctx)
queue := session.GetEventQueue()

// Drain in a separate goroutine before calling SubmitMessage
go func() {
    for chunk := range queue.Recv() {
        fmt.Printf("[chunk] %s: %s\n", chunk.Type, chunk.Delta)
    }
}()

response, _ := session.SubmitMessage(ctx, "hello")
// queue is drained; goroutine exits when session is closed
```

---

## Permission modes

```go
const (
    PermissionModeBypass      = "bypass"       // skip all checks
    PermissionModeAuto        = "auto"         // ML classifier auto-approves safe actions
    PermissionModeAcceptEdits = "acceptEdits"  // auto-approve safe file ops in working dir
    PermissionModeOnRequest   = "onRequest"    // ask user (default)
    PermissionModeNever       = "never"        // deny non-trivially-safe uses
)
```

For headless automation use `bypass` or `never` depending on trust level.

---

## Session response

```go
type SessionResponse struct {
    SessionID   string
    Messages    []types.Message   // full updated transcript
    StopReason  string            // "end_turn" | "max_tokens" | "stop_sequence"
    TurnNumber  int
    IsComplete  bool
    Usage       *types.TokenUsage // InputTokens, OutputTokens, CacheReadTokens, CacheWriteTokens
    ToolUses    []types.ToolUseContent
    ToolResults []tool.CallResult
}
```

---

## Monitoring

```go
m := client.GetMonitoring()

// Prometheus text format
snapshot, _ := m.GetMetricsSnapshot("prometheus")
fmt.Println(snapshot)

// JSON format
snapshot, _ = m.GetMetricsSnapshot("json")
```

---

## Custom tool implementation

Implement the `Tool` interface from `internal/tools/registry`:

```go
type MyTool struct{}

func (t *MyTool) Definition() tool.Definition {
    return tool.Definition{
        Name:               "my_tool",
        DisplayName:        "My Tool",
        Description:        "What it does and when to use it",
        Category:           "custom",
        InputSchema:        schema.FromMap(map[string]any{
            "type": "object",
            "properties": map[string]any{
                "input": map[string]any{"type": "string", "description": "The input"},
            },
            "required": []string{"input"},
        }),
        IsReadOnly:         true,
        RequiresPermission: false,
    }
}

func (t *MyTool) Call(ctx context.Context, input tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
    val, _ := input.Parsed["input"].(string)
    return tool.NewTextResult("processed: " + val), nil
}

func (t *MyTool) ValidateInput(_ context.Context, input map[string]any) (map[string]any, error) {
    if _, ok := input["input"].(string); !ok {
        return nil, fmt.Errorf("input must be a string")
    }
    return input, nil
}

func (t *MyTool) CheckPermissions(_ context.Context, _ map[string]any, _ tool.ToolUseContext) types.PermissionResult {
    return types.Passthrough(nil)
}

func (t *MyTool) BackfillInput(_ context.Context, input map[string]any) map[string]any { return input }
func (t *MyTool) FormatResult(data any) string                                          { return fmt.Sprintf("%v", data) }
func (t *MyTool) Description(_ context.Context) (string, error)                         { return t.Definition().Description, nil }
func (t *MyTool) IsConcurrencySafe(_ map[string]any) bool                               { return true }
func (t *MyTool) IsReadOnly(_ map[string]any) bool                                      { return true }
func (t *MyTool) IsEnabled() bool                                                       { return true }
```

Register with the client before creating sessions:

```go
client.RegisterTool(&MyTool{})
```

---

## Hook lifecycle

Hooks run at specific points in the session turn cycle. Register a handler with `RegisterHook`:

```go
// Available events: HookEventPreTool, HookEventPostTool,
//                  HookEventSessionStart, HookEventSessionEnd,
//                  HookEventTurnStart, HookEventTurnEnd
id := client.RegisterHook(sdk.HookEventPreTool, func(ctx context.Context, event sdk.HookEvent, data map[string]any) (sdk.HookResult, error) {
    toolName, _ := data["tool_name"].(string)
    fmt.Printf("about to call tool: %s\n", toolName)
    return sdk.HookResult{Action: sdk.HookActionContinue}, nil
})

// Remove when no longer needed.
client.HookRegistry().Unregister(id)
```

To stop execution from a hook:

```go
return sdk.HookResult{
    Action:  sdk.HookActionStop,
    Message: "blocked by policy",
}, nil
```

---

## Error handling

SDK functions return `(result, error)`. Errors wrap a structured `types.EngineError`:

```go
resp, err := session.SubmitMessage(ctx, "prompt")
if err != nil {
    var engineErr *types.EngineError
    if errors.As(err, &engineErr) {
        switch engineErr.Code {
        case types.ErrCodeAPIRateLimit:
            // back off and retry
        case types.ErrCodeAPIAuth:
            // invalid or expired API key
        case types.ErrCodeContextOverflow:
            // conversation too long; compaction may help
        case types.ErrCodePermissionDenied:
            // tool call blocked by permission mode
        }
    }
}
```

Permanent errors (auth, invalid model) are never retried internally. Transient errors (rate limit, network) are retried with exponential backoff up to `RetryConfig.MaxAttempts`.

---

## Memory features

Enable long-term memory to persist learnings across sessions:

```go
config := sdk.DefaultClientConfig()
config.EnableMemory = true
config.MemoryConfig = &sdk.MemoryConfig{
    UserID:    "user-123",
    ProjectID: "proj-abc",
    Backend:   "sqlite", // or "qdrant", "chroma", "pgvector"
}
client, _ := sdk.NewClient(config)
```

The memory service automatically:
- Learns from tool usage patterns
- Stores session summaries
- Injects relevant context into future sessions
- Scopes memories per user / per project

Access errors during memory init via `client.MemoryInitError()` — non-fatal, the client still works.

---

## Session methods reference

```go
type Session struct { /* ... */ }

// SubmitMessage sends a prompt and returns the full result.
func (s *Session) SubmitMessage(ctx context.Context, prompt string) (*SessionResponse, error)

// SubmitMessageStream sends a prompt with streaming chunks via the configured
// ResponseChunkFn callback. Returns the final result when streaming ends.
func (s *Session) SubmitMessageStream(ctx context.Context, prompt string) (*SessionResponse, error)

// GetTranscript returns all messages in the current session.
func (s *Session) GetTranscript() []Message

// GetEventQueue returns the per-session event queue for consuming RuntimeEvents.
func (s *Session) GetEventQueue() *EventQueue

// SessionID returns the unique identifier for this session.
func (s *Session) SessionID() SessionID

// Close persists session state and releases resources.
func (s *Session) Close() error
```

`SessionResponse` fields:

```go
type SessionResponse struct {
    Content     string         // Final text output
    Thinking    string         // Extended thinking (if enabled)
    SessionID   SessionID
    TurnNumber  int
    StopReason  string         // "end_turn" | "tool_use" | "max_tokens"
    IsComplete  bool
    Usage       *TokenUsage    // Input/output/cache token counts
    ToolUses    []ToolUseContent
    ToolResults []CallResult
}
```

---

## Monitoring

```go
m := client.GetMonitoring()

// Prometheus text format
snapshot, _ := m.GetMetricsSnapshot("prometheus")
fmt.Println(snapshot)

// JSON format
snapshot, _ = m.GetMetricsSnapshot("json")
```
