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

Tools marked **stub** are registered but `IsEnabled()` returns `false` — they appear here so contributors know where to implement them. See their package-level doc comments for implementation notes.

---

### File tools (`internal/tools/files/`)

| Tool | Description |
|---|---|
| `file_read` | Read file content — text, image (PNG/JPG), or PDF. Supports line-range limits. |
| `file_write` | Create or fully overwrite a file. |
| `file_edit` | Replace an exact string in a file (string-targeted, not line-based). |
| `file_patch` | Apply a unified diff patch to a file. |
| `glob` | List files matching a glob pattern relative to the working directory. |
| `grep` | Search file content with a regular expression. |
| `fs_create_directory` | Create a directory (and parents) if it does not exist. |
| `fs_get_metadata` | Stat a path — size, permissions, mod time, type. |
| `fs_list_directory` | List directory contents (non-recursive by default). |
| `fs_remove` | Delete a file or empty directory. |
| `read_url` | Fetch a URL and return its content as Markdown or plain text. |

---

### Bash tool (`internal/tools/bash/`)

| Tool | Description |
|---|---|
| `bash` | Execute a shell command. Captures stdout/stderr, supports background jobs and timeouts. Includes a safety scanner for dangerous patterns. |
| `bash_write_stdin` | Write data to a running background job's stdin. |
| `bash_job_output` | Read buffered stdout/stderr from a background job. |
| `bash_job_kill` | Send SIGTERM/SIGKILL to a background job. |
| `monitor` | Start a shell command and stream its stdout line-by-line as notifications. Designed for log tailing, build watching, and long-running processes. |

---

### Notebook tools (`internal/tools/notebook/`)

Interact with Jupyter notebooks and live kernels. The kernel sub-tools require a running Jupyter server — set `JUPYTER_SERVER_URL` and `JUPYTER_TOKEN`.

| Tool | Description |
|---|---|
| `notebook_create` | Create a new `.ipynb` file with optional initial cells. Fails if file already exists. |
| `notebook_read` | Read notebook cells, outputs, and kernel metadata. Supports index-based filtering. |
| `notebook_write` | Create or fully overwrite a notebook (for programmatic generation). |
| `notebook_edit` | Edit cells in place — replace/insert/delete. Supports single-op or batch `ops[]` array. |
| `notebook_execute` | Execute cells via a live Jupyter kernel and write outputs back to the notebook file. |
| `notebook_run` | Run arbitrary code in a kernel without creating/modifying a notebook file. |
| `notebook_kernel` | Manage Jupyter kernels — list/start/restart/interrupt/stop. |

---

### Web tools (`internal/tools/web/`)

| Tool | Description |
|---|---|
| `web_fetch` | HTTP fetch with Markdown conversion, response caching, and Docling PDF support. |
| `web_search` | Web search. Backends: DuckDuckGo (default), Exa, Jina, Tavily, SearXNG. |

---

### Browser tools (`internal/tools/web/browser/`)

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

---

### Math tools (`internal/tools/math/`)

| Tool | Description |
|---|---|
| `calculator` | Evaluate arithmetic and algebraic expressions. Supports variables, functions, and unit-aware evaluation. |
| `unit_convert` | Convert values between units (length, mass, temperature, speed, …). |
| `statistics` | Descriptive statistics — mean, median, std dev, percentiles, correlation. |
| `financial` | Financial calculations — compound interest, NPV, IRR, amortisation. |

---

### Social tools (`internal/tools/social/`)

Tools for community and developer platforms. Read-only tools require no credentials.

| Tool | Status | Description |
|---|---|---|
| `hn_search` | ✅ live | Full-text search of Hacker News stories and comments via Algolia. |
| `hn_stories` | ✅ live | Fetch HN feed — top/new/best/ask/show/job stories. |
| `hn_item` | ✅ live | Get a story with its full comment thread. |
| `devto_feed` | ✅ live | Browse dev.to articles by tag, popularity, or date. |
| `devto_article` | ✅ live | Fetch a single dev.to article by ID or URL. |
| `devto_publish` | ✅ live | Publish or update a dev.to article (requires `DEV_TO_API_KEY`). |
| `reddit_search` | stub | Search Reddit posts and comments (requires `REDDIT_CLIENT_ID`). |
| `reddit_posts` | stub | Browse subreddit posts by sort (hot/new/top). |
| `twitter_search` | stub | Search tweets (requires `TWITTER_BEARER_TOKEN`). |
| `twitter_tweet` | stub | Post a tweet (requires OAuth 1.0a). |

---

### Notification tools (`internal/tools/notifications/`)

Direct-messaging channels — one-way delivery to a person or system. All require credentials; all registered as stubs until implemented.

| Tool | Status | Env vars | Description |
|---|---|---|---|
| `slack_send` | stub | `SLACK_WEBHOOK_URL` or `SLACK_BOT_TOKEN` | Send to a Slack channel via Incoming Webhook or Bot API. Supports Block Kit. |
| `discord_send` | stub | `DISCORD_WEBHOOK_URL` or `DISCORD_BOT_TOKEN` | Send to a Discord channel via webhook or Bot API. Supports embeds. |
| `telegram_send` | stub | `TELEGRAM_BOT_TOKEN` | Send via Telegram Bot API. HTML and MarkdownV2 parse modes. |
| `email_send` | stub | `SMTP_HOST`, `SMTP_USER`, `SMTP_PASSWORD` | Send email via SMTP (net/smtp, no extra deps). Supports HTML + plain multipart. |
| `whatsapp_send` | stub | `WHATSAPP_PHONE_NUMBER_ID`, `WHATSAPP_TOKEN` | Send via WhatsApp Business Cloud API. Supports text and approved templates. |

