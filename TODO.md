# TODO: bugs to fix (important)

**These are errors, not wishes.** They were found by reading the code on 2026-10-09 (branch `dev`, commit `a1eee2f`, rechecked on `dea175c`) while documenting the runtime for the website. Each one is either a behavior that does not match what the code is supposed to do, a security gap, or a feature that exists in the code but is never connected, so it never runs.

Status of each item: **verified** means the code was read and the problem reproduced by reading the call sites. **candidate** means a scan found a name that is never referenced; check before deleting or wiring it.

When you fix one, move it to a "Fixed" section at the bottom with the PR number, and update the website page named in the item (docs are in the `seshat-ai` repository, `website/src/content/docs/en`).

---

## High

### H1. Compaction summary always uses an Anthropic model (verified)

`internal/engine/factory.go:38` and `pkg/sdk/client.go:119` build the compactor with `compact.DefaultConfig()`. Its `SummaryModel` is hardcoded to `claude-3-5-haiku-20241022` on the Anthropic provider (`internal/runtime/memory/config.go:25`), whatever provider the session uses.

**Effect:** a session on OpenAI, DeepSeek, Ollama or any non-Anthropic provider, without an Anthropic key, fails when it needs a summary. `maybeAutoCompact` then returns an error and the turn ends with `auto_compact_error`. Compaction only needs a summary when micro-compaction (trimming tool results) is not enough, so it shows up on long sessions.

**Fix:** use the session model (or a configurable summary model) and expose the compaction settings (threshold, target, summary model) in `ClientConfig`. Add a test with a non-Anthropic provider.

### H2. Compaction circuit breaker can never trip (verified)

`AutoCompact` stops trying after `MaxConsecutiveFailures` (3) failures (`internal/runtime/memory/engine.go:166`). But the counter lives in the per-turn state (`state.AutoCompactFailureCount`, `internal/engine/state.go:578`), which `initializeState` recreates at the start of every `Loop.Run`. A failed compaction also ends the turn at once (`loop.go:293`), so the counter never goes above 1.

**Effect:** a session whose summary call keeps failing fails again on every turn. The protection is dead code.

**Fix:** keep the count on the session (or loop) across turns, reset it on success, and decide whether a failed compaction should still end the turn. A task for this is already open. Website page: `concepts/memory`.

### H3. Project instruction files bypass project trust (verified, security)

`readProjectInstructions` (`internal/engine/project_instructions.go`) reads `SESHAT.md`, `AGENTS.md` or `.seshat/instructions.md` from the working directory and `session_prompt.go:61` injects it into the system prompt, with a repo map appended. There is no trust check. `docs/project-trust.md` says a project's configuration is ignored until `seshat trust`, and lists `context_paths` and `skills_paths` as guarded, but these files are not.

**Effect:** a repository you just cloned can place instructions in the model's prompt. This is the same class of problem the 1.3.0 audit closed for `.seshat.json`.

**Fix:** gate the file behind the same trust, or show it to the user for approval on first use (hash-based, like the other project files). Document the decision in `docs/project-trust.md`. Website pages: `sdk/prompt`, `concepts/security-and-trust`.

### H4. Hook events are defined and exported, but most are never emitted (verified)

`internal/types/hooks.go` defines 40 events and `pkg/sdk/types.go` re-exports them. The runtime only fires these through the hook executor: `query_start`, `iteration_start`, `iteration_stop`, `iteration_continue`, `iteration_complete`, `tool_uses_start`, `tool_uses_complete`, `query_complete` (`internal/engine/loop.go`), and `subagent_start`, `subagent_stop`, `on_error` (`internal/agent/runner.go`).

Never fired: `session_start`, `session_end`, `turn_start`, `turn_end`, `turn_stop`, `stop_failure`, `pre_tool_use`, `post_tool_use`, `post_tool_use_fail`, `pre_compact`, `post_compact`, `pre_api_call`, `post_api_call`, `user_prompt_submit`, `permission_request`, `permission_denied`, `notification`, `setup`, `config_change`, `teammate_idle`, `task_created`, `task_completed`, `elicitation`, `elicitation_result`, `worktree_create`, `worktree_remove`, `instructions_loaded`, `cwd_changed`, `file_changed`.

**Effect:** `client.RegisterHook(sdk.HookEventPreToolUse, ...)` compiles, returns an id and never runs. Nothing warns.

Also, for the events that do fire, only `iteration_start` and `tool_uses_start` can stop the turn. The other results are ignored. The `modify` and `retry` actions and a `deny` message are only logged (`executeHookWithResult`, `loop.go:1504`): nothing applies `UpdatedInput`, retries, or sends the message to the model.

