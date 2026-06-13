# Current TUI Workstream — Session Tasks, System Surfaces, and Streaming Integrity

## What we are doing now

The current TUI workstream is no longer only about the original live-stream ordering bug.
That problem has already been fixed, and the focus has moved to a broader goal:
turning system-state tools into proper UI surfaces instead of noisy transcript artifacts.

The main active surface right now is the `task_*` system.
We are replacing the old transcript-centered todo mental model with session-scoped,
persistent tasks that the user can follow visually while the agent is working.

## Why this work matters

The product goal is to make the chat transcript easier to read while still keeping
execution progress visible.

The old model had several problems:

- system tools polluted the transcript like normal user-facing actions
- task state was not persistent or reliably session-scoped
- task inspection tools were inconsistent (`task_create` / `task_update` used one system while `task_get` / `task_stop` still used background runtime tasks)
- long-running work lacked a clear execution-tracking surface outside plain assistant prose

The new direction is:

- keep chat focused on user intent, assistant synthesis, and important execution evidence
- move system-state transitions into dedicated UI surfaces where possible
- make tasks persistent per session and safe across reload/resume
- support both lightweight ambient visibility and richer inspection/detail views

## What is already done

### 1. Streaming / transcript correctness

- fixed live-stream ordering so tool calls stay above later synthesis text instead of being reordered incorrectly during streaming
- ensured the live path and reload path now agree structurally

### 2. Plan mode system surfaces

- `enter_plan_mode` and `exit_plan_mode` are hidden from normal chat transcript rendering
- the header now reflects the real execution mode live (`execute`, `plan`, `pair`)
- `submit_plan` has a dedicated review surface instead of behaving like a normal chat bubble

### 3. Runtime root and session artifacts

- the TUI runtime root is aligned to `~/.config/nexus-tui`
- sent pastes are persisted under the session runtime only at send-time, avoiding orphaned files
- session plans and other system artifacts now follow the intended runtime structure more closely

### 4. Session task persistence

- `task_*` state is now persisted in SQLite, scoped by `session_id`
- session reloads restore tracked tasks cleanly
- runtime events refresh the current session task surface

### 5. Task tool semantic realignment

The task tools were inconsistent before.
Now the behavior is aligned like this:

- `task_create` / `task_update`: session-scoped tracked execution tasks
- `task_list`: defaults to session tasks when the session already has tracked tasks; background tasks remain available explicitly
- `task_get`: looks up session tasks first, then falls back to background runtime tasks
- `task_stop`: stops tracking a session task first, then falls back to background runtime tasks

This removes the mismatch where IDs created by task tracking could not be retrieved or stopped by the related tools.

### 6. Current task UI

There are now two visible layers for tasks:

- a compact `Tasks` panel in the lower chat area for ambient progress visibility
- task-aware rendering for `task_list`, `task_get`, and `task_stop` in the chat transcript

The compact panel now:

- shows `Tasks` instead of `To-Do`
- follows session task updates correctly
- preserves chat auto-scroll during streaming/task refreshes
- auto-collapses only if it was auto-opened by the system and all tasks are complete
- stays open if the user opened it manually

## Current product reasoning

For now, keeping both a compact task panel and a future richer task sidebar is intentional.

Reasoning:

- the compact panel gives constant low-friction visibility during work
- a richer sidebar is better for detail, navigation, dependencies, and future actions
- these two surfaces are complementary rather than redundant if their responsibilities stay distinct

The intended split is:

- compact task panel: summary / ambient progress / lightweight visibility
- sidebar task surface: detailed inspection, navigation, rich metadata, future controls

## What is not finished yet

The task work is **not fully finished**.

The remaining work is mostly UI/UX completion rather than core data correctness.

Key remaining items:

1. wire a true right-side task sidebar into the main `uiChat` layout instead of relying only on the compact bottom panel
2. decide the final coexistence policy between compact panel and sidebar once the sidebar exists in the real chat layout
3. add richer task details such as dependencies (`blocked by`, `blocks`) in the detailed surface
4. potentially make the sidebar more interactive beyond passive inspection
5. continue the broader system-surface pass for other tools after tasks are fully settled

## Short summary

The active workstream is about making the Nexus TUI feel like a real execution workspace,
not just a transcript renderer.

