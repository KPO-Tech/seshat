# Changelog

All notable changes to seshat are documented here.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).  
Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Changed
- `pkg/dataflow`: `AgentCaller.Ask` gains a fourth parameter, `tools []string` - a list of tool names to scope an "agent" node's turn to, empty for "use the resolved agent's own tools" (unchanged behavior). This is a breaking change to a public interface; every `AgentCaller` implementation needs updating to the new signature. The built-in `agentNode` gained a matching optional `tools` property (comma-separated, parsed via the new `StringListParam` helper). Actually enforcing a tools override end-to-end (resolving a tool name to a registrable `sdk.Tool`) is not wired into `internal/automation`'s `sessionAgentCaller` yet - it fails loudly with a clear error if a graph sets `tools`, rather than silently ignoring the request.
- `pkg/dataflow`: `Node` gains a `Tools []string` field (agent nodes only) naming other nodes in the same graph an agent's LLM may call as tools during its turn, in addition to (never instead of) whatever it's wired to via `Connections` - dual-use, not exclusive. A node referenced only this way (no real `Connections` of its own) is no longer scheduled eagerly by the engine; it stays dormant until actually invoked (see the new `BuildAgentTools`/`ToolSpec`). `AgentCaller.Ask` gains a fifth parameter, `graphTools []ToolSpec`, for this - another breaking interface change, same reasoning as `tools` above. `internal/automation`'s `sessionAgentCaller` wires this up for real (register/unregister the resolved tools on the session for just that one turn); every other `AgentCaller` implementation needs updating to the new signature.
- `pkg/dataflow`: the catch-all `agent` node type splits into three - `agent` (now pure identity: just an `agent` slug parameter, a no-op `Execute`), `query` (the actual task - `prompt`/`tools` parameters moved here, plus a new `Agent` field referencing the `agent` node providing its persona), and `tools` (a reusable, individually-toggleable group of capability nodes - its own `Tools` field lists members, a new `disabled` parameter temporarily excludes some without removing the wiring). `BuildAgentTools` is renamed `ResolveTools` and now expands a `tools`-type target recursively, so several `query` nodes can share one capability group. `Node.Tools` ownership widens from "agent only" to "query or tools"; `Validate` gains matching checks (a `query` node requires a valid `Agent` reference; `Tools` may no longer target a `query` node, may target a `tools` node). Breaking change to the old single-node shape - no `AgentCaller` interface change, since only *which* node type calls `Ask` moved, not the signature.

## [1.2.38] - 2026-09-09

### Fixed
- `internal/providers`: tunes the Z.ai request timeout behavior and provider tests so long-running model calls have a better chance to complete without weakening the shared provider client path.

### Added
- `pkg/types`: exposes model context-window metadata through the public types surface, with coverage for the new API contract.

## [1.2.37] - 2026-09-07

### Added
- `pkg/dataflow`: node execution can now retry individual node failures with per-node retry configuration.
- `pkg/dataflow`: nodes can opt into `OnError` continuation behavior so a graph can continue after selected failures instead of always aborting the whole run.

## [1.2.36] - 2026-09-07

### Added
- `pkg/dataflow`: promotes `$vars` expression binding for dataflow `Variables` onto the normal `dev` -> `main` release path.

### Notes
- `v1.2.35` was tagged on an intermediate equivalent commit that is not on the current `main` ancestry; `v1.2.36` is the mainline release carrying the same `$vars` feature.

## [1.2.35] - 2026-09-07

### Added
- `pkg/dataflow`: adds `$vars` expression binding so graph expressions can read workflow-level variables alongside item JSON.

### Notes
- This tag was cut before the final `dev` -> `main` promotion; consumers should prefer `v1.2.36` or newer.

## [1.2.34] - 2026-09-07

### Added
- `internal/vector`: adds an OpenSearch vector store implementation, integration coverage, and configuration plumbing.
- `internal/rag`: adds Docling-based chunking and chunk-cache support for richer RAG ingestion.
- `pkg/docling`, `pkg/rag`, `pkg/vector`, and `pkg/config`: expose the new RAG/vector configuration and Docling chunking surfaces to embedders.

### Changed
- RAG documentation and database-schema notes now describe the new chunking/vector-store behavior.

## [1.2.33] - 2026-09-05

### Added
- `pkg/dataflow`: adds pinned node data so tests and authoring tools can freeze a node output and replay downstream graph behavior deterministically.

## [1.2.32] - 2026-09-05

### Added
- `pkg/dataflow`: adds expression resolution for dataflow nodes, enabling node parameters to be computed from incoming item data during execution.

## [1.2.31] - 2026-09-05

### Added
- `pkg/dataflow`: tracks per-item provenance and per-port outputs so graph consumers can inspect where each item came from and which branch produced it.

## [1.2.30] - 2026-09-05

### Added
- `pkg/dataflow/nodes`: `webhook_trigger` gains `responseMode` support for immediate responses or delayed responses after workflow completion.

## [1.2.29] - 2026-09-05

