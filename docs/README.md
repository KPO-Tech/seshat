# Nexus Engine — Documentation

Nexus Engine is a headless AI coding runtime written in Go. It orchestrates multi-turn LLM conversations with tool use, session persistence, streaming, and multi-provider routing.

## Contents

| Document | What it covers |
|---|---|
| [Architecture](./architecture.md) | Full system design, layer diagrams, data flows |
| [Prompt System](./prompt-system.md) | Section assembly, stage overlays, caching, tool hints |
| [Tool System](./tools.md) | Contract interface, built-in tools, permission pipeline |
| [Providers](./providers.md) | Multi-provider routing, retry, circuit breaker, streaming |
| [SDK Guide](./sdk.md) | Go SDK usage, ClientConfig, sessions, callbacks |
| [Transports And Setup](./transports.md) | Installation, env, HTTP API, gRPC, protobuf, curl/grpcurl |

## Quick orientation

```
cmd/api        HTTP REST + SSE server (port 8090)
cmd/cli        Interactive terminal chat
cmd/grpc       gRPC server (port 50051)

internal/
  engine/      Session lifecycle, main query loop
  execution/   Tool orchestration, EventQueue
  prompt/      System prompt assembly
  providers/   LLM clients (Anthropic, Bedrock, Vertex, …)
  tools/       30+ built-in tools
  permissions/ Permission engine and auto-mode classifier
  memory/      Long-term memory (project / user / cross-session)
  modes/       Execution modes (plan, execute, browse, pair)
  runtime/     Compaction engine, state persistence
  hooks/       Lifecycle hook system
  monitoring/  Prometheus metrics
  db/          SQLite schema and session store
  auth/        OAuth and identity

pkg/
  sdk/         Public Go SDK
  mcp/         MCP protocol wrapper
  skills/      Skills system
  config/      Config loader
  grpc/        gRPC proto bindings
```

## Build

```bash
go build ./cmd/api   # HTTP server
go build ./cmd/cli   # CLI
go build ./cmd/grpc  # gRPC server
go test ./...        # Run all tests
```