The `task_*` system is now persistent, session-scoped, tool-consistent, and visibly integrated.
What remains is finishing the richer task UI surface and finalizing how compact progress and detailed navigation should coexist.

---

# Bug audit: tool call ordering during live streaming

## What is broken

During live streaming in the TUI chat, **tool call items appear above synthesis text that was generated after them**. Concretely:

1. Agent runs a tool (e.g. `List Directory`, `Web Search`)
2. Gets the tool result back
3. Writes a synthesis/analysis paragraph based on that result
4. **Display shows**: synthesis paragraph → tool call (wrong)
5. **Expected display**: tool call → synthesis paragraph

/home/amiche/Projects/AI/ai/nexus-product/private/captures/issue1.png
/home/amiche/Projects/AI/ai/nexus-product/private/captures/issue2.png

After quitting the session and resuming, things look correct because `setSessionMessages` reconstructs the list from the final message state. The live-streaming path produces the wrong order.

Screenshots in `private/captures/issue1.png` and `private/captures/issue2.png`.

---

## Root cause hypothesis (needs verification)

The probable cause is in how `message.Message.AppendContent` works combined with how `ExtractMessageItems` creates chat items from the resulting Parts slice.

### Step 1 — AppendContent always appends to the first TextContent

File: `internal/nexustui/message/content.go` — method `AppendContent`.

```go
func (m *Message) AppendContent(delta string) {
    for i, part := range m.Parts {
        if c, ok := part.(TextContent); ok {
            c.Text += delta
            m.Parts[i] = c
            return
        }
    }
    m.Parts = append(m.Parts, TextContent{Text: delta})
}
```

This finds the **first** TextContent in Parts and appends to it. If a ToolCall was added between two text segments, the subsequent text still ends up in that same first TextContent. The result is a Parts slice like:

```
[TextContent{"pre-tool text" + "post-tool synthesis"}, ToolCall{tc1}]
```

instead of the semantically correct:

```
[TextContent{"pre-tool text"}, ToolCall{tc1}, TextContent{"post-tool synthesis"}]
```

### Step 2 — ExtractMessageItems creates one AssistantMessageItem for all text

File: `internal/nexustui/ui/chat/messages.go` — `ExtractMessageItems`.

It walks Parts in order. On `TextContent` or `ReasoningContent` it creates one `AssistantMessageItem` (which renders `msg.Content().Text` = the merged TextContent). On `ToolCall` it creates a `ToolMessageItem`. Because all text is merged, the items list always becomes:

```
[AssistantMessageItem{all text}, ToolItem{tc1}]
```

All text is at the top; the tool call is below — even if half the text was written after the tool result was received.

### Step 3 — Live streaming re-uses this same merged state

During streaming, `updateSessionMessage` calls `assistantItem.SetMessage(&msg)` on each UpdatedEvent. The AssistantMessageItem always renders `msg.Content()` = the merged TextContent. So the synthesis text that arrives after the tool result immediately appears at the top of the AssistantMessageItem, not below the tool.

---

## Files to read completely before proposing any fix

### Core message model
- `internal/nexustui/message/content.go`
  - `AppendContent` and `AppendReasoningContent`
  - `Content()` — returns first TextContent; important for rendering
  - How `Parts` is populated by providers during streaming

### Chat item extraction
- `internal/nexustui/ui/chat/messages.go`
  - `ExtractMessageItems` — how it walks Parts
  - `ShouldRenderAssistantMessage`
- `internal/nexustui/ui/chat/assistant.go`
  - `AssistantMessageItem.Render` / `renderMessageContent` — what it actually displays

### Live streaming update path
- `internal/nexustui/ui/model/ui.go`
  - `appendSessionMessage` — first CreatedEvent handling
  - `updateSessionMessage` — UpdatedEvent handling, how tool items are positioned (pay special attention to the anchor logic and `InsertMessagesAfter`)
  - `setSessionMessages` — why does this produce correct order after reload?

### Chat list
- `internal/nexustui/ui/model/chat.go`
  - `AppendMessages`, `InsertMessagesAfter`, `InsertMessagesBefore`
  - `idInxMap` consistency after inserts/removes
  - `RemoveMessage` — does it correctly rebuild nested tool IDs?