### Added
- `pkg/dataflow/nodes`: adds a `webhook_trigger` node type for HTTP-triggered dataflow workflows.

## [1.2.28] - 2026-09-05

### Added
- `pkg/dataflow/nodes`: adds a `schedule_trigger` node type for time-based dataflow workflow starts.
- `pkg/dataflow`: `NodeDescription` gains `IsTrigger` so authoring UIs and runtimes can identify trigger nodes explicitly.

## [1.2.27] — 2026-09-03

### Added
- `pkg/dataflow`: `NodeDescription.Properties []NodeProperty` — a small, flat parameter schema (string/text/number/boolean/options/json field types, a single-condition `DisplayIf` for conditional visibility) so a consuming authoring UI can render real per-field forms instead of falling back to a raw-JSON textarea for every node type. Populated for `http_request`, `filter`, `if`, `switch` (`casesOrder`/`cases` stay JSON — a paired repeatable structure the flat model doesn't fit), `wait`, `agent`, and all six database node types. Left empty, deliberately, on `subworkflow` (a nested `workflow.Definition` isn't representable as flat fields) and `set` (its one real parameter is an arbitrary-keyed map, structurally like a fixed collection this system doesn't support). Additive and `omitempty` — any node type without `Properties` simply has none, the correct signal to fall back to raw JSON.
- `pkg/dataflow`: `NodePropertyType` gains `"secretRef"`, and the 5 `*SecretRef` parameters across the database node types (`dsnSecretRef`, `uriSecretRef`, `addrSecretRef`, `passwordSecretRef`, `baseURLSecretRef`) are now typed with it instead of plain `string` — gives a consuming UI a real signal to render a secret picker instead of pattern-matching on the field name. Metadata/JSON-tag only, no `Execute`/`ValidateParameters` logic touched.

## [1.2.26] — 2026-09-02

### Added
- `pkg/dataflow`: a new deterministic node-graph workflow engine — level-based topological execution, cycle detection, port-based data routing for conditional branching (`if`/`switch`). Complements `pkg/workflow` (a multi-agent DAG where every node costs an LLM call) rather than replacing it: two built-in node types, `agent` (a single scoped agent turn) and `subworkflow` (delegates to `pkg/workflow.Run`), let a deterministic graph invoke a multi-agent chain for the one step that genuinely needs judgment, without either engine reimplementing the other's execution model. `Runtime` carries per-run dependencies (secrets, agent/subworkflow callers) injected by the caller rather than owned by this package, so it never stores a credential or holds an `sdk.Client` directly.
- `pkg/dataflow/expr`: a `goja` runtime pool with a compiled-bytecode cache for `$json`-bound boolean/value expressions — same `"$"`-prefixed convention `internal/inbox/event_filter.go` already established in seshat-ai, so a graph's `filter`/`if`/`switch`/`set` nodes and an inbox event filter read the same way regardless of which evaluates them. Unlike a one-VM-per-call check (fine for a single message), a graph node may evaluate the same expression against many items in one run, so runtimes and compiled programs are reused across calls.
- `pkg/dataflow/nodes`: `http_request` (with an SSRF guard blocking private/link-local/cloud-metadata addresses), `filter`, `if`, `switch`, `set`, `merge`, `wait` — the node types a graph needs regardless of tenant/product, no credentials involved.
- `pkg/dataflow/nodes/database`: `postgres`, `mysql`, `sqlite` (shared `database/sql` executor), `redis`, `mongodb`, `elasticsearch` — credential-resolving nodes for a graph step that reads/writes a database directly. `sqlite` uses `modernc.org/sqlite` (pure Go, no cgo); production code deliberately doesn't blank-import it itself (see the package doc comment) since the Go sqlite ecosystem has multiple packages that each self-register under the driver name `"sqlite"`, and a consumer already using `glebarez/sqlite` for its own storage would panic at init time on a second registration.
- `internal/automation.Job` gains an optional `Graph *dataflow.Definition`, an alternative to the existing flat `Task` — every existing job (the primary consumer being the Scheduling-page style use case) keeps working unchanged, since `Task` is simply ignored when `Graph` is set. `RunnerConfig` gains `NodeRegistry`/`Secrets`; a job with a `Graph` fails clearly rather than silently no-oping when `NodeRegistry` isn't configured, since which node types are available (registering the database package pulls in Postgres/MySQL/MongoDB/Redis/Elasticsearch client code) is a deployment choice, not something this package defaults on its own. `jobWorkflow`'s new `agent`/`subworkflow` node adapters reuse the job's own session across every node call, unlike `sdk.Client.RunWorkflow`'s default executor (`pkg/sdk/workflow.go`), which starts a fresh session per node.

Design and several pieces (the level-based execution/cycle-detection approach, the HTTP node's SSRF blocklist) adapted from [neul-labs/m9m](https://github.com/neul-labs/m9m) (MIT license, see `pkg/dataflow/NOTICE`) — studied as a reference after deciding not to import it as a dependency (two young projects coupling was judged too risky for something this central). Redis/MongoDB/SQLite deliberately diverge from m9m's own choices where those don't fit a self-hosted deployment (m9m's Redis/MongoDB nodes only work through an HTTP REST proxy — Upstash-style for Redis, MongoDB Atlas Data API for Mongo, neither available for a plain self-hosted instance) or force a cgo dependency onto every consumer.

## [1.2.25] — 2026-08-31

### Fixed
- `pkg/automation/automation.go`: `TriggerTypeEvent` (added in 1.2.24) was never added to this package's public re-export const block, unlike the other three trigger types - an external consumer (e.g. seshat-ai) could reference `Trigger.IsEvent()`/`RunEvent` fine (methods pass through the `Trigger`/`JobScheduler` type aliases automatically), but had no way to actually construct a `Trigger{Type: automation.TriggerTypeEvent}` at all. Found immediately while wiring the first real consumer (seshat-ai's Inbox workflow feature) - `go vet` caught it as an undefined identifier on the very first build.

