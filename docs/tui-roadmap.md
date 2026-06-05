# TUI Roadmap

This note tracks the current UX progress of the Nexus CLI TUI and the next interaction work.

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
- Added cached renderers by width and per-renderer locking, following the same structural idea as Crush.
- Kept a Nexus-specific style decision: markdown headings no longer show visible `##` / `###` prefixes in the main chat renderer.
- Status: done

## Partially Done

### 7. Footer token/context usage
- The footer now shows cumulative token usage for the current observed session in the TUI.
- Per-turn token usage is rendered on the final assistant meta line instead of duplicating it in multiple places.
- Remaining work:
  - expose reliable model context-window capacity
  - add a context percentage once that data is available
- Status: in progress

### 8. Commands / settings reorganization
- The footer has already been simplified and older noise removed.
- `ctrl+p` is now a true settings hub with nested sections for commands, providers, models, tools, MCP, and skills.
- Generic slash commands are no longer advertised there; slash input is now reserved for skills in the chat composer.
- The `Tools`, `MCP`, and `Skills` sections now load live data from the current workspace/runtime instead of showing only static placeholder copy.
- Status: done

## Next Priorities

### 9. Mouse-first selection and copy
- Implemented:
  - mouse event routing in the main model
  - drag-to-copy text selection in chat
  - copy on mouse release
  - persistent colored selection after mouse release
  - double-click word selection
  - triple-click line selection
  - auto-scroll while dragging at viewport edges
  - `ctrl+shift+c` copy shortcut
  - right-click copy attempt when the terminal forwards the event
  - accurate clipboard-availability notice when Linux clipboard backends are missing
  - `ctrl+shift+c` copy shortcut
  - right-click copy attempt when the terminal forwards the event
  - accurate clipboard-availability notice when Linux clipboard backends are missing
- Remaining work:
  - refine copy semantics for visual chat markers versus plain content where needed
- Status: in progress

### 10. Clickable tool rows and richer interactions
- Implemented:
  - tool rows can now be clicked
  - clicking a tool row selects it
  - explicit click targets exist for expand and details
  - thinking blocks can be expanded or collapsed directly with the mouse
- Remaining work:
  - smoother IDE-like interactions around the side pane
- Status: in progress

### 11. Commands / settings panel expansion
- Completed:
  - skills
  - tools
  - MCP
  - model/provider settings
  - session actions
- Remaining work:
  - deepen each section into richer management views instead of simple searchable lists
- Status: in progress

### 11b. Manual compaction trigger
- A true manual compact action is still missing.
- The runtime currently auto-compacts, but the TUI/SDK surface does not yet expose a dedicated manual compaction API.
- Do not fake this with a normal prompt command; it should be a real engine operation once exposed.
- Candidate UX later:
  - Settings / Commands entry: `Compact Context`
  - optional shortcut such as `ctrl+l` once the runtime hook exists
- Status: planned

### 12. Context percentage and model capacity visibility
- Once model context capacity is reliably available in TUI state, show clear session usage such as:
  - `12.4k total`
  - `31% context`
- Status: planned

## Recommended Implementation Order

1. Mouse-first selection and copy behavior
2. Clickable tool rows and detail interactions
3. Commands/settings reorganization
4. Context-window percentage and model-capacity display

## Notes

- Crush remains the right reference for markdown renderer structure, mouse selection, and interaction polish.
- Nexus intentionally diverges from Crush on some visual choices, especially markdown heading presentation and chat chrome.
- `AGENTS.md` should stay focused on engineering rules; roadmap items belong in docs like this file.
