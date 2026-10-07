# Memory and Compaction in Seshat

Seshat is a Go agent runtime built around persistent, multi-turn sessions. Memory is the mechanism that keeps context alive across turns, compresses it when it grows large, and lets agents store durable notes across sessions.

> Full documentation: [seshat-ai.com/docs/concepts/memory-rag](https://seshat-ai.com/docs/concepts/memory-rag)

---

## Three layers of memory

### 1. Session memory (SQLite)

Every conversation is persisted to a local SQLite database. Sessions survive restarts, crashes, and upgrades. You can resume any past session by ID:

```bash
seshat chat --resume <session-id>
seshat chat --continue          # resume the most recent session
seshat sessions list            # browse all sessions
```

The session store holds: message history, tool call results, token usage, metadata (provider, model, timestamps).

### 2. In-context memory (active window)

During a session, the agent works within the model's context window. As the conversation grows, Seshat tracks token usage and compares it against a configurable threshold.

When the context approaches the limit, **compaction** runs automatically.

### 3. Agent memory tool

The memory tools let agents persist notes that survive across sessions:

The memory tools store a small knowledge graph of entities and observations:

```
memory_create_entities   create entities (a person, a project, a decision)
memory_add_observations  add facts to an entity
memory_search_nodes      search what is stored
memory_open_nodes        read specific entities
```

These notes are injected into the system prompt at the start of each new session, giving the agent a form of long-term memory across conversations.

---

## Context compaction

When a session's token count approaches the model's context window limit, Seshat compacts the conversation automatically.

**How it works:**

1. Seshat detects that the active context is above the compaction threshold (default: 85% of the model's window).
2. The oldest messages in the conversation are summarized into a compact representation.
3. The summary replaces the original messages in the active context.
4. The full history remains in SQLite and is never deleted.

The result: sessions can run indefinitely without hitting context limits, while the full history remains queryable.

**Configuration:** in the Go SDK, `ClientConfig.AutoCompact` turns automatic compaction on (it is on in `DefaultClientConfig`). The engine compacts at 85% of the usable window by default and aims for about 50% afterwards.

Manual compaction (from the TUI): planned for a future release once the runtime exposes a manual-compaction hook.

---

## Related docs

- [RAG System](./rag.md) - retrieving external knowledge into the agent's context
- [MCP Client](./mcp.md) - plugging in external memory and knowledge servers
- [SDK Guide](./sdk.md) - configuring memory options programmatically
