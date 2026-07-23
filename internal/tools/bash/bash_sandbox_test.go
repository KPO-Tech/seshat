package bash

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/KPO-Tech/seshat/internal/sandbox"
)

// requireDocker skips the test when the docker CLI isn't on PATH or the
// daemon isn't reachable. These tests exercise real Docker, not a mock, and
// this file carries no //go:build constraint (unlike bash_test.go, which is
// Linux/Landlock-only) — Docker sandboxing is meant to work on every OS.
func requireDocker(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not on PATH — skipping bash Docker-sandbox integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "docker", "version").Run(); err != nil {
		t.Skip("docker daemon not reachable — skipping bash Docker-sandbox integration tests")
	}
}

// TestNewToolWithDockerSandboxRoutesExecutionThroughContainer is the
// end-to-end proof that NewTool → executeLocalCommand → executeViaSandbox
// actually reaches DockerExecutor, not just that each layer compiles.
func TestNewToolWithDockerSandboxRoutesExecutionThroughContainer(t *testing.T) {
	requireDocker(t)

	cfg := DefaultToolConfig()
	cfg.SandboxKind = sandbox.EnvironmentDocker
	tool := NewTool(cfg)

	if tool.sandboxExecutor == nil {
		t.Fatal("expected NewTool to initialize sandboxExecutor when SandboxKind is EnvironmentDocker and Docker is healthy")
	}
	t.Cleanup(func() {
		if closer, ok := tool.sandboxExecutor.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	})

	execCtx := &ExecutionContext{
		Command:       "cat /proc/1/cgroup 2>/dev/null | head -1; echo ---; hostname",
		Timeout:       60 * time.Second,
		MaxOutputSize: MaxOutputSize,
	}

	result, err := tool.executeCommand(context.Background(), execCtx)
	if err != nil {
		t.Fatalf("executeCommand: %v", err)
	}
	if !result.Sandboxed {
		t.Fatal("expected CommandResult.Sandboxed to be true when routed through the Docker sandbox")
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", result.ExitCode, result.Stderr)
	}
	// A container's hostname is its (short) container ID, not this test
	// process's real machine hostname — confirms the command genuinely ran
	// in an isolated container, not on the host.
	hostHostname, hostErr := exec.Command("hostname").Output()
	if hostErr == nil {
		hostName := strings.TrimSpace(string(hostHostname))
		if strings.Contains(result.Stdout, hostName) {
			t.Fatalf("sandboxed command reported the host's own hostname %q — not actually isolated: %q", hostName, result.Stdout)
		}
	}
}

func TestNewToolFallsBackWhenDockerSandboxRequestedButUnavailable(t *testing.T) {
	cfg := DefaultToolConfig()
	cfg.SandboxKind = sandbox.EnvironmentDocker
	cfg.SandboxDocker = sandbox.DefaultDockerConfig()
	cfg.SandboxDocker.DockerBinary = "seshat-definitely-not-a-real-binary"

	tool := NewTool(cfg)
	if tool.sandboxExecutor != nil {
		t.Fatal("expected sandboxExecutor to stay nil (fallback to Landlock/local) when the configured Docker backend is unhealthy")
	}
}
