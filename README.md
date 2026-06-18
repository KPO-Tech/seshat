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

## 🖥️ Terminal UI

`nexus chat` drops you into a full-featured terminal interface built for long-running agent sessions.

<table>
  <tr>
    <td align="center" width="50%">
      <img src="docs/captures/home.png" alt="Welcome screen" width="100%">
      <br><sub><b>Welcome</b> — up and running in 10 seconds</sub>
    </td>
    <td align="center" width="50%">
      <img src="docs/captures/commands_pannel.png" alt="Settings panel" width="100%">
      <br><sub><b>Settings panel</b> — every shortcut, one keystroke away (<code>ctrl+p</code>)</sub>
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

> **Keyboard shortcuts:** `ctrl+p` settings · `ctrl+m` models · `ctrl+s` sessions · `ctrl+,` providers · `ctrl+n` new session · `ctrl+t` tasks · `ctrl+u` copy last response · `ctrl+y` toggle yolo · `ctrl+o` open editor · `ctrl+g` help · `ctrl+c` quit

> **Clipboard note (Linux):** selection copy works best when `wl-clipboard` (Wayland) or `xclip`/`xsel` (X11) is installed. Without a system clipboard backend, Nexus can request terminal clipboard access but cannot guarantee a real system copy.

> **Compaction note:** transcript compaction is automatic today. A dedicated manual compact action is planned for the TUI once the runtime exposes a real manual-compaction hook.

---

## 🔀 Three Ways to Use It

### 1. 💻 CLI — `nexus`

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
nexus config --provider anthropic --api-key sk-ant-...
nexus config --model anthropic:claude-sonnet-4-20250514

