# TUI Tool Rendering Audit — June 2026

## Purpose

This note is the canonical TUI rendering roadmap for tools.

It exists to answer four questions consistently:

1. Which tools should appear in the chat transcript?
2. If they appear, should they render full output, compact output, or metadata only?
3. Which tools should update another UI surface instead of creating a normal tool bubble?
4. Which legacy TUI tool names are no longer part of the real runtime surface?

The canonical runtime sources are:

- `internal/tools/builtin/builtin.go`
- `pkg/sdk/client.go`

`internal/nexustui/agent/tools/tool_names.go` is not canonical anymore. It is a partial, legacy TUI vocabulary and must not be used as the source of truth for future renderer work.

## Legend

- `Done`: implemented and already validated in the TUI
- `Planned`: desired target state
- `Open`: direction identified, but product detail still needs confirmation
- `Chat policy` values:
  - `hidden`: no normal tool bubble in transcript
  - `meta`: show only a small metadata summary, not the raw payload
  - `compact`: show a visible but condensed transcript item
  - `full`: show a richer transcript item with existing truncation / expand behavior
  - `generic`: fallback renderer is acceptable for now
- `Surface` values:
  - `chat`
  - `header`
  - `sidebar`
  - `modal`
  - `artifact`
  - `workspace chrome`

## Confirmed Decisions Already Done

- [x] Tool result bubbles are slightly narrower than assistant and user messages.
- [x] `read_file` is now `meta` in chat instead of dumping file contents inline.
- [x] Normal truncation behavior stays enabled for write/edit style outputs; the transcript is not globally forced into wrapping mode.
- [x] The tool-ordering streaming fix is compatible with this roadmap: tool items stay inline, but post-tool assistant synthesis now lands in the correct place.
- [x] `enter_plan_mode` and `exit_plan_mode` are now hidden from the chat transcript, and the header reflects the real execution mode (`execute` / `plan` / `pair`).
- [x] `request_permissions` is now hidden from the chat transcript; the permission modal is the sole UX surface.
- [x] `enter_worktree` and `exit_worktree` are now hidden from the transcript. Header displays `⎇ <path>` when a worktree is active. `exit_worktree` now always prompts (`Ask`) instead of silently denying via `CheckPermissions`.
- [x] `rm -rf *` default permission rule changed from `Deny` to `Ask` — destructive shell commands now always surface the permission dialog instead of being silently blocked.
- [x] Tool name color changed from Malibu blue (`#00A4FF`) to Tang orange (`#FF985A`) to match the logo palette.
- [x] Tool body content indentation increased from 2 to 4 spaces to visually anchor body content under the tool name.
- [x] All tool body content now uses muted grey (`ContentText` / `ContentLine`) so tool output sits below agent conclusion text in the visual hierarchy. `glob`, `grep`, and `list_directory` compact renderers were aligned to this rule (previously `glob` and `grep` used bright `fgBase` for filenames).
- [x] `glob`, `grep`, and `list_directory` now have dedicated compact renderers: pattern/path + counts in the header, file/match lists in the body with `+N more` truncation and expand/collapse support.
- [x] `read_file` success body is now fully silent — removed "Content hidden in transcript" line. Only errors surface a body. Skill-backed read and image reads are exceptions and keep their body.
- [x] `remove_file`, `create_directory`, `get_file_metadata` now have dedicated quiet renderers: header-only on success; error body on failure. They were previously falling through to `GenericToolMessageItem` which echoed the raw result text (`"Removed: /path"`, `"Directory created: /path"`, JSON blob).
- [x] Permission panel `PromptFn` now builds typed params structs (`WritePermissionsParams`, `EditPermissionsParams`, `BashPermissionsParams`, etc.) from the raw `tool_input` metadata. Previously `Params` was always `nil`, causing every dialog type-assertion to fail and the content area to be empty.
- [x] `write_file` permission dialog now shows a real diff: old content is read from disk before the write, new content is the incoming `content` param. `edit_file` permission dialog shows a diff of `old_string` → `new_string`.
- [x] `edit_file` chat diff now renders correctly. The edit tool stores the original file under `"original_file"` (not `"old_content"`); `extractEditDiffContent()` reads the correct key and computes `newContent` via string replacement, producing the full red/green diff.
- [x] Permission panel `renderBashContent` now syntax-highlights the bash command via `SyntaxHighlight("command.sh")` before wrapping it in the content panel.
- [x] Notebook permission panels now have dedicated typed previews. `notebook_write` and `notebook_create` render notebook cells visually in the modal, using the same wide layout as file write/edit diffs when cells are present. `notebook_edit` renders markdown/code previews for non-delete edits and shows a full notebook diff for cell deletions in both the permission panel and the chat transcript.

