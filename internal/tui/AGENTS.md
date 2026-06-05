# TUI Development Guide

This file describes how to work on the `internal/tui` package safely and
incrementally.

---

## Purpose

The Nexus TUI is the interactive terminal shell for `nexus-engine`.
It is intentionally thinner than the core runtime:

- `internal/engine` owns agent execution.
- `internal/tui` owns presentation, input routing, and local interaction state.
- `internal/tui/workspace.go` is the boundary between the UI and the engine.

Do not move engine logic into the TUI layer.

---

## Current Architecture

The TUI currently follows a **single top-level Bubble Tea model** pattern:

- `app/model.go` owns the main `Model`, the high-level UI state, and the message
  switch in `Update`.
- Sub-components in `components/` are stateful structs with imperative methods:
  `chat`, `session_list`, `permission_dialog`, `model_picker`,
  `command_palette`, `config_panel`, `file_completions`, and `attachments`.
- `workspace.go` defines the interface used by the TUI. The CLI provides the
  concrete implementation and pushes `tea.Msg` events into the model.

This is already close to Crush's architecture. Keep going in that direction.

---

## Rules

- Keep the top-level `Model` as the only Bubble Tea model.
- Do not create nested Bubble Tea sub-model trees unless there is a very strong
  reason.
- Do not do blocking I/O or expensive work inside `Update`.
- Use `tea.Cmd` for side effects and asynchronous work.
- Do not mutate model state from inside a command. Return a message and update
  state in `Update`.
- Keep TUI state local to the TUI package. Runtime state belongs to the
  workspace / engine boundary.
- Preserve the current separation:
  - `workspace` does engine work
  - `model` decides how that work is displayed
  - `common` holds shared rendering helpers and styles
  - `notification`, `pubsub`, `anim`, `fsext`, `csync` stay as supporting
    packages

---

## Component Guidance

### `app/model.go`

Own here:

- high-level state machine
- focus management
- layout calculations
- command routing
- overlay orchestration

Do not let sub-components start calling each other directly in complex ways.
Route transitions through the main model.

### `components/chat.go`

Keep chat behavior deterministic and testable:

- item creation
- continuation behavior after tool calls
- thinking block collapse / expand
- tool progress updates
- content extraction helpers

If rendering becomes more complex, prefer splitting renderers into a dedicated
subpackage later rather than growing one file indefinitely.

### Overlay / dialog-style components

Session browser, permission prompts, model picker, commands palette, and
provider config should remain independent state holders with:

- `SetSize(...)`
- small mutation methods
- one `View()` method

That keeps them easy to test without a running terminal.

---

## Testing Strategy

Manual terminal testing is still useful, but it is not enough once the TUI
grows. Add automated tests for logic that is stable and deterministic.

Prioritize:

1. state transitions
2. filtering / selection logic
3. continuation logic after tool calls
4. layout sizing calculations
5. pure rendering helpers with stable text output

Avoid starting with broad snapshot tests of the whole screen. They become
fragile too early. Start with small unit tests around component behavior.

Good first targets:

- `sessionList` filtering, cursor movement, deletion
- `chat` thinking blocks, tool progress, assistant continuation rules
- `Model.relayout()` and other pure layout helpers

When the TUI structure stabilizes, introduce golden-style tests for key views.

---

## Refactor Direction

The next structural step should move the TUI closer to Crush's package
discipline without copying its full complexity.

Recommended future extraction order:

1. `internal/tui/common`
2. `internal/tui/components`
3. `internal/tui/app`

Do this gradually. Do not attempt a one-shot rewrite.

---

## Definition Of Done

For non-trivial TUI changes:

- the feature works in a real terminal
- at least one automated test covers the new logic when practical
- `go test ./...` still passes
- the change does not push engine concerns into the UI layer

