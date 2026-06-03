# Nexus Engine Transports And Setup

Cette note documente l'installation locale, les binaires serveur exposés par `nexus-engine`, les contrats HTTP et gRPC actuellement câblés, et la régénération protobuf.

Elle décrit l'état réel du code à ce jour.

## Scope

- moteur et SDK Go: `pkg/sdk`
- API HTTP: `cmd/api`
- serveur gRPC: `cmd/grpc`
- contrat protobuf: `pkg/grpc/proto/nexus.proto`

## Prérequis

### Go

Le module déclare `go 1.25.1` dans `go.mod`.

Installation minimale recommandée:

```bash
go version
```

### Protobuf And gRPC Codegen

Pour modifier `pkg/grpc/proto/nexus.proto` et régénérer les stubs:

```bash
protoc --version
which protoc-gen-go
which protoc-gen-go-grpc
```

Installation type Linux:

```bash
mkdir -p "$HOME/.local"

cd /tmp
curl -LO https://github.com/protocolbuffers/protobuf/releases/download/v34.1/protoc-34.1-linux-x86_64.zip
unzip -o protoc-34.1-linux-x86_64.zip -d "$HOME/.local"

go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

Ajouter au `PATH`:

```bash
export PATH="$HOME/.local/bin:$(go env GOPATH)/bin:$PATH"
```

### Client Tools

Outils pratiques pour tester les transports:

- HTTP: `curl`
- gRPC: `grpcurl`

Exemple d'installation `grpcurl`:

```bash
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
```

### Provider Credentials

Selon le provider utilisé, le runtime lit les variables suivantes:

```bash
ANTHROPIC_API_KEY=...
OPENAI_API_KEY=...
GOOGLE_API_KEY=...
OLLAMA_API_KEY=...
```

Le runtime partagé lit aussi une config `NEXUS_*`:

```bash
NEXUS_CWD=/absolute/or/relative/path
NEXUS_MODEL=anthropic:claude-3-5-sonnet-20241022
NEXUS_API_KEY=...
NEXUS_DB_PATH=/tmp/nexus_engine/nexus.sqlite
NEXUS_DEBUG=true
NEXUS_PROVIDER_BASE_URL=...
NEXUS_PROVIDER_REGION=...
NEXUS_PROVIDER_PROJECT_ID=...
NEXUS_PROVIDER_RESOURCE=...
NEXUS_ADMIN_EMAIL=admin@example.com
NEXUS_ADMIN_PASSWORD=change-me
NEXUS_ADMIN_PASSWORD_HASH=...
WEB_SEARCH_PROVIDER=tavily
```

Le loader lit aussi:

- `.env` dans le répertoire courant
- `../../../.env`
- `.nexus.yaml` dans `$HOME` ou dans le répertoire courant

## Binaires Disponibles

### API HTTP

Lancer:

```bash
go run ./cmd/api
```

Port actuel: `8090`

Endpoints annoncés au boot:

- `GET /health`
- `GET /metrics`
- `GET /api/v1/skills`
- `GET /api/v1/skills/{name}`
- `GET /api/v1/mcp`
- `POST /api/v1/query`
- `POST /api/v1/query/stream`
- `GET /api/v1/models`
- `GET /api/v1/metrics`
- `GET /api/v1/sessions`
- `POST /api/v1/sessions`
- `DELETE /api/v1/sessions/{id}`
- `POST /api/v1/auth/login`
- `POST /api/v1/auth/logout`
- `GET /api/v1/auth/me`

Endpoints admin présents:

- `GET /api/v1/auth/admin/ping`
- `GET|POST /api/v1/users`
- `GET|PATCH|DELETE /api/v1/users/{id}`
- `GET|POST /api/v1/organizations`
- `GET|PATCH /api/v1/organizations/{id}`
- `GET|POST /api/v1/workspaces`
- `GET|PATCH /api/v1/workspaces/{id}`

### gRPC

Lancer:

```bash
go run ./cmd/grpc
```

Port actuel: `50051`

Service principal:

- `nexus.NexusService`

Méthodes principales:

- `Query`
- `QueryStream`
- `ListSkills`
- `GetSkillDetails`
- `ListMCP`
- `ConnectMCP`
- `DisconnectMCP`
- `GetModels`
- `HealthCheck`

Services additionnels dans le `.proto`:

- `FileService`
- `SystemService`

Important: ces services additionnels existent dans le contrat protobuf, mais `cmd/grpc` n'enregistre aujourd'hui que `NexusService`.

Important: le serveur gRPC n'active pas la reflection actuellement. Pour `grpcurl`, il faut fournir explicitement le fichier `.proto`.

## API HTTP

## Auth HTTP

La plupart des endpoints métier sont protégés par `Authorization: Bearer <token>`.

Pour pouvoir se connecter, il faut bootstrapper au moins un admin au démarrage:

```bash
export NEXUS_ADMIN_EMAIL=admin@example.com
export NEXUS_ADMIN_PASSWORD=change-me
go run ./cmd/api
```

Login:

```bash
curl -s http://localhost:8090/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.com","password":"change-me"}'
```

Réponse type:

```json
{
  "token": "....",
  "expires_at": "2026-05-09T12:00:00Z",
  "user": {
    "id": "usr_...",
    "email": "admin@example.com",
    "display_name": "Administrator",
    "status": "active"
  },
  "roles": ["admin"]
}
```

Appel authentifié:

```bash
TOKEN="..."

