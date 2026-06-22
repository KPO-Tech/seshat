# TUI Roadmap

This note tracks the current UX progress of the Seshat CLI TUI and the next interaction work.

## Completed

### 1. Welcome wordmark cleanup
- Removed the extra leading bullet from the welcome screen wordmark.
- Status: done

### 2. Footer simplification
- Removed `ctrl+e` select mode from the visible happy path.
- Simplified the default footer actions.
- Tool navigation is no longer advertised as `tab chat/tools` in the primary footer flow.
- Status: done

### 3. Working status lane above composer
- Moved the active `working` indicator out of the header.
- Added a status lane directly above the composer for runtime visibility.
- The lane now focuses on runtime state only: `working`, `failed`, or `ready`.
- Status: done

### 4. Primary chat layout polish
- The app now uses near-full-width layout with small left/right margins instead of a narrow centered column.
- User messages use an inline blue `● >` marker.
- Assistant messages use an orange `●` marker.
- Intermediate assistant segments created around tool calls no longer show false `done` states.
- Final assistant metadata is attached only to the true end of the turn.
- Status: done

### 5. Tool rendering baseline
- Added richer tool summaries and previews for core tools.
- Kept completed tools visually more neutral so green is reserved for actual turn completion.
- Added a right-side details pane for selected tools.
- Status: done

### 6. Shared markdown renderer
- Switched from raw environment-configured glamour usage to a shared markdown helper in `internal/tui/common/markdown.go`.
- Added cached renderers by width and per-renderer locking.
- Markdown headings no longer show visible `##` / `###` prefixes in the main chat renderer.
- Status: done

### 7. Config/credentials isolation (CLI vs backend)
- CLI sets `NEXUS_RUNTIME_ROOT` → `~/.config/seshat-cli`; backend stays on `~/.config/seshat`.
- `LoadInto()` uses `runtimepath.ResolveRoot("")` instead of a hardcoded path.
- `ParseModelIdentifier("")` returns an empty `ModelIdentifier{}` instead of the Anthropic SDK default.
- No provider configured at startup → welcome screen shows `ctrl+p` hint instead of auto-opening settings.
- Status: done

### 8. Credentials in SQLite DB
- API keys, model selection, Ollama URL, search keys stored in scoped SQLite DB keys.
- `SetModel` / `SaveProviderField` persist to DB; `loadCredsIntoConfig` restores on restart.
- Status: done

### 9. Clipboard paste in secret fields
- `ctrl+v` pastes from clipboard in config panel and search panel.
- `ctrl+r` toggles reveal/hide (was previously bound to `ctrl+v` by mistake).
- Status: done

### 10. Model picker — configured-only filtering
- Model picker only shows providers that have credentials configured.
- Ollama is probed at startup in the background; models are cached in the DB.
- Ollama endpoint is configurable in the provider settings panel.
- Saving a new Ollama URL triggers a re-probe automatically.
- Embedding models (bert, *-embed) are filtered out.
- Status: done

### 11. Mouse-first selection and copy
- Mouse event routing, drag-to-copy, persistent colored selection after release.
- Double-click word, triple-click line selection.
- Auto-scroll while dragging at viewport edges.
- `ctrl+shift+c` copy shortcut; right-click copy attempt.
- Accurate clipboard-availability notice when Linux clipboard backends are missing.
- Remaining work: refine copy semantics for visual markers vs plain content.
- Status: in progress

### 12. Clickable tool rows and richer interactions
- Tool rows can be clicked to select; expand and details have explicit click targets.
- Thinking blocks can be expanded or collapsed with the mouse.
- Remaining work: smoother IDE-like interactions around the side pane.
- Status: in progress

### 13. Commands / settings panel
- `ctrl+p` settings hub with nested sections: commands, providers, models, tools, MCP, skills.
- Sections load live data from the current workspace/runtime.
- Remaining work: deepen each section into richer management views.
- Status: in progress

