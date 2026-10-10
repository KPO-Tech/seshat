# TODO: bugs to fix (important)

**These are errors, not wishes.** They were found by reading the code on 2026-10-09 (branch `dev`, commit `a1eee2f`, rechecked on `dea175c`) while documenting the runtime for the website. Each one is either a behavior that does not match what the code is supposed to do, a security gap, or a feature that exists in the code but is never connected, so it never runs.

Status of each item: **verified** means the code was read and the problem reproduced by reading the call sites. **candidate** means a scan found a name that is never referenced; check before deleting or wiring it.

When you fix one, move it to a "Fixed" section at the bottom with the PR number, and update the website page named in the item (docs are in the `seshat-ai` repository, `website/src/content/docs/en`).

---

## High

None open - H1 through H8 are all fixed, see the Fixed section at the bottom.

---

## Medium

### M2. The auto-mode classifier ignores project instructions and keeps no transcript (verified)

`SetSeshatMdProvider` (`internal/permissions/auto/seshat_md.go`) is never called, so the classifier never sees the user's `SESHAT.md` preferences. `BuildTranscriptForClassifier`, `LogParseFailure`, `BuildSystemPromptWithSeshatMd` and `ExtractSeshatMdRules` are never called, and `internal/permissions/engine.go` still carries the comment "Track classifier transcript for debug, not yet implemented".

Also, when no classifier is configured, auto mode allows every call, including destructive ones (`checkGlobalPermission`, step 4.5). A classifier is normally wired (`SetAutoModeProviderClient`), so this is the fallback, but it fails open.

**Fix:** wire the provider (and decide how it relates to H3), or remove the code. Consider failing closed (ask) when no classifier is available.

**Not started (2026-10-10):** this is security-relevant (fails open, similar in spirit to H3) and comparable in size to H4/H8 - needs its own focused pass, not a quick fix alongside smaller Medium items.

### M4. MCP features that exist but are not connected - partially fixed (candidate)

Never called: `ConnectWithOAuth`, `RemoveMcpServer`, `SetMcpServerEnabled`, `RegisterPluginMcpServer`, `GetPluginMcpServers` and `ExpandEnvVars` (`internal/tools/system/mcp/config.go`).

**Fixed (2026-10-10):** `ExpandEnvVars` is now called from `LoadMcpConfigs` for every server as it loads, so `${VAR}` in a server's `Command`/`Args`/`URL`/`Env`/`Headers` is actually substituted - a missing variable produces a non-fatal `ValidationError` (severity `warning`) instead of silently passing the literal placeholder to the server process. Its own missing-variable detection was separately broken (checked the *expanded* string for `${`, which `os.ExpandEnv` always already consumes - so it could never fire) and is fixed too. `RemoveMcpServer`/`SetMcpServerEnabled` are now re-exported as `pkg/mcp.RemoveServer`/`pkg/mcp.SetServerEnabled`, mirroring the existing `pkg/mcp.AddServer` pattern exactly. Tests: `TestLoadMcpConfigs_ExpandsEnvVarsInLocalConfig`, `TestLoadMcpConfigs_WarnsOnMissingEnvVar` (`internal/tools/system/mcp/load_configs_env_test.go`).

**Not started:** `ConnectWithOAuth` (a real OAuth consent/callback flow) and `RegisterPluginMcpServer`/`GetPluginMcpServers` ("plugin" MCP servers) are substantial, under-specified feature work, not a quick wire-up - deferred.

### M5. Skill features that exist but are not connected (candidate)

Never called: `ActivateConditionalSkillsForPaths`, `DiscoverSkillDirsForPaths`, `RegisterMCPSkillBuilder`, `InvalidateSkillCache`, `ClearSkillCaches`, `OnSkillChange`, `NotifySkillChange`, `RegisterSkillAlias`, `ResolveSkillAlias` (`internal/tools/system/skills`). Skills that depend on file paths, skills coming from MCP servers, and live reload are therefore not active. Wire them or delete them.

### M5b. Skill header fields that are read and never applied (verified)

