<p align="center">
  <img src="docs/images/nexus.png" alt="Nexus Engine" width="120">
</p>

<h1 align="center">Nexus Engine</h1>

<p align="center">
  <b>Open-source Go runtime for AI agents</b><br>
  <i>One runtime. Any LLM. Any language. Any deployment.</i>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Status-Active%20Development-orange?style=for-the-badge">
  <img src="https://img.shields.io/badge/Go-1.25%2B-00ADD8?style=for-the-badge&logo=go">
  <img src="https://img.shields.io/badge/License-Apache%202.0-brightgreen?style=for-the-badge">
  <img src="https://img.shields.io/badge/Providers-15-4F6EF7?style=for-the-badge">
  <img src="https://img.shields.io/badge/Tools-60%2B-A855F7?style=for-the-badge">
</p>

---

## Terminal UI

`nexus chat` drops you into a full-featured terminal interface built for long-running agent sessions.

<table>
  <tr>
    <td align="center" width="50%">
      <img src="docs/captures/home.png" alt="Welcome screen" width="100%">
      <br><sub><b>Welcome</b> — up and running in 10 seconds</sub>
    </td>
    <td align="center" width="50%">
      <img src="docs/captures/commands_pannel.png" alt="Commands palette" width="100%">
      <br><sub><b>Commands palette</b> — every shortcut, one keystroke away (<code>ctrl+p</code>)</sub>
    </td>
  </tr>
  <tr>
    <td align="center" width="50%">
      <img src="docs/captures/model_selction.png" alt="Model selection" width="100%">
      <br><sub><b>Model selection</b> — switch across 20+ models in 2 keystrokes</sub>
    </td>
    <td align="center" width="50%">
      <img src="docs/captures/provider_config.png" alt="Provider configuration" width="100%">
      <br><sub><b>Provider config</b> — API keys encrypted, scoped per provider</sub>
    </td>
  </tr>
  <tr>
    <td align="center" width="50%">
      <img src="docs/captures/working1.png" alt="Agent working" width="100%">
      <br><sub><b>Agent at work</b> — full reasoning trace, thinking blocks + tool calls</sub>
    </td>
    <td align="center" width="50%">
      <img src="docs/captures/working2.png" alt="Agent completing task" width="100%">
      <br><sub><b>Streaming results</b> — streamed responses with markdown + tool timings</sub>
    </td>
  </tr>
</table>

> **Keyboard shortcuts:** `ctrl+p` commands · `ctrl+m` model · `ctrl+s` sessions · `ctrl+,` provider config · `ctrl+e` select mode · `ctrl+c` cancel/quit

---

## Three ways to use it

### 1. CLI — `nexus`

An AI agent in your terminal. Multi-provider, local-first, skills-aware.

**Build from source**

```bash
git clone https://github.com/EngineerProjects/nexus-engine
cd nexus-engine
make build           # produces bin/nexus and bin/nexus-grpc
```

Add `bin/` to your PATH, or copy `bin/nexus` to `/usr/local/bin`:

```bash
export PATH="$PATH:$(pwd)/bin"
# or
sudo cp bin/nexus /usr/local/bin/nexus
```

**Configure a provider**

```bash
# Set your API key and default model
nexus config --provider anthropic --api-key sk-ant-...
nexus config --model anthropic:claude-sonnet-4-20250514

# Check current config
nexus config --print
```

**Run**

```bash
nexus chat                                   # interactive TUI session
nexus run "list all TODO comments in this codebase"  # one-shot task
nexus sessions list                          # browse past sessions
nexus sessions list --status active          # active sessions only
nexus help                                   # full command reference
```

Sessions are persisted locally in SQLite. Skills are loaded from `.nexus/skills/` in your project. The full tool set is available — file edits, sandboxed bash, web search, browser, MCP servers, sub-agents.

---

### 2. gRPC server

Run nexus-engine as a gRPC service and generate clients for any language.