### 14. Model picker freeze fix (this session)
- `SetModel` was doing synchronous SQLite I/O on the BubbleTea event-loop goroutine → blocked all input.
- Fix: DB persistence moved to a goroutine inside `SetModel`.
- `ctrl+c` / `ctrl+q` were silently swallowed by overlay-state handlers (stateModelSelect, stateCommands, etc.) that all end with `return true, nil`.
- Fix: global quit check added at the top of `handleKey`, before all state-specific blocks.
- Status: done

### 15. `ask_user_question` inline interactive bubble
- New tool renderer with keyboard-driven option picker (↑↓ navigate, Space multi-select, Enter confirm).
- `askUserBroker` in workspace routes `PromptTypeChoice`/`PromptTypeText` to the TUI; `PromptTypeConfirm` stays on `permBroker`.
- `ask_user_question` and `list_directory` added to `isAlwaysSafeTool` — no permission dialog for these.
- Batch questions: each Q→A pair is shown in history inside the same bubble.
- "Other" option routes focus to the editor for free-text input.
- Bug fixed: Space key registered as `case "space":` not `case " ":` (ultraviolet `KeySpace.String()` → `"space"`).
- Status: done

### 16. Timer and auto-scroll bug fixes
- **Timer**: `Finish.Time` and `Message.CreatedAt` are both milliseconds (`UnixMilli()`), but `renderContent` and `NewAssistantInfoItem` were calling `time.Unix(x, 0)` → duration 1000× too large (87 s shown as 24h5m6s). Fixed with `time.UnixMilli(x)` + `.Round(time.Second)`.
- **"done" footer layout**: was `done · 3.2s·············`; changed to `done · ················ 3.2s` (dots fill middle, time anchors right).
- **`AtBottom()` bug**: early-exit `totalHeight > l.height` did not subtract `offsetLine`, so the function returned `false` when at the bottom of a tall item → `ScrollBy(positive)` never re-enabled `follow` mode → auto-scroll resume after user scroll was broken. Fixed by moving the check to after accumulating each item's height: `totalHeight - offsetLine > height`.
- Status: done

---

## In Progress / Next

### 16. Tool row compression (high priority)
**Problem**: rows are verbose and noisy.
- Current: `► ⊞ ✓ Task Create done Tool task_create completed (49ms)`
- Target: `✓ TaskCreate  #1780…  (49ms)`

**Changes**:
- Single status icon (`●` pending / `✓` done / `✗` error) — remove the triple-icon prefix `► ⊞ ✓`.
- Remove redundant text: "done", "Tool", "completed" — the icon + duration is enough.
- Show the first relevant argument (truncated) in the row: file path for Write/Read/Edit, command for Bash, task title for TaskCreate.
- Duration shown only when > 0.

**Grouping** (consecutive calls of the same tool type):
- **Grouped** (output is uniform per call): `Read`, `Write`, `TaskCreate`, `TaskUpdate`, `ListDirectory`.
- **Not grouped** (each call has unique and important output): `Bash`, `Edit`, `WebFetch`, `WebSearch`.
- A group collapses into one row: `✓ Read (4×)  main.go, config.go, …  (total Xms)`.
- Selecting the group in the right panel lists all individual calls with their outputs.

**Thinking blocks**:
- Replace "click to expand" with "ctrl+t to toggle" (TUI is keyboard-driven, not click-driven).
- Add a subtle left border or background tint to distinguish from regular text.

Status: planned

---

### 17. Plan mode state tracking + enter/exit tool rows
**Problem**: `enter_plan_mode` and `exit_plan_mode` appear as tool rows in the chat — they are state transitions, not user-visible actions.

**Changes**:
- Track plan mode state in the TUI (increment/decrement counter based on intercepted tool progress messages for `enter_plan_mode` / `exit_plan_mode`).
- When plan mode is active, show a `◈ Plan Mode` status pill in the header (next to the model pill) or as a banner between the header and chat area.
- Suppress `enter_plan_mode` / `exit_plan_mode` tool rows from the chat area entirely.

Status: planned

---

### 18. Task list display (replaces task tool rows)
**Problem**: `task_create` × N and `task_update` × N appear as individual rows with no structure. The user can't see the plan progress at a glance.

