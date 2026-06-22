# Quickstart

Get Seshat running in under 5 minutes.

---

## Prerequisites

- **Go 1.25+** — `go version`
- **At least one provider API key** (see table below)
- Optional: `ripgrep` for the grep tool, `make`, `golangci-lint` for development

---

## 1. Clone and build

```bash
git clone https://github.com/EngineerProjects/seshat
cd seshat
make build
```

This produces two binaries in `bin/`:

| Binary | Purpose |
|---|---|
| `bin/seshat` | Interactive CLI — chat, run tools, manage sessions |
| `bin/seshat-grpc` | gRPC server — embed in larger systems |

---

## 2. Configure a provider

Set at least one of these environment variables:

| Provider | Env var | Notes |
|---|---|---|
| Anthropic | `ANTHROPIC_API_KEY` | Claude models — recommended default |
| OpenAI | `OPENAI_API_KEY` | GPT models + DALL·E image generation |
| Mistral | `MISTRAL_API_KEY` | Mistral models |
| OpenRouter | `OPENROUTER_API_KEY` | Access to many models via one key |
| Ollama | *(none)* | Local models — start `ollama serve` first |
| Google Gemini | `GOOGLE_API_KEY` | Gemini models |
| DeepSeek | `DEEPSEEK_API_KEY` | DeepSeek models |
| Z.ai | `Z_AI_API_KEY` | Z.ai models |

The full provider and model reference is in [`docs/providers.md`](./docs/providers.md).

```bash
export ANTHROPIC_API_KEY=sk-ant-...
```

---

## 3. Optional: multimedia tools

These tools are registered automatically but only activate when the matching key is set:

| Tool | Env var |
|---|---|
| `generate_image` | `OPENAI_API_KEY` (uses DALL·E) |
| `text_to_speech` | `OPENAI_API_KEY` (uses TTS endpoint) |
| `speech_to_text` | `OPENAI_API_KEY` (uses Whisper) |

---

## 4. Optional: social tools

| Tool | Env var | Notes |
|---|---|---|
| `devto_publish` | `DEV_TO_API_KEY` | Read tools work without a key |
| `hn_*` | *(none)* | Hacker News — fully public |

---

## 5. First run

```bash
# Start an interactive session
./bin/seshat chat

# Run a one-shot prompt (headless)
./bin/seshat run "Summarise the last 10 commits in this repo"

# Start the gRPC server
./bin/seshat-grpc --port 50051
```

---

## 6. Run as a library (SDK)

```go
import "github.com/EngineerProjects/seshat/pkg/sdk"

client, err := sdk.NewClient(sdk.DefaultClientConfig())
if err != nil { ... }
defer client.Close()

resp, err := client.Query(ctx, "Write a Go function that reverses a string", nil)
```

See [`docs/sdk.md`](./docs/sdk.md) for the full SDK reference.

---

## 7. Notebook tools (optional)

The `notebook_*` tools connect to a live Jupyter kernel. Start Jupyter before using them:

```bash
pip install jupyter
jupyter server --no-browser
```

Then set:

```bash
export JUPYTER_SERVER_URL=http://localhost:8888
export JUPYTER_TOKEN=<token from jupyter output>
```

---

## What's next

- [`docs/tools.md`](./docs/tools.md) — full built-in tool reference
- [`docs/providers.md`](./docs/providers.md) — all supported models and env vars
- [`docs/sdk.md`](./docs/sdk.md) — embedding Seshat in your application
- [`CONTRIBUTING.md`](./CONTRIBUTING.md) — how to add tools, providers, and submit PRs
- [`docs/architecture.md`](./docs/architecture.md) — internal package map