## Cross-Cutting Product Rules

- Read-heavy tools should not dump large payloads into the main transcript by default.
- Write and mutation tools are important execution evidence and should usually stay visible in chat.
- System-state tools should update dedicated UI surfaces instead of looking like normal user-facing tool calls.
- Generic rendering is acceptable as a temporary fallback, but only when the tool is low-frequency or low-value in the transcript.
- The next renderer passes should follow this order:
  1. system surfaces that should hide tool bubbles entirely
  2. task tools
  3. multi-agent tools
  4. remaining read/list/search compaction
  5. long-tail generic cleanup

## Category A — Hidden From Chat, Represented Elsewhere

These tools should not produce normal transcript tool items. Their effect should be visible through dedicated UI state.

| Tool(s) | Surface | State | Notes |
|---|---|---|---|
| `enter_plan_mode`, `exit_plan_mode` | `header` | Done | Hidden from the transcript. The header mode badge now follows the real execution mode (`execute` / `plan` / `pair`). |
| `submit_plan` | `artifact`, `header` | Planned | Prefer plan artifact / review state over a normal tool bubble. |
| `task_create`, `task_update` | `compact task panel` | Done | Hidden from transcript. Session-scoped task state is persisted and rendered in the compact `Tasks` panel. |
| `task_list`, `task_get`, `task_stop` | `chat`, `compact task panel` | Done | Visible in chat as compact inspection/control tools. The compact `Tasks` panel is the current source of truth for task progress in the main chat layout. |
| `enter_worktree`, `exit_worktree` | `header`, `workspace chrome` | Done | Hidden from transcript. Header displays `⎇ <worktree-path>` when a worktree is active, reading live from the worktree session registry. Path clears automatically on `exit_worktree`. |
| `request_permissions` | `modal` | Done | Hidden from transcript. The permission modal/panel is the sole UX surface for this tool. |

### `submit_plan` long-term roadmap

The `submit_plan` flow is intentionally more complex than the other hidden tools and should be treated as a dedicated review surface, not a normal chat item.

Long-term product objective:

- `submit_plan` becomes a full plan-review workflow, separate from the normal transcript
- the user can read the submitted markdown cleanly, review it, annotate it, request revisions, and approve it without chat noise
- the system preserves plan history across revisions
- approval remains distinct from execution: after approval the agent still exits plan mode explicitly and then creates `task_*` items

#### V1 — first implementation target

Scope locked for implementation:

- no normal transcript bubble for `submit_plan`
- opens a dedicated review surface, close to permission review but document-oriented
- primary content is the submitted markdown plan, rendered read-only with visible line numbers
- supports line-attached comments on a single line
- supports an additional global review comment
- review actions are `Approve` and `Request changes`
- `Request changes` returns structured review data, not just a free-form chat message
- version history is preserved, with the latest version as the main focus
- approval does not execute directly; after approval the agent remains responsible for calling `exit_plan_mode` and then creating `task_*` items

Explicit non-goals for V1:

- multi-line range comments
- full visual diff review between plan versions
- direct plan execution from the review surface

#### V2 — richer review workflow

Desired additions after V1 is stable:

- better version browsing between plan submissions
- side-by-side or inline revision awareness between versions
- comment management improvements such as resolved / unresolved state
- clearer review timeline showing submit, feedback, resubmit, approval
- stronger linking between comments and the next submitted revision