**Fix:** either emit the events that make sense (at minimum `pre_tool_use`, `post_tool_use`, `pre_compact`, `post_compact`, `session_start`, `session_end`, `turn_start`, `turn_end`) or stop exporting the others, and make `RegisterHook` return an error for an event that is never fired. Implement or remove the `modify`, `retry` and `deny` message semantics. Website page: `sdk/events-and-hooks`.

### H5. `AddToolHook` cannot be used from outside the module (verified)

`pkg/sdk` exposes `Client.AddToolHook(ToolHook)` and aliases `ToolHook`, `ToolHookInput`, `ToolHookStage`, but not the type that `ToolHook.Execute` returns (`ToolHookResult`, with `ToolHookStop`). A host in another module cannot name that type, so it cannot write a tool hook. Only the shell hooks (`PreToolHooks`) work. A task for this is already open.

**Fix:** alias `ToolHookResult` and `ToolHookStop` in `pkg/sdk/types.go` and add an external test.

### H6. Four runtime event types are defined and never emitted (verified)

`tool.permission_required`, `prompt.request`, `turn.stage`, `plan.status_changed` (`internal/types/runtime_events.go`). Nothing emits them. Permission questions only reach a host through the prompt function (`SetPromptFn`), so an interface that listens to runtime events to show an approval dialog never gets one.

**Fix:** emit them where the corresponding thing happens, or delete the names. Website page: `sdk/events-and-hooks`.

---

## Medium

### M1. The blocking limit of compaction is never enforced (verified)

`BlockingLimitTokens` and `ManualCompactBufferTokens` (`internal/runtime/memory/engine.go:91`, `config.go`) are never used. Nothing checks that a request fits the window before it is sent, and the error codes `ErrCodeCompactFailed` and `ErrCodeCompactFull` (`internal/types/errors.go`) are never returned. A request that does not fit fails at the provider.

**Fix:** check before sending, return a typed error, or remove the dead settings.

### M2. The auto-mode classifier ignores project instructions and keeps no transcript (verified)

`SetSeshatMdProvider` (`internal/permissions/auto/seshat_md.go`) is never called, so the classifier never sees the user's `SESHAT.md` preferences. `BuildTranscriptForClassifier`, `LogParseFailure`, `BuildSystemPromptWithSeshatMd` and `ExtractSeshatMdRules` are never called, and `internal/permissions/engine.go` still carries the comment "Track classifier transcript for debug, not yet implemented".

Also, when no classifier is configured, auto mode allows every call, including destructive ones (`checkGlobalPermission`, step 4.5). A classifier is normally wired (`SetAutoModeProviderClient`), so this is the fallback, but it fails open.

**Fix:** wire the provider (and decide how it relates to H3), or remove the code. Consider failing closed (ask) when no classifier is available.

### M3. Default limits disagree (verified)

`engine.DefaultConfig()` sets `MaxTurns` to 100 (`internal/engine/config.go:71`), the loop defaults to 200 (`loop.go:86`), and the SDK default is 100. The comment on `Config.MaxIterations` says "0 = loop default (10)" but the loop default is 100. Pick one source of truth and fix the comments.

### M4. MCP features that exist but are not connected (candidate)

Never called: `ConnectWithOAuth`, `RemoveMcpServer`, `SetMcpServerEnabled`, `RegisterPluginMcpServer`, `GetPluginMcpServers` and `ExpandEnvVars` (`internal/tools/system/mcp/config.go`). So OAuth-protected MCP servers, enabling, disabling and removing a server at runtime, plugin servers and `${VAR}` expansion in MCP configs are not reachable. Check what the CLI and the SDK do instead, then wire or delete. Website page: `concepts/skills-and-mcp`.

### M5. Skill features that exist but are not connected (candidate)

Never called: `ActivateConditionalSkillsForPaths`, `DiscoverSkillDirsForPaths`, `RegisterMCPSkillBuilder`, `InvalidateSkillCache`, `ClearSkillCaches`, `OnSkillChange`, `NotifySkillChange`, `RegisterSkillAlias`, `ResolveSkillAlias` (`internal/tools/system/skills`). Skills that depend on file paths, skills coming from MCP servers, and live reload are therefore not active. Wire them or delete them.

### M6. Automation API keys cannot be created or revoked (candidate)

The tables and migrations exist (`internal/db/automation_daemon_migrations.go`), but `CreateAutomationAPIKey`, `ListAutomationAPIKeys`, `RevokeAutomationAPIKey` and `DeleteAutomationAPIKey` (`internal/db/automation_apikeys.go`) are never called. Check what authenticates the automation daemon API today, and wire key management or remove the schema.

### M7. The tweet tool is never registered (candidate)

`twitter.NewTweetTool` exists but nothing registers it (the search tools are registered, and the Reddit tools were wired by recent commits). It posts to a public account, so leaving it out may be deliberate (it is also left out of the always-safe list on purpose). Decide: register it behind credentials, or delete it.

