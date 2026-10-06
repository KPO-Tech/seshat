# Transports & Setup

This document covers the gRPC server, proto codegen, and environment configuration for seshat.

> The HTTP REST/SSE API (`cmd/api`) is part of the products built on seshat — **[SeshatOS](https://github.com/KPO-Tech/SeshatOS)** (local backend) and **[SeshatCloud](https://github.com/KPO-Tech/SeshatCloud)** (multi-tenant server) — not seshat itself. See their documentation for that surface.

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
SESHAT_MODEL=anthropic:claude-sonnet-4-6   # default model (provider:model)
SESHAT_API_KEY=...                          # alternative to provider-specific key
SESHAT_CWD=/path/to/working/directory      # agent working directory
SESHAT_DB_PATH=/tmp/seshat/seshat.sqlite     # SQLite database path
SESHAT_DEBUG=true                          # verbose logging
SESHAT_PROVIDER_BASE_URL=...               # custom provider base URL
WEB_SEARCH_PROVIDER=tavily                # web search provider
```

The config loader reads, in order: environment variables → `.env` in the current directory → `~/.seshat.yaml`.

---

## gRPC server

### Start

```bash
go run ./cmd/grpc   # 127.0.0.1:50051
# or
make build-grpc && ./bin/seshat-grpc
```

### Security and configuration

The server runs an agent for whoever can call it, with the provider keys of the host, and `ConnectMCP` can start a process. It is closed by default:

| Variable | Default | Effect |
|---|---|---|
| `SESHAT_GRPC_HOST` | `127.0.0.1` | Address to listen on. Anything but the loopback (`0.0.0.0`, a LAN address) needs the token below, or `SESHAT_GRPC_ALLOW_INSECURE_REMOTE=true`: the server refuses to start otherwise. In a container, set `SESHAT_GRPC_HOST=0.0.0.0` and a token. |
| `SESHAT_GRPC_PORT` | `50051` | Port. |
| `SESHAT_GRPC_AUTH_TOKEN` | empty | When set, every call but `HealthCheck` must carry the metadata `authorization: Bearer <token>`, or fail with `Unauthenticated`. |
| `SESHAT_GRPC_TLS_CERT`, `SESHAT_GRPC_TLS_KEY` | empty | Turn TLS on (both or neither). The token is a secret: over a network, send it only over TLS (the server warns when it is not). |
| `SESHAT_GRPC_ALLOW_INSECURE_REMOTE` | `false` | Accept a non-loopback address with no token, because the network in front of the server is the protection. |
| `SESHAT_GRPC_ALLOW_STDIO_MCP` | `false` | Let `ConnectMCP` start a stdio MCP server, which is a command line (`command`, `args`, `env`) run on the host. Refused with `PermissionDenied` otherwise; `http`, `sse` and `ws` servers are not affected. |
| `SESHAT_GRPC_ENABLE_REFLECTION` | `false` | Server reflection. |
| `SESHAT_GRPC_MAX_CONCURRENT_RPCS`, `SESHAT_GRPC_KEEPALIVE_TIME` | `10`, `30s` | Limits. |

With a token, a client passes it as metadata: `grpcurl -H "authorization: Bearer $TOKEN" ...`, or in Go `metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)`.

### Service: `seshat.SeshatService`

Defined in `pkg/grpc/proto/seshat.proto`.

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

> Note: the gRPC surface is experimental and partial: `SeshatService` only. `FileService` and `SystemService` were removed from the `.proto` (they were never implemented). `QueryRequest.stream` and `QueryRequest.temperature` are deprecated and ignored by the server; use `QueryStream` for streaming.

> Note: server reflection is not enabled. Provide the `.proto` file explicitly when using `grpcurl`.

> Note: authentication is one shared token (see above), not per-user: put the server behind your own gateway if callers must be told apart.

### Query (unary)

```bash
grpcurl \
  -plaintext \
  -import-path pkg/grpc/proto \
  -proto seshat.proto \
  -d '{"prompt":"hello","model":"anthropic:claude-sonnet-4-6"}' \
  localhost:50051 seshat.SeshatService/Query
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
  -proto seshat.proto \
  -d '{"prompt":"hello","model":"anthropic:claude-sonnet-4-6"}' \
  localhost:50051 seshat.SeshatService/QueryStream
```

### Other methods

```bash
# Health check
grpcurl -plaintext -import-path pkg/grpc/proto -proto seshat.proto \
  -d '{}' localhost:50051 seshat.SeshatService/HealthCheck

# Available models
grpcurl -plaintext -import-path pkg/grpc/proto -proto seshat.proto \
  -d '{}' localhost:50051 seshat.SeshatService/GetModels

# List skills
grpcurl -plaintext -import-path pkg/grpc/proto -proto seshat.proto \
  -d '{}' localhost:50051 seshat.SeshatService/ListSkills
```

---

## Proto codegen

After any modification to `pkg/grpc/proto/seshat.proto`, regenerate the Go stubs:

```bash
PATH="$HOME/go/bin:$HOME/.local/bin:$PATH" \
protoc \
  --proto_path=pkg/grpc/proto \
  --go_out=. \
  --go_opt=module=github.com/KPO-Tech/seshat \
  --go-grpc_out=. \
  --go-grpc_opt=module=github.com/KPO-Tech/seshat \
  pkg/grpc/proto/seshat.proto
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
