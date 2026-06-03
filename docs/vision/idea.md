# The Idea Behind Nexus Engine

## The problem

AI tooling in 2025 splits into two extremes.

On one end: **chat interfaces** — powerful for conversation, useless when you need an agent to actually do something: read your code, run commands, interact with your stack, make decisions over multiple steps, and deliver a result.

On the other end: **developer frameworks** (LangGraph, OpenAI Agents SDK, Claude Agent SDK) — low-level, opinionated, tied to a language (Python or TypeScript), tied to a provider, and not meant to run as standalone infrastructure. You build *with* them, you don't deploy *them*.

Between the two, something is missing: **a runtime**. A self-contained, deployable binary that turns an LLM into a capable agent — with real tools, real permissions, real sessions — that you can call from anywhere, in any language, through any interface.

That is Nexus Engine.

---

## The thesis

> **One runtime. Any LLM. Any language. Any deployment.**

An AI agent runtime should work like a database server: you deploy it, you connect to it, it handles the hard parts. You shouldn't have to embed it, configure its internals, or rewrite it in your language of choice.

Concretely:
- A Go developer embeds it via the **SDK** with full control.
- A Python or TypeScript team calls it via **gRPC** with generated stubs.
- A developer at the terminal uses it as a **CLI** — the same way they use Claude Code or Codex, but provider-agnostic and self-hosted.
- A product team builds multi-user infrastructure on top of it, adding auth, billing, and workspaces without touching the core runtime.

The runtime itself stays clean. No users. No billing. No lock-in.

---

## Design principles

### 1. Local-first

The runtime runs on your machine, in your infrastructure, with your data. No telemetry, no mandatory cloud calls. Models can be local (Ollama) or remote — your choice per session.

This matters for privacy, for cost control, and for reliability. A runtime that works offline is more trustworthy than one that depends on a SaaS.

### 2. Provider-agnostic

15 providers are supported today. A session can switch models mid-run. If a provider fails, the engine falls back automatically. Skills and sessions are portable across providers.

Lock-in is a design flaw. The runtime should not care which company made the model.

### 3. Production-grade by default

The runtime includes, by default, the things you need in production:
- Permission engine with configurable modes per session
- Bash sandboxing via Linux Landlock (filesystem isolation at the kernel level)
- Automatic context compaction when approaching the model's window
- Retry with exponential backoff, circuit breakers, and fallback routing
- Session persistence with full transcript and checkpoint recovery
- Prometheus metrics and OpenTelemetry tracing

These are not optional add-ons. They are the baseline.

### 4. Extensible without forking

The runtime is extended through well-defined interfaces, not by modifying internals:
- **Tools** — implement the `Tool` interface and register it; the engine handles permissions, concurrency, and streaming automatically
- **Skills** — Markdown files injected into the system prompt; no code needed
- **MCP servers** — plug in any MCP server at startup or runtime; the engine exposes all its tools to the agent
- **Stop hooks** — post-turn policy checks that can append messages or request continuation
- **Credential resolvers** — inject per-request API key resolution without touching the core

### 5. Written in Go — intentionally

Go was chosen for specific reasons:
- **Single binary deployment** — no runtime, no package manager, no venv
- **<100ms startup** — critical for CLI use and per-request server scenarios
- **Native concurrency** — tool calls execute in parallel goroutines; streaming and permissions run concurrently without blocking
- **Cross-platform** — one codebase compiles to Linux, macOS, Windows without CGO
- **gRPC-first** — Go's gRPC implementation is production-grade; the proto file generates stubs for every major language

The choice of Go is a commitment to the runtime being lightweight and deployable, not a statement about other languages.

---

## What makes a good agent runtime

A capable agent runtime needs to handle five things well:

**1. Tool execution** — Tools run with correct permissions, in the right concurrency model, with proper error recovery and progress reporting. The agent should be able to call 30 tools in parallel if needed, with each tool checking permissions independently.

**2. Context management** — LLMs have finite context windows. The runtime must track token usage, compact the transcript when needed, and preserve the structural integrity of the conversation (tool-use pairs cannot be split). This is harder than it sounds.

**3. Permission governance** — An agent touching real files, running real commands, and calling real APIs needs explicit permission boundaries. The runtime enforces these at the tool level, not at the application level, so every consumer gets the same safety guarantees.

**4. Session continuity** — Agentic tasks take time. The session must survive process restarts, network failures, and long pauses. Checkpointing, transcript persistence, and session resumption are not optional for production use.

**5. Observability** — When an agent does something unexpected, you need to understand why. Structured runtime events, turn-level tracing, tool call logs, and token usage metrics are the minimum required to debug and improve agentic behavior.

Nexus Engine is designed around these five requirements. Every architectural decision traces back to making one of them work correctly under real conditions.

---

## What nexus-engine is not

**Not a framework.** You don't build your agent logic inside nexus-engine. You call nexus-engine from your code (SDK), from your service (gRPC), or from your terminal (CLI). The runtime handles execution; your code handles intent.

**Not a product.** nexus-engine has no concept of users, organizations, billing, or access control. Those belong in a product layer built on top.

**Not a Python/TypeScript library.** nexus-engine is a Go runtime exposed via interfaces. Python and TypeScript consumers call it through gRPC, not by importing it.

**Not a chatbot.** nexus-engine is not optimized for single-turn Q&A. It is optimized for multi-turn agentic sessions where the model uses tools to accomplish a goal.

**Not complete.** The runtime is in active development. Some capabilities (autonomous long-running agents, team coordination) are future work. The current focus is making the single-agent, single-session path robust enough for daily production use.
