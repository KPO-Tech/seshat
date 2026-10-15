# Planning Mode and Execution Modes

Seshat is a Go agent runtime that supports two distinct execution modes. Each mode controls how much autonomy the agent has before taking actions. You can switch modes per session or configure a default.

> Full documentation: [seshat-ai.com/docs/guides/workflows](https://seshat-ai.com/docs/guides/workflows)

---

## The three modes

### 1. Execute (default)

The agent acts immediately. When it decides to call a tool (write a file, run a command, make a web request), it does so without pausing for review.

Best for: trusted environments, automated pipelines, developers who know what the agent will do.

```bash
seshat chat                        # default: execute mode
seshat run "add error handling to main.go"
```

```go
session, _ := client.CreateSession(ctx, &sdk.SessionConfig{
    ExecutionMode: sdk.ModeExecute,
})
```

---

### 2. Plan (review before act)

The agent produces a plan before taking any action. You review the plan, approve or modify it, and only then does the agent execute.

The plan shows: what tools will be called, in what order, with what inputs, and why.

Best for: production systems, sensitive environments, code review workflows, onboarding.

```bash
seshat chat --mode plan
```

```go
session, _ := client.CreateSession(ctx, &sdk.SessionConfig{
    ExecutionMode: sdk.ModePlan,
})
```

**Flow in plan mode:**

1. User sends a message.
2. Agent investigates read-only and designs the approach. It asks with
   `ask_user_question` only for real requirement gaps, not for the plan itself.
3. Agent submits the plan with `submit_plan` (no implementation tools called
   yet). The plan is saved and shown to the user as an interactive artifact, and
   the session stays in plan mode.
4. User sends feedback — the agent revises and resubmits — or approves the plan.
5. Approving records the decision, which is what unblocks `exit_plan_mode`. The
   agent calls it to leave plan mode (or the host exits the mode for it), then
   executes the approved plan.

The approval is a gate rather than a label: `exit_plan_mode` is refused while the
plan you were shown is still awaiting your answer, so the model cannot grant
itself write access by leaving the mode before you have decided. Revising a plan
after feedback puts it back in review, because a revised plan is a different plan.
A session that never submitted anything keeps its `exit_plan_mode`: `submit_plan`
is optional, and a gate that assumed otherwise would leave plan mode with no door.

**Proposing a plan without plan mode:**

`submit_plan` works in any mode. In execute mode the agent calls it when a choice
belongs to you and is cheaper to settle before it starts (several defensible
designs, work that is hard to undo, no clear definition of "done"), or when you
asked to validate the plan first (a research you want to approve, a large change).
The plan opens in the same review screen; you approve it or ask for changes, and
the agent carries on after your answer. There is no mode to leave: the approval
message says "Proceed with the plan". Before this, `submit_plan` refused outside
plan mode, so an agent in execute mode could only write the plan as text and ask
"do you approve?", with no screen and no button to answer.

**What plan mode is for:**

Plan mode adds one thing to that: a guard. While it is on the agent cannot change
files, only read, investigate and design. Use it when you want that guarantee
(an unfamiliar repository, a sensitive environment, "design it, change nothing
yet"). The agent does not need it to propose a plan and does not enter it for that;
it enters it only when you ask. You can toggle it yourself: `shift+tab` switches
between plan and execute mode, and the `toggle_plan` command in the command
palette does the same.

**Recording the approval from your own UI:**

A host that reviews plans outside the terminal has to record the answer itself,
because the gate looks at what the user decided and not at the plan document:

```go
sdk.ApprovePlan(session.ID) // the user accepted the plan this session submitted
```

The package-level form takes a session id rather than a session because a host
that persists its sessions reloads them per turn — between two turns there is no
live `*sdk.Session` to call, while the review state is process-global and keyed by
session id, so the answer a user clicks in a plan panel still has to reach the
next turn's check. On a live session, `session.ApprovePlan()` is the same call.
Leaving plan mode is still separate: the call records the decision only, and
`exit_plan_mode` (or `ClearPlanMode`, which records nothing) applies the change.

---

## Permission engine

Orthogonal to execution modes, the permission engine controls which tools require explicit approval:

| Permission level | Behavior |
|---|---|
| `auto` | LLM classifier decides per-tool whether approval is needed |
| `acceptEdits` | File edits auto-approved, all others require confirmation |
| `onRequest` | Every tool call requires explicit approval |
| `bypass` | All tools auto-approved (use only in trusted pipelines) |
| `never` | Never prompts the user; a tool call that would need approval is denied and reported back to the agent instead (headless/background sessions) |

```go
session, _ := client.CreateSession(ctx, &sdk.SessionConfig{
    PermissionMode: sdk.PermissionOnRequest,
})
```

---

## Yolo mode

In interactive sessions, `ctrl+y` toggles **yolo mode**: all permission checks are bypassed for the rest of the session. Useful for exploratory sessions when you trust the agent's judgment.

---

## Related docs

- [RAG System](./rag.md) - how the agent retrieves knowledge before planning
- [Memory and Compaction](./memory.md) - keeping context alive across long planning sessions
- [MCP Client](./mcp.md) - extending what the agent can act on
- [SDK Guide](./sdk.md) - full SessionConfig reference
