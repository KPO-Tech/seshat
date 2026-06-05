# TUI Roadmap

This note tracks the next UX and interaction improvements for the Nexus CLI TUI.

## Immediate Fixes

### 1. Welcome wordmark cleanup
- Remove the extra leading dot/bullet from the welcome wordmark because the visual logo is already rendered above it.
- Status: done

### 2. Footer simplification
- Remove `ctrl+e` select mode from the default footer flow.
- The terminal should feel copy/select-friendly by default instead of exposing a special selection mode.
- Reduce footer noise and keep only the actions that matter most during normal chat use.
- Status: planned

### 3. Spinner relocation
- Move the active `working` indicator from the top-right header into a status lane directly above the chat input.
- Goal: the user should see immediately whether the agent is still working while they are focused on the composer.
- Status: planned

## Interaction Changes

### 4. Mouse-first selection and copy
- Replace the current explicit `select mode` approach with proper always-available mouse selection behavior.
- Crush already handles this with cell-motion mouse mode, selection drag, release-to-copy, and clickable rows.
- Nexus should move in the same direction.
- Expected work:
  - enable proper mouse event routing in the main model
  - add chat selection state and drag handling
  - keep clickable tool rows compatible with selection
- Status: planned

### 5. Commands / settings reorganization
- `tab` should probably stop advertising `chat/tools` as a primary action in the footer.
- Tool navigation can stay available, but it should feel secondary rather than front-and-center.
- Introduce a better commands/settings layer for:
  - skills
  - tools
  - MCP
  - model/provider settings
  - session actions
- Reserve slash commands for skills only, with the target UX being `/skill_name`.
- Status: planned

## Runtime Visibility

### 6. Footer token/context usage
- Show token usage in the footer after each turn.
- Data already reaches the TUI via `TurnDoneMsg.InputTokens` and `TurnDoneMsg.OutputTokens`.
- A context-window percentage is possible only after the active model context limit is exposed reliably in TUI state for the current session/model.
- Suggested display:
  - `12.4k tokens`
  - `31% context` once model context capacity is wired in
- Status: partially unblocked

### 7. Working status lane above composer
- Add a narrow status strip above the input for:
  - spinner / working state
  - last stop reason
  - last token usage summary
- This would remove pressure from both the header and footer.
- Status: planned

## Recommended Implementation Order

1. Footer cleanup and removal of `ctrl+e` from the visible happy path
2. Working status lane above the composer
3. Token usage display in footer/status lane
4. Commands/settings reorganization
5. Proper mouse selection and clickable interactions

## Notes

- Crush is the right reference for mouse selection and copy behavior.
- The current Nexus TUI already has the right structural direction for tool rendering, but interaction still needs a second pass.
- `AGENTS.md` should stay focused on engineering rules; roadmap items belong in docs like this file.
