# Transports & Setup

This document covers the gRPC server, proto codegen, and environment configuration for nexus-engine.

> The HTTP REST/SSE API (`cmd/api`) is part of **nexus-product**, not nexus-engine. See nexus-product documentation for that surface.

---

## Prerequisites

### Go

The module requires Go 1.25+. Check your version:

```bash
go version
```

### Proto codegen (only needed if modifying the .proto file)

```bash
# Check tools
protoc --version
which protoc-gen-go
which protoc-gen-go-grpc

# Install protoc (Linux)
mkdir -p "$HOME/.local"
curl -LO https://github.com/protocolbuffers/protobuf/releases/download/v34.1/protoc-34.1-linux-x86_64.zip
unzip -o protoc-34.1-linux-x86_64.zip -d "$HOME/.local"

# Install Go plugins
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

export PATH="$HOME/.local/bin:$(go env GOPATH)/bin:$PATH"
```

### grpcurl (for manual testing)

```bash
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
```

---

## Environment variables

### Provider credentials

```bash
ANTHROPIC_API_KEY=sk-ant-...
OPENAI_API_KEY=sk-...
GOOGLE_API_KEY=...
OLLAMA_API_KEY=...            # optional, only needed if Ollama requires auth
```

### Runtime configuration

```bash
NEXUS_MODEL=anthropic:claude-sonnet-4-6   # default model (provider:model)
NEXUS_API_KEY=...                          # alternative to provider-specific key
NEXUS_CWD=/path/to/working/directory      # agent working directory
NEXUS_DB_PATH=/tmp/nexus/nexus.sqlite     # SQLite database path
NEXUS_DEBUG=true                          # verbose logging
NEXUS_PROVIDER_BASE_URL=...               # custom provider base URL
WEB_SEARCH_PROVIDER=tavily                # web search provider
```

The config loader reads, in order: environment variables → `.env` in the current directory → `~/.nexus.yaml`.

---

## gRPC server

### Start

```bash
go run ./cmd/grpc   # port 50051
# or
make build-grpc && ./bin/nexus-engine-grpc
```

### Service: `nexus.NexusService`

Defined in `pkg/grpc/proto/nexus.proto`.

| Method | Type | Description |
|---|---|---|
| `Query` | unary | Single-turn query, returns when complete |
| `QueryStream` | server-streaming | Streaming response with chunks + runtime events |
| `ListSkills` | unary | List available skills |
| `GetSkillDetails` | unary | Get a skill's content |
| `ListMCP` | unary | List connected MCP servers |
| `ConnectMCP` | unary | Connect an MCP server |
| `DisconnectMCP` | unary | Disconnect an MCP server |
| `GetModels` | unary | List available models from provider registry |
| `HealthCheck` | unary | Health status |

> Note: `FileService` and `SystemService` are defined in the `.proto` but not yet implemented in `cmd/grpc`.

> Note: server reflection is not enabled. Provide the `.proto` file explicitly when using `grpcurl`.

> Note: `cmd/grpc` has no authentication layer. It is intended for local use or trusted network deployment.

### Query (unary)

```bash
grpcurl \
  -plaintext \
  -import-path pkg/grpc/proto \
  -proto nexus.proto \
  -d '{"prompt":"hello","model":"anthropic:claude-sonnet-4-6"}' \
  localhost:50051 nexus.NexusService/Query
```

### QueryStream (server-streaming)

The stream emits `QueryResponse` messages with an `item_type` field:

| `item_type` | Fields | Description |
|---|---|---|
| `chunk` | `content`, `chunk.type`, `chunk.delta_type`, `chunk.delta` | Text delta |
| `runtime_event` | `runtime_event.type`, `.session_id`, `.turn_number`, `.tool_name`, `.stop_reason`, … | Structured engine event |
| `final` | `conversation_id`, `content`, `token_usage`, `stopped` | Final result |

```bash
grpcurl \
  -plaintext \
  -import-path pkg/grpc/proto \
  -proto nexus.proto \
  -d '{"prompt":"hello","model":"anthropic:claude-sonnet-4-6"}' \
  localhost:50051 nexus.NexusService/QueryStream
```

### Other methods

```bash
# Health check
grpcurl -plaintext -import-path pkg/grpc/proto -proto nexus.proto \
  -d '{}' localhost:50051 nexus.NexusService/HealthCheck

# Available models
grpcurl -plaintext -import-path pkg/grpc/proto -proto nexus.proto \
  -d '{}' localhost:50051 nexus.NexusService/GetModels

# List skills
grpcurl -plaintext -import-path pkg/grpc/proto -proto nexus.proto \
  -d '{}' localhost:50051 nexus.NexusService/ListSkills
```

---

## Proto codegen

After any modification to `pkg/grpc/proto/nexus.proto`, regenerate the Go stubs:

```bash
PATH="$HOME/go/bin:$HOME/.local/bin:$PATH" \
protoc \
  --proto_path=pkg/grpc/proto \
  --go_out=. \
  --go_opt=module=github.com/EngineerProjects/nexus-engine \
  --go-grpc_out=. \
  --go-grpc_opt=module=github.com/EngineerProjects/nexus-engine \
  pkg/grpc/proto/nexus.proto
```

Verify:

```bash
go build ./cmd/grpc
go test ./cmd/grpc/...
go test ./pkg/grpc/...
```

---

## Smoke tests

```bash
# gRPC
go test ./cmd/grpc/...
go test ./pkg/grpc/...

# Full suite
go test ./...
```
