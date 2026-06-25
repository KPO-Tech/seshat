# seshat-slack-bot

Slack bot that brings the full Seshat agent runtime into a Slack workspace.

## What it does

- Listens for `@Seshat` mentions in channels and direct messages via Socket Mode
- Routes each message to a persistent Seshat session (one session per Slack channel)
- The agent has access to all built-in Seshat tools: web search, file ops, browser, math, notebooks, memory, RAG, sub-agents, and any connected MCP server
- Posts a "thinking..." placeholder immediately, then updates it with the final response
- Sessions are persisted in SQLite; context is remembered across restarts

## Architecture

```
Slack Workspace
  └── @Seshat mention or DM
        ↓ Socket Mode (WebSocket)
  cmd/slack-bot/main.go
        ↓ pkg/sdk
  Seshat Session
        ├── Web search (Tavily, DuckDuckGo, Exa...)
        ├── Browser automation
        ├── Memory + RAG
        ├── Sub-agents (spawn_agent, wait_agent)
        └── MCP servers (slack-mcp-server → read Slack history/channels)
        ↓ slack-go PostMessage / UpdateMessage
  Slack thread reply
```

## Required env vars

| Variable | Description |
|---|---|
| `NEXUS_SLACK_BOT_TOKEN` | `xoxb-...` - Bot User OAuth Token from api.slack.com (Install App) |
| `NEXUS_SLACK_APP_TOKEN` | `xapp-...` - App-Level Token with `connections:write` scope (Socket Mode) |

All other config (LLM provider, API keys, search backends) is loaded from the standard Seshat env vars. See `private/.env.dev`.

## Running

```bash
# Development (loads private/.env.dev automatically)
make slack-bot

# Production
NEXUS_SLACK_BOT_TOKEN=xoxb-... NEXUS_SLACK_APP_TOKEN=xapp-... ./bin/seshat-slack
```

## Model selection

The bot defaults to `mistral:mistral-small-latest` (free, good for testing).

To switch models, set `NEXUS_MODEL` in your env:

```bash
# Use Claude via OpenRouter (recommended for the demo)
NEXUS_MODEL=openrouter:anthropic/claude-sonnet-4-5

# Use Mistral (free)
NEXUS_MODEL=mistral:mistral-small-latest

# Use any OpenRouter model
NEXUS_MODEL=openrouter:google/gemini-2.0-flash-exp
```

## Session persistence

Sessions are stored in `~/.config/seshat-slack/sessions.db` by default.
Override with `NEXUS_SLACK_DB_PATH=/path/to/sessions.db`.

One session per Slack channel. Context is maintained across messages — the agent
remembers the full conversation history within a channel.

## Adding the bot to a Slack channel

After starting the bot:
1. Go to the channel in Slack
2. Type `/invite @Seshat`
3. Write `@Seshat your question`

Or in a DM: open a direct message with Seshat and write directly.

## Hackathon context

This bot is the Slack surface for the **Seshat for Slack** submission to the
Slack Agent Builder Challenge (devpost.com, deadline July 13 2026).

The goal: demonstrate that a full agent runtime (60+ tools, multi-provider LLM,
multi-agent coordination, persistent memory) can be surfaced inside Slack in a
way that is actually useful for teams — not just a chatbot wrapper.

Planned modes:
- **Ask mode** (current): @Seshat answers questions with web search + Slack context
- **Mission mode** (next): delegate complex tasks to a team of specialized agents
- **Briefing mode** (bonus): scheduled reports posted to channels