## [1.2.24] — 2026-08-31

### Added
- `internal/automation/job.go`: `TriggerTypeEvent` — a job trigger that fires on demand (`JobScheduler.RunEvent`) instead of on a schedule, for embedding applications that want to react to their own events (e.g. seshat-ai's Inbox reacting to a newly-arrived message) rather than polling. `Trigger` gains `EventType`/`EventFilter` (opaque strings this package stores/passes through — matching an event against `EventType` and evaluating `EventFilter` against its payload is entirely the caller's responsibility, keeping this package free of any event-specific dependency); `Trigger.IsEvent()` reports the new type. `AddJob`/`UpdateJob`/`ResumeJob`/`rehydrate` skip schedule computation for it, and it's naturally invisible to the time-based ticker (`tick`) since it never has a `NextRunAt`. New `JobScheduler.RunEvent(ctx, job, contextText)` mirrors `RunNow` but takes an already-loaded `Job` plus event context text to append to its task, for a caller that already has the job in hand from its own event-matching pass. First real consumer: seshat-ai's Inbox workflow-automation feature (Inbox Agent-authored "when a message arrives matching X, do Y" rules), the reason this session studied `neul-labs/m9m` in the first place.

## [1.2.23] — 2026-08-31

### Fixed
- `internal/automation/scheduler.go`: `CronSchedule.Next` required BOTH day-of-month and day-of-week to match whenever neither was `*` (AND logic) — e.g. `0 9 15 * 1` ("9am on the 15th, and every Monday") only fired when the 15th happened to land on a Monday. Standard POSIX cron semantics require EITHER to match in that case (OR logic), only requiring both when one of the two fields is `*`. The hand-rolled bitmask parser is replaced with `robfig/cron/v3` (MIT), which implements this correctly; `CronSchedule` is now a thin adapter over it so `Trigger.ToSchedule`/`JobScheduler` are unaffected. Idea to use a battle-tested cron library instead of a hand-rolled parser came from studying `neul-labs/m9m` (MIT), which does the same.
- `internal/engine/loop.go`: `waitRecoveryBackoff` (the retry delay for every recoverable provider error — rate limits, timeouts, transient API/network errors, across the whole SDK) grew linearly (`multiplier*attempt`: 250ms, 500ms, 750ms...) with no jitter, so many sessions/jobs hitting the same upstream outage would all retry in lockstep against an already-struggling provider. Now exponential (doubling each attempt, capped by a new `LoopConfig.RecoveryBackoffMaxDelay`, default 30s) plus up to 25% jitter — standard practice for exactly this "thundering herd" failure mode. `RecoveryBackoffMultiplier`'s existing meaning (base delay in ms, default 250) and `MaxRecoveryAttempts` are unaffected. Idea from studying `neul-labs/m9m` (MIT), which does the same for its own retry policy; seshat's own error *classification* (`recoveryLabel`/`ClassifyHTTPError`, structured/typed) was already more robust than m9m's substring-matching approach, so only the delay formula changed.

### Added
- `internal/automation/job.go`: `Job.MaxDuration` (per-execution wall-clock timeout, via `context.WithTimeout`) and `Job.MaxRuns` (total execution count cap, tracked via a new `Job.RunCount`) — a scheduled job with a hung agent turn previously ran forever, and there was no way to cap how many times a recurring job should fire. Idea and shape from studying `neul-labs/m9m` (MIT), which has the same two concepts on its schedule config; reimplemented natively here (small enough that no code was actually copied). `internal/db`: new SQLite migration (`20260831_017_automation_job_limits`) adds the backing columns.

## [1.2.22] — 2026-08-30

### Fixed
- `internal/tools/agents/agent_tool.go`: the `agent` tool's own `run_in_background: true` mode never notified callers of real completion — only `TaskGet`/`TaskList` polling could ever learn the outcome, and the engine's generic tool-execution pipeline emits its own "completed" `tool.progress` event for the dispatch itself (creating the background task, not finishing it) almost instantly. Since this tool is literally named `agent` — the same name the synchronous, blocking call path uses, whose "completed" event genuinely IS the real completion — clients had no way to tell a real completion apart from a mere dispatch acknowledgement for this specific mode. Fixed the same way `spawn_agent` (`spawn_agent.go`) already handles this: a background goroutine now blocks on `Manager.WaitForTask` and emits a second `tool.progress` event carrying `metadata.subagent_finished=true` once the task actually reaches a terminal state — the same marker `spawn_agent`'s own notifier already uses, so a consuming client can apply one shared rule for both. The dispatch's own immediate response text no longer claims "there is no completion notification for this."

