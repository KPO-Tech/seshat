# TODO: bugs to fix (important)

**These are errors, not wishes.** They were found by reading the code on 2026-10-09 (branch `dev`, commit `a1eee2f`, rechecked on `dea175c`) while documenting the runtime for the website. Each one is either a behavior that does not match what the code is supposed to do, a security gap, or a feature that exists in the code but is never connected, so it never runs.

Status of each item: **verified** means the code was read and the problem reproduced by reading the call sites. **candidate** means a scan found a name that is never referenced; check before deleting or wiring it.

When you fix one, move it to a "Fixed" section at the bottom with the PR number, and update the website page named in the item (docs are in the `seshat-ai` repository, `website/src/content/docs/en`).

---

## High

### H8. Skills in `.seshat/skills` are listed by the terminal interface but the agent cannot load them (verified)

There are two skill systems. The runtime loader (`internal/tools/system/skills/loader.go`) reads `.claude/skills` in the project, `.seshat/commands`, and the user, managed, builtin and repos folders under the runtime root. `NewFileSkillLoader("")` defaults its directory to `.seshat/skills` (line 71) but `GetSkillDirCommandsForUser` never uses it, so that folder is not read. The terminal interface (`internal/seshattui/skills`) discovers skills in `.agents/skills`, `.seshat/skills`, `.claude/skills` and `.cursor/skills` and shows them in its catalog, but nothing feeds them to the agent: `ToPromptXML` is never called and `SeshatWorkspace.ReadSkill` returns nothing.

**Effect:** a skill put in `.seshat/skills/` (the folder the old documentation recommended) is visible in the interface and unusable by the model.

**Fix:** make the terminal interface and the runtime read the same folders, from one list. Website page: `concepts/skills-and-mcp`.

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

### M5b. Skill header fields that are read and never applied (verified)

In the runtime path (the `skill` tool calls `ExecuteSkillPrompt` and returns the body): `allowed-tools` and `model` are ignored (they only matter for a skill that also has an `agent:` field, which `internal/agent/registry.go` turns into a sub-agent type); `context: fork` sets a field nobody reads (`ExecutionContextFork`); `shell` and `hooks` are parsed and never executed; `when_to_use` is not part of the list the model sees (`FormatSkillsList` shows name, description and triggers); `disable-model-invocation` is not checked there. `arguments` only reads a list of names or a comma-separated string, so the structured form documented in `docs/skills.md` (name, description, default) gives no argument names. `MatchTrigger` and `GetSkillsForContext` (triggers and `paths`) are only exposed through `pkg/skills`, nothing in the runtime calls them.

**Fix:** apply these fields (run the skill in a sub-agent for `fork`, restrict tools, run the shell commands) or stop parsing them, and correct `docs/skills.md`. Website page: `concepts/skills-and-mcp`.

### M5c. Sub-agents always run in `bypass` mode, and two built-in types do not match their name (verified)

`agent_tool.go` (lines 441, 795, 935) starts every sub-agent with `PermissionMode: types.PermissionModeBypass`. After one approval to start it, a `general-purpose` sub-agent has every tool, including the shell, with no further question (deny rules and the safety check still apply). That may be the intent for headless runs, but it should be a decision: the parent's mode is not inherited, and `never` or `onRequest` in the parent does not constrain the child.

The `plan` type has `write_file` and `edit_file` (`internal/agent/loader.go`), so it is not read-only. The `verify` type is not a general "check the result" reviewer: it validates agent definitions, security constraints and tool permissions, read-only. Rename it, or give the type a prompt that matches what users expect. Website page: `concepts/agents-and-delegation`.

### M5d. Configuration: keys in clear text, missing variable, stale comments (verified)

The terminal interface saves a provider key typed in its providers screen as `providers.<id>.api_key` in `seshat.json` (`internal/seshattui/config/store.go:322`), in clear text with owner-only permissions. `seshat config` stores its keys encrypted (AES-GCM, `internal/db/credentials.go`). The two paths protect keys differently, and the interface never offers the encrypted store.

