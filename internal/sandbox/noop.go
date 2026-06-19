package sandbox

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"
)

// NoopExecutor runs commands directly on the host OS with no additional
// isolation. This is the default executor and mirrors the current behavior
// of the bash tool before sandboxing was introduced.
//
// Isolation: none. The CommandPolicy and FilesystemPolicy still apply, but
// there is no OS-level boundary between the agent and the host filesystem.
type NoopExecutor struct{}

// NewNoopExecutor returns a NoopExecutor ready to use.
func NewNoopExecutor() *NoopExecutor { return &NoopExecutor{} }

func (e *NoopExecutor) Kind() EnvironmentKind { return EnvironmentLocal }

func (e *NoopExecutor) Healthy(_ context.Context) error { return nil }

func (e *NoopExecutor) Run(ctx context.Context, req RunRequest) (RunResult, error) {
	shell := req.Shell
	if len(shell) == 0 {
		shell = []string{"/bin/sh", "-c"}
	}

	cmdCtx := ctx
	var cancel context.CancelFunc
	if req.Timeout > 0 {
		cmdCtx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	args := append(shell[1:], req.Command)
	cmd := exec.CommandContext(cmdCtx, shell[0], args...)

	if req.WorkDir != "" {
		cmd.Dir = req.WorkDir
	}
	if len(req.Env) > 0 {
		cmd.Env = append(cmd.Environ(), envMapToSlice(req.Env)...)
	}
	if req.Stdin != nil {
		cmd.Stdin = req.Stdin
	}

	var stdout, stderr bytes.Buffer
	if !req.Background {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}

	start := time.Now()
	err := cmd.Run()
	dur := time.Since(start)

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			err = nil // non-zero exit is not a Run error
		}
	}

	return RunResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Duration: dur,
	}, err
}

func envMapToSlice(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	return out
}

// enviroToMap converts an os.Environ()-style slice to a map.
// Exported for use in tests.
func EnvSliceToMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, kv := range env {
		if idx := strings.IndexByte(kv, '='); idx >= 0 {
			m[kv[:idx]] = kv[idx+1:]
		}
	}
	return m
}