nexus config --print
```

**Run**

```bash
nexus chat                                        # interactive TUI session
nexus chat --resume <session-id>                  # resume a specific session
nexus chat --continue                             # resume the most recent session
nexus run "list all TODO comments in this codebase"  # one-shot task
nexus sessions list                               # browse past sessions
nexus sessions list --status active               # active sessions only
nexus help                                        # full command reference
```

Sessions are persisted locally in SQLite. Skills are loaded from `.nexus/skills/` in your project. The full tool set is available: file edits, sandboxed bash, web search, browser, MCP servers, sub-agents.

---

### 2. 🌐 gRPC Server

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

### 3. 📦 Go SDK

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

## 📊 How Nexus Engine Compares

| Feature | **nexus-engine** | Claude Agent SDK | OpenAI Agents SDK | LangGraph | CrewAI |
|---|:---:|:---:|:---:|:---:|:---:|
| Language | **Go** | Python/TS | Python | Python | Python |
| Single binary (no deps) | ✅ | ❌ | ❌ | ❌ | ❌ |
| CLI included | ✅ | ❌ | ❌ | ❌ | ❌ |
| gRPC server (any language) | ✅ | ❌ | ❌ | ❌ | ❌ |
| Multi-provider | ✅ (15) | ❌ Claude only | ❌ OpenAI only | ✅ | ✅ |
| MCP client | ✅ | ✅ | ✅ | Partial | ❌ |
| Sandboxed bash (Landlock) | ✅ | ✅ | ❌ | ❌ | ❌ |
| Skills system | ✅ | ❌ | ❌ | ❌ | ❌ |
| Built-in RAG | ✅ | ❌ | ❌ | ❌ | ❌ |
| Browser automation | ✅ | ✅ | ❌ | ❌ | ❌ |
| Session persistence | ✅ | ❌ | ❌ | ✅ | ❌ |
| OTel tracing | ✅ | ❌ | ❌ | ✅ | ❌ |
| Open-source license | Apache 2.0 | MIT | MIT | MIT | Apache 2.0 |

---

## ✨ Capabilities

| Capability | Details |
|---|---|
| 🌍 **Multi-provider** | 15 providers: Anthropic, OpenAI, Gemini, Mistral, DeepSeek, Ollama, OpenRouter, AWS Bedrock, GCP Vertex, Azure Foundry, Codex, MiniMax, Z.ai, OpenCode, Cloudflare Workers AI |
| 🛠️ **60+ built-in tools** | File read/write/patch, bash (Landlock sandbox), web search, web fetch, browser (Playwright), grep/glob, LSP, sub-agents, RAG, tasks, memory, worktree, notebooks, image generation, TTS/STT |
| 🔌 **MCP client** | Universal MCP client: plug in any MCP server (GitHub, Postgres, Slack, Docker, Notion, ...) |
| ⚡ **Skills** | Markdown instruction files injected into the system prompt: encode your team's conventions and domain expertise |
| 🎯 **Execution modes** | `execute` (default), `plan` (review before act), `pair_programming` (collaborative) |
| 🔒 **Permission engine** | Per-tool deny rules, auto-mode LLM classifier, configurable per session (`auto` / `acceptEdits` / `onRequest` / `bypass` / `never`) |
| 💾 **Session persistence** | SQLite-backed multi-turn sessions, resumable across restarts |
| 📡 **Streaming** | Text chunks + structured runtime events (tool calls, plan events, permission requests, token usage) |
| 🧠 **Long-context compaction** | Automatic context compression when approaching the model's window (configurable threshold) |
| 📉 **Observability** | Prometheus metrics + OpenTelemetry tracing (OTLP gRPC export, no-op when endpoint not set) |

---

## 🗂️ Repository Structure

```
nexus-engine/
├── cmd/
│   ├── cli/              ← nexus CLI entrypoint (TUI + one-shot commands)
│   └── grpc/             ← gRPC server entrypoint
├── pkg/                  ← public API (safe to import from outside)
│   ├── sdk/              ← Go SDK: Client, sessions, streaming, callbacks
│   ├── types/            ← shared types: Message, ToolUse, TokenUsage, ...
│   ├── agent/            ← agent definitions, built-in registry
│   ├── providers/        ← LLM provider abstraction, routing, fallback
│   ├── mcp/              ← MCP client: stdio, SSE, HTTP transports
│   ├── rag/              ← chunking, embedding, hybrid vector search
│   ├── skills/           ← skill loading, frontmatter parsing, injection
│   ├── memory/           ← in-session state, compaction strategies
│   ├── web/              ← web search, fetch, browser (Playwright)
│   ├── storage/          ← artifact store: S3, local filesystem
│   ├── vector/           ← vector DB abstraction
│   ├── contract/         ← Tool interface, CallResult, registry
│   ├── auth/             ← provider auth abstraction, OAuth device flow
│   ├── workspace/        ← sandbox path resolution, workspace layout
│   ├── monitoring/       ← Prometheus metrics, OTel spans
│   ├── docling/          ← PDF/DOCX/audio conversion via docling-serve
│   ├── grpc/             ← proto definitions and generated code
│   └── config/           ← app-level config from env
└── internal/             ← private implementation (do not import directly)
```

> nexus-ai and any third-party consumer must import `pkg/*` only, never `internal/*`.

---

## 🌐 Supported Providers

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

## 🚀 Quick Start

```bash
# 1. Clone and build
git clone https://github.com/EngineerProjects/nexus-engine
cd nexus-engine
make build                    # produces bin/nexus and bin/nexus-grpc
export PATH="$PATH:$(pwd)/bin"

# 2. Set your API key and model
nexus config --provider anthropic --api-key sk-ant-...
nexus config --model anthropic:claude-sonnet-4-20250514

# 3. Start chatting
nexus chat                          # new session
nexus chat --continue               # resume last session
nexus chat --resume <session-id>    # resume a specific session

# One-shot task in the current directory
nexus run "list all TODO comments in this codebase"

# Start the gRPC server (port 50051)
ANTHROPIC_API_KEY=sk-ant-... ./bin/nexus-grpc
```

> **No API key?** Use Ollama for free local inference:
> `nexus config --provider ollama --model ollama:llama3.2`
> (requires [Ollama](https://ollama.com) running locally)

---

## ⚡ Skills

Skills are Markdown files that encode expertise injected into the agent's system prompt at runtime.

```
.nexus/skills/
  go-conventions.md     # "always use context.Context as the first parameter..."
  git-workflow.md       # "never commit to main, always open a PR, squash before merge..."
  security-rules.md     # "never log secrets, validate all external input at boundaries..."
```

The official skill collection is [nexus-skills](https://github.com/EngineerProjects/nexus-skills), installable from any URL directly from the CLI.

---

## 🔌 MCP

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

## 🏗️ Architecture

<p align="center">
  <img src="docs/images/ideal_vision.png" alt="Nexus Engine Architecture" width="800">
</p>

Full architecture diagrams (Mermaid): [`docs/vision/diagrams.md`](./docs/vision/diagrams.md).

---

## 🌐 The Nexus Ecosystem

nexus-engine is the **headless runtime**: pure Go, no UI, no users, no billing. It is the foundation everything else builds on.

### 🖥️ nexus-ai — Desktop & Platform

**[→ nexus-ai](https://github.com/EngineerProjects/nexus-ai)** is the full production platform built on top of this engine. If you want a ready-to-use application rather than a library, that is where you want to go.

| | nexus-engine (this repo) | nexus-ai |
|---|---|---|
| **What it is** | Go runtime + SDK + CLI | Desktop app + REST API platform |
| **Stack** | Go | Go (API) + TypeScript/React/Electron (desktop) |
| **License** | Apache 2.0 | AGPL-3.0 |
| **Who it's for** | Developers embedding agents in their own apps | End users, teams, self-hosters |
| **Includes** | Engine, tools, providers, gRPC, CLI/TUI | Multi-user auth, workspaces, knowledge base, scheduler, desktop UI |

**What nexus-ai gives you today:**
- 🖥️ Native desktop app (Electron + React) with chat, tool views, plans, settings and a visual skills creator
- 👥 Multi-user backend with organizations, workspaces, per-user API keys, quotas and audit log
- 📡 REST + SSE HTTP API compatible with the Anthropic `/v1/messages` format
- 📚 Knowledge base with hybrid BM25 + vector search and file ingestion
- ⏰ Scheduled tasks, memories, plans and MCP server management

**Coming next:**
- 🤝 Agent teams: persistent groups of specialized agents collaborating on shared missions, each with its own inbox, role and memory
- 🤖 Automation and background workflows triggered by schedule, events or voice
- 🖼️ Image generation integrated directly into the chat and workspace
- 🎙️ Voice input and audio output so you can talk to your agents naturally
- 🌐 A multi-workspace environment covering code, research, creation and learning, all sharing the same runtime and data layer

### 🤝 Contribution split

| If you want to... | Contribute to... |
|---|---|
| Improve execution speed, reduce latency, optimize the agent loop | **nexus-engine** (Go) |
| Add a new LLM provider or tool | **nexus-engine** (Go) |
| Expose new capabilities in the SDK or gRPC API | **nexus-engine** (Go) |
| Improve the desktop UI, add new views, fix UX | **nexus-ai** (TypeScript/React) |
| Build features like agent teams, automation or scheduling | **nexus-ai** (Go API + React) |

The engine is intentionally kept minimal and fast. If you need something from the SDK that is not exposed yet, open an issue and we will prioritize it.

### 📦 Prebuilt binaries (coming soon)

You will soon be able to download a single binary to use the CLI or embed the SDK in your application without building from source. Watch this repo for releases.

---

## 📖 Documentation

| Doc | What it covers |
|---|---|
| [Vision & Roadmap](./docs/vision/README.md) | Project idea, design principles, Level 1->2->3 roadmap |
| [Architecture](./docs/architecture.md) | System design, layer diagrams, query loop state machine |
| [SDK Guide](./docs/sdk.md) | `ClientConfig`, sessions, streaming, callbacks, MCP |
| [Tools](./docs/tools.md) | Built-in tools reference, permission pipeline |
| [Providers](./docs/providers.md) | Multi-provider routing, retry, circuit breaker |
| [Prompt System](./docs/prompt-system.md) | Section assembly, stage overlays, cache control |
| [Skills](./docs/skills.md) | Skills system, loading order, injection |
| [Transports & Setup](./docs/transports.md) | gRPC setup, proto codegen, env vars |
| [Multi-Agent Teams](./docs/team.md) | Agent profiles, mailbox, dispatcher, TeamBus |

---

## 🛠️ Development

### First-time setup

```bash
# Linux / macOS — installs all dependencies, builds, wires git hooks
make setup

# Windows (PowerShell — make is not available by default on Windows)
powershell -ExecutionPolicy Bypass -File scripts\setup.ps1
```

`make setup` handles everything: Go version check, ripgrep, uv, Python venv with docling-serve, and the final build.

### Daily commands

```bash
make build         # build CLI and gRPC binaries to bin/
make test          # run all tests
make test-race     # run tests with race detector
make lint          # golangci-lint
make fmt           # gofmt
make hooks         # (re-)install git pre-commit hooks
make install-deps  # install ripgrep only (included in make setup)
make install-python  # install/update the Python venv + docling-serve only
make start-docling   # start docling-serve manually (auto-started by nexus chat)
```

### Runtime data directory

Nexus stores sessions, config, and the Python venv under a platform-appropriate directory:

| OS | Default path |
|---|---|
| Linux | `~/.config/nexus-cli/` (respects `$XDG_CONFIG_HOME`) |
| macOS | `~/.config/nexus-cli/` |
| Windows | `%APPDATA%\nexus-cli\` |

Override with `NEXUS_RUNTIME_ROOT=/your/path nexus chat`.

### Runtime dependencies

> **ripgrep** — the `glob` and `grep` tools require [`ripgrep`](https://github.com/BurntSushi/ripgrep) (`rg`). Included in `make setup`; install separately with `make install-deps`.

> **docling-serve** (optional) — enables the `read_document_url` tool for PDF/DOCX conversion. Included in `make setup` via `uv` (no system Python required). Nexus auto-starts it on launch when installed.

### OS compatibility

> **Linux** — primary development and testing platform. Fully supported.
>
> **macOS** — code is written to support macOS and basic testing has been done, but **macOS support is not yet fully validated**. If you hit an issue, please [open a report](https://github.com/EngineerProjects/nexus-engine/issues).
>
> **Windows** — PowerShell setup script included and paths are handled (`%APPDATA%`), but **Windows support has not been tested yet**. Contributions and test reports welcome.

See [`CONTRIBUTING.md`](./CONTRIBUTING.md) for the full contribution guide.

---

## 🔒 Security

To report a vulnerability, see [`SECURITY.md`](./SECURITY.md).

---

## 📄 License

[Apache 2.0](./LICENSE)
