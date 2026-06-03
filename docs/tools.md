# Tool System

## Tool contract

Every tool implements the `Tool` interface in `internal/tools/contract/interface.go`:

```go
type Tool interface {
    Definition() Definition
    Call(ctx context.Context, input CallInput, permCheck types.CanUseToolFn) (CallResult, error)
    Description(ctx context.Context) (string, error)
    ValidateInput(ctx context.Context, input map[string]any) (map[string]any, error)
    BackfillInput(ctx context.Context, input map[string]any) map[string]any
    CheckPermissions(ctx context.Context, input map[string]any, toolCtx ToolUseContext) types.PermissionResult
    IsConcurrencySafe(input map[string]any) bool
    IsReadOnly(input map[string]any) bool
    IsEnabled() bool
    FormatResult(data any) string
}
```

### Optional capabilities

```go
// PreparePermissionMatcher lets the tool supply a content-specific matcher
// that the permission engine uses to evaluate allow/deny rules.
type PermissionMatcherTool interface {
    PreparePermissionMatcher(ctx context.Context, input map[string]any) (matchFn, error)
}

// ExecutesInPlanMode marks tools that run even in plan mode.
// By default, tools are blocked in plan mode.
type PlanModeExecutableTool interface {
    ExecutesInPlanMode(input map[string]any) bool
}

// RequiresUserInteraction marks tools that need a terminal attached.
type RequiresUserInteractionTool interface {
    RequiresUserInteraction() bool
}
```

---

## Execution model

The orchestrator (`internal/execution/orchestrator.go`) partitions concurrent and sequential tool uses based on `IsConcurrencySafe`:

- **Concurrent batch**: tools that return `IsConcurrencySafe = true` run in parallel (max 10 goroutines by default). Context updates (tool results injected into the transcript) are applied in invocation order after all finish.
- **Serial batch**: tools run one at a time. Context updates are applied immediately after each call.

A turn's tool uses may contain multiple concurrent batches interleaved with serial ones.

---

## Built-in tools reference

### File tools (`internal/tools/files/`)

| Tool | Description |
|---|---|
| `Read` | Read file content (text, image, PDF). Respects line limits and cancellation. |
| `Edit` | Replace a string in a file with proper line-range targeting. |
| `Write` | Create or overwrite a file. |
| `Glob` | List files matching a glob pattern. |
| `Grep` | Search file content with regex. |
| `NotebookEdit` | Edit a Jupyter notebook cell. |

### Bash tool (`internal/tools/bash/`)

| Tool | Description |
|---|---|
| `Bash` | Execute a shell command with stdout/stderr capture. Includes safety scanner for dangerous patterns and background job tracking. |

### Web tools (`internal/tools/web/`)

| Tool | Description |
|---|---|
| `WebFetch` | HTTP fetch with markdown conversion and response caching. |
| `WebSearch` | Web search. Backends: DuckDuckGo, Exa, Jina, Tavily. |

### Browser tools

Browser automation via Playwright (go-rod). Available when a browser instance is configured.

| Tool | Description |
|---|---|
| `browser_open` | Open a URL in a new or existing tab |
| `browser_navigate` | Navigate the active tab to a URL |
| `browser_snapshot` | Capture a DOM/accessibility snapshot |
| `browser_extract` | Extract text content from the current page |
| `browser_screenshot` | Take a screenshot (returns image artifact) |
| `browser_click` | Click an element by selector or coordinates |
| `browser_type` | Type text into an input |
| `browser_press` | Press a key (Enter, Tab, Escape, …) |
| `browser_scroll` | Scroll the page |
| `browser_wait` | Wait for a condition |
| `browser_select_page` | Switch to a tab by index |
| `browser_close_page` | Close a tab |
| `browser_list_pages` | List all open tabs |
| `browser_get_network` | List captured network requests |
| `browser_list_downloads` | List completed downloads |
| `browser_search_content` | Search page text |
| `browser_set_network_policy` | Allow/deny requests by domain |
| `browser_get_network_policy` | Read the current network policy |

### Special tools (`internal/tools/special/`)

