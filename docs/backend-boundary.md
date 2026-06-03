# Backend Boundary — Nexus Engine

This document defines what belongs in `internal/backend/` vs the core runtime.

> For the full audit and rationale, see [`internal-boundary-audit.md`](./internal-boundary-audit.md).

---

## Rule

```
cmd/api  ──►  internal/backend  ──►  internal/{core packages}
```

`internal/backend` is the **product layer**: multi-user identity, tenancy, auth sessions, query orchestration for the HTTP/gRPC surface. It depends on core packages but nothing in core may depend on backend.

---

## What lives in `internal/backend/`

| Sub-package | Responsibility |
|---|---|
| `backend/bkerr` | Shared error kinds (no internal deps) |
| `backend/auth` | Login/logout, token resolution, Principal model |
| `backend/query` | Query/session service, QueryRuntime interface, SDKRuntime adapter |
| `backend/resources` | CRUD for users, organizations, workspaces |
| `backend/metrics` | Metrics snapshot service, Snapshotter interface |
| `backend/app.go` | App struct wiring all services (root package) |

---

## What stays in core (never moves to backend)

- `internal/rag`, `internal/vector`, `internal/storage` — headless data layer
- `internal/engine`, `internal/execution`, `internal/runtime/*` — main agent loop
- `internal/providers` — LLM clients (except `providers/auth.go` which is CLI-era, kept in place)
- `internal/web`, `internal/tools` — tool implementations
- `internal/db` — single migration sequence, not split before Postgres (Slice 5)

---

## No-go list

- Do not move rag/vector/storage/web into backend
- Do not fragment `internal/db` before Slice 5 (Postgres)
- Do not merge FileStore + DB credentials
- Do not create a generic `CredentialResolver` interface in core
- Do not create a domain model / repository layer between backend and db
