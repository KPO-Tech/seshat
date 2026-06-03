# Contributing to Nexus Engine

Thank you for your interest in contributing. This guide covers everything you need to get started.

---

## Before you open a PR

- **Bug fix** — open an issue first to confirm the bug and discuss the fix approach.
- **New feature** — open an issue or discussion before writing code. Features need to align with the [project vision](./docs/vision/idea.md) and the [current roadmap](./docs/vision/roadmap.md).
- **Documentation** — PRs are welcome without prior discussion.
- **Refactor** — discuss first. We are conservative about structural changes that could introduce regressions.

---

## Development setup

### Requirements

- Go 1.25+
- `golangci-lint` for linting
- `protoc` + `protoc-gen-go` + `protoc-gen-go-grpc` if modifying the proto file (see [`docs/transports.md`](./docs/transports.md))

### Clone and build

```bash
git clone https://github.com/EngineerProjects/nexus-engine
cd nexus-engine

# Install git pre-commit hooks (run once)
make hooks

# Build
make build

# Test
make test
make test-race
```

### Environment

Copy `.env.example` (if present) or set provider credentials directly:

```bash
export ANTHROPIC_API_KEY=sk-ant-...
export OPENAI_API_KEY=sk-...
```

---

## Code conventions

### Go style

- Format with `gofmt` — enforced by the pre-commit hook and CI.
- Follow `go vet` — enforced by CI.
- Follow `golangci-lint` rules — see [`.golangci.yml`](./.golangci.yml) for the active linter set.
- No `fmt.Printf` in non-test code — use the structured logger in `internal/monitoring`.
- No global mutable state outside of registered singletons.

### Package boundaries

- `pkg/` is the public API. Only add to `pkg/` what external consumers need. Do not expose `internal/` types directly.
- `internal/` packages must not import `pkg/` — the dependency goes one way.
- `internal/backend` does not exist in this repository — it lives in `nexus-product`. Do not recreate it here.
- New tools go in `internal/tools/<category>/`. New providers go in `internal/providers/`.

See [`docs/architecture.md`](./docs/architecture.md) for the full package map.

### Adding a new tool

1. Create `internal/tools/<category>/<toolname>/tool.go`.
2. Implement the `tool.Tool` interface (see [`docs/tools.md`](./docs/tools.md)).
3. Register in `internal/tools/builtin/builtin.go`.
4. Add to the built-in tools reference in `docs/tools.md`.
5. Write at least one test in `<toolname>/tool_test.go`.

### Adding a new provider

1. Add the provider constant to `internal/providers/registry.go`.
2. Implement or reuse a wire-format adapter in `internal/providers/adapter.go`.
3. Add model entries to the provider info table.
4. Add the env var to `docs/transports.md` and `docs/providers.md`.
5. Add golden tests in `internal/providers/adapter_test.go`.

### Commit messages

We use [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <short description>

[optional body]

[optional footer]
```

Types: `feat`, `fix`, `refactor`, `docs`, `test`, `chore`, `perf`, `ci`.

Examples:
```
feat(tools): add generate_image tool with OpenAI and Gemini providers
fix(providers): handle rate limit retry-after header in anthropic adapter
docs(sdk): document RuntimeEventFn callback and event types
```

Breaking changes: append `!` and add a `BREAKING CHANGE:` footer.

---

## Testing

- Run `make test-race` before submitting — the race detector catches concurrency bugs.
- Tests must not make real API calls. Use mock providers or recorded fixtures.
- Security-sensitive code (sandbox, permissions) requires tests that assert the deny path, not just the allow path.
- Table-driven tests preferred for cases with multiple inputs.

---

## Pull request checklist

- [ ] `go build ./...` passes
- [ ] `go test -race ./...` passes
- [ ] `golangci-lint run ./...` passes (or failures are pre-existing and noted)
- [ ] New code is covered by tests
- [ ] Public API changes are reflected in `docs/`
- [ ] Commit messages follow Conventional Commits
- [ ] Breaking changes are called out in the PR description

---

## What we will not merge

- Features that move `internal/backend` concerns into nexus-engine (auth, users, orgs, billing).
- Provider-specific logic that bypasses the adapter interface.
- External HTTP calls in the main execution path without explicit user configuration.
- Changes that disable or weaken the permission system by default.
- New global mutable state.
- Code that uses `//nolint` without an explanation comment.

---

## Code of conduct

This project follows the [Contributor Covenant](./CODE_OF_CONDUCT.md). Be respectful and constructive.

---

## License

By contributing, you agree that your contributions will be licensed under the [Apache 2.0 License](./LICENSE).