### Added
- `pkg/sdk/shell_hooks.go`: `Client.ReloadPreToolHooks(cfgs []PreToolHookConfig) error` replaces a client's entire shell pre-tool hook set live. Previously `ClientConfig.PreToolHooks` was only ever read once, inside `NewClient` — a consumer resolving hook configuration from a remote source (an org-shared catalog, for example) had no way to pick up a change without restarting the whole process, unlike MCP servers (`ReloadMCPServers`). Removes any existing registration by its fixed `ToolHook.ID` before re-registering, so repeated calls replace the set atomically instead of accumulating duplicate, double-firing registrations.
- `internal/runtime/hooks/registry.go`: `Registry.Remove(id string) int` drops every hook with the given ID and reports how many were removed — the missing counterpart to the existing `Add`, needed for `ReloadPreToolHooks` above.
- `internal/execution/orchestrator.go`: `Orchestrator.RemoveHook(id string) int` exposes `Registry.Remove` at the orchestrator level, mirroring `AddHook`.
- `pkg/sdk/client_sessions.go`: `Client.RemoveToolHook(id string)` exposes the above at the public `Client` level, mirroring `AddToolHook`.

## [1.2.20] — 2026-08-28

A broad provider-reliability pass following up on the `previous_response_id`
work in 1.2.19, plus two critical fixes found by live-testing Codex against
a real ChatGPT-account session (documented in
`docs/audits/providers-incident-2026-08.md`).

