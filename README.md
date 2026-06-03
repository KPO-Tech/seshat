# Nexus Engine

`nexus-engine` contient le moteur réutilisable et les surfaces publiques consommées par les autres apps.

- moteur et SDK: `pkg/sdk`
- config publique réutilisable: `pkg/config`
- surfaces publiques complémentaires: `pkg/mcp`, `pkg/skills`
- doc transports, API HTTP, gRPC, protobuf et setup: [docs/transports.md](/home/amiche/Projects/AI/ai/nexus_ai/nexus-engine/docs/transports.md)

CLI de dev intégré:

- code: `cmd/cli`
- chat interactif: `go run ./cmd/cli chat`
- exécution one-shot: `go run ./cmd/cli run "prompt"`
- config provider/modèle: `go run ./cmd/cli config`
- sessions persistées: `go run ./cmd/cli sessions`

Serveurs exposés:

- API HTTP: `go run ./cmd/api`
- gRPC: `go run ./cmd/grpc`

## Environment Variables

### API Keys (API Key Auth)

```bash
OPENAI_API_KEY=sk-...      # OpenAI API key
ANTHROPIC_API_KEY=sk-ant-... # Anthropic API key  
GOOGLE_API_KEY=...         # Google/Gemini API key
OLLAMA_API_KEY=...      # Ollama API key (optional)
```

### OAuth (Device Code Flow)

```bash
OPENAI_CLIENT_ID=...  # OAuth client ID for ChatGPT (optional)
NEXUS_AUTH_PATH=~/.nexus/auth.json  # Auth store path
```

### Auth Methods

- **API Key**: Set `PROVIDER_API_KEY` env var
- **OAuth**: Use `nexus login <provider> --oauth` (requires client ID)