**Design**:
- When the first `task_create` tool call completes in a turn, open a "Plan" panel inside the chat — a structured task list with the task titles.
- Each task starts with `○` (open).
- When a `task_update` with `status=complete` arrives for a task, its row updates to `✓`.
- While a task is actively being executed (a tool call group runs after a specific task), it shows a spinner `●`.
- The task list is shown as a chat block, not as individual tool rows.
- `task_create` and `task_update` tool rows are suppressed from the main tool row stream.

Status: planned

---

### 19. Plan file overlay (plan mode review flow)
**Background**: When Claude is in plan mode, a system prompt forces it to write a detailed markdown plan file before proceeding. This overlay intercepts that file and presents it for review.

**Plan file location** (finalized):
```
{working_dir}/.seshat/plans/{short_session_id}/{workspace_slug}_{YYYYMMDD-HHMMSS}.md
```
- `.seshat/plans/` → hidden project-scoped folder, unambiguous TUI detection pattern.
- `{short_session_id}/` subfolder → groups all revisions of a session together.
- `{workspace_slug}_{YYYYMMDD-HHMMSS}.md` → each review revision gets a new timestamped file.
- Multiple revisions per session are supported (Review → Claude rewrites → new file → overlay again).

**DB keys**:
- `plan_status:{sessionID}` → `pending` | `reviewing` | `validated`
- Latest plan file = most recent `.md` in the session subfolder (sorted by filename/mtime).
- On session resume: if `plan_status` is `pending` or `reviewing`, re-show the overlay.

**Trigger**: A `write` tool call completes AND the written path matches `**/.seshat/plans/**/*.md`.
  - No plan-mode-active check needed — the path pattern is unambiguous.

**Flow**:
1. Read the written file content.
2. Send a `planReviewMsg{content, filePath, sessionID}` to the TUI.
3. Show a full-screen overlay (similar to the permission dialog but taller):
   - Top: rendered markdown content of the plan (scrollable).
   - Bottom: editable comment area + two action buttons.
   - `[V] Validate` — accept the plan as-is.
   - `[R] Review` — submit comments back to Claude.
4. If **Validate**: send a follow-up message to Claude ("Plan validated. Exit plan mode and begin execution.").
   - Claude exits plan mode automatically and starts creating tasks.
   - DB: `plan_status:{sessionID}` → `validated`.
5. If **Review**: send a follow-up message ("Here are my review comments: [comments]. Please revise the plan.").
   - DB: `plan_status:{sessionID}` → `reviewing`.
   - The overlay closes; Claude revises and writes a new plan file → triggers the overlay again.

Status: planned

---

### 20. Text overflow with sidebar open (bug)
**Problem**: When the right-side details panel is open, some chat text is truncated at the right edge instead of wrapping.
**Likely cause**: `chatW` calculation in `viewChat()` does not propagate correctly to all text renderers when the sidebar is open.
Status: planned (low effort)

---

### 21. Context percentage and model capacity visibility
- Once model context capacity is reliably available, show `31% context` in the footer or header.
- Status: planned

---

### 22. Manual compaction trigger
- A dedicated "Compact Context" action (settings hub + optional `ctrl+l`).
- Do not fake this with a normal prompt — it must be a real engine operation once exposed.
- Status: planned

---

## Implementation Order

1. Tool row compression (16) — most visible, standalone change
2. Plan mode state tracking (17) — prerequisite for 18 and 19
3. Task list display (18) — depends on 17
4. Plan file overlay (19) — depends on 17; needs plan file pattern decision
5. Text overflow fix (20) — quick bug fix, any time
6. Context percentage (20) — needs upstream data

## Reference

- Crush (`/home/amiche/Projects/AI/ai/nexus-product/helps/crush`) remains the primary reference for tool row style, thinking blocks, and animation patterns.
- Seshat intentionally diverges from Crush on markdown heading presentation and chat chrome.
- `AGENTS.md` stays focused on engineering rules; roadmap items belong here.
