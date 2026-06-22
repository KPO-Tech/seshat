# Seshat — Roadmap

## Guiding rule

> **Level by level.** Each level must be robust and proven in production before the next one starts. We do not build the floor above foundations that are not finished.

---

## Level 1 — Single-agent runtime (current focus)

**Goal:** a runtime that a demanding developer can use every day, trusts with their codebase, and deploys in production without surprises. One agent. One session. Full capability.

### What is already working

- Multi-turn agent loop with tool use, recovery, compaction, and streaming
- 15 LLM providers with automatic fallback and circuit breakers
- 60+ built-in tools: file ops, bash (Landlock sandbox on Linux), web search, browser (Playwright), MCP, tasks, memory, RAG, image generation, audio
- Permission engine with configurable modes per session
- Session persistence (filesystem, SQLite) with full transcript and checkpoint recovery
- Skills system (project-scoped and user-scoped Markdown files)
- Universal MCP client (plug in any MCP server at startup or runtime)
- Prometheus metrics and OpenTelemetry tracing (OTLP gRPC)
- gRPC server with generated proto stubs (Python, TypeScript, Java, …)
- Artifact storage (local filesystem, S3-compatible)
- Sub-agent primitives: `spawn_agent` / `wait_agent` exist for isolated single-turn delegation

### What still needs work

- **gRPC surface** — `FileService` and `SystemService` are defined in the proto but not yet implemented in `cmd/grpc`. Sessions are not yet manageable via gRPC.
- **CLI** — `cmd/cli` works but is not yet polished enough to be the primary user-facing tool. Experience improvements, installation docs, and deep skills integration are planned.
- **Sandbox hardening** — Landlock read isolation (`/usr`, `/lib`, `/bin` read-only allowlist) is deferred due to cross-platform complexity. Current implementation restricts write/delete to the workspace but allows read from the full filesystem.
- **MCP spec conformance** — The MCP client implements the core spec. Async tasks and elicitation (MCP November 2025 spec) are not yet implemented.
- **Observability depth** — OTel tracing is wired at the transport layer. Per-session, per-turn, per-tool spans are not yet emitted consistently.

---

## Level 2 — Sub-agent orchestration (next)

**Goal:** a single agent can delegate work to sub-agents — both one-shot isolated tasks and parallel multi-agent execution — and coordinate the results. The orchestrating agent decides what to delegate, to whom, and when to collect.

> Level 2 does not start until Level 1 is stable in production with real daily users.

### The idea

Level 1 gives you a capable agent. Level 2 gives that agent a team of specialists it can call on.

The orchestrating agent is still a single Level 1 agent with full tools and memory. What it gains in Level 2:
- **One-shot task delegation** — spawn a sub-agent with a focused prompt and context, wait for its result, continue. The sub-agent runs in an isolated session with its own transcript and tool surface.
- **Parallel delegation** — spawn multiple sub-agents simultaneously, collect their results when all are done. Useful for independent research, parallel implementation, concurrent test writing.
- **Result integration** — the orchestrator receives sub-agent outputs as tool results and incorporates them into its own reasoning.

```
Main agent (orchestrator)
    │
    ├─ spawn_agent("research competitor A")  ─┐
    ├─ spawn_agent("research competitor B")  ─┤  runs in parallel
    ├─ spawn_agent("analyse our pricing")    ─┘
    │
    └─ wait_agent([all three])
         │
         └─ integrates results → writes final report
```

### What Level 2 adds (on top of Level 1 primitives)

| Capability | Description |
|---|---|
| **Parallel spawn** | `spawn_agent` with a concurrency pool — spawn N agents, collect when all complete |
| **Typed delegation** | Named roles with skill injection at spawn time — "spawn a Python expert", "spawn a security reviewer" |
| **Result streaming** | Sub-agent progress events surfaced to the parent session's runtime event stream |
| **Resource limits** | Per-sub-agent token budget, time limit, and tool restrictions enforced at spawn |
| **Sub-agent registry** | Named, reusable agent templates defined in skills files |

`spawn_agent` and `wait_agent` already exist as Level 1 primitives. Level 2 hardens them, adds parallel execution with proper resource governance, and makes them production-reliable.

---

## Level 3 — Team runtime (future)

**Goal:** multiple full Level 1 agents running as a persistent team, coordinating on a shared mission through async communication and a shared task board. No central orchestrator — each agent acts based on its inbox and its role.

> Level 3 does not start until Level 2 is stable. No open P0/P1 issues.

### The idea

The goal of Level 3 is to replicate how a human team works — not a pipeline, not a coordinator-worker hierarchy, but a real team where each member has agency.

```
Mission: "Implement OAuth2 authentication"

Alex (CEO)     → reads mission → posts initial tasks to the board
Jordan (CTO)   → claims architecture → sends design to Sam via mailbox
Sam (Dev)      → claims implementation → codes → spawns workers for tests
Taylor (QA)    → reviews output → sends bug report back to Sam
Robin (DevOps) → claims deployment → deploys → marks mission complete
```

Each team member is a full Level 1 agent: its own tools, memory, skills, sessions, and permissions. No central orchestrator tells them what to do. They act based on their inbox, the shared task board, and their role's skill file.

### What Level 3 requires (new primitives)

| Primitive | Description |
|---|---|
| `send_mail` / `read_mail` | Async message passing between named agents |
| `post_task` / `claim_task` / `complete_task` | Shared task board with ownership and status |
| Team memory | Shared vector store searchable by all members; each agent's private memory stays private |
| Budget enforcement | Hard token budget and wall-clock timeout per agent, per mission |
| Mission lifecycle | Start, monitor, suspend, resume, cancel a multi-agent mission |

Everything else (tools, sessions, skills, permissions, streaming) comes from Level 1 unchanged.

### What the engine already provides

Sessions run in parallel today. The skills system handles role-specific behavior. Long-term memory has project/user/cross-session tiers. Level 2 sub-agent delegation provides the execution substrate. Level 3 builds coordination *above* these.

### Rough sequence (when Level 2 is stable)

| Phase | Scope | Estimate |
|---|---|---|
| Phase 0 — PoC | 2 agents, in-memory mailbox, 1 test mission | ~2 weeks |
| Phase 1 — Infrastructure | DB-backed mailbox + task board, 5 agents, budget enforcement | ~3 weeks |
| Phase 2 — Observability | OTel spans per agent/mission, SSE mission stream, basic UI | ~2 weeks |
| Phase 3 — Hardening | Concurrent activation, fault tolerance, benchmarks | ~3 weeks |

Earliest realistic start: Q4 2026.

---

## What success looks like

### Level 1

A developer runs `seshat run "write tests for the auth module"` and gets a pull request — correct tests, in their coding style, within their project's conventions — without manual intervention. They review the result like a colleague's PR, not a code generator's output.

### Level 2

A developer runs `seshat run "research these 5 competitors and write a comparison report"`. The agent spawns five sub-agents in parallel, one per competitor, collects their findings, and writes the report. Total time: 4 minutes instead of 2 hours. The orchestrator's reasoning is visible in the session trace.

### Level 3

A team configures a five-agent mission: CEO, CTO, two developers, QA. They describe a feature in one sentence. The team researches, architects, implements, tests, and opens a pull request — asynchronously, over several hours — while the humans do other work. They review the result, not the process.
