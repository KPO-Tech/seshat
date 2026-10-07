# Seshat — Documentation

Technical documentation for the seshat runtime. For project overview and quick start, see the [root README](../README.md).

The website has the documentation written for users: [installation](https://seshat-ai.com/en/docs/getting-started/installation), [configuration](https://seshat-ai.com/en/docs/getting-started/configuration), the [concepts](https://seshat-ai.com/en/docs/concepts/what-is-seshat) (architecture, memory, security and trust, skills and MCP, RAG), the [Go SDK](https://seshat-ai.com/en/docs/sdk/go-sdk), the [gRPC API](https://seshat-ai.com/en/docs/sdk/grpc-api) and the [CLI guide](https://seshat-ai.com/en/docs/guides/cli), all at [seshat-ai.com/en/docs](https://seshat-ai.com/en/docs). The files in this directory are the engineering notes that stay next to the code.

## Contents

| Document | What it covers |
|---|---|
| [**Vision**](./vision/README.md) | Project idea, design principles, roadmap |
| [Architecture](./architecture.md) | Full system design, layer diagrams, query loop state machine, data flows |
| [Prompt System](./prompt-system.md) | Section assembly, stage overlays, cache control, tool hints |
| [Tools](./tools.md) | Tool contract interface, built-in tools reference, permission pipeline |
| [Providers](./providers.md) | Multi-provider routing, retry, circuit breaker, streaming |
| [SDK Guide](./sdk.md) | Go SDK usage, `ClientConfig`, sessions, callbacks, MCP |
| [Skills](./skills.md) | Skills system, loading order, injection into prompt |
| [Transports & Setup](./transports.md) | gRPC setup, proto codegen, env vars |
| [Licensing and provenance](./licensing.md) | Apache-2.0, and the Crush origin of the terminal UI |
| [Project trust](./project-trust.md) | what a project's `.seshat.json` may do in the terminal UI, and `seshat trust` |
| [Permissions And Sandbox Roadmap](./issues/permissions-and-sandbox-roadmap.md) | Permission audit fixes and the planned Docker sandbox strategy |

## Quick orientation

```
cmd/
  cli/       Interactive terminal agent (seshat binary)
  grpc/      gRPC server (port 50051)

pkg/
  sdk/       Public Go SDK — main consumer entry point
  grpc/      gRPC proto bindings (generated stubs + .proto)
  mcp/       MCP protocol wrapper
  skills/    Skills loading and resolution
  config/    Config loader (env, .env, .seshat.yaml)

internal/
  engine/      Session lifecycle, main query loop
  execution/   Tool orchestration, EventQueue, streaming
  prompt/      System prompt assembly (4-layer pipeline)
  providers/   LLM clients (Anthropic, OpenAI, Gemini, Ollama, …)
  tools/       30+ built-in tool implementations
  permissions/ Permission engine and auto-mode LLM classifier
  memory/      Long-term memory (project / user / cross-session)
  modes/       Execution modes (plan, execute, browse, pair)
  runtime/     Compaction engine, session state persistence
  hooks/       Lifecycle hook system
  monitoring/  Prometheus metrics
  db/          SQLite schema and session store
  auth/        OAuth device flow and credential store (CLI-era)
  web/         Browser (Playwright), fetch, search providers
  storage/     ArtifactStore (local + S3)
  vector/      Vector store (memory + SQLite)
  rag/         RAG ingestion and search primitives
```

## Build

```bash
make build          # builds CLI + gRPC → bin/
go build ./cmd/cli
go build ./cmd/grpc
go test ./...
```