| Tool | Description |
|---|---|
| `ask_user` | Prompt for user input during a session |
| `todo_write` / `todo_read` | Manage a structured task list within the session |
| `lsp` | Language Server Protocol integration (go-to-definition, hover, diagnostics, rename) |
| `worktree_enter` / `worktree_exit` | Enter/exit a git worktree (isolated branch) |
| `tool_search` | Search available (including deferred) tools by keyword |
| `monitor` | Monitor a background process |
| `spawn_agent` / `wait_agent` | Spawn a sub-agent with isolated context; wait for its result |
| `send_agent_message` / `close_agent` | Communicate with or close a running sub-agent |
| `plan_mode_enter` / `plan_mode_exit` | Switch the session execution mode |
| `plan_submit` | Submit a plan for user review |
| `request_permissions` | Request additional permissions at runtime |

### Memory tools (long-term memory)

Available when `EnableMemory: true` (default).

| Tool | Description |
|---|---|
| `memory_create_entities` | Create named entities in the knowledge graph |
| `memory_add_observations` | Add observations to existing entities |
| `memory_search_nodes` | Semantic search across the knowledge graph |
| `memory_open_nodes` | Retrieve specific nodes by name |

### RAG tools

| Tool | Description |
|---|---|
| `rag_search` | Semantic search across indexed documents |
| `rag_ingest` | Ingest a file or directory into the vector store |

### Image generation

Enabled when an image provider is configured (`ClientConfig` → `BuiltinConfig.ImageGenerator`).

| Tool | Input | Output |
|---|---|---|
| `generate_image` | `prompt: string` | `provider`, `model`, `image_base64` or `image_url`, `mime_type`, `revised_prompt` |

Providers implemented: **OpenAI** (DALL-E 3), **Google Gemini** (Imagen).

The tool is always registered but `IsEnabled()` returns `false` when no generator is configured — it will not appear in the tool list sent to the LLM.

### Audio tools

Enabled when audio providers are configured.

| Tool | Input | Output |
|---|---|---|
| `text_to_speech` | `text: string` | `provider`, `model`, `audio_base64`, `content_type`, `characters_used` |
| `speech_to_text` | `audio_base64: string`, `mime_type?: string` | `text`, `language`, `duration`, `model`, `provider` |

Supported formats for `speech_to_text`: MP3, WAV, WebM, M4A.

Provider implemented: **OpenAI** (TTS-1/TTS-1-HD for synthesis, Whisper for transcription).

Both tools are always registered but disabled when no provider is configured.

### System tools (`internal/tools/system/`)

| Tool | Description |
|---|---|
| `MCP*` | Exposes MCP server resources as callable tools. |
| `EnterPlanMode` / `ExitPlanMode` | Switch the session execution mode. |
| `PairProgramming` | Toggle pair-programming collaboration mode. |
| `Skill*` | Run a bundled or MCP-discovered skill. |

### Task tools (`internal/tools/task/`)

| Tool | Description |
|---|---|
| `TaskCreate` | Create a background task with metadata. |
| `TaskList` | List tasks with filtering. |
| `TaskGet` | Fetch task details. |
| `TaskUpdate` | Update task status and fields. |
| `TaskStop` | Stop a running background task. |
| `TaskOutput` | Get the output of a task. |

### Agent tool (`internal/tools/agent/`)

Runs a sub-agent (skill) in isolation with its own transcript and memory adapter. Results are returned as tool output.

---

## Writing a custom tool

```go
type MyTool struct{}

func (t *MyTool) Definition() tool.Definition {
    return tool.Definition{
        Name:        "my_tool",
        Description: "Does something useful.",
        InputSchema: map[string]any{
            "type": "object",
            "properties": map[string]any{
                "input": map[string]any{"type": "string"},
            },
            "required": []string{"input"},
        },
        IsReadOnly:        true,
        IsConcurrencySafe: true,
    }
}

func (t *MyTool) Call(ctx context.Context, input tool.CallInput, _ types.CanUseToolFn) (tool.CallResult, error) {
    value, _ := input.Parsed["input"].(string)
    return tool.NewTextResult("result: " + value), nil
}

// ... implement remaining interface methods
```

Register before creating sessions:

```go
client.RegisterTool(&MyTool{})
// or per-session:
session.RegisterTool(&MyTool{})
```

---

## Tool registry and deferred tools

The `Registry` (`internal/tools/registry/`) holds all available tools. Tools can be:

- **Eager**: available from session start
- **Deferred**: not included in the initial tool list; revealed by `ToolSearch` during a session

The `ToolSearch` tool returns tool names matching a query. The loop then resolves them from the registry and adds them to the active tool surface for the remainder of the turn.