#### V3 — full document-review surface

Longer-term target if the plan workflow proves central:

- multi-line range comments and richer document anchoring
- more advanced review interactions closer to code-review / doc-review tools
- stronger artifact-centric navigation between plan, comments, approval state, and resulting tasks
- potential tighter integration with sidebar/system surfaces so plan review, approval, and task kickoff feel like one continuous workflow

Implementation note:

- this should reuse the permission-style interaction model where useful, but the document viewer, line targeting, review payload, and version handling are plan-specific
- we should implement V1 first and treat V2/V3 as roadmap, not as hidden scope inside the first pass

### General sidebar direction

The dedicated right-side sidebar is no longer part of the current `task_*` completion criteria.

Updated product decision:

- `task_*` is considered complete for the current phase without a true right-side sidebar in `uiChat`
- the compact `Tasks` panel in the lower chat area is the current execution-progress surface
- a future right-side sidebar should be treated as a broader application shell feature, not as a task-only blocker
- when that general sidebar exists, tasks can become one section inside it rather than forcing a task-specific sidebar architecture first

Reasoning:

- the compact `Tasks` panel already provides continuous, low-noise progress visibility
- `task_list`, `task_get`, and `task_stop` now provide consistent inspection/control in chat
- this avoids coupling task completion to a larger layout redesign that belongs to the whole app shell

Possible future sidebar sections, when that broader app sidebar is designed:

- workspace path / current worktree
- session title and high-level runtime state
- execution mode / plan state
- tasks
- maybe later artifacts, MCP state, or other system surfaces

### `task_*` current status

The `task_*` work is now considered complete for the current phase.

Delivered behavior:

- `task_create` and `task_update` are hidden from the normal transcript
- task state is session-scoped and persisted in SQLite
- reload/resume restores task state cleanly
- the compact `Tasks` panel replaces the old todo-centric progress surface in the main chat layout
- `task_list`, `task_get`, and `task_stop` remain visible in chat with dedicated compact renderers
- `task_list` now defaults to session tasks when the session already has tracked tasks
- `task_get` resolves session tasks first, with background-task fallback
- `task_stop` stops tracking a session task first, with background-task fallback
- task refreshes preserve chat auto-scroll during live generation
- the compact task panel auto-collapses only when it was auto-opened by the system and all tasks are done

Current product stance:

- keep the compact `Tasks` panel as the active progress surface
- do not require a true right-side sidebar to call `task_*` complete for this phase
- treat any future sidebar work as a separate general-application sidebar project

What is intentionally deferred out of the current task scope:

- a true right-side sidebar in the main `uiChat` layout
- richer task-detail navigation inside that future sidebar
- final placement of tasks inside the future general sidebar shell
- more advanced dependency visualization such as `blocked by` / `blocks` inside a dedicated detailed panel

Implementation note:

- the old roadmap assumed the sidebar task surface was part of the core `task_*` delivery
- that is no longer the active goal
- the compact `Tasks` panel plus the corrected `task_*` tool semantics are now the accepted completion point for this phase

## Category B — Metadata Only In Chat

These tools should remain visible in the transcript, but only as summaries. The raw payload is too noisy for normal chat flow.

| Tool(s) | Surface | State | Notes |
|---|---|---|---|
| `read_file` | `chat` | Done | Header-only on success (file path, optional line range in header params). No body text — not even "Content hidden". Image and skill-backed reads still show their body. |
| `read_document_url` | `chat` | Planned | Same philosophy as `read_file`: source URL + conversion/read summary, not full extracted body. |
| `get_file_metadata` | `chat` | Done | Header-only on success (path in header param). JSON blob suppressed. Errors still surface a body. |
| `browser_snapshot`, `browser_extract` | `chat` | Planned | Summarize what was captured and from which page; hide large extraction bodies by default. |
| `browser_network_list`, `browser_list_downloads`, `browser_list_pages`, `browser_search_content` | `chat` | Planned | Show counts, active page, matched items, and key identifiers only. |
| `list_agents` | `chat` | Planned | If it stays in chat at all, render agent count and brief status summaries instead of raw payload. |
| `get_goal` | `chat` | Planned | Goal state is useful, but raw structure should collapse into a concise status card. |
| `nexus_read_skill` | `chat` | Planned | Skill content can be large; show which skill was read and a concise summary instead of the full body. |
| `memory_search_nodes`, `memory_open_nodes` | `chat` | Planned | Show match counts / node IDs / short titles, not full memory payloads. |