curl -s http://localhost:8090/api/v1/auth/me \
  -H "Authorization: Bearer $TOKEN"
```

## Query HTTP

### POST `/api/v1/query`

Requête:

```json
{
  "prompt": "Résume ce dépôt",
  "session_id": "optional-existing-session-id"
}
```

Réponse:

```json
{
  "session_id": "sess_...",
  "content": "Réponse finale",
  "stop_reason": "end_turn",
  "turn_number": 1,
  "is_complete": true,
  "usage": {
    "input_tokens": 123,
    "output_tokens": 45
  },
  "tool_uses": [],
  "tool_results_count": 0
}
```

Exemple:

```bash
curl -s http://localhost:8090/api/v1/query \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"hello"}'
```

### POST `/api/v1/query/stream`

Transport: `text/event-stream`

Événements réellement émis:

- message SSE par défaut pour les chunks texte:

```text
data: {"type":"chunk","chunk_type":"content_block_delta","delta":"hel","delta_type":"text_delta"}
```

- `event: runtime` pour les événements runtime structurés:

```text
event: runtime
data: {"type":"turn.started","session_id":"...","turn_id":"...","turn_number":1}
```

- `event: done` pour le résultat final:

```text
event: done
data: {"session_id":"...","content":"hello","stop_reason":"end_turn","turn_number":1,"is_complete":true}
```

- `event: error` en cas d'échec:

```text
event: error
data: {"error":"..."}
```

Exemple:

```bash
curl -N http://localhost:8090/api/v1/query/stream \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"hello"}'
```

## Sessions HTTP

Créer une session:

```bash
curl -s http://localhost:8090/api/v1/sessions \
  -X POST \
  -H "Authorization: Bearer $TOKEN"
```

Lister les sessions:

```bash
curl -s http://localhost:8090/api/v1/sessions \
  -H "Authorization: Bearer $TOKEN"
```

Supprimer une session:

```bash
curl -i http://localhost:8090/api/v1/sessions/<session-id> \
  -X DELETE \
  -H "Authorization: Bearer $TOKEN"
```

## Skills, MCP, Metrics, Models HTTP

Endpoints:

- `GET /api/v1/skills`
- `GET /api/v1/skills/{name}`
- `GET /api/v1/mcp`
- `GET /metrics`
- `GET /api/v1/metrics`
- `GET /api/v1/models`

Exemple:

```bash
curl -s http://localhost:8090/health
curl -s http://localhost:8090/metrics
curl -s http://localhost:8090/api/v1/models
```

Important: `GET /api/v1/models` retourne actuellement une liste statique codée en dur dans `cmd/api/main.go`. Ce endpoint n'est pas encore aligné sur `providers.AllProvidersInfo()`.

## gRPC

## Contrat

Source canonique:

- [pkg/grpc/proto/nexus.proto](/home/amiche/Projects/AI/ai/nexus_ai/apps/nexus-engine/pkg/grpc/proto/nexus.proto)

Stubs générés:

- [pkg/grpc/nexus/nexus.pb.go](/home/amiche/Projects/AI/ai/nexus_ai/apps/nexus-engine/pkg/grpc/nexus/nexus.pb.go)
- [pkg/grpc/nexus/nexus_grpc.pb.go](/home/amiche/Projects/AI/ai/nexus_ai/apps/nexus-engine/pkg/grpc/nexus/nexus_grpc.pb.go)

## Query gRPC

### `Query`

Usage simple, non streamé.

Exemple `grpcurl`:

```bash
grpcurl \
  -plaintext \
  -import-path /home/amiche/Projects/AI/ai/nexus_ai/apps/nexus-engine/pkg/grpc/proto \
  -proto nexus.proto \
  -d '{"prompt":"hello","model":"anthropic:claude-3-5-sonnet-20241022"}' \
  localhost:50051 nexus.NexusService/Query
