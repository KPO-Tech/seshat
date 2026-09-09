package bash

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/KPO-Tech/seshat/internal/sandbox"
	tool "github.com/KPO-Tech/seshat/internal/tools/registry"
)

// TestMain lets this test binary double as a "hangs forever" fake docker
// binary for TestDockerHealthCheckIsBoundedWhenDockerHangs - the standard
// exec.Command re-exec test pattern (see Go's own os/exec test suite),
// portable across every OS this SDK targets, unlike shelling out to a
// platform-specific "sleep" command. Every other test in this package is
// unaffected: this branch only triggers when BASH_TEST_HANG_FOREVER is set,
// which only that one test's re-exec'd child process ever has.
func TestMain(m *testing.M) {
	if os.Getenv("BASH_TEST_HANG_FOREVER") == "1" {
		select {}
	}
	os.Exit(m.Run())
}

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
	if got := tool.SandboxKind(); got != sandbox.EnvironmentLocal {
		t.Fatalf("expected SandboxKind() to report EnvironmentLocal on Docker health-check failure, got %v", got)
	}
}

// TestDockerHealthCheckIsBoundedWhenDockerHangs is the regression guard for
// the real bug this fix addresses: NewTool used to call
// executor.Healthy(context.Background()) - a Docker binary that's present
// but never returns (a hung daemon, stuck WSL2 integration, ...) hung this
// constructor, and therefore every embedder's own startup, forever. The fake
// "docker" here is this very test binary re-exec'd into an infinite select{}
// (see TestMain) - a real subprocess that genuinely never exits on its own,
// not a mock standing in for the timeout behavior being tested.
func TestDockerHealthCheckIsBoundedWhenDockerHangs(t *testing.T) {
	selfBinary, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	t.Setenv("BASH_TEST_HANG_FOREVER", "1")

	cfg := DefaultToolConfig()
	cfg.SandboxKind = sandbox.EnvironmentDocker
	cfg.SandboxDocker = sandbox.DefaultDockerConfig()
	cfg.SandboxDocker.DockerBinary = selfBinary

	done := make(chan *Tool, 1)
	go func() { done <- NewTool(cfg) }()

	// newDockerExecutor's own orphan-container reap already carries its own
	// bounded 10s context (unrelated to this fix, pre-existing), and it also
	// shells out to the same hung fake binary before Healthy() ever runs -
	// so the real worst case here is "both stages hit their own bound", not
	// just Healthy()'s 5s alone. 20s gives real slack above that combined
	// worst case while still failing fast if either stage regresses to
	// truly unbounded.
	select {
	case tl := <-done:
		if tl.sandboxExecutor != nil {
			t.Fatal("expected sandboxExecutor to stay nil when the docker binary never responds")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("NewTool did not return within 20s - the Docker health check's own bounded timeout did not bound it")
	}
}

// TestSandboxKindReportsLocalWhenDockerNotRequested proves SandboxKind()
// doesn't default to reporting Docker just because it's the "better"
// backend — a Tool that never requested it must report EnvironmentLocal,
// matching what NewTool actually initialized (see NewTool's nil-executor
// path when config.SandboxKind isn't EnvironmentDocker).
func TestSandboxKindReportsLocalWhenDockerNotRequested(t *testing.T) {
	tool := NewTool(DefaultToolConfig())
	if got := tool.SandboxKind(); got != sandbox.EnvironmentLocal {
		t.Fatalf("expected SandboxKind() to report EnvironmentLocal when Docker was never requested, got %v", got)
	}
}

// TestSandboxKindReportsDockerWhenHealthy is the counterpart to
// TestNewToolWithDockerSandboxRoutesExecutionThroughContainer, isolating
// just the SandboxKind() reporting behavior (that test already asserts
// sandboxExecutor != nil and Sandboxed == true; this asserts the new public
// accessor agrees).
// TestCallRefusesUnconfinedExecutionWhenRequireSandboxIsSet is the one test
// this package was missing for its own central multi-tenant-safety
// mechanism (grepping the whole package for "RequireSandbox" before this
// test existed turned up only the field/config plumbing, never an actual
// exercise of the enforcement branch in executeLocalCommand). Deliberately
// carries no //go:build constraint, unlike bash_test.go's Landlock-specific
// suite — on any host where neither Landlock nor Docker sandboxing actually
// applies (this test suite's own CI/dev machines included, whenever they
// aren't Linux-with-Landlock), RequireSandbox must refuse instead of
// running the command unconfined; that's exactly the platform this test
// exercises for real, no mocking needed.
func TestCallRefusesUnconfinedExecutionWhenRequireSandboxIsSet(t *testing.T) {
	if landlockAvailable() {
		t.Skip("this host has Landlock available — RequireSandbox would apply real confinement instead of refusing, see the Linux-specific suite in bash_test.go for that path")
	}

	cfg := DefaultToolConfig()
	cfg.RequireSandbox = true
	tl := NewTool(cfg)

	result, err := tl.Call(context.Background(), tool.CallInput{
		Parsed: map[string]any{"command": "echo hi"},
	}, nil)
	if err != nil {
		t.Fatalf("Call returned a Go error instead of a tool-result error: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected RequireSandbox to refuse execution when no OS-level sandbox is available on this platform, got a successful result")
	}
}

func TestSandboxKindReportsDockerWhenHealthy(t *testing.T) {
	requireDocker(t)

	cfg := DefaultToolConfig()
	cfg.SandboxKind = sandbox.EnvironmentDocker
	tool := NewTool(cfg)
	t.Cleanup(func() {
		if closer, ok := tool.sandboxExecutor.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	})

	if got := tool.SandboxKind(); got != sandbox.EnvironmentDocker {
		t.Fatalf("expected SandboxKind() to report EnvironmentDocker when Docker is healthy, got %v", got)
	}
}