`providerEnvVars` in `pkg/sdk/auth.go:294` has no case for Kimi, so it returns the generic `API_KEY`, while `pkg/config/provider_catalog.go:146` accepts `MOONSHOT_API_KEY` and `KIMI_API_KEY`. Two lists of provider variables exist (`providerEnvVars`, `ProviderCredentialEnvVars`) and they disagree for Kimi and in order for Z.ai and Codex.

Code comments still say `.seshat.yaml` (`pkg/config/config.go:150`, `internal/hooks/hook.go:69`) but the file read is `config.yaml` (viper, `pkg/config/config.go:230`). `docs/` and the CLI help should be checked for the same name. Also, `cmd/cli/appdir/appdir.go` documents `~/.config/seshat-tui/` while `main.go` sets `seshat-cli`: three names for the runtime folder.

`generate_image`, `text_to_speech` and `speech_to_text` stay inactive unless a provider is set in the `image_generation`, `text_to_speech` or `speech_to_text` section: with only `OPENAI_API_KEY` set, nothing happens and nothing says why. Website page: `getting-started/configuration`.

### M5e. Plan mode is not enforced, and other modes and fields that do nothing (verified)

Plan mode (`ForcePlanMode`, `enter_plan_mode`) only changes instructions and the permission context. No tool is blocked by it: the comment on `ExecutionModePlan` says "tools are described but not executed", which is not what happens. Worse, `write_file` and `edit_file` auto-allow writes inside the working directory when `ExecutionMode == "plan"` (`internal/tools/files/write/write.go:495`, `edit/edit.go:411`), so a plan-mode session asks fewer questions about writing files than a normal one. Decide: enforce read-only in plan mode (deny writes except the plan file), or rename the mode and fix the comments.

`ExecutionModePairProgramming` has helper functions and setters (`internal/modes/execution/pair_programming.go`) but nothing reads it except a label in the terminal interface. Implement it or remove it. Comments in `contract/context.go` and `types/permissions.go` mention a `browse` execution mode that does not exist.

Workflows (`pkg/workflow/workflow.go`): a `router` node never routes, because `Run` executes every node whose dependencies succeeded, and a `verifier` that answers FAIL is still a success. The default executor in `pkg/sdk/workflow.go` ignores `agent` and `max_turns`. A level also waits for the whole previous level. Decide whether kinds should control the flow, and implement it or document them as prompt roles only (the website now says so). Website pages: `sdk/workflows`, `concepts/runtime-modes`.

### M5f. The default Anthropic routing falls back to OpenAI (verified, privacy)

`internal/providers/config.go:53` gives the Anthropic provider `FallbackProviders: [openai]`. A session that selected Anthropic can silently continue on OpenAI, if an OpenAI key is present and Anthropic fails. For a product whose promise is that data stays where the user put it, the default should be no cross-vendor fallback, or an explicit opt-in. Website page: `sdk/providers-and-auth`.

### M5g. `sdk.SandboxAvailable()` only reports Landlock (verified)

It calls `bashTool.SandboxAvailable()` which returns `landlockAvailable()`. A host with a working Docker sandbox still gets `false` on macOS and Windows. Add a function that reports the sandbox actually in use, or document it.

### M6. Automation API keys cannot be created or revoked (candidate)

The tables and migrations exist (`internal/db/automation_daemon_migrations.go`), but `CreateAutomationAPIKey`, `ListAutomationAPIKeys`, `RevokeAutomationAPIKey` and `DeleteAutomationAPIKey` (`internal/db/automation_apikeys.go`) are never called. Check what authenticates the automation daemon API today, and wire key management or remove the schema.

### M7. The tweet tool is never registered (candidate)

`twitter.NewTweetTool` exists but nothing registers it (the search tools are registered, and the Reddit tools were wired by recent commits). It posts to a public account, so leaving it out may be deliberate (it is also left out of the always-safe list on purpose). Decide: register it behind credentials, or delete it.

### M8. Compaction transcript helpers are never called (candidate)