### M8. Compaction transcript helpers are never called (candidate)

In `internal/types/message.go`: `MessagesFromTranscriptEntries`, `FilterCanonicalTranscriptEntries`, `LastCompactionMessage`, `BuildCompactionMetadata`, `WithoutCompactionMetadata`, `IsTranscriptCompactionEntry`, `IsTranscriptSystemEntry`, `MessageTranscriptMetadataSignature`, `CountCompactionArtifacts`, `CountPreservedTail*`. Resuming a session after compaction may use another path. Check, then delete or wire.

---

## Low

### L1. Dead functions and methods (candidate)

| Where | What |
|---|---|
| `internal/engine/loop.go` | `Loop.sendAPIRequest` |
| `internal/execution/streaming_executor.go` | `SubmitToolUseChunk`, `GetCompletedResults`, `GetPendingCount` |
| `internal/providers` | `Client.buildURL`, `Client.calculateNextBackoff`, `Client.waitDuration` (is retry backoff implemented twice?) |
| `internal/tools/special/tool_search` | `IsToolSearchEnabled`, `GetDeferredTools`, `GetNonDeferredTools`, `GetAutoToolSearchCharThreshold`, `ModelSupportsToolReference`, `ExtractDiscoveredToolNames`, `DefaultToolSearchToolConfig` |
| `internal/modes/execution` | many getters and setters of plan and pair-programming state |
| `internal/storage` | `StorePDF`, `StoreScreenshot`, `StoreDocument`, `LoadDocument`, `GetFileURL`, `StatFile`, `ListFiles`, `OpenFileReader`, `ReadFileMetadata`, `RunGarbageCollection`, `MoveFile`, `DownloadFile`, `PDFKey`, `DocumentKey` |
| `internal/nativedoc` | `dlaRecordPostUnclip`, `dlaFlushPostUnclip`, `math_min`, `math_max` |
| `internal/utils/git.go` | `GetCachedDefaultBranch`, `GetCachedRemoteUrl`, `DirIsInGitRepo`, `IsAtGitRoot`, `GetGitExe` |
| `internal/tools/task` | `StatusToString`, `FormatTaskOutput`, `cloneRuntimeTask`, `sortRuntimeTasks` |
| `internal/tools/bash/security.go` | `isWorkspaceWriteCommand` |
| `internal/tools/files` | `edit.normalizedBaseName`, `glob.encodeOutput`, `glob.decodeOutput` |
| `internal/web` | `screenshotKey`, `downloadKey`, `readPageTitle`, `GetCurrentMode`, `truncatePreview` |

### L2. Unused constants (candidate)

Error codes (`ErrCodeToolExecution`, `ErrCodeToolPermission`, `ErrCodeToolTimeout`, `ErrCodePermissionError`, `ErrCodeSession*`, `ErrCodeNotSupported`), `HookStagePost`, `HookStageAround`, `SessionStatusIdle`, `SectionTypeUser`, `SectionTypeSystem`, `SectionTypeAppend`, `EnvironmentWorktree`, `EnvironmentUnknown`, `AccessNetwork`, `ToolSurfaceProfileSkillAgent`, `MetricTypeTimer`, `AgentSourceProject`, `AgentEventTurnUpdate`, `AgentEventToolUse`, `StatusUsageLimited`, `APIProviderGoogle`, `APIProviderAzure`, `APIProviderZAI`, `TokenTypeID`. Unused error codes mean failures are reported with generic errors, so callers cannot tell them apart.

### L3. Old default model identifiers (candidate)

The engine, the SDK and the compaction defaults name `claude-3-5-sonnet-20241022` and `claude-3-5-haiku-20241022`. These are 2024 identifiers. Check they still resolve, and move the defaults to one place.

---

## Already corrected in the documentation

The website pages were fixed on 2026-10-09 to match the code (see the `seshat-ai` repository, pull requests 139 and 140): hook events, runtime events, the permission order, the `bypass` and `auto` modes, the blocking limit, and the default number of turns. Fixing the code does not need the pages to change before, but the pages must be updated when an item above is fixed.

## How this list was made, and its limits

Hand reading of `internal/engine`, `internal/runtime/memory`, `internal/permissions`, `internal/execution`, `internal/prompt`, `pkg/sdk` and the hook and event definitions, then a scan of `internal/`, `pkg/` and `cmd/` (excluding the `seshattui` fork and generated protobuf code) for functions, methods and constants whose name is referenced nowhere else. A name that appears anywhere counts as used, so the scan misses dead code that shares a name with live code: the real amount is probably larger. Items marked candidate have not been checked by hand. A static tool such as `deadcode` or `staticcheck` (U1000) would give a more exact list and could run in CI.