### Fixed
- `internal/providers/client_codex.go`, `internal/engine/loop.go`: Codex was completely broken for real ChatGPT-account sessions since `previous_response_id` continuation shipped in 1.2.19 — confirmed via a live smoke test against a real account, `chatgpt.com/backend-api/codex` rejects `"store": true` outright (`{"detail":"Store must be set to false"}`), and `previous_response_id` can only reference a response the API actually stored, so the whole continuation hint could never be validly redeemed. `store` is back to `false`; `recordPreviousResponse` is now a permanent no-op for Codex rather than populating a hint that would only fail a later call. The surrounding plumbing (`APIRequest`/`MutableState` fields, `buildAPIRequest`'s read side) is left in place but inert, in case a store-compatible path for this backend is found later.
- `internal/providers/client.go`: model aliases (`ModelAliasMapping`, e.g. Codex's `"default"` → `"gpt-5.6-sol"`) were never actually applied to the outgoing request — confirmed via the same live smoke test, the real API received the literal string `"default"` and rejected it (`{"detail":"The 'default' model is not supported..."}`). `Config.ResolveModel` was only ever called inside `GetEndpoint`, which only matters for providers whose endpoint URL embeds the model (Gemini); every OpenAI-compatible provider and Codex put the model in the JSON body via `ModelIdentifier.ProviderModelName()`, which has no access to alias mapping. New `Client.resolveRequestModel` is now called at the top of every public entry point (`CreateMessage`, `CreateMessageStreamResultWithCallback`, `CreateMessageStream`) so every adapter — body and endpoint alike — sees the resolved model consistently. This means every alias for every provider, not just Codex's, was silently non-functional against the real API until now.
- `internal/providers/config.go`: `config.go`'s Codex `ModelAliasMapping` (`default`/`codex` → `gpt-5.3-codex`, `5.2` → `gpt-5.2-codex`) pointed at model IDs `registry.go`'s own catalog comment already documented as confirmed broken against a real ChatGPT-account session. Repointed at the confirmed-working catalog (`gpt-5.6-sol`, `gpt-5.4-mini`, plus a new `5.5` alias); the `5.2`/`5.3` shorthand aliases are removed rather than silently repointed. Live-tested all 6 known Codex model IDs afterward (including 3 the catalog comment had marked unconfirmed) — all work.
- `internal/engine/loop.go`: `isRecoverableError` treated every `*types.EngineError` with `Code: ErrCodeAPIResponse` as permanent, no matter what caused it — but all 5 provider stream parsers (Anthropic/Bedrock/Vertex/Foundry/WorkersAI, OpenAI-compatible, Gemini, Ollama, Codex) wrap *every* body-read/decode I/O error with that same generic code, including a genuinely transient mid-stream socket death. A dropped connection during streaming was therefore never retried, only ever treated as a terminal failure. `isRecoverableError` now falls through to the structured network classifier (`providerretry.ClassifyHTTPError`) on the wrapped cause when the code is `ErrCodeAPIResponse` and a cause is present, instead of trusting the generic code alone.
- `internal/providers/config.go`, `internal/providers/fetch.go`: MiniMax was non-functional end to end. Its `GetEndpoint` double-appended `/chat/completions` onto a `BaseURL` that was already a full endpoint path, guaranteeing a 404; fixing that surfaced a deeper problem verified directly against `platform.minimax.io`'s own docs — `api.minimax.chat` isn't a real MiniMax domain at all, and the endpoint path it pointed at (`/v1/text/chatcompletion_v2`) is documented deprecated. `BaseURL` is now `https://api.minimax.io/v1` (a real API root, like every other OpenAI-compatible provider here), routed through the standard `/chat/completions` path. This also fixes model-sync discovery (`fetch.go`'s `DefaultBaseURL`), since the corrected domain genuinely serves `/v1/models`.
- `internal/providers/registry.go`: MiniMax's registry description said "Anthropic-compatible"; it actually routes through `openAICompatAdapter` and sends OpenAI-shaped bodies. Cosmetic only, but misleading.
- `internal/providers/adapter.go`, `internal/providers/config.go`, `pkg/config/provider_catalog.go`: WorkersAI was completely non-functional — absent from `adapterForProvider`'s real dispatch cases (silently fell through to the Anthropic Messages wire format, wrong request shape, wrong `x-api-key` auth), and its default `BaseURL` (`https://workers.ai/v1/chat`) wasn't a real Cloudflare domain. Verified directly against Cloudflare's docs: Workers AI exposes a genuine OpenAI-compatible endpoint (`.../accounts/{account_id}/ai/v1/chat/completions`, Bearer auth), so it now routes through the same `openAICompatAdapter` shared by 7 other providers. The Cloudflare account id reuses the same `ProjectID`/`provider_project_id` slot Vertex uses for its GCP project id (an account/tenant identifier embedded in the request URL); the CLI setup wizard surfaces it with no changes needed in `cmd/cli`, since that wiring was already provider-generic. Model-sync discovery is still unavailable for WorkersAI — Cloudflare's OpenAI-compatible surface has no `/v1/models` at all.
- `internal/providers/registry.go`, `internal/providers/config.go`: DeepSeek's registered models (`deepseek-chat`, `deepseek-reasoner`, `deepseek-coder-v2`) are legacy V3.1-era IDs; DeepSeek's own pricing docs (checked directly, not a secondary source) list only the current V4 lineup (`deepseek-v4-flash`, `deepseek-v4-pro`, `deepseek-v4-flash-vision-exp`), consistent with reports the old IDs were retired 2026-07-24. This wasn't just a wrong context-window number as originally suspected — the whole catalog was likely targeting model IDs that no longer resolve. Registry and alias mapping repointed at the current models.
- `internal/providers/client.go`: Foundry's structured `SystemPromptBlocks` (carrying `cache_control`) were only preserved for `provider == APIProviderAnthropic`; every other provider sharing this request builder — including Foundry, which speaks the identical Anthropic Messages format and already gets tool-definition caching — fell back to a flattening path that drops `CacheControl` entirely, silently degrading Foundry's system-prompt cache to uncached.
- `internal/types/message.go`, `internal/providers/client.go`: Anthropic prompt caching only ever covered the stable half of the system prompt and the last tool definition — `TextContent` and `ToolResultContent` had no `CacheControl` field at all, so the growing conversation/tool-result history in a long agentic loop was resent uncached on every single call. Both types now carry `CacheControl`, and a new `anthropicMessagesWithCacheControl` marks the last cacheable block of the last message before serialization, mirroring the existing tool-caching pattern.
- `internal/providers/client.go`: `ResponseHeaderTimeout` (60s) was shared by every provider, with no override for Ollama — a local request can legitimately block behind loading a large model into memory longer than that. Ollama now gets a 5-minute timeout instead of the shared 60s default.
- `internal/providers/registry.go`: three registry corrections, each verified against official docs before changing anything — Kimi's `SupportsPC` was `false` despite Moonshot's fully automatic server-side caching (no client action required, so the flag was wrong regardless of what this codebase's own request building does); OpenRouter's provider-level `SupportsPC: true` was never backed by any of its 3 listed models, and this codebase routes it through the OpenAI-compatible adapter with no `cache_control` field at all, so it's now `false` to match what's actually implemented; Mistral Small's context window (`32768`) matched the superseded Mistral Small 3, corrected to `131072` (128K) for the current Mistral Small 3.1.

### Changed
- `docling_convert`'s tool description now explicitly tells the agent to convert a whole PPTX/document in a single call rather than improvising a per-slide/per-image workaround (terminal commands, ad-hoc scripts, individually OCR'ing extracted slide images) — observed in a real session, this produced many small tool calls instead of the one call docling-serve is designed to handle in a single pass, with worse layout/reading-order/table-structure results than letting docling process the whole document at once.
- `internal/engine/session.go`: async session-title generation (`generateTitleAsync`, wired through `Engine.SetOnSessionTitled`) now fires as soon as a new session's first message is persisted, instead of after that first turn finishes. It only ever reads the first user message text — never the assistant's response — so gating it on turn completion had no functional reason and just meant a session sat as "Untitled" through the entirety of its first (often the longest) reply before a client polling for the title would ever see one. `firstUserMessageText`, only used by the old post-turn call site, is removed; the trigger now passes the already-in-hand submitted text directly.
- `internal/officetext`: `Extract`'s `.pptx` path only reported whether *any* text was found (`ok`), not whether it was plausible for the deck's size — a slide deck that's mostly screenshots/diagrams with only a title or two of real text would "succeed" and short-circuit before ever reaching docling-serve for OCR, silently dropping almost all of the deck's actual content. `Extract` now also returns `sparse` (true when extracted text is below `MinCharsPerSlide` per slide, mirroring `pdftext.Result.Sparse`'s existing per-page heuristic for scanned PDFs), and `read_file`'s docling fallback path treats a sparse PPTX the same as an empty one: automatic escalation to docling, no agent judgment call required. `Extract`'s signature changed (`(markdown, ok, err)` → `(markdown, ok, sparse, err)`) — a breaking change for the handful of internal and external (seshat-ai) callers, all updated in this release.

### Removed
- `internal/providers/config.go`: `Config.BuildAuthHeaders()` had zero references anywhere, including tests — a stale, unmaintained duplicate of the auth logic that actually runs (`adapter.go`'s `applyAuthHeaders`).

## [1.2.3] — 2026-08-01

### Fixed
- `internal/permissions/engine.go`: `get_file_metadata` (a read-only `stat()` call, `RequiresPermission: false` in its own definition) was missing from `isAlwaysSafeTool`, so in Auto permission mode every call went through the two-stage LLM classifier instead of being heuristically allowed — extra API calls per check, and, observed live, an incorrect "blocking for safety" deny when the classifier's own response failed to parse, compounding with provider rate limits.
- Same file: audited every registered tool for the same gap and added 19 more with an identical, individually-verified safety profile (`IsReadOnly: true`, `RequiresPermission: false`, `IsDestructive: false`, and a trivial passthrough `CheckPermissions` with no conditional logic) — `notebook_read`; the read/search/fetch tools in `devto`, `hackernews`, `reddit`, `twitter` (their write/publish siblings are correctly left requiring classification); `get_config`, `get_goal`, `rag_search`, `repo_map`, `workflow_draft`; `seshat_list_skills`, `seshat_read_skill`, `seshat_validate_skill`; `list_agents`, `wait_agent`.

### Notes
- Deliberately not touched in this pass, pending a follow-up decision: the browser read tools (`browser_snapshot`/`browser_screenshot`/etc. — mechanically read-only, but a live browser session can be authenticated into real accounts, a different risk profile than a filesystem read) and `job_output`/`task_get`/`task_list`/`task_output`/`monitor` (heterogeneous `CheckPermissions` implementations — `task_get` has real conditional deny logic that `isAlwaysSafeTool`'s short-circuit would bypass entirely, unlike the trivial-passthrough tools added above).

## [1.2.2] — 2026-08-01

### Fixed
- `internal/agent/goal`: the durable goal store wrote through to its SQLite backend on every plain `Get()` (not just on mutation), and held its mutex during backend I/O in `Update`/`RecordTokenUsage` — under an active goal the runner reads goal state up to three times per agent turn, so this added needless write amplification and let one session's backend latency stall unrelated sessions through the shared lock. Reads are now lock-scoped snapshots that never write, and mutation methods release the lock before calling the backend. Returned `Goal` values are now always defensive clones, so callers can no longer mutate store-internal state without going through the mutex.
- `internal/seshattui/ui/dialog/doctor.go`, `internal/seshattui/ui/model/ui.go`: opening or refreshing the TUI's Doctor dialog ran `DoctorReport()` (SQLite ping, `git`/`uv` subprocess checks) synchronously inside Bubble Tea's event loop, freezing the whole UI for as long as those checks took. Both paths now dispatch through a `tea.Cmd`.
- `cmd/cli/background.go`: `seshat run --bg --name X` had a TOCTOU race — the name-availability check only scanned already-saved session files, but a new session isn't saved until after its process starts, so two concurrent launches with the same name could both pass the check and leave two live sessions sharing one name. Name reservation is now atomic (`O_EXCL`) and happens before the process starts.
- `pkg/companion`: `companion.Save` wrote profile data with a plain truncate-and-write, so a crash mid-write could corrupt `companion.json` and turn the next load into a hard error instead of falling back to defaults. It now writes to a temp file and renames it into place, matching the pattern already used for background session metadata.

### Notes
- The `v1.2.0` GitHub release was cut prematurely from a feature branch before it merged to `main` and has been marked as a superseded pre-release; `v1.2.1` was the first correct build of the feature set below.

## [1.2.1] — 2026-08-01

### Added
- Goals created through `create_goal` are now persisted in the SQLite session database when `SessionSQLitePath` is configured, and successful session turns record token usage against the active goal so `get_goal`/`update_goal` can survive client/session restart.
- TUI chat now replaces hidden `task_create`/`task_update` plan tool calls with a compact "Plan" task checklist block, keeping plan progress visible without exposing internal task tools.
- `seshat doctor` command backed by reusable `pkg/doctor` diagnostics for local config, runtime paths, SQLite session storage, provider credentials, and helper tools.
- TUI settings hub now exposes the same doctor diagnostics in a scrollable "Doctor" dialog with refresh support.
- `pkg/repomap`, `seshat repomap`, and the read-only `repo_map` tool provide Go-first repository structure summaries, with optional system-context injection via `SESHAT_REPO_MAP=1`.
- Local background session commands: `seshat run --bg [--name NAME] "PROMPT"` starts a detached child process, while `seshat ps`, `seshat logs <id-or-name> [-f]`, `seshat attach <id-or-name>`, and `seshat kill <id-or-name>` manage tracked background runs.
- `pkg/workflow`, `Client.RunWorkflow`, and `seshat workflow run <file>` provide a minimal static YAML/JSON DAG runner: nodes declare `needs`, independent nodes run in parallel, and dependency outputs are injected into downstream prompts.
- `workflow_draft` and `sdk.DraftWorkflow` let agents and headless hosts validate and render reusable static workflow DAG definitions before saving or running them.
- Workflow nodes now support `kind: agent|verifier|critic|router`; role-specific prompt scaffolding helps agents build review, critique, and routing stages while keeping the runner's execution model static.
- `pkg/companion`, `ClientConfig.Companion`, and `seshat companion` add a headless-first companion profile that can inject a lightweight collaboration presence into sessions without replacing the core Seshat system prompt.
- Kimi/Moonshot is now consistently available from CLI and TUI provider setup, using the international `https://api.moonshot.ai/v1` endpoint and OpenAI-compatible Bearer auth.

## [1.1.0] — 2026-08-01

### Added
- `excel_edit` tool: create/edit `.xlsx` cells and formulas natively (excelize), with a read-before-write gate matching `write_file`/`edit_file`.
- `docx_edit` tool: create Word documents from plain/markdown-ish text (`#`.."######" → real Word heading styles) or find/replace edit existing ones, with fuzzy-match diagnostics on a near-miss.
- `write_pdf` tool: create/append/delete-pages PDFs via `go-pdf/fpdf` + `pdfcpu` — no headless-browser dependency.
- `search_start` tool: cancellable, streaming background content search (reuses `job_output`/`job_kill`) that also searches inside `.docx`/`.pptx`/`.xlsx` content, which ripgrep can't see.
- `get_config` tool (read-only): exposes the effective security policy — denied command fragments/patterns, commands requiring approval, read/write-denied path prefixes, file-read limits, sandbox availability, default shell.
- `internal/tools/files/docling` package: `read_document_url` (moved from `files/read_url`) plus new `docling_convert` for explicit local-file conversion (OCR, complex slide decks, audio transcription) — FileRead's own automatic docling fallback is unchanged.
- `rag_delete` tool: delete an entire corpus or a single file's chunks — previously unreachable capability (`Service.DeleteNamespace`/`DeleteFileChunks` existed but no tool exposed them).
- `rag_ingest` gains an optional `file_id` param (defaults to `filename`) for idempotent re-ingest — re-ingesting the same file now replaces its chunks instead of accumulating duplicates.
- **Vectorless RAG**: `rag_ingest`/`rag_search` now work without any embedding provider configured, via pure BM25/keyword ranking (real BM25 through SQLite FTS5; keyword-overlap scoring on HNSW/Memory). Previously RAG was hard-disabled with no embedder at all.
- RAG now falls back to the SQLite vector backend when the embedded HNSW backend is unavailable — notably on Windows, where `github.com/coder/hnsw`'s atomic-write dependency doesn't build, which previously disabled RAG entirely regardless of configuration.
- Reranking now actually runs: `SetReranker` was implemented but never wired up; RAG search now reranks results when a reranker is configured.
- Reranker generalized beyond LangSearch's hosted API into `HTTPReranker`, additionally speaking HuggingFace TEI's shape — point `RAG_RERANK_URL` at a self-hosted TEI/vLLM instance (e.g. `BAAI/bge-reranker-v2-m3`) for a free alternative requiring no API key.
- `SemanticChunker` (embedding-similarity sentence grouping) is now actually used by the CLI's RAG wiring when an embedder is configured, instead of always falling back to the naive `ParagraphChunker`.
- No-API-key DuckDuckGo fallback provider for `web_search`.
- `pkg/rag/embedder`: `FromEnv()`/`NewFromEnv()` (parity with `pkg/rag/reranker`).
- `pkg/web/search/providers`: `NewDuckDuckGoProvider()` and `ProviderModeDuckDuckGo` (parity with the other providers in the same file).
- `pkg/rag`: `ParagraphChunker`, `SemanticChunker`, `DefaultChunker()`, `NewSemanticChunker()`, `ArtifactKey()` — `NewService` requires a `Chunker` argument, but no implementation was previously reachable from the public API.
- `pkg/image` + `pkg/image/providers` (OpenAI DALL-E, Google Imagen) and `pkg/audio/stt` + `pkg/audio/tts` + `pkg/audio/providers` (OpenAI Whisper/TTS): standalone client libraries, mirroring `pkg/docling` — a host application can now call image/speech generation directly (e.g. a UI where a human types a prompt) without going through the agent's LLM tool-calling loop for a deterministic action. The same provider implementations power both the direct call and the `generate_image`/`text_to_speech`/`speech_to_text` agent tools. `WithSTTBaseURL`/`WithOpenAIBaseURL`/`WithTTSBaseURL` all support pointing at a self-hosted, OpenAI-API-compatible server (e.g. a local whisper.cpp instance) with no API key.
- `pkg/fim` + `pkg/fim/providers` (Mistral Codestral, DeepSeek): same standalone-client treatment for Fill-in-the-Middle code completion, previously reachable only through the `code_complete` agent tool. FIM is explicitly suited to IDE integrations/in-editor ghost text, where a tool-calling LLM turn per keystroke makes no sense.
- Session auto-title generation feature: AI auto-titles sessions after the first successful turn using the user message.
- `OnSessionTitled` callback and `DisableTitleGeneration` configuration options in `ClientConfig`.
- `CredentialResolver` interface in `pkg/sdk` — allows per-request API key injection without touching `ClientConfig.APIKey`
- `generate_image` tool backed by `image.Generation` interface (OpenAI DALL-E 3 and Google Gemini Imagen providers)
- `text_to_speech` and `speech_to_text` tools backed by pluggable audio providers (OpenAI TTS-1, Whisper)
- OpenTelemetry tracing (`internal/monitoring/tracer.go`) — OTLP gRPC export, no-op when `OTEL_EXPORTER_OTLP_ENDPOINT` is unset
- `pkg/monitoring` public package exposing `InitTracer` and `Tracer`
- `.github/workflows/ci.yml` — build, test (race), lint on push/PR
- `.github/workflows/release.yml` — cross-platform binary release on `v*` tags
- `.githooks/pre-commit` — gofmt + go vet + golangci-lint (install with `make hooks`)
- `SetDefault` and `GetDefaultByUserID` on `ProviderSettingStore`
- Community files: `SECURITY.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `AGENTS.md`
- Architecture diagrams in `docs/vision/diagrams.md` (Mermaid)
- Project vision and 3-level roadmap in `docs/vision/`

### Changed
- Makefile: replaced `build-api` (removed) with `build-grpc`; added `fmt`, `vet`, `hooks` targets
- `docs/architecture.md`: removed `cmd/api` from entry points (it lives in nexus-product)
- `docs/transports.md`: translated to English, removed HTTP API section (nexus-product), fixed absolute paths

### Fixed
- `internal/tools/files/patch/patch.go`: replaced `if HasSuffix` with `strings.TrimSuffix` (golangci-lint S1017)
- `internal/providers/auth.go`: removed OAuth credential values from log output (security: S1)
- `pkg/rag/embedder`: `EmbedTexts` now splits large requests into batches (default 64 texts), since some hosted embedding APIs (e.g. Mistral) reject an oversized request outright instead of truncating it
- `readDoclingFile` (FileRead's docling path) now records read-state on all three success paths, fixing a false "file has not been read yet" block on binary formats read via docling.
- A re-ingested RAG file that shrank (fewer chunks than its previous version) no longer leaves the old version's stale trailing chunks behind — `Service.Ingest` now cleans them up via the existing (but previously unused) `DeleteFileChunks`.
- `ParagraphChunker.Split` now slices by rune instead of byte index — a byte-index cut could land inside a multi-byte UTF-8 character (e.g. accented French text) and corrupt it at the chunk boundary.
- Fixed a "typed nil" bug found while wiring up vectorless RAG: passing a nil `*embedder.Embedder` into an `Embedder` interface parameter produced a non-nil interface wrapping a nil pointer, so `embedder != nil` checks were incorrectly true and the first call panicked on a nil receiver.

### Removed
- `cmd/api` — moved to `nexus-product` (the open-source engine does not own the HTTP product layer)
- `docs/backend-boundary.md` and `docs/internal-boundary-audit.md` — monorepo-era documents, no longer relevant
- `docs/archive/` — speculative archived docs removed
- `internal/tools/special/docx` (docxgo-based): registered until 2026-06-03, deregistered but left orphaned since 2026-06-18. Its `content` param was inserted verbatim with zero markdown parsing — the new `docx_edit` converts `#`.."######" to real Word heading styles instead. Also drops the now-unused `mmonterroca/docxgo/v2` dependency.
- `internal/tools/special/brief`: a "send message to the user" tool from the pre-rename codebase, never registered at any point in its history.
- `internal/tools/special/config/configTool.go`: an arbitrary key/value settings store predating the current `contract.Tool` interface (incompatible signatures — could never have been registered as-is), replaced by the real `get_config` tool.

[Unreleased]: https://github.com/KPO-Tech/seshat/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/KPO-Tech/seshat/compare/v1.0.4...v1.1.0
