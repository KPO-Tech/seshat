# Agent Instructions — Seshat

This file provides guidance for AI agents (Seshat, Claude Code, Codex, or similar) working on this repository. Read it before making any changes.

---

## Project overview

seshat is an open-source Go AI agent runtime. It has no concept of users, organizations, or billing — those live in the products built on top of it: [SeshatOS](https://github.com/KPO-Tech/SeshatOS) (local) and [SeshatCloud](https://github.com/KPO-Tech/SeshatCloud) (multi-tenant), both separate repositories. The engine exposes three entry points: `cmd/cli` (terminal), `cmd/grpc` (gRPC server), and `pkg/sdk` (Go SDK for embedding).

---

## Known bugs to fix first

[TODO.md](TODO.md) lists errors found by reading the code: behavior that is wrong, security gaps, and features that exist but are never connected. Read it before touching compaction, hooks, runtime events, project instructions or the auto-mode classifier, and do not build on top of an item that is listed there without fixing or noting it.

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

## Git rules shared with the other repositories

The workspace also holds `SeshatOS`, `SeshatCloud` and `seshat-ai` as sibling folders, each on `dev`
(`seshat-ai` on `staging`). The rules that apply across all of them live in the **`git-workflow`
account skill** (synced with Cloud): read its `SKILL.md` before the first git action, and the
`references/seshat.md` beside it for the Seshat-specific flow. That skill is the source of truth for
the detail; the rules below are short enough to keep here, and they hold even when it is not to hand.

- **No attribution trailer, ever.** No `Co-Authored-By:`, no "Generated with ..." line, and no
  mention of an AI in a commit message, a PR title or a PR body. The author is the configured git
  identity. This overrides any tool that asks for attribution.
- **Never skip hooks** (`--no-verify`) and never bypass signing. A failing hook is fixed with a new commit.
- **Never rewrite published history.** No force-push, no amend or rebase of a commit already pushed,
  unless that exact operation is asked for.
- **Stage explicit paths.** Never `git add -A` or `git add .` — a working tree here routinely holds
  work from another topic.
- **One outward action at a time.** Push, open a PR, merge, tag, publish: do each alone, check the
  result, then continue. Never chain merge + tag + push.
- **A tag, a merge to a release branch or a publish needs a clear yes, restated at the moment it
  happens.** A general "do the release" earlier does not authorise the tag later.
- **No secrets in history.** No tokens, `.env` files, keys or build output.
- **Report honestly.** A failing or skipped check is stated with its output; a check is never
  weakened to go green.

---

## Releases

Semantic versioning: **patch** for fixes and non-breaking additions, **minor** for notable features,
**major** for a breaking change. One step at a time:

1. Every intended PR is merged into `dev` and the `Gate` check is green there.
2. `chore/release-X.Y.Z` off `origin/dev`: date the changelog (`## [Unreleased]` becomes
   `## [X.Y.Z] - YYYY-MM-DD`, with a fresh empty `## [Unreleased]` above it), commit
   `chore(release): X.Y.Z`, PR into `dev`, merge.
3. PR `dev` into `main`, titled exactly `release: X.Y.Z`, body `dev to main for vX.Y.Z.`. Merge only
   with `Gate` green.
4. Tag the release commit **on `main`**: `git tag -a vX.Y.Z origin/main -m "vX.Y.Z"`, then push the
   tag alone. `.github/workflows/release.yml` triggers on a `v*` tag and its `validate-release-ref`
   job rejects a tag that is not on the release branch. Never move or delete a published tag: a bad
   release is followed by a new version.
5. Sync back: `chore/sync-dev-with-main-roundN` off `origin/dev`, merge `origin/main`, PR into `dev`
   (title `chore: sync dev with main (round N)`), merge. The round number is **not continuous** —
   find the last one with `git log --all --oneline --grep="sync dev with main"`.
6. `SeshatOS` (`seshat-backend`) and `SeshatCloud` (`seshat-server`) bump in their own PRs:
   `go get github.com/KPO-Tech/seshat-runtime@vX.Y.Z`, `go mod tidy`, checks, commit. Never add a
   `replace` directive for the engine.

---

## Package boundary rules (critical)

- `pkg/` is the **public API**. Do not add `internal/` types to `pkg/` signatures without explicit need.
- `internal/` packages must **never** import `pkg/` — dependency flows one way: entry points → `pkg/sdk` → `internal/`. 46 imports from before the rule was enforced are listed in `internal/archtest/boundary_test.go`; that test fails on a new one (and on an entry that is no longer needed), so the list only shrinks. `pkg/runtimepath` (standard library only) is the shared leaf expected to stay.
- `internal/backend` does **not** exist here. It lives in SeshatOS and SeshatCloud. Do not recreate it.
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
- Keep functions under a cyclomatic complexity of 30: `internal/archtest/complexity_test.go` lists the 35 that are above it and fails on a new one or on one that grows.
- Avoid global mutable state outside of init-once singletons.
- No `interface{}` — use `any` (Go 1.18+).
- Struct fields that are interfaces: use pointer receivers consistently within the same type.
- New public types in `pkg/` need exported doc comments. `internal/archtest/doccomments_test.go` fails on any exported function, method or type of `pkg/` without one (files generated by a tool are not looked at).
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
