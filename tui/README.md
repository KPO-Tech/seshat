# seshat terminal interface

The `seshat` command: the terminal chat, the headless `run` mode and the commands around them (`tui/cmd/cli`), and the
terminal interface they run (`tui/internal/seshattui`). It is a Go module of its own, apart from the engine at the root of
the repository, because part of it is derived from [Crush](https://github.com/charmbracelet/crush) and is under the
Functional Source License 1.1 (MIT Future License). See [NOTICE](../NOTICE) and [docs/licensing.md](../docs/licensing.md) for
what that allows and what it does not (a paid product or service that competes with Crush).

The engine (`pkg/`, `cmd/grpc`, the rest of `internal/`) does not depend on this module.

```bash
go build -o bin/seshat ./tui/cmd/cli    # from the repository root
go test ./tui/...
```
