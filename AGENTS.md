# Agent Instructions — Seshat

This file provides guidance for AI agents (Seshat, Claude Code, Codex, or similar) working on this repository. Read it before making any changes.

---

## Project overview

seshat is an open-source Go AI agent runtime. It has no concept of users, organizations, or billing — those live in nexus-product (a separate private repository). The engine exposes three entry points: `cmd/cli` (terminal), `cmd/grpc` (gRPC server), and `pkg/sdk` (Go SDK for embedding).

---

## Build and test

Always run these before finishing:

```bash
go build ./...          # must pass — no exceptions
go vet ./...            # must pass — no exceptions
go test -race ./...     # must pass — fix failures, do not skip
golangci-lint run ./... # pass or explain why an existing violation is pre-existing
gofmt -w .              # always run after editing Go files
```

If `go test` fails, investigate and fix the root cause. Do not disable tests or add `t.Skip()` to work around failures.

---

## Branching strategy

```
main        production-ready, tagged releases only
  └── dev   stable integration — all work lands here first via PR
        └── <type>/<slug>   one branch per issue
```

- Always branch off `dev`.
- Never commit or push directly to `main`.
- Never commit or push directly to `dev`.
- All work must land through a PR: `<type>/*` → `dev`, then `dev` → `main`.
- PRs target `dev`. The only allowed PR into `main` is `dev` → `main` for final validation by maintainers.
- Direct PRs from topic branches into `main` must be closed without merge.
- The **Gate CI check** (Build + Test + Lint) must be green before any PR can merge.
- Name branches `<type>/<short-slug>` where `<type>` is one of `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `perf`, or `ci`.

If an exceptional direct push ever lands on `main`, stop feature work and
realign `dev` from `main` immediately before opening new feature branches.

---

## Package boundary rules (critical)

- `pkg/` is the **public API**. Do not add `internal/` types to `pkg/` signatures without explicit need.
- `internal/` packages must **never** import `pkg/` — dependency flows one way: entry points → `pkg/sdk` → `internal/`.
- `internal/backend` does **not** exist here. It lives in nexus-product. Do not recreate it.
- New tools go in `internal/tools/<category>/`. Register them in `internal/tools/builtin/builtin.go`.
- New providers go in `internal/providers/`. Add a wire-format adapter in `internal/providers/adapter.go`.

### Multi-agent package boundaries (strict — no cycles)

```
internal/agent    →  AgentProfile, ProfileRegistry  (who agents are)
internal/mailbox  →  Message, Mailbox               (how they communicate)
internal/team     →  Dispatcher, TeamBus            (coordination layer)
```

`internal/team` imports `agent` and `mailbox`. Neither `agent` nor `mailbox` imports `team`.
See [`docs/team.md`](./docs/team.md) for the full multi-agent system documentation.

---

## What not to do

- Do not add `cmd/api`, `internal/backend`, user/org/workspace models, billing, or authentication to this repository.
- Do not add `fmt.Printf` or `log.Printf` for secrets, tokens, or credentials — anywhere.
- Do not disable or weaken the permission system (`internal/permissions/`) by default.
- Do not add network calls in the main execution path without explicit user-provided configuration.
- Do not use `//nolint` without an explanation comment on the same line.
- Do not modify `internal/tools/bash/landlock_linux.go` or `internal/tools/bash/landlock_stub.go` unless you fully understand Linux Landlock syscalls.
- Do not add `replace` directives to `go.mod` — the workspace uses `go.work`.
- Do not commit large binaries, credentials, or generated artifacts.

---

## Go conventions for this codebase

- Always pass `context.Context` as the first parameter for any function that may do I/O.
- Use `errors.Is` / `errors.As` for error comparisons — never string matching on `err.Error()`.
- Prefer table-driven tests (`[]struct{ name, input, want }`).
- Avoid global mutable state outside of init-once singletons.
- No `interface{}` — use `any` (Go 1.18+).
- Struct fields that are interfaces: use pointer receivers consistently within the same type.
- New public types in `pkg/` need exported doc comments.
- Value receivers on types that contain `sync.Mutex` or `sync.RWMutex` are forbidden — always use pointer receivers.

---

## Documentation updates

When you add or change behavior, also update:

| Change | Update |
|---|---|
| New tool | `docs/tools.md` — add to the relevant section |
| New provider | `docs/providers.md` — add to the provider table + model list |
| New `ClientConfig` field | `docs/sdk.md` — add to the ClientConfig reference |
| New env var | `docs/transports.md` — add to the env var table |
| New multi-agent feature | `docs/team.md` — update the relevant section |
| Breaking change | `CHANGELOG.md` — add to `[Unreleased] → Changed` |
| New public API | `docs/sdk.md` or the relevant doc file |

---

## Commit message format

```
<type>(<scope>): <short description>
```

Types: `feat`, `fix`, `refactor`, `docs`, `test`, `chore`, `perf`, `ci`.

Examples:
```
feat(team): add TeamRegistry with SQLite-backed CRUD
fix(providers): honour retry-after header on 429 responses
docs(team): document Dispatcher.Assign routing strategy
```

---

## When in doubt

- Read [`docs/architecture.md`](./docs/architecture.md) for the package map.
- Read [`docs/team.md`](./docs/team.md) for the multi-agent system.
- Read [`docs/vision/idea.md`](./docs/vision/idea.md) for design principles.
- Read [`CONTRIBUTING.md`](./CONTRIBUTING.md) for the full contribution guide.
- Do not make speculative changes. Only change what is needed to accomplish the stated task.