In the runtime path (the `skill` tool calls `ExecuteSkillPrompt` and returns the body): `allowed-tools` and `model` are ignored (they only matter for a skill that also has an `agent:` field, which `internal/agent/registry.go` turns into a sub-agent type); `context: fork` sets a field nobody reads (`ExecutionContextFork`); `shell` and `hooks` are parsed and never executed; `when_to_use` is not part of the list the model sees (`FormatSkillsList` shows name, description and triggers); `disable-model-invocation` is not checked there. `arguments` only reads a list of names or a comma-separated string, so the structured form documented in `docs/skills.md` (name, description, default) gives no argument names. `MatchTrigger` and `GetSkillsForContext` (triggers and `paths`) are only exposed through `pkg/skills`, nothing in the runtime calls them.

**Fix:** apply these fields (run the skill in a sub-agent for `fork`, restrict tools, run the shell commands) or stop parsing them, and correct `docs/skills.md`. Website page: `concepts/skills-and-mcp`.

**Not started (2026-10-10):** comparable in size to H4 (several independent fields, each needing its own small design decision) - needs its own focused pass.

### M5c. Sub-agents always run in `bypass` mode, and two built-in types do not match their name (verified)

`agent_tool.go` (lines 441, 795, 935) starts every sub-agent with `PermissionMode: types.PermissionModeBypass`. After one approval to start it, a `general-purpose` sub-agent has every tool, including the shell, with no further question (deny rules and the safety check still apply). That may be the intent for headless runs, but it should be a decision: the parent's mode is not inherited, and `never` or `onRequest` in the parent does not constrain the child.

The `plan` type has `write_file` and `edit_file` (`internal/agent/loader.go`), so it is not read-only. The `verify` type is not a general "check the result" reviewer: it validates agent definitions, security constraints and tool permissions, read-only. Rename it, or give the type a prompt that matches what users expect. Website page: `concepts/agents-and-delegation`.

**Deferred (2026-10-10):** both halves are product decisions (what sub-agent permission inheritance should be; what `plan`/`verify` should be named or do), not bugs with one clear correct fix - left to the maintainer rather than decided unilaterally.

### M5d. Configuration: keys in clear text, missing variable, stale comments - partially fixed (verified)

The terminal interface saves a provider key typed in its providers screen as `providers.<id>.api_key` in `seshat.json` (`internal/seshattui/config/store.go:322`), in clear text with owner-only permissions. `seshat config` stores its keys encrypted (AES-GCM, `internal/db/credentials.go`). The two paths protect keys differently, and the interface never offers the encrypted store. **Not started:** rewiring the terminal interface's settings UI to the encrypted store is a larger change to a user-facing credential flow - deferred rather than risked under time pressure.

`generate_image`, `text_to_speech` and `speech_to_text` stay inactive unless a provider is set in the `image_generation`, `text_to_speech` or `speech_to_text` section: with only `OPENAI_API_KEY` set, nothing happens and nothing says why. Website page: `getting-started/configuration`. **Not started.**