## Category C — Compact Chat Items

These tools should stay visible in chat, but the transcript item should be intentionally condensed.

| Tool(s) | Surface | State | Notes |
|---|---|---|---|
| `list_directory` | `chat` | Done | Compact renderer: path + item count in header; dir/file list with right-aligned sizes, `+N more` truncation, expand/collapse. Entries use muted grey (`ContentText`) to match bash output style. |
| `glob` | `chat` | Done | Compact renderer: pattern + file count in header; file path list (max 8, then `+N more`), expand/collapse. File paths use muted grey (`ContentText`). |
| `grep` | `chat` | Done | Compact renderer: pattern (+ include filter) + file count + match count in header; `file:match` lines (max 6, then `+N more`), expand/collapse. File part uses `ContentText`, match body uses `ResultItemDesc`. |
| `web_search` | `chat` | Done | Header: query + `N results` count (from `additional.result_count` metadata). Body: structured hit list — number (dim) + title (`ResultItemName`) + URL (`WebSearchURL` info-blue) + description (`ResultItemDesc` dim). Max 5 hits visible; `+N more` truncation, expand/collapse. |
| `web_fetch` | `chat` | Done | Header: URL. Body: prompt shown as a subtle `↳ …` context line (`WebFetchPrompt`), then fetched content as rendered markdown with normal truncation/expand. `WebFetchParams` now includes `Prompt` and `RenderMode` fields. |
| `browser_open`, `browser_navigate`, `browser_select_page`, `browser_close_page` | `chat` | Planned | Navigation state changes are useful, but should be one-line or near one-line items. |
| `browser_click`, `browser_type`, `browser_press`, `browser_scroll`, `browser_wait` | `chat` | Planned | Compact action log, not full dumps. |
| `close_agent`, `wait_agent`, `send_agent_message` | `chat` | Planned | Compact event-style rendering unless a richer multi-agent panel supersedes it. |
| `create_goal`, `update_goal` | `chat` | Planned | Good candidates for compact state transition cards. |
| `nexus_list_skills`, `nexus_validate_skill`, `tool_search` | `chat` | Planned | Compact count/result rendering is enough. |
| `mcp` | `chat` | Planned | Keep visible but summarized by operation, target server, and success/failure. |
| `rag_search`, `rag_ingest` | `chat` | Planned | Compact counts and identifiers, not large result bodies. |
| `memory_create_entities`, `memory_add_observations` | `chat` | Planned | Summarize created / updated node counts and names. |

## Category D — Rich Chat Rendering With Normal Truncation

These tools are meaningful execution steps and should stay visibly represented as richer transcript items.