In `internal/types/message.go`: `MessagesFromTranscriptEntries`, `FilterCanonicalTranscriptEntries`, `LastCompactionMessage`, `BuildCompactionMetadata`, `WithoutCompactionMetadata`, `IsTranscriptCompactionEntry`, `IsTranscriptSystemEntry`, `MessageTranscriptMetadataSignature`, `CountCompactionArtifacts`, `CountPreservedTail*`. Resuming a session after compaction may use another path. Check, then delete or wire.

---

## Low

### L0. Repository documents that describe things the code does not do (verified)

`README.md`, `docs/sdk.md` and `docs/mcp.md` (MCP examples without `Transport`, see H7). `docs/skills.md` (structured `arguments`, `allowed-tools` and `model` as if they applied to every skill, `context: fork`, `shell`, project folders). Fix them with the code, or before.

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

## Fixed

Fixed on 2026-10-10, on branch `docs/todo-bugs`, not yet merged (no PR number - review/commit pending). Build, `go vet`, `golangci-lint` and `go test -race` all pass for every changed package (`internal/engine`, `internal/runtime/memory`, `pkg/sdk`, `internal/types`, `internal/seshattui/config`, `internal/seshattui/workspace`, `cmd/cli`). The website pages for these items still need the matching update in the `seshat-ai` repository.

