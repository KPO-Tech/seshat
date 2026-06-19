package sandbox

import (
	"context"
	"errors"
)

// ErrDaggerNotImplemented is returned by all DaggerExecutor methods until
// the Dagger backend is fully implemented (see GitHub issue for the roadmap).
// This sentinel lets callers distinguish "backend not built yet" from other
// errors so they can fall back to NoopExecutor gracefully.
var ErrDaggerNotImplemented = errors.New(
	"dagger sandbox backend: not implemented — see GitHub issue for roadmap",
)

// DaggerExecutor runs commands inside isolated OCI containers managed by the
// Dagger engine (https://dagger.io). Each agent session gets its own container
// environment; changes are tracked as git commits so the full history is
// recoverable.
//
// ┌─────────────────────────────────────────────────────────────────────┐
// │  STATUS: STUB — methods return ErrDaggerNotImplemented              │
// │                                                                     │
// │  Implementation is tracked in GitHub issue #XX.                    │
// │  DO NOT implement pieces here without first reading the issue.      │
// └─────────────────────────────────────────────────────────────────────┘
//
// Architecture overview (to be implemented):
//
//	DaggerExecutor
//	  │
//	  ├─ dagger.Connect(ctx) → *dagger.Client          [once per session]
//	  │
//	  ├─ environments map[string]*Environment           [per environment ID]
//	  │     └─ container  *dagger.Container             [persistent across calls]
//	  │     └─ gitRepo    *repository.Repository        [tracks file changes]
//	  │
//	  └─ Run(ctx, req) → RunResult
//	        ├─ resolve or create Environment
//	        ├─ container.WithExec(req.Command)
//	        ├─ capture stdout/stderr
//	        ├─ persist mutated container state
//	        └─ if req.Dagger.ExposePorts: tunnel via dag.Host().Tunnel()
//
// Dependency: dagger.io/dagger (NOT yet in go.mod — add when implementing)
// Minimum Dagger engine version: v0.18+
type DaggerExecutor struct {
	cfg DaggerExecutorConfig
	// client  *dagger.Client        // uncomment when implementing
	// mu      sync.RWMutex
	// envs    map[string]*daggerEnv
}

// newDaggerExecutor is the internal constructor called by NewExecutor when
// Kind == EnvironmentDocker. Returns ErrDaggerNotImplemented until fully built.
func newDaggerExecutor(cfg DaggerExecutorConfig) (*DaggerExecutor, error) {
	if cfg.BaseImage == "" {
		cfg = DefaultDaggerConfig()
	}
	return &DaggerExecutor{cfg: cfg}, ErrDaggerNotImplemented
}

func (e *DaggerExecutor) Kind() EnvironmentKind { return EnvironmentDocker }

// Healthy checks that the Dagger engine is reachable.
// Stub: always returns ErrDaggerNotImplemented.
func (e *DaggerExecutor) Healthy(_ context.Context) error {
	return ErrDaggerNotImplemented
	// TODO: dagger.Connect(ctx) — if it succeeds, the engine is up
}

// Run executes a shell command inside the Dagger container environment.
// Stub: always returns ErrDaggerNotImplemented.
func (e *DaggerExecutor) Run(_ context.Context, _ RunRequest) (RunResult, error) {
	return RunResult{}, ErrDaggerNotImplemented
	// TODO:
	// 1. resolve environment (create or reuse existing container)
	// 2. env.container = env.container.WithExec([]string{"sh", "-c", req.Command})
	// 3. stdout, err := env.container.Stdout(ctx)
	// 4. if req.Background: fire-and-forget via goroutine + service binding
	// 5. if req.Dagger.ExposePorts: dag.Host().Tunnel(svc, ...) → EndpointMappings
	// 6. save mutated container state (env.container = updated)
	// 7. optionally commit file changes to git branch
}

// ─── Internal types (stubs — filled in during implementation) ─────────────────

// daggerEnv holds the persistent state for a single named environment.
// One environment = one container + one git branch.
// type daggerEnv struct {
// 	id        string
// 	container *dagger.Container
// 	repo      *repository.Repository
// 	createdAt time.Time
// 	updatedAt time.Time
// }