| Tool(s) | Surface | State | Notes |
|---|---|---|---|
| `bash` | `chat` | Done | Rich renderer. Header shows first line of command + `(+N lines)` for multi-line scripts. Output body uses `toolOutputCodeContent` (line numbers, JSON auto-detection via `bashOutputLang`). Empty output shows a styled `(no output)` indicator instead of a bare header. |
| `write_stdin` | `chat` | Planned | Treat as a compact or rich shell continuation event under the shell family. |
| `job_output` | `chat` | Done | Shell output renderer via `renderJobTool`: line numbers + JSON auto-detection (same `bashOutputLang` path as bash). `(no output)` indicator when the job produced nothing. |
| `job_kill` | `chat` | Done | Quiet pattern: header-only on success (✓ icon communicates the result); error body on failure. No longer routes through `renderJobTool`. |
| `write_file`, `edit_file`, `apply_patch`, `notebook_edit`, `notebook_create`, `notebook_write` | `chat` | Done | `write_file`: renders new content (markdown-interpreted for `.md`, syntax-highlighted otherwise). `edit_file`: full red/green diff via `extractEditDiffContent`. `apply_patch`: file list with semantic color per operation (+ green / ~ grey / - red / → teal). `notebook_edit`: code/markdown preview for non-delete edits and full notebook diff for deletes. `notebook_create`: header-only success, error body when creation fails. `notebook_write`: action + cell count in header with first-cell preview in the body. |
| `create_directory`, `remove_file` | `chat` | Done | Header-only on success (path in header param); error body on failure. Raw result text (`"Directory created: …"`, `"Removed: …"`) is suppressed. |
| `docx` | `chat` | Planned | File-generation style tool; visible result is useful. |
| `agent`, `spawn_agent`, `resume_agent` | `chat` | Planned | Multi-agent orchestration deserves dedicated rendering, not generic JSON blobs. |
| `generate_image`, `tts`, `stt`, `code_complete` | `chat` | Planned | Result is user-visible and worth a richer item, even if renderer stays simple at first. |
| `lsp` | `chat` | Planned | The generic LSP payload is too vague today; needs a proper status/result renderer. |
| `monitor` | `chat` | Planned | Should read as a tracked background-execution item, close to shell/task UX. |
| `ask_user_question` | `chat` | Done | Inline interactive bubble with ↑↓ navigation, Space multi-select, Enter confirm; history shows past Q→As; "Other" routes to editor for free-text; `askUserBroker` in workspace wires the full flow. |

## Category E — Generic For Now, Revisit Later

These tools can remain generic temporarily without blocking the UX cleanup.

| Tool(s) | Surface | State | Notes |
|---|---|---|---|
| `browser_get_network_policy`, `browser_set_network_policy` | `chat` | Planned | Low-frequency; generic is acceptable until the browser family gets a unified renderer pass. |
| `browser_screenshot` | `chat` | Planned | Could later show thumbnail / artifact metadata, but generic is acceptable short-term. |
| `skill` | `chat` | Planned | Depends on how much of skill execution should be surfaced as system state vs transcript content. |

## Runtime Inventory By Family

This is the complete canonical inventory to categorize future work against.

### Shell and background execution

- `bash`
- `write_stdin`
- `job_output`
- `job_kill`
- `monitor`

### File read, search, and filesystem

- `read_file`
- `read_document_url`
- `glob`
- `grep`
- `get_file_metadata`
- `list_directory`
- `create_directory`
- `remove_file`

### File mutation and generation

- `write_file`
- `edit_file`
- `apply_patch`
- notebook tool family (`notebook_edit`, `notebook_create`, `notebook_write`)
- `docx`

### Web and browser

- `web_fetch`
- `web_search`
- `browser_open`
- `browser_navigate`
- `browser_snapshot`
- `browser_extract`
- `browser_list_pages`
- `browser_network_list`
- `browser_list_downloads`
- `browser_search_content`
- `browser_get_network_policy`
- `browser_set_network_policy`
- `browser_select_page`
- `browser_close_page`
- `browser_click`
- `browser_type`
- `browser_press`
- `browser_scroll`
- `browser_wait`
- `browser_screenshot`

### Planning, tasks, permissions, and user interaction

- `task_stop`
- `task_list`
- `task_get`
- `task_create`
- `task_update`
- `enter_plan_mode`
- `exit_plan_mode`
- `submit_plan`
- `request_permissions`
- `ask_user_question`

### Agent, workspace, and orchestration

- `agent`
- `spawn_agent`
- `resume_agent`
- `wait_agent`
- `list_agents`
- `send_agent_message`
- `close_agent`
- `enter_worktree`
- `exit_worktree`

### System, MCP, skills, and retrieval

- `tool_search`
- `mcp`
- `skill`
- `nexus_list_skills`
- `nexus_read_skill`
- `nexus_validate_skill`
- `rag_search`
- `rag_ingest`

### LSP, memory, goals, and media

