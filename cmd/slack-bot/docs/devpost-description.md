# Seshat for Slack — DevPost Submission Description

## Inspiration

Most AI assistants in Slack are glorified chatbots — you ask a question, you get an answer, and that's it. We wanted to build something fundamentally different: a **real autonomous agent** that can *do* things, not just respond. An agent that searches your workspace, browses the web, writes and runs code, remembers context across conversations, and delivers actual files and reports — all inside a Slack thread.

## What it does

**Seshat for Slack** is a fully autonomous AI agent integrated natively into Slack via Socket Mode. It brings the complete power of the [Seshat](https://github.com/EngineerProjects/seshat) runtime directly into your workspace.

**Core capabilities:**
- **Slack workspace search** — Uses Slack's Real-Time Search API (`assistant.search.context`) to find messages, files, and channels across your organization. Ask Seshat to find a document, recall a past decision, or summarize what was discussed in a channel.
- **MCP server integration** — Connects to MCP servers (LinkedIn, SearXNG, and any custom server via `mcp.json`) to extend its tool surface without code changes.
- **Web research** — Searches the web via Tavily, LangSearch, and SearXNG, then synthesizes results into structured answers.
- **File & code work** — Creates files, writes and executes code, produces reports, and uploads the results directly to the Slack thread as attachments.
- **Browser automation** — Can navigate websites, extract structured data, and take screenshots.
- **Long-term memory** — Remembers facts about your team, projects, and preferences across sessions using a persistent SQLite store.
- **Interactive questions** — When the agent needs clarification, it posts Block Kit buttons (choice/confirm) or waits for a thread reply — no stdin, no blocking.
- **Streaming responses** — Replies update in real time with live tool-progress indicators (`web_search running...`, `writing file...`).

## How we built it

Seshat for Slack is built on top of the open-source **Seshat** (`seshat`), a Go runtime for autonomous AI agents with 60+ built-in tools, 15 LLM provider adapters, a permission engine, session persistence, and an MCP client.

**Architecture:**
- **Transport**: Slack Socket Mode (WebSocket) — no public endpoint, no webhooks, works behind a firewall.
- **Session model**: One persistent Seshat session per Slack channel. Context accumulates across messages. Sessions survive bot restarts via SQLite.
- **Real-Time Search API**: A custom `slack_search` tool registered with the Seshat agent calls `assistant.search.context` to search the workspace in real time. Results include message context, file metadata, and channel info.
- **MCP integration**: The bot reads `~/.config/seshat-slack/mcp.json` at startup and connects to any configured MCP server (stdio, HTTP, SSE, WebSocket transports). Falls back to the CLI's `mcp.json` for shared servers.
- **Per-session workspace**: Each session gets an isolated `sessions/{id}/workspace/` directory. After each turn, newly created files, generated images, and TTS audio are automatically uploaded to the Slack thread.
- **Runtime isolation**: The bot runs under `~/.config/seshat-slack/` — completely separate from other Seshat clients. Memory, sessions, and artifacts are co-located per session.

**Stack**: Go 1.24, slack-go v0.26.0, SQLite, Seshat SDK, GLM-4.5 (z-ai).

## Challenges we ran into

- **`assistant.search.context` scope matrix**: The Real-Time Search API requires different scopes depending on whether you use a bot token or user token. Bot tokens need an `action_token` from AI assistant thread events; user tokens need `search:read.public` (distinct from `search:read`). We implemented a fallback strategy and clear documentation.
- **Streaming in Slack**: Slack doesn't support true server-sent events in messages. We implemented a polling loop that updates a single placeholder message every 1.5s with the accumulated response and live tool status, staying within Slack's rate limits.
- **`ask_user_question` without stdin**: The Seshat engine normally prompts via stdin. For the autonomous Slack context, we implemented a `PromptFn` hook that routes choice/confirm prompts to Block Kit buttons and text prompts to thread replies — with 5-minute timeouts.
- **Artifact filtering**: Not all generated files should be uploaded (screenshots, logs, cache). We scan only the session workspace and `artifacts/images` + `artifacts/audio` directories for files modified during the current turn.

## Accomplishments

- A production-ready Slack agent backed by 60+ tools and full MCP client support
- Real-Time Search API fully integrated as a first-class Seshat tool
- Zero public endpoint required — runs entirely via Socket Mode
- Per-session workspace with automatic artifact delivery to Slack threads
- Clean runtime isolation that makes it trivially deployable as a server-side service

## What's next

- **AI assistant thread support**: Handle `assistant_thread_started` events to get native `action_token` and unlock private channel search
- **Proactive notifications**: Let the agent send scheduled reports, monitor feeds, and alert on conditions
- **Multi-workspace**: Per-workspace credential isolation for team deployments
- **Seshat AI**: Managed cloud hosting with team workspaces, governance, and a skill marketplace

## Built with

`Go` · `Slack Socket Mode` · `Slack Real-Time Search API (assistant.search.context)` · `MCP (Model Context Protocol)` · `SQLite` · `GLM-4.5 (z-ai)` · `Seshat (open source)`
