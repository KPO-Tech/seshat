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

### Special tools (`internal/tools/special/`)

| Tool | Description |
|---|---|
| `AskUser` | Prompt for user input during a session. |
| `TodoWrite` / `TodoRead` | Manage a structured task list within the session. |
| `LSP*` | Language Server Protocol integration (go-to-definition, hover, diagnostics, rename). |
| `Research` | Academic research (arXiv, Semantic Scholar). |
| `Worktree` | Enter/exit a git worktree (isolated branch). |
| `Wikipedia` | Wikipedia article lookup. |
| `Tree` | Directory tree visualization. |
| `Monitor` | Monitor a background process. |
| `ToolSearch` | Search available (including deferred) tools by keyword. |
| `Brief` | Summarize information for context management. |
| `Config` | Query the current tool/session configuration. |

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

Register it with the tool registry before creating sessions:

```go
registry.Register("my_tool", &MyTool{})
```

---

## Tool registry and deferred tools

The `Registry` (`internal/tools/registry/`) holds all available tools. Tools can be:

- **Eager**: available from session start
- **Deferred**: not included in the initial tool list; revealed by `ToolSearch` during a session

The `ToolSearch` tool returns tool names matching a query. The loop then resolves them from the registry and adds them to the active tool surface for the remainder of the turn.