```bash
# Development
ANTHROPIC_API_KEY=sk-ant-... go run ./cmd/grpc

# From build
ANTHROPIC_API_KEY=sk-ant-... ./bin/nexus-grpc
```

Server starts on `:50051`. The contract lives in `pkg/grpc/proto/nexus.proto`. Generate a client for Python, TypeScript, Java, Rust, or any gRPC-supported language:

```bash
# Python
python -m grpc_tools.protoc -I pkg/grpc/proto --python_out=. --grpc_python_out=. nexus.proto

# TypeScript
npx grpc-tools --js_out=. --grpc_out=. pkg/grpc/proto/nexus.proto
```

One runtime. Every language.

---

### 3. Go SDK

Embed the full runtime in your own Go application.

```bash
go get github.com/EngineerProjects/nexus-engine/pkg/sdk
```

```go
import "github.com/EngineerProjects/nexus-engine/pkg/sdk"

client, err := sdk.NewClient(&sdk.ClientConfig{
    APIKey: os.Getenv("ANTHROPIC_API_KEY"),
    Model:  sdk.ModelIdentifier{Provider: "anthropic", Model: "claude-sonnet-4-20250514"},
})
if err != nil {
    log.Fatal(err)
}
defer client.Close()

session, _ := client.CreateSession(ctx)
resp, _ := session.SubmitMessage(ctx, "Write a Go HTTP handler for /health")
fmt.Println(resp.Content)
```

---

## Capabilities

| Capability | Details |
|---|---|
| **Multi-provider** | 15 providers: Anthropic, OpenAI, Gemini, Mistral, DeepSeek, Ollama, OpenRouter, AWS Bedrock, GCP Vertex, Azure Foundry, Codex, MiniMax, Z.ai, OpenCode, Cloudflare Workers AI |
| **60+ built-in tools** | File read/write/patch, bash (Landlock sandbox on Linux), web search, web fetch, browser (Playwright), grep/glob, LSP, sub-agents, RAG, tasks, memory, worktree, notebooks, image generation, TTS/STT |
| **MCP client** | Universal MCP client — plug in any MCP server (GitHub, Postgres, Slack, Docker, Notion, …) |
| **Skills** | Markdown instruction files injected into the system prompt — encode your team's conventions and domain expertise |
| **Execution modes** | `execute` (default), `plan` (review before act), `pair_programming` (collaborative) |
| **Permission engine** | Per-tool deny rules, auto-mode LLM classifier, configurable per session (`auto` / `acceptEdits` / `onRequest` / `bypass` / `never`) |
| **Session persistence** | SQLite-backed multi-turn sessions, resumable across restarts |
| **Streaming** | Text chunks + structured runtime events (tool calls, plan events, permission requests, token usage) |
| **Long-context compaction** | Automatic context compression when approaching the model's window (configurable threshold) |
| **Observability** | Prometheus metrics + OpenTelemetry tracing (OTLP gRPC export, no-op when endpoint not set) |

---

## Supported providers

| Provider ID | Service | Auth |
|---|---|---|
| `anthropic` | Anthropic | `ANTHROPIC_API_KEY` |
| `openai` | OpenAI | `OPENAI_API_KEY` |
| `gemini` | Google Gemini | `GOOGLE_API_KEY` |
| `mistral` | Mistral AI | `MISTRAL_API_KEY` |
| `deepseek` | DeepSeek | `DEEPSEEK_API_KEY` |
| `ollama` | Ollama (local) | none |
| `openrouter` | OpenRouter | `OPENROUTER_API_KEY` |
| `bedrock` | AWS Bedrock | `AWS_ACCESS_KEY_ID` + region |
| `vertex` | GCP Vertex AI | `ANTHROPIC_VERTEX_PROJECT_ID` + region |
| `foundry` | Azure AI Foundry | `ANTHROPIC_FOUNDRY_API_KEY` |
| `codex` | ChatGPT Pro (OAuth) | device-code flow |
| `minimax` | MiniMax | `MINIMAX_API_KEY` |
| `z-ai` | Z.ai | `Z_AI_API_KEY` |
| `opencode` | OpenCode Zen | `OPENCODE_API_KEY` |
| `workers-ai` | Cloudflare Workers AI | `CLOUDFLARE_API_KEY` |

