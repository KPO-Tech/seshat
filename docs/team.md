# Multi-Agent Teams

Nexus supports running **teams of persistent agents** that communicate asynchronously through a mailbox system. Each agent has a named identity, a role, and a system prompt — and can send tasks, replies, and broadcasts to other agents without being co-located or running at the same time.

---

## Concepts

### AgentProfile

An `AgentProfile` is a persistent team-member persona. Unlike a sub-agent (a short-lived tool invocation), a profile represents a named individual that lives across sessions.

```
ID        → UUID, globally unique, never changes
Nickname  → personal name injected into the system prompt ("You are Maria…")
Role      → functional tag used for routing ("researcher", "engineer", "manager")
TeamID    → optional group scope ("alpha", "beta")
SystemPromptTemplate → base persona, supports {{.Nickname}} substitution
Model     → preferred model in "provider:model" format (empty = global default)
Skills    → skill file names to preload
```

Multiple agents can share the same **Role** across different teams:

```
Maria     role=researcher  team=alpha   id=uuid-1
Faouziath role=researcher  team=beta    id=uuid-2
```

Both are reachable independently. `FindByRole("researcher")` returns both; `FindByTeam("alpha")` returns only Maria.

---

### Mailbox

Each agent has a persistent, async inbox backed by SQLite. Messages are typed:

| Kind | Purpose |
|---|---|
| `task` | Delegate a unit of work to another agent |
| `reply` | Respond to a previous message (linked via `ReplyTo`) |
| `broadcast` | Fan-out to all agents in a team |
| `event` | System notification (agent started, finished, errored) |

Messages are never lost between restarts — the inbox persists on disk.

---

### Dispatcher

The `Dispatcher` is the routing layer. It sits on top of `ProfileRegistry` and `Mailbox` and provides four operations:

| Method | Description |
|---|---|
| `Send(from, to, subject, body)` | Direct task to a specific agent UUID |
| `Reply(from, to, replyToID, subject, body)` | Threaded reply linked to a parent message |
| `Broadcast(from, teamID, subject, body)` | Fan-out to all team members (sender excluded) |
| `Assign(from, role, teamID, subject, body)` | Route to the first agent matching role (+ optional team) |

---

### TeamBus

The `TeamBus` polls every registered agent's inbox at a configurable interval and invokes a `MessageHandler` for each unread message. It is the bridge between the mailbox and the execution layer.

```
TeamBus
  ↓ polls every agent inbox
  ↓ marks message as read (before handler, so panics don't cause redelivery)
  ↓ calls MessageHandler(ctx, AgentProfile, Message)
        ↓ your code: run a session, call a tool, send a reply…
```

---

## Package structure

```
internal/agent/    AgentProfile, ProfileRegistry  — who the agents are
internal/mailbox/  Message, Mailbox               — how they communicate
internal/team/     Dispatcher, TeamBus            — how the team is coordinated
```

Dependency direction (no cycles):

```
internal/team  →  internal/agent
internal/team  →  internal/mailbox
```

---

## Usage

### 1. Create and register agents

```go
import (
    "github.com/EngineerProjects/nexus-engine/internal/agent"
    "github.com/EngineerProjects/nexus-engine/internal/db"
)

database, _ := db.Open(ctx, db.Config{Driver: db.DriverSQLite, DSN: "nexus.db", AutoMigrate: true})

registry := agent.NewProfileRegistry(database)
registry.Seed(ctx) // inserts built-in profiles (Nexus, Aria, Kai) if absent

// Create a custom agent
maria := agent.NewAgentProfile("Maria", "researcher", `
You specialise in market research and competitive analysis.
Deliver findings as structured reports with an executive summary.
`)
maria.TeamID = "product-team"
maria.Model  = "anthropic:claude-opus-4-8"

registry.Register(ctx, maria)
```

### 2. Wire up the mailbox

```go
import "github.com/EngineerProjects/nexus-engine/internal/mailbox"

// agentLister resolves team members for broadcast expansion
agentLister := func(ctx context.Context, teamID string) ([]string, error) {
    profiles, err := registry.FindByTeam(ctx, teamID)
    if err != nil {
        return nil, err
    }
    ids := make([]string, len(profiles))
    for i, p := range profiles {
        ids[i] = p.ID
    }
    return ids, nil
}

mb := mailbox.New(database, agentLister)
```

### 3. Create a Dispatcher and send messages

```go
import "github.com/EngineerProjects/nexus-engine/internal/team"

dispatcher := team.NewDispatcher(registry, mb)

// Direct task
dispatcher.Send(ctx, orchestrator.ID, maria.ID,
    "Research competitors",
    "List the top 5 competitors in the LLM space with pricing and positioning.",
)

// Assign by role (picks first available researcher in product-team)
dispatcher.Assign(ctx, orchestrator.ID, "researcher", "product-team",
    "Analyse user feedback",
    "Summarise the last 30 days of support tickets.",
)

// Broadcast to the whole team
dispatcher.Broadcast(ctx, orchestrator.ID, "product-team",
    "Sprint kick-off",
    "New sprint starts Monday. Check your individual tasks.",
)
```

### 4. Start the TeamBus

```go
handler := func(ctx context.Context, profile agent.AgentProfile, msg mailbox.Message) {
    fmt.Printf("[%s] received: %s\n", profile.Nickname, msg.Subject)
    // TODO: run a session here using profile.SystemPrompt() as the system prompt
}

bus := team.NewTeamBus(registry, mb, handler, 2*time.Second)
bus.Start(ctx)
defer bus.Stop()
```

---

## Built-in profiles

Three profiles are seeded automatically on first use. Their UUIDs are fixed so they are stable across restarts.

| UUID suffix | Nickname | Role | Description |
|---|---|---|---|
| `...000001` | Nexus | manager | Coordinates the team, delegates, synthesises results |
| `...000002` | Aria | researcher | Web search, document analysis, structured summaries |
| `...000003` | Kai | engineer | Code implementation, review, debugging |

You can rename them by calling `Register` with the same ID and a different `Nickname`.

---

## Conversation threads

`Reply` links messages via `ReplyTo`. Use `mailbox.Thread(rootID)` to retrieve a full conversation:

```go
// Aria receives a task and replies
msgs, _ := mb.Receive(ctx, aria.ID)
task := msgs[0]

mb.Send(ctx, mailbox.Message{
    Kind:      mailbox.KindReply,
    FromAgent: aria.ID,
    ToAgent:   task.FromAgent,
    Subject:   "Re: " + task.Subject,
    Body:      "Research complete. See attached summary.",
    ReplyTo:   task.ID,
})

// Retrieve the full thread
thread, _ := mb.Thread(ctx, task.ID) // [task, reply]
```

---

## Roadmap

| Feature | Status |
|---|---|
| AgentProfile + ProfileRegistry | ✅ done |
| Mailbox (SQLite, all message kinds) | ✅ done |
| Dispatcher + TeamBus | ✅ done |
| Session execution via MessageHandler | ⏳ next |
| Team definition (named group of profiles) | ⏳ planned |
| Load-balanced Assign (idle-agent routing) | ⏳ planned |
| TUI — team view and message inbox | ⏳ planned |
