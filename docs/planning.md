# Planning Mode and Execution Modes

Seshat is a Go agent runtime that supports three distinct execution modes. Each mode controls how much autonomy the agent has before taking actions. You can switch modes per session or configure a default.

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
2. Agent reasons about the task and emits a structured plan (no tools called yet).
3. Plan is shown to the user: list of proposed actions with arguments.
4. User approves (`yes`), modifies, or rejects.
5. On approval, the agent executes exactly the approved plan.

---

### 3. Pair programming (collaborative)

The agent proposes each action one at a time and waits for confirmation before proceeding. Unlike plan mode (which shows the full plan upfront), pair mode is step-by-step.

Best for: exploratory work, learning sessions, pair programming where you want to stay in the loop for each decision.

```bash
seshat chat --mode pair
```

```go
session, _ := client.CreateSession(ctx, &sdk.SessionConfig{
    ExecutionMode: sdk.ModePairProgramming,
})
```

---

## Permission engine

Orthogonal to execution modes, the permission engine controls which tools require explicit approval:

| Permission level | Behavior |
|---|---|
| `auto` | LLM classifier decides per-tool whether approval is needed |
| `acceptEdits` | File edits auto-approved, all others require confirmation |
| `onRequest` | Every tool call requires explicit approval |
| `bypass` | All tools auto-approved (use only in trusted pipelines) |
| `never` | No tools are ever called (read-only reasoning sessions) |

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