Full model listings and capabilities: [`docs/providers.md`](./docs/providers.md).

---

## Quick start

```bash
# 1. Clone and build
git clone https://github.com/EngineerProjects/nexus-engine
cd nexus-engine
make build                    # → bin/nexus  and  bin/nexus-grpc
export PATH="$PATH:$(pwd)/bin"

# 2. Set your API key and model
nexus config --provider anthropic --api-key sk-ant-...
nexus config --model anthropic:claude-sonnet-4-20250514

# 3. Start chatting
nexus chat

# One-shot task in the current directory
nexus run "list all TODO comments in this codebase"

# Start the gRPC server (port 50051)
ANTHROPIC_API_KEY=sk-ant-... ./bin/nexus-grpc
```

> **No API key?** Use Ollama for free local inference:
> `nexus config --provider ollama --model ollama:llama3.2`
> (requires [Ollama](https://ollama.com) running locally)

---

## Skills

Skills are Markdown files that encode expertise injected into the agent's system prompt at runtime.

```
.nexus/skills/
  go-conventions.md     # "always use context.Context as the first parameter..."
  git-workflow.md       # "never commit to main, always open a PR, squash before merge..."
  security-rules.md     # "never log secrets, validate all external input at boundaries..."
```

---

## MCP

Any MCP server is immediately usable — no additional development needed.

```go
client, _ := sdk.NewClient(&sdk.ClientConfig{
    MCPServers: []sdk.MCPServerConfig{
        {Name: "github",   Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-github"}},
        {Name: "postgres", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-postgres", "postgresql://..."}},
        {Name: "slack",    Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-slack"}},
    },
})
```

---

## Architecture

<p align="center">
  <img src="docs/images/ideal_vision.png" alt="Nexus Engine Architecture" width="800">
</p>

Full architecture diagrams (Mermaid): [`docs/vision/diagrams.md`](./docs/vision/diagrams.md).

---

## Building a product on top

nexus-engine is the open-source core runtime — no users, no billing, no access control.

If you need multi-user auth, organizations, workspaces, per-user provider credentials, and a REST/SSE HTTP API, those live in **nexus-product** — a private product layer built on top of this engine.

---

## Documentation

| Doc | What it covers |
|---|---|
| [Vision & Roadmap](./docs/vision/README.md) | Project idea, design principles, Level 1→2→3 roadmap |
| [Architecture](./docs/architecture.md) | System design, layer diagrams, query loop state machine |
| [SDK Guide](./docs/sdk.md) | `ClientConfig`, sessions, streaming, callbacks, MCP |
| [Tools](./docs/tools.md) | Built-in tools reference, permission pipeline |
| [Providers](./docs/providers.md) | Multi-provider routing, retry, circuit breaker |
| [Prompt System](./docs/prompt-system.md) | Section assembly, stage overlays, cache control |
| [Skills](./docs/skills.md) | Skills system, loading order, injection |
| [Transports & Setup](./docs/transports.md) | gRPC setup, proto codegen, env vars |
| [Multi-Agent Teams](./docs/team.md) | Agent profiles, mailbox, dispatcher, TeamBus |

---

## Development

```bash
make build       # build CLI and gRPC binaries → bin/
make test        # run all tests
make test-race   # run tests with race detector
make lint        # golangci-lint
make hooks       # install git pre-commit hooks (run once after cloning)
```

See [`CONTRIBUTING.md`](./CONTRIBUTING.md) for the full contribution guide.

---

## Security

To report a vulnerability, see [`SECURITY.md`](./SECURITY.md).

---

## License

[Apache 2.0](./LICENSE)