### Provider streaming
- All files under `internal/providers/` and `internal/nexustui/workspace/`
  - How do providers populate `Parts` during streaming?
  - Specifically: does the provider call `AppendContent` AFTER a ToolCall is appended to Parts, and if so, does it land before or after the ToolCall?

---

## Key questions to answer from the audit

1. **Is `AppendContent` the real cause?** When the provider appends post-tool synthesis text, does it call `AppendContent` → landing in the pre-existing first TextContent (before the ToolCall in Parts)? Or does it create a new TextContent after the ToolCall?

2. **Does `ExtractMessageItems` handle `[TextContent, ToolCall, TextContent]`?** The `assistantAdded` flag means only ONE AssistantMessageItem is ever created per message. If Parts has a second TextContent after a ToolCall, that second text is silently ignored (or merged). Is this the case?

3. **Is the issue in the live streaming path only, or also in the reload path?** The user reports that after reload, the order looks correct. Does `setSessionMessages` somehow produce a different display? Why?

4. **Multi-message vs. single-message**: For the models used (glm-4.5 via z-ai provider), does the synthesis text after a tool result arrive in a **new** assistant message (`AssistantMsg2`) or is it appended to the **same** assistant message (`AssistantMsg1`) via `AppendContent`? This changes everything about the fix.

5. **Is `idInxMap` ever stale?** Could `m.chat.MessageItem(msg.ID)` return nil even when the AssistantMessageItem exists, due to a missing or shifted entry?

---

## What the correct fix should achieve

During live streaming:

- Pre-tool text streams into AssistantMessageItem (same as now)
- When a tool call arrives, ToolItem appears **directly below** AssistantMessageItem
- Post-tool synthesis text should appear **below** the ToolItem

The fix depends on the answer to question 4 above:

**If synthesis is in AssistantMsg2** (a separate message): The fix is purely about insertion order in `updateSessionMessage` — ensuring AssistantMsg2's item is appended AFTER the ToolItem, not before. Investigate whether `appendSessionMessage` ever places a new assistant message BEFORE an existing tool item.

**If synthesis is in the same message (AppendContent merges it)**: The fix requires one of:

- **Option A** — Fix `AppendContent`: when Parts already has a ToolCall after the last TextContent, create a new TextContent part instead of appending to the first one. This gives `[TextContent{pre}, ToolCall, TextContent{post}]`. Then `ExtractMessageItems` must be updated to create a separate display item for each TextContent segment.

- **Option B** — Fix `ExtractMessageItems`: render only text that precedes the first ToolCall in the main AssistantMessageItem; accumulate text after each tool call into separate display items.

- **Option C** — Fix `AssistantMessageItem.renderMessageContent`: walk `msg.Parts` to identify which text came before the first tool call and display only that. Leave post-tool text for a separate renderer.

---

## What has already been tried (don't retry these)

1. Added `InsertMessagesAfter(afterID, msgs)` to `Chat` — inserts new tool items after an anchor item and rebuilds idInxMap. Still shows tools at bottom.

2. Added `InsertMessagesBefore(beforeID, msgs)` to `Chat` — symmetric counterpart.

3. In `updateSessionMessage`, changed from `AppendMessages(items...)` to `InsertMessagesAfter(lastAnchorID, items...)` where `lastAnchorID` starts at `msg.ID` and is updated to each existing tool call's ID found in Parts order. Still broken.

4. Added defensive creation of `AssistantMessageItem` in `updateSessionMessage` when `existingItem == nil && shouldRenderAssistant` — inserts it before the first existing tool. Handles one edge case but doesn't fix the main issue.

These changes are on branch `feat/tui-subagent-inline-streaming`, commit `2cb950e`.

---

## Context

- Framework: BubbleTea v2 (Elm-style, single-threaded Update loop)
- Chat list: `internal/nexustui/ui/list/list.go` — `List` with `items []Item`; `internal/nexustui/ui/model/chat.go` — `Chat` with `idInxMap map[string]int`
- Message model: `internal/nexustui/message/content.go` — `Message.Parts []Part` where `Part` can be `TextContent`, `ReasoningContent`, `ToolCall`, `ToolResult`
- Providers: OpenAI-compatible streaming via SSE; tool calls and text deltas arrive as separate events and are assembled into `Parts` by the provider client
- Branch: `feat/tui-subagent-inline-streaming`
- Go module: `github.com/EngineerProjects/nexus-engine`