---

### VCS tools (`internal/tools/git/`)

Structured git operations — return typed JSON, not raw text. Requires the `git` binary on PATH. All registered as stubs until implemented via `os/exec`.

| Tool | Status | Description |
|---|---|---|
| `git_status` | stub | Working tree status — staged, unstaged, untracked, ahead/behind. |
| `git_log` | stub | Commit history with filters (branch, since, author, grep). |
| `git_diff` | stub | Diff between refs or working tree. Returns structured file list + patches. |
| `git_commit` | stub | Stage files and create a commit. `RequiresPermission: true`. |
| `git_branch` | stub | List, create, switch, or delete branches. |

---

### Multimedia tools (`internal/tools/multimedia/`)

Always registered; `IsEnabled()` returns `false` when no provider is configured.

| Tool | Env var | Description |
|---|---|---|
| `generate_image` | `OPENAI_API_KEY` | Generate an image from a text prompt. Returns base64 or URL. |
| `text_to_speech` | `OPENAI_API_KEY` | Synthesise speech audio from text. Returns base64-encoded audio. |
| `speech_to_text` | `OPENAI_API_KEY` | Transcribe audio to text via Whisper. Input: base64-encoded audio. |

Provider implemented: **OpenAI** (DALL-E 3 for images, TTS-1/TTS-1-HD for synthesis, Whisper for transcription).

---

### Agent tools (`internal/tools/agents/`)

| Tool | Description |
|---|---|
| `spawn_agent` | Launch a sub-agent with isolated context, tool set, and system prompt. Returns an agent ID. |
| `wait_agent` | Block until a sub-agent completes; returns its final output. |
| `resume_agent` | Resume a previously paused sub-agent with a new message. |
| `list_agents` | List all active sub-agents and their current status. |
| `send_agent_message` | Send a message to a running sub-agent. |
| `close_agent` | Terminate a sub-agent and release its resources. |

---

### Goal tools (`internal/tools/special/goal/`)

| Tool | Description |
|---|---|
| `goal_create` | Create a tracked goal with title, description, and optional sub-goals. |
| `goal_get` | Retrieve the current state of a goal and its sub-goals. |
| `goal_update` | Update goal status (in_progress, completed, abandoned) or details. |

---

### Memory tools (`internal/tools/special/memory/`)

Available when long-term memory is configured.

| Tool | Description |
|---|---|
| `memory_create_entities` | Create named entities in the knowledge graph. |
| `memory_add_observations` | Add observations to existing entities. |
| `memory_search_nodes` | Semantic search across the knowledge graph. |
| `memory_open_nodes` | Retrieve specific nodes by name. |

---

### RAG tools (`internal/tools/special/rag/`)

| Tool | Description |
|---|---|
| `rag_search` | Semantic search across indexed documents. |
| `rag_ingest` | Ingest a file or directory into the vector store. |

---

### System tools (`internal/tools/system/`)

| Tool | Description |
|---|---|
| `mcp_*` | Exposes MCP server tools. Each connected MCP server contributes one or more tools prefixed with the server name. |
| `nexus_list_skills` | List available skills with name and description. |
| `nexus_read_skill` | Read the full content of a skill file. |
| `nexus_validate_skill` | Validate a skill file for structural correctness. |
| `skill_run` | Invoke a skill by name with optional arguments. |

---

### Task tools (`internal/tools/task/`)

| Tool | Description |
|---|---|
| `task_create` | Create a session-scoped tracked task for execution progress. |
| `task_list` | List tracked session tasks and, when requested, background runtime tasks. |
| `task_get` | Fetch details for a tracked session task. |
| `task_update` | Update tracked task status, details, and dependencies. |
| `task_stop` | Stop tracking a session task. |
| `task_output` | Get the output of a background runtime task. |

---

### Special tools (`internal/tools/special/`)

| Tool | Description |
|---|---|
| `ask_user` | Prompt the user for input during a session. Supports timeout. |
| `lsp` | Language Server Protocol — go-to-definition, hover, diagnostics, rename. |
| `worktree_enter` / `worktree_exit` | Enter/exit a git worktree (isolated branch checkout). |
| `tool_search` | Search available tools by keyword, including deferred ones. |
| `plan_mode_enter` / `plan_mode_exit` | Switch to plan mode (no execution, only planning). |
| `plan_submit` | Submit a plan for user review and approval. |
| `request_permissions` | Request additional permissions at runtime. |
| `fim` | Fill-in-the-middle code completion at the cursor position. |

---

## Writing a custom tool

```go
type MyTool struct{}

func (t *MyTool) Definition() tool.Definition {
    return tool.Definition{
        Name:        "my_tool",
        Description: "Does something useful.",
        InputSchema: schema.FromMap(map[string]any{
            "type": "object",
            "properties": map[string]any{
                "input": map[string]any{"type": "string"},
            },
            "required": []string{"input"},
        }),
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

Register in `internal/tools/builtin/builtin.go`:

```go
tools := []tool.Tool{
    // ...
    &MyTool{},
}
```

See [`CONTRIBUTING.md`](../CONTRIBUTING.md) for the full tool-addition checklist.

---

## Tool registry and deferred tools

The `Registry` (`internal/tools/registry/`) holds all available tools. Tools can be:

- **Eager**: available from session start (included in every tool list sent to the LLM)
- **Deferred**: not included in the initial tool list; revealed by `tool_search` during a session

Deferred tools are useful for large tool sets — they keep the initial context small while still making every tool discoverable.
