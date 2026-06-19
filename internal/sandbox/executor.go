package sandbox

import (
	"context"
	"io"
	"time"
)

// Executor is the interface all sandbox backends implement.
// The engine routes bash/shell execution through the active Executor.
//
// Backends:
//   - NoopExecutor   — runs commands directly on the host OS (default, no isolation)
//   - DaggerExecutor — runs commands inside an isolated Dagger/OCI container
//
// Selection at startup via ExecutorConfig.Kind. Tools that are read-only
// (file read, glob, grep) bypass the Executor and go directly to the host FS.
type Executor interface {
	// Run executes a command inside the sandbox and streams its output.
	// Implementations must respect ctx cancellation and RunRequest.Timeout.
	Run(ctx context.Context, req RunRequest) (RunResult, error)

	// Kind identifies which backend this is.
	Kind() EnvironmentKind

	// Healthy returns nil when the executor is ready to accept work.
	// A non-nil error means the backend is unavailable (Dagger engine down,
	// Docker socket missing, etc). The engine calls this once at startup.
	Healthy(ctx context.Context) error
}

// RunRequest is the input to Executor.Run.
type RunRequest struct {
	// Command is the shell command string to execute.
	// Passed as-is to the interpreter defined by Shell.
	Command string

	// Shell specifies the interpreter (default: /bin/sh -c).
	// Ignored by DaggerExecutor which always uses sh inside the container.
	Shell []string

	// Env is a set of KEY=VALUE pairs injected into the command environment
	// on top of the executor's base environment.
	Env map[string]string

	// WorkDir is the working directory for the command.
	// For DaggerExecutor, this is relative to the container's /workdir.
	WorkDir string

	// Stdin is connected to the command's standard input (may be nil).
	Stdin io.Reader

	// Timeout is the maximum duration for the command.
	// Zero means no timeout beyond the parent context deadline.
	Timeout time.Duration

	// Background requests a fire-and-forget execution (no output captured).
	// Used for long-running processes (servers, watchers).
	Background bool

	// Dagger-specific extensions — ignored by NoopExecutor.
	Dagger *DaggerRunOptions
}

// DaggerRunOptions carries Dagger-specific per-run parameters.
// These are only meaningful when the active Executor is a DaggerExecutor.
type DaggerRunOptions struct {
	// ExposePorts lists container ports to tunnel back to the host.
	// The RunResult.Endpoints map is populated with host:port mappings.
	ExposePorts []int

	// EnvironmentID selects a named, persistent environment.
	// Empty means the default session environment.
	EnvironmentID string
}

// RunResult is the output of Executor.Run.
type RunResult struct {
	// Stdout and Stderr capture the command output.
	// Empty when Background=true.
	Stdout string
	Stderr string

	// ExitCode is the process exit code. Non-zero indicates failure.
	ExitCode int

	// Duration is the wall-clock time the command took.
	Duration time.Duration

	// Endpoints maps exposed port numbers to "host:port" strings.
	// Populated by DaggerExecutor when DaggerRunOptions.ExposePorts is set.
	Endpoints map[int]string
}

// ExecutorConfig is the configuration passed to NewExecutor at startup.
type ExecutorConfig struct {
	// Kind selects the backend. Defaults to EnvironmentLocal (noop).
	Kind EnvironmentKind

	// Dagger holds Dagger-specific configuration.
	// Only used when Kind == EnvironmentDocker.
	Dagger DaggerExecutorConfig
}

// DaggerExecutorConfig holds static configuration for the DaggerExecutor.
// All fields have sensible defaults and can be overridden by the operator.
type DaggerExecutorConfig struct {
	// BaseImage is the OCI image used for new environments.
	// Default: "ubuntu:24.04"
	BaseImage string

	// SetupCommands are shell commands run once when building the base image.
	// Example: ["apt-get update -y", "apt-get install -y python3 nodejs"]
	SetupCommands []string

	// WorkDir is the working directory inside the container.
	// Default: "/workdir"
	WorkDir string

	// Env is a set of KEY=VALUE pairs always injected into the container.
	Env map[string]string

	// StorageDir is the host directory where environment state is persisted.
	// Default: ~/.config/nexus/sandbox/
	StorageDir string

	// NetworkAccess controls whether the container can reach the internet.
	// Default: true (containers have internet access by default).
	// Set to false for fully air-gapped sandboxes.
	NetworkAccess bool
}

// DefaultDaggerConfig returns a DaggerExecutorConfig with sensible defaults.
func DefaultDaggerConfig() DaggerExecutorConfig {
	return DaggerExecutorConfig{
		BaseImage:     "ubuntu:24.04",
		WorkDir:       "/workdir",
		NetworkAccess: true,
	}
}

// NewExecutor creates the Executor selected by cfg.Kind.
// Returns an error when the requested backend is unavailable.
func NewExecutor(cfg ExecutorConfig) (Executor, error) {
	switch cfg.Kind {
	case EnvironmentDocker:
		return newDaggerExecutor(cfg.Dagger)
	default:
		return NewNoopExecutor(), nil
	}
}