- `lsp`
- `memory_create_entities`
- `memory_add_observations`
- `memory_search_nodes`
- `memory_open_nodes`
- `create_goal`
- `get_goal`
- `update_goal`
- `generate_image`
- `tts`
- `stt`
- `code_complete`

## Not On The Active Canonical Runtime Surface

These names still appear in legacy TUI code or config, but they are not part of the canonical runtime registry described above.

| Legacy name | Status | Notes |
|---|---|---|
| `todos` | Removed | Renderer, constants (`TodosToolName`, `TodosParams`, `TodosResponseMetadata`), and config entry removed. `FormatTodosList` and helpers retained in `todos.go` as shared utilities used by `task.go` and `pills.go`. |
| `multi_edit` | Legacy | TUI still has a renderer, but canonical runtime centers on `apply_patch`, `edit_file`, and `write_file`. |
| `download` | Legacy | Not part of the current builtin registry. |
| `fetch` | Legacy | Superseded by `web_fetch` and `read_document_url` depending on intent. |
| `agentic_fetch` | Legacy | Not part of the current builtin registry. |
| `sourcegraph` | Legacy | Not part of the current builtin registry. |
| `task_output` | Deprecated / inactive | Defined in `internal/tools/task/constants.go`, but not registered in the active builtin runtime surface. Prefer file reads on task outputs instead. |


## Legacy Candidate Audit Before Any Revival

I did not find a local `crush` repository near this workspace. For now, the closest reliable references are:

- the existing legacy TUI renderers in `internal/nexustui/ui/chat/`
- the current structured tool blocks in `../nexus-ui/src/renderer/components/chat/`

That is enough to make tool-by-tool decisions without reintroducing old names blindly.

| Legacy tool | What the old UI already does well | Recommendation for Nexus |
|---|---|---|
| `multi_edit` | Full-width diff rendering, file-focused header, failed-edit handling. | Do not revive the legacy name. Keep the renderer ideas and fold them into canonical mutation flows such as `apply_patch` or any future batch-edit surface. |
| `download` | Shows URL, optional destination path, timeout, and result body. | Do not revive by default. Only bring back a dedicated UX if downloaded artifacts become an intentional product surface rather than an implementation detail. |
| `fetch` | Syntax-highlighted body by format (`text`, `html`, markdown fallback). | Do not revive the legacy name. Reuse the useful parts for `web_fetch` and `read_document_url`, which are the current canonical read/fetch surfaces. |
| `agentic_fetch` | Best legacy pattern in the set: prompt tag plus nested tool tree and final synthesized body. | Keep as a strong design reference, but apply it to canonical orchestration surfaces such as `agent`, `spawn_agent`, `resume_agent`, or future sub-run containers instead of restoring `agentic_fetch` itself. |
| `sourcegraph` | Query + count/context header with plain result body. | Only worth reviving if Nexus intentionally reintroduces an external code-search backend. If that happens, prefer a compact search-family renderer over a legacy-specific special case. |
| `todos` | Strong visual language: ratio header, progress/change summary, checklist body, started/completed feedback. | Preserve these UX ideas, but migrate them into the upcoming task sidebar and `task_*` visual system rather than keeping todo transcript bubbles alive. |

### Extra note from `nexus-ui`

`nexus-ui` already confirms two product directions that match this audit:

- `ask_user_question` is treated as a dedicated rich interaction, not a generic block
- `task_create` / `task_update` / `task_list` already have dedicated rendering, which supports moving further toward a system-managed task surface

This means we should treat the legacy TUI renderers as a pattern library, not as a registry to restore.

## Recommended Execution Order

1. Hide system-state tools from chat and wire them to header / sidebar / modal surfaces.
2. Decide the final task sidebar UX and migrate `task_*` away from the old `todos` mental model.
3. Add dedicated multi-agent renderers for `agent`, `spawn_agent`, `resume_agent`, `wait_agent`, `send_agent_message`, `close_agent`, and `list_agents`.
4. Convert read/list/search families to summary-first rendering.
5. Remove or quarantine legacy TUI-only tool names so the renderer map tracks the canonical runtime surface again.
