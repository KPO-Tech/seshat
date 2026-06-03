# Agent Instructions — Nexus Engine

This file provides guidance for AI agents (Nexus, Claude Code, Codex, or similar) working on this repository. Read it before making any changes.

---

## Project overview

nexus-engine is an open-source Go AI agent runtime. It has no concept of users, organizations, or billing — those live in nexus-product (a separate private repository). The engine exposes three entry points: `cmd/cli` (terminal), `cmd/grpc` (gRPC server), and `pkg/sdk` (Go SDK for embedding).

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

## Package boundary rules (critical)

- `pkg/` is the **public API**. Do not add `internal/` types to `pkg/` signatures without explicit need.
- `internal/` packages must **never** import `pkg/` — dependency flows one way: entry points → `pkg/sdk` → `internal/`.
- `internal/backend` does **not** exist here. It lives in nexus-product. Do not recreate it. If you need a server-side feature, ask whether it belongs in nexus-product.
- New tools go in `internal/tools/<category>/`. Register them in `internal/tools/builtin/builtin.go`.
- New providers go in `internal/providers/`. Add a wire-format adapter in `internal/providers/adapter.go`.

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

---

## Documentation updates

When you add or change behavior, also update:

| Change | Update |
|---|---|
| New tool | `docs/tools.md` — add to the relevant section |
| New provider | `docs/providers.md` — add to the provider table + model list |
| New `ClientConfig` field | `docs/sdk.md` — add to the ClientConfig reference |
| New env var | `docs/transports.md` — add to the env var table |
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
feat(tools): add generate_image tool with OpenAI and Gemini support
fix(providers): handle retry-after header correctly in Anthropic adapter
docs(sdk): document RuntimeEventFn callback and event type list
```

---

## When in doubt

- Read [`docs/architecture.md`](./docs/architecture.md) for the package map.
- Read [`docs/vision/idea.md`](./docs/vision/idea.md) for design principles.
- Read [`CONTRIBUTING.md`](./CONTRIBUTING.md) for the full contribution guide.
- Do not make speculative changes. Only change what is needed to accomplish the stated task.