```

### `QueryStream`

Le stream retourne maintenant des `QueryResponse` typés avec `item_type`.

Valeurs actuelles:

- `item_type = "chunk"`
- `item_type = "runtime_event"`
- `item_type = "final"`

Pour `chunk`:

- `content` contient encore le delta texte brut
- `chunk.type`
- `chunk.delta_type`
- `chunk.delta`

Pour `runtime_event`:

- `runtime_event.type`
- `runtime_event.session_id`
- `runtime_event.turn_id`
- `runtime_event.turn_number`
- `runtime_event.timestamp`
- `runtime_event.stop_reason`
- `runtime_event.token_usage`
- `runtime_event.chunk`
- `runtime_event.tool_name`
- `runtime_event.tool_stage`
- `runtime_event.tool_message`
- `runtime_event.tool_percent_complete`
- `runtime_event.error`

Pour `final`:

- `conversation_id`
- `content`
- `token_usage`
- `stopped`

Exemple `grpcurl`:

```bash
grpcurl \
  -plaintext \
  -import-path /home/amiche/Projects/AI/ai/nexus_ai/apps/nexus-engine/pkg/grpc/proto \
  -proto nexus.proto \
  -d '{"prompt":"hello","model":"anthropic:claude-3-5-sonnet-20241022"}' \
  localhost:50051 nexus.NexusService/QueryStream
```

## Skills, MCP, Models, Health gRPC

Exemples:

```bash
grpcurl \
  -plaintext \
  -import-path /home/amiche/Projects/AI/ai/nexus_ai/apps/nexus-engine/pkg/grpc/proto \
  -proto nexus.proto \
  -d '{}' \
  localhost:50051 nexus.NexusService/HealthCheck

grpcurl \
  -plaintext \
  -import-path /home/amiche/Projects/AI/ai/nexus_ai/apps/nexus-engine/pkg/grpc/proto \
  -proto nexus.proto \
  -d '{}' \
  localhost:50051 nexus.NexusService/GetModels
```

## gRPC Auth

Le serveur `cmd/grpc` n'ajoute actuellement aucune couche d'authentification.

Ça veut dire:

- pas de bearer token
- pas de session d'auth HTTP réutilisée
- pas de TLS configuré ici
- transport prévu surtout pour usage local/dev tant qu'une couche auth n'est pas branchée

## Régénération protobuf

À faire après toute modification de `pkg/grpc/proto/nexus.proto`:

```bash
PATH="$HOME/go/bin:$HOME/.local/bin:/usr/local/bin:/usr/bin:/bin" \
protoc \
  --proto_path=pkg/grpc/proto \
  --go_out=. \
  --go_opt=module=github.com/EngineerProjects/nexus-engine \
  --go-grpc_out=. \
  --go-grpc_opt=module=github.com/EngineerProjects/nexus-engine \
  pkg/grpc/proto/nexus.proto
```

Puis vérifier:

```bash
go test ./cmd/grpc
go test ./pkg/grpc/nexus
```

## Comportements Importants Aujourd'hui

- HTTP `query` et `query/stream` passent par l'auth API.
- gRPC ne passe pas par l'auth API.
- HTTP streaming expose les chunks texte et les événements runtime SSE.
- gRPC streaming expose les chunks texte, les événements runtime typés, puis la réponse finale.
- HTTP `/api/v1/models` est statique.
- gRPC `GetModels` reflète la registry providers côté code.
- Le `.proto` contient `FileService` et `SystemService`, mais `cmd/grpc` ne les implémente pas aujourd'hui.

## Vérifications Utiles

Smoke test HTTP:

```bash
go test ./cmd/api
```

Smoke test gRPC:

```bash
go test ./cmd/grpc
go test ./pkg/grpc/nexus
```