**Fixed (2026-10-10):**
- `providerEnvVars` in `pkg/sdk/auth.go` had no case for Kimi (fell through to the generic `API_KEY`) and disagreed in order with `pkg/config/provider_catalog.go`'s `ProviderCredentialEnvVars` for Z.ai and Codex. Added the missing Kimi case and reordered Z.ai/Codex to match the catalog, which is now the one source of truth for env var priority.
- Comments in `pkg/config/config.go` and `internal/hooks/hook.go` said config lived in `.seshat.yaml`; the file `LoadInto` actually reads (confirmed by its viper setup) is `config.yaml`. Comments corrected.
- Investigated "three names for the runtime folder" (`seshat-cli` vs `seshat-tui` vs `appdir`'s doc comment): `seshat-cli` and `seshat-tui` are two **intentionally separate** runtime roots for the CLI's headless mode and the interactive TUI respectively - confirmed by an existing test (`cmd/cli/tui_seshat_test.go`) that locks in `seshat-tui` as correct. Not a bug; no change made.

### M5e. Plan mode is not enforced, and other modes and fields that do nothing (verified)

Plan mode (`ForcePlanMode`, `enter_plan_mode`) only changes instructions and the permission context. No tool is blocked by it: the comment on `ExecutionModePlan` says "tools are described but not executed", which is not what happens. Worse, `write_file` and `edit_file` auto-allow writes inside the working directory when `ExecutionMode == "plan"` (`internal/tools/files/write/write.go:495`, `edit/edit.go:411`), so a plan-mode session asks fewer questions about writing files than a normal one. Decide: enforce read-only in plan mode (deny writes except the plan file), or rename the mode and fix the comments.

`ExecutionModePairProgramming` has helper functions and setters (`internal/modes/execution/pair_programming.go`) but nothing reads it except a label in the terminal interface. Implement it or remove it. Comments in `contract/context.go` and `types/permissions.go` mention a `browse` execution mode that does not exist.

Workflows (`pkg/workflow/workflow.go`): a `router` node never routes, because `Run` executes every node whose dependencies succeeded, and a `verifier` that answers FAIL is still a success. The default executor in `pkg/sdk/workflow.go` ignores `agent` and `max_turns`. A level also waits for the whole previous level. Decide whether kinds should control the flow, and implement it or document them as prompt roles only (the website now says so). Website pages: `sdk/workflows`, `concepts/runtime-modes`.

**Not started (2026-10-10):** three largely independent problems (plan-mode write enforcement, pair-programming mode, workflow node semantics), each comparable in size to H4 - needs its own focused pass, likely split further.

### M7. The tweet tool is never registered (candidate)

`twitter.NewTweetTool` exists but nothing registers it (the search tools are registered, and the Reddit tools were wired by recent commits). It posts to a public account, so leaving it out may be deliberate (it is also left out of the always-safe list on purpose). Decide: register it behind credentials, or delete it.

**Deferred (2026-10-10):** enabling a tool that posts publicly, or deleting the code outright, is a product decision - left to the maintainer rather than decided unilaterally.

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

Fixed on 2026-10-10, on branch `docs/todo-bugs`. H1-H8 are committed (`a416dc6`, `da4bfc6`); M1/M3/M4/M5d(partial)/M5f/M5g/M6/M8 below are not yet committed. Build, `go vet`, `golangci-lint` and `go test -race` all pass for every changed package. The website pages for these items still need the matching update in the `seshat-ai` repository.

- **H1** (compaction summary always used an Anthropic model): `generateSummary` now defaults to the session's own model (`internal/runtime/memory/engine.go`), with an explicit override exposed as `ClientConfig.CompactSummaryModel` / `engine.Config.CompactSummaryModel`. Tests: `TestGenerateSummary_NonAnthropicProvider`, `TestGenerateSummary_ExplicitOverrideWins` (`internal/runtime/memory/summary_test.go`).
- **H2** (compaction circuit breaker could never trip): `AutoCompactFailureCount` moved from `MutableState` (per-turn, reset every `Loop.Run`) to `SessionState` (persists across turns), threaded through new `RunRequest`/`RunResult.AutoCompactFailureCount` fields and `Session.submitWithMessage`. Test: `TestAutoCompactFailureCountPersistsAcrossRuns` (`internal/engine/engine_test.go`).
- **H3** (project instruction files bypassed project trust): `SESHAT.md`/`AGENTS.md`/`.seshat/instructions.md` now gated behind the same hash-based trust store as the other project configuration files (`internal/seshattui/config/trust.go`'s `IsTrustedProjectFile`, `TrustProject`/`UntrustProject`/`ProjectConfigStatus` extended to cover them). New `engine.Config.TrustProjectInstructions` / `ClientConfig.TrustProjectInstructions` callback, nil-fails-closed by default; wired in `cmd/cli/runtime.go` and `seshat_workspace.go` to preserve existing behavior for trusted projects. `docs/project-trust.md` updated. Tests: `TestReadTrustedProjectInstructions_*` (`internal/engine/engine_test.go`), `TestUntrustedProjectInstructionsAreNotTrusted`, `TestTrustProjectIncludesInstructionFiles`, `TestTrustProjectIgnoresAbsentInstructionFiles` (`internal/seshattui/config/trust_instructions_test.go`).
- **H4** (most hook events never emitted): added `session_start`/`session_end` (`Engine.NewSessionFromState`/`Session.Close`), `turn_start`/`turn_end` (`Session.submitWithMessage`), `pre_tool_use`/`post_tool_use` (per tool call in `Loop.Run`), `pre_compact`/`post_compact` (`Loop.maybeAutoCompact`). `pre_tool_use` Modify rewrites the tool's input before execution; every stop-capable hook (`iteration_start`, `tool_uses_start`, `turn_start`, `pre_tool_use`, `post_tool_use`, `pre_compact`) now appends the hook's deny `Message` to the transcript instead of only logging it (`appendSystemNoteFromHook`/`appendHookDenialAndStop` in `loop.go`). `Retry` is still a documented no-op (needs a rework of the batched tool-execution pipeline - out of scope here). `sdk.Client.RegisterHook` now returns `(string, error)` and rejects any event not in the new `types.ImplementedHookEvents` - a breaking change, with zero in-repo callers to migrate. Tests: `TestPreCompactHookDenyPreventsCompaction`, `TestPostCompactHookFiresWithDidCompact`, `TestPreToolUseHookModifiesInput`, `TestPreToolUseHookDenyStopsTurnBeforeExecution`, `TestPostToolUseHookDenyStopsTurnAfterExecution`, `TestSessionStartAndEndHooksFire`, `TestTurnStartHookCanDenyTurn` (`internal/engine/engine_test.go`), `TestRegisterHook_RejectsUnimplementedEvent`, `TestRegisterHook_AcceptsImplementedEvent` (`pkg/sdk/client_sessions_test.go`).
- **H5** (`AddToolHook` unusable from outside the module): aliased `ToolHookResult`/`ToolHookStop` in `pkg/sdk/types.go`, next to the existing `ToolHook`/`ToolHookInput`/`ToolHookStage`. Test: `TestAddToolHook_UsableFromOutsideModule` (`pkg/sdk/tool_hook_external_test.go`, package `sdk_test` - proves it compiles and runs with only the public API).
- **H6** (4 runtime event types never emitted): implemented `tool.permission_required` (`internal/permissions/integration.go`, right before the `PromptFn` ask-approval call), `prompt.request` (`internal/tools/special/ask_user/askUserQuestionTool.go`, both prompt call sites), and `turn.stage` (`Session.submitWithMessage`, one `"preparing_turn"` stage before `buildAPIRequest`). Deleted `plan.status_changed` instead of implementing it: `PlanStore.SetStatus` (the only thing that could mutate a plan's status) has zero callers anywhere in the codebase, so the event described a feature that does not exist - implementing it would have meant inventing a plan-approval workflow, out of scope for a bug fix. Tests: `TestResolverEmitsToolPermissionRequiredBeforePrompting`, `TestResolverNoRuntimeEventWithoutEmitter` (`internal/permissions/runtime_event_test.go`), `TestAskQuestionsEmitsPromptRequiredEvent` (`internal/tools/special/ask_user/runtime_event_test.go`), `TestTurnStageEventFiresBeforeBuildingRequest` (`internal/engine/engine_test.go`).
- **H7** (MCP servers silently skipped without `Transport`): `mcp.NewClient` now infers `Transport` from `Command` (stdio) or `URL` (http) when unset, matching every example in `README.md`/`docs/sdk.md`/`docs/mcp.md` - those examples now work instead of silently connecting nothing. Added a note to `docs/mcp.md` documenting the inference and the pre-existing `websocket`-uses-HTTP-transport nuance. Tests: `TestNewClient_InfersStdioTransportFromCommand`, `TestNewClient_InfersHTTPTransportFromURL`, `TestNewClient_ExplicitTransportNotOverridden`, `TestNewClient_NeitherCommandNorURLStillErrors` (`internal/tools/system/mcp/transport_inference_test.go`).
- **H8** (`.seshat/skills` visible in the terminal interface but unusable by the model): investigation found three separate skill systems, not two - `internal/tools/system/skills` (the file loader) and `internal/tools/system/seshatskill` (the `seshat_list_skills`/`seshat_read_skill` tools actually exposed to the model) both go through one aggregation function, `GetSkillDirCommandsForUser`; `internal/seshattui/skills` is a third, independent catalog (different frontmatter fields, used only for the terminal interface's own display and manual skill-attach) that was already compatible enough with the first system's parser not to need touching. Fixed the one gap that actually caused the reported bug: `GetSkillDirCommandsForUser` built its own hardcoded source list and never consulted the receiving loader's own `Directory` field (which `NewFileSkillLoader("")` already defaults to `.seshat/skills`) - now it does, so a skill placed there reaches `seshat_list_skills`/`seshat_read_skill`/the `skill` tool like every other source. Also implemented `SeshatWorkspace.ReadSkill`, previously a complete stub (always returned nil/empty) that silently broke the terminal interface's "attach skill" action; it now delegates to the catalog package's own already-correct `skills.ReadContent`. Full folder-list unification across all three systems (and their two different frontmatter formats) was investigated but kept out of scope - not needed to resolve the reported bug, and the terminal interface's catalog was already working correctly on its own terms. Tests: `TestGetSkillDirCommandsForUser_ReadsLegacySkillsDirectory`, `TestGetSkillDirCommandsForUser_ReadsCustomLoaderDirectory` (`internal/tools/system/skills/legacy_skills_dir_test.go`), `TestReadSkillReturnsRealContent`, `TestReadSkillUnknownIDReturnsError` (`internal/seshattui/workspace/seshat_workspace_read_skill_test.go`).

- **M1** (compaction blocking limit never enforced): added `CheckBlockingLimit` to the `CompactionStrategy` interface, implemented on `compact.Engine` (uses the already-computed `BlockingLimitTokens`/`CalculateUsage`), called from `Loop.maybeAutoCompact` right after an auto-compact attempt (whether or not it actually compacted). A request that still doesn't fit now fails with a typed `ErrCodeCompactFull` `*types.EngineError` before ever reaching the provider. Gated behind `AutoCompact` (so a nil compactor with `AutoCompact: false`, the common test setup, is unaffected). Tests: `TestCheckBlockingLimit_ReturnsTypedErrorWhenOverLimit`, `TestCheckBlockingLimit_NilWhenWithinLimit` (`internal/runtime/memory/blocking_limit_test.go`), `TestMaybeAutoCompact_BlockingLimitExceeded_EndsTurnWithoutCallingModel` (`internal/engine/engine_test.go`).
- **M3** (default limits disagree): unified `MaxTurns` to 100 everywhere - `DefaultLoopConfig`/`normalizeLoopConfig` (was 200, though this field turned out to be dead: nothing in `loop.go` reads `LoopConfig.MaxTurns`, the live limit is `engine.Config.MaxTurns` via `Session.enforceMaxTurns` - documented in place rather than removed, since `json:"max_turns"` may be serialized by a caller). Fixed `engine.Config.MaxIterations`'s stale doc comment (said "loop default (10)", actually 100). Also found and fixed a third disagreement the TODO item didn't mention: `pkg/sdk.NewClient` built straight from a `&ClientConfig{}` that skipped `DefaultClientConfig()` got `MaxTurns: 0` = unlimited, not 100 - `buildEngineConfig` now defaults it like `MaxTokens` already does. Tests: `TestBuildEngineConfig_DefaultsMaxTurnsWhenUnset`, `TestBuildEngineConfig_PreservesExplicitMaxTurns` (`pkg/sdk/max_turns_default_test.go`).
- **M5f** (default Anthropic→OpenAI fallback, privacy): removed the hardcoded `FallbackProviders: [openai]` from Anthropic's default `providers.Config`; same-vendor `FallbackModels` (sonnet→haiku) kept. Cross-vendor fallback is now opt-in only, via `ProviderConfig.Routing.FallbackProviders`. Test: `TestAnthropicDefaultConfigHasNoCrossVendorFallback` (`internal/providers/no_default_cross_vendor_fallback_test.go`).
- **M5g** (`sdk.SandboxAvailable()` Landlock-only): found already fixed in a prior session - `Client.SandboxKind()` exists, reports the sandbox this specific client actually resolved to (Docker only if requested and healthy at construction), with its own tests (`TestClientSandboxKindReportsLocalByDefault`, `TestClientSandboxKindFallsBackWhenDockerUnavailable` in `pkg/sdk/client_test.go`) and a doc comment on the package-level `SandboxAvailable()` pointing hosts to it. No code change needed; moved here for bookkeeping.
- **M6** (automation daemon API keys never wired): removed rather than wired. `internal/db/automation_apikeys.go` (all 10 functions, not just the 4 TODO named - including the key-verification path, `GetAutomationAPIKeyByHash`) and `internal/db/automation_daemon_migrations.go` deleted; `DB.InitializeAutomationDaemon`/`runSQLiteAutomationDaemonMigrations`/`migrationScopeAutomationDaemon` removed. Confirmed vestigial: no `cmd/automation-daemon` binary exists in this repo to call `InitializeAutomationDaemon` (its own doc comment said to call it "in the seshat-automation binary"), and `internal/tools/automation/client.go` shows this repo is only a *client* of an external daemon authenticated by one static bearer token (`AUTOMATION_API_KEY`), not a host of one - matches automation having moved to SeshatCloud.
- **M8** (compaction/transcript helpers never called): deleted 10 TODO-named dead functions in `internal/types/message.go` plus 7 more that only became dead as a result (`IsCompactionArtifactMessage`, `IsCompactionSummaryMessage`, `IsCompactionMarkerMessage`, `preservedTailForMetrics`, `IsCanonicalTranscriptEntry`) - 17 total, all confirmed zero callers by exact-match grep, none re-exported in `pkg/`. The live replacements (`CanonicalMessagesFromTranscriptEntries`, `CanonicalTranscriptPrefix`, etc.) were left untouched.
- **M4** / **M5d** (partial): see the updated M4/M5d entries above in the Medium section for what was fixed vs. deferred within each - both items were split between a quick, safe fix and a larger deferred half.

**Side-finding (not fixed, not yet in this list):** while investigating H6's `plan.status_changed`, found that `PlanStore.SetStatus` (`internal/tools/builtin/config.go`) - the interface method for marking a plan document pending/validated/rejected - has no caller anywhere in the codebase either. `SeshatWorkspace.ApprovePlan` only exits plan mode; it never calls `SetStatus`. This looks like an abandoned/incomplete plan-approval feature, separate from H8's skill-loading gap. Worth its own TODO item if the feature is still wanted.

## Already corrected in the documentation

The website pages were fixed on 2026-10-09 to match the code (see the `seshat-ai` repository, pull requests 139 and 140): hook events, runtime events, the permission order, the `bypass` and `auto` modes, the blocking limit, and the default number of turns. Fixing the code does not need the pages to change before, but the pages must be updated when an item above is fixed.

## How this list was made, and its limits

Hand reading of `internal/engine`, `internal/runtime/memory`, `internal/permissions`, `internal/execution`, `internal/prompt`, `pkg/sdk` and the hook and event definitions, then a scan of `internal/`, `pkg/` and `cmd/` (excluding the `seshattui` fork and generated protobuf code) for functions, methods and constants whose name is referenced nowhere else. A name that appears anywhere counts as used, so the scan misses dead code that shares a name with live code: the real amount is probably larger. Items marked candidate have not been checked by hand. A static tool such as `deadcode` or `staticcheck` (U1000) would give a more exact list and could run in CI.