- **H1** (compaction summary always used an Anthropic model): `generateSummary` now defaults to the session's own model (`internal/runtime/memory/engine.go`), with an explicit override exposed as `ClientConfig.CompactSummaryModel` / `engine.Config.CompactSummaryModel`. Tests: `TestGenerateSummary_NonAnthropicProvider`, `TestGenerateSummary_ExplicitOverrideWins` (`internal/runtime/memory/summary_test.go`).
- **H2** (compaction circuit breaker could never trip): `AutoCompactFailureCount` moved from `MutableState` (per-turn, reset every `Loop.Run`) to `SessionState` (persists across turns), threaded through new `RunRequest`/`RunResult.AutoCompactFailureCount` fields and `Session.submitWithMessage`. Test: `TestAutoCompactFailureCountPersistsAcrossRuns` (`internal/engine/engine_test.go`).
- **H3** (project instruction files bypassed project trust): `SESHAT.md`/`AGENTS.md`/`.seshat/instructions.md` now gated behind the same hash-based trust store as the other project configuration files (`internal/seshattui/config/trust.go`'s `IsTrustedProjectFile`, `TrustProject`/`UntrustProject`/`ProjectConfigStatus` extended to cover them). New `engine.Config.TrustProjectInstructions` / `ClientConfig.TrustProjectInstructions` callback, nil-fails-closed by default; wired in `cmd/cli/runtime.go` and `seshat_workspace.go` to preserve existing behavior for trusted projects. `docs/project-trust.md` updated. Tests: `TestReadTrustedProjectInstructions_*` (`internal/engine/engine_test.go`), `TestUntrustedProjectInstructionsAreNotTrusted`, `TestTrustProjectIncludesInstructionFiles`, `TestTrustProjectIgnoresAbsentInstructionFiles` (`internal/seshattui/config/trust_instructions_test.go`).
- **H4** (most hook events never emitted): added `session_start`/`session_end` (`Engine.NewSessionFromState`/`Session.Close`), `turn_start`/`turn_end` (`Session.submitWithMessage`), `pre_tool_use`/`post_tool_use` (per tool call in `Loop.Run`), `pre_compact`/`post_compact` (`Loop.maybeAutoCompact`). `pre_tool_use` Modify rewrites the tool's input before execution; every stop-capable hook (`iteration_start`, `tool_uses_start`, `turn_start`, `pre_tool_use`, `post_tool_use`, `pre_compact`) now appends the hook's deny `Message` to the transcript instead of only logging it (`appendSystemNoteFromHook`/`appendHookDenialAndStop` in `loop.go`). `Retry` is still a documented no-op (needs a rework of the batched tool-execution pipeline - out of scope here). `sdk.Client.RegisterHook` now returns `(string, error)` and rejects any event not in the new `types.ImplementedHookEvents` - a breaking change, with zero in-repo callers to migrate. Tests: `TestPreCompactHookDenyPreventsCompaction`, `TestPostCompactHookFiresWithDidCompact`, `TestPreToolUseHookModifiesInput`, `TestPreToolUseHookDenyStopsTurnBeforeExecution`, `TestPostToolUseHookDenyStopsTurnAfterExecution`, `TestSessionStartAndEndHooksFire`, `TestTurnStartHookCanDenyTurn` (`internal/engine/engine_test.go`), `TestRegisterHook_RejectsUnimplementedEvent`, `TestRegisterHook_AcceptsImplementedEvent` (`pkg/sdk/client_sessions_test.go`).
- **H5** (`AddToolHook` unusable from outside the module): aliased `ToolHookResult`/`ToolHookStop` in `pkg/sdk/types.go`, next to the existing `ToolHook`/`ToolHookInput`/`ToolHookStage`. Test: `TestAddToolHook_UsableFromOutsideModule` (`pkg/sdk/tool_hook_external_test.go`, package `sdk_test` - proves it compiles and runs with only the public API).
- **H6** (4 runtime event types never emitted): implemented `tool.permission_required` (`internal/permissions/integration.go`, right before the `PromptFn` ask-approval call), `prompt.request` (`internal/tools/special/ask_user/askUserQuestionTool.go`, both prompt call sites), and `turn.stage` (`Session.submitWithMessage`, one `"preparing_turn"` stage before `buildAPIRequest`). Deleted `plan.status_changed` instead of implementing it: `PlanStore.SetStatus` (the only thing that could mutate a plan's status) has zero callers anywhere in the codebase, so the event described a feature that does not exist - implementing it would have meant inventing a plan-approval workflow, out of scope for a bug fix. Tests: `TestResolverEmitsToolPermissionRequiredBeforePrompting`, `TestResolverNoRuntimeEventWithoutEmitter` (`internal/permissions/runtime_event_test.go`), `TestAskQuestionsEmitsPromptRequiredEvent` (`internal/tools/special/ask_user/runtime_event_test.go`), `TestTurnStageEventFiresBeforeBuildingRequest` (`internal/engine/engine_test.go`).
- **H7** (MCP servers silently skipped without `Transport`): `mcp.NewClient` now infers `Transport` from `Command` (stdio) or `URL` (http) when unset, matching every example in `README.md`/`docs/sdk.md`/`docs/mcp.md` - those examples now work instead of silently connecting nothing. Added a note to `docs/mcp.md` documenting the inference and the pre-existing `websocket`-uses-HTTP-transport nuance. Tests: `TestNewClient_InfersStdioTransportFromCommand`, `TestNewClient_InfersHTTPTransportFromURL`, `TestNewClient_ExplicitTransportNotOverridden`, `TestNewClient_NeitherCommandNorURLStillErrors` (`internal/tools/system/mcp/transport_inference_test.go`).

**Side-finding (not fixed, not yet in this list):** while investigating H6's `plan.status_changed`, found that `PlanStore.SetStatus` (`internal/tools/builtin/config.go`) - the interface method for marking a plan document pending/validated/rejected - has no caller anywhere in the codebase either. `SeshatWorkspace.ApprovePlan` only exits plan mode; it never calls `SetStatus`. This looks like an abandoned/incomplete plan-approval feature, separate from H8's skill-loading gap. Worth its own TODO item if the feature is still wanted.

## Already corrected in the documentation

The website pages were fixed on 2026-10-09 to match the code (see the `seshat-ai` repository, pull requests 139 and 140): hook events, runtime events, the permission order, the `bypass` and `auto` modes, the blocking limit, and the default number of turns. Fixing the code does not need the pages to change before, but the pages must be updated when an item above is fixed.

## How this list was made, and its limits

Hand reading of `internal/engine`, `internal/runtime/memory`, `internal/permissions`, `internal/execution`, `internal/prompt`, `pkg/sdk` and the hook and event definitions, then a scan of `internal/`, `pkg/` and `cmd/` (excluding the `seshattui` fork and generated protobuf code) for functions, methods and constants whose name is referenced nowhere else. A name that appears anywhere counts as used, so the scan misses dead code that shares a name with live code: the real amount is probably larger. Items marked candidate have not been checked by hand. A static tool such as `deadcode` or `staticcheck` (U1000) would give a more exact list and could run in CI.
