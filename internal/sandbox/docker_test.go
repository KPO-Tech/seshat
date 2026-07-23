package sandbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// requireDocker skips the test when the docker CLI isn't on PATH or the
// daemon isn't reachable — these tests exercise a real container runtime,
// not a mock, so CI/dev machines without Docker installed must not fail.
func requireDocker(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not on PATH — skipping DockerExecutor integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "docker", "version").Run(); err != nil {
		t.Skip("docker daemon not reachable — skipping DockerExecutor integration tests")
	}
}

func newTestDockerExecutor(t *testing.T) *DockerExecutor {
	t.Helper()
	cfg := DefaultDockerConfig()
	ex, err := newDockerExecutor(cfg)
	if err != nil {
		t.Fatalf("newDockerExecutor: %v", err)
	}
	t.Cleanup(func() {
		if err := ex.Close(); err != nil {
			t.Errorf("DockerExecutor.Close: %v", err)
		}
	})
	return ex
}

func TestDockerExecutorHealthy(t *testing.T) {
	requireDocker(t)
	ex := newTestDockerExecutor(t)
	if err := ex.Healthy(context.Background()); err != nil {
		t.Fatalf("Healthy: %v", err)
	}
}

// TestDockerExecutorPublishesAndReportsPorts verifies port publishing +
// RunResult.Endpoints resolution end-to-end against the real Docker daemon,
// cross-checked against `docker port`'s raw output as independent ground
// truth (rather than trusting our own parsing of it) — proves the ports are
// genuinely published on the container, not just present in our own map.
func TestDockerExecutorPublishesAndReportsPorts(t *testing.T) {
	requireDocker(t)
	cfg := DefaultDockerConfig()
	cfg.PublishPorts = []int{18080, 18081}
	ex, err := newDockerExecutor(cfg)
	if err != nil {
		t.Fatalf("newDockerExecutor: %v", err)
	}
	t.Cleanup(func() { _ = ex.Close() })

	res, err := ex.Run(context.Background(), RunRequest{Command: "true", Timeout: 60 * time.Second})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d: %v", len(res.Endpoints), res.Endpoints)
	}

	ex.mu.Lock()
	var containerID string
	for _, env := range ex.envs {
		containerID = env.containerID
	}
	ex.mu.Unlock()

	verifyCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rawOut, err := exec.CommandContext(verifyCtx, "docker", "port", containerID).Output()
	if err != nil {
		t.Fatalf("docker port: %v", err)
	}

	for _, port := range []int{18080, 18081} {
		addr, ok := res.Endpoints[port]
		if !ok {
			t.Fatalf("missing RunResult.Endpoints entry for port %d (got %v)", port, res.Endpoints)
		}
		if !strings.HasPrefix(addr, "localhost:") {
			t.Fatalf("expected endpoint for port %d to start with \"localhost:\", got %q", port, addr)
		}
		hostPort := strings.TrimPrefix(addr, "localhost:")
		if !strings.Contains(string(rawOut), strconv.Itoa(port)+"/tcp -> ") || !strings.Contains(string(rawOut), ":"+hostPort) {
			t.Fatalf("resolved endpoint %q for container port %d not corroborated by `docker port` ground truth:\n%s", addr, port, rawOut)
		}
	}

	// A second Run call on the same environment must report the identical
	// mapping — it's fixed for the container's lifetime, not re-resolved.
	res2, err := ex.Run(context.Background(), RunRequest{Command: "true", Timeout: 60 * time.Second})
	if err != nil {
		t.Fatalf("Run (second call): %v", err)
	}
	if res2.Endpoints[18080] != res.Endpoints[18080] || res2.Endpoints[18081] != res.Endpoints[18081] {
		t.Fatalf("expected stable endpoints across calls, got %v then %v", res.Endpoints, res2.Endpoints)
	}
}

// TestDockerExecutorHealthyFailsForUnregisteredRuntime proves the
// "requested stronger isolation must never silently downgrade" contract:
// Healthy(ctx) must reject an unregistered Runtime rather than pretending
// the runtime request was honored. This machine genuinely doesn't have
// gVisor's runsc registered (confirmed separately via `docker info`), so
// "runsc" here is a real, not simulated, unregistered-runtime case.
func TestDockerExecutorHealthyFailsForUnregisteredRuntime(t *testing.T) {
	requireDocker(t)
	cfg := DefaultDockerConfig()
	cfg.Runtime = "runsc"
	ex, err := newDockerExecutor(cfg)
	if err != nil {
		t.Fatalf("newDockerExecutor: %v", err)
	}
	if err := ex.Healthy(context.Background()); err == nil {
		t.Fatal("expected Healthy to fail for a runtime that isn't registered with the daemon")
	}
}

// TestDockerExecutorRuntimeRegisteredDetectsRunc proves the positive path
// of runtimeRegistered against a runtime we know for certain is present on
// any machine with Docker installed — runc — without needing gVisor.
func TestDockerExecutorRuntimeRegisteredDetectsRunc(t *testing.T) {
	requireDocker(t)
	ex := newTestDockerExecutor(t)
	ok, err := ex.runtimeRegistered(context.Background(), "runc")
	if err != nil {
		t.Fatalf("runtimeRegistered: %v", err)
	}
	if !ok {
		t.Fatal("expected runc to be reported as registered — every Docker install has it")
	}
}

// TestDockerExecutorExplicitRuntimeIsHonored exercises the full --runtime
// plumbing end-to-end using "runc" as a stand-in for a real alternative
// runtime like gVisor's runsc (not installed on this machine) — proves
// Healthy passes and a container genuinely starts when Runtime names a
// registered runtime that merely isn't the implicit default.
func TestDockerExecutorExplicitRuntimeIsHonored(t *testing.T) {
	requireDocker(t)
	cfg := DefaultDockerConfig()
	cfg.Runtime = "runc"
	ex, err := newDockerExecutor(cfg)
	if err != nil {
		t.Fatalf("newDockerExecutor: %v", err)
	}
	t.Cleanup(func() { _ = ex.Close() })

	if err := ex.Healthy(context.Background()); err != nil {
		t.Fatalf("Healthy: %v", err)
	}
	res, err := ex.Run(context.Background(), RunRequest{Command: "echo ok", Timeout: 60 * time.Second})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.TrimSpace(res.Stdout) != "ok" {
		t.Fatalf("unexpected stdout: %q", res.Stdout)
	}
}

func TestDockerExecutorHealthyFailsForMissingBinary(t *testing.T) {
	cfg := DefaultDockerConfig()
	cfg.DockerBinary = "seshat-definitely-not-a-real-binary"
	ex, err := newDockerExecutor(cfg)
	if err != nil {
		t.Fatalf("newDockerExecutor: %v", err)
	}
	if err := ex.Healthy(context.Background()); err == nil {
		t.Fatal("expected Healthy to fail for a nonexistent docker binary")
	}
}

func TestDockerExecutorRunSimpleCommand(t *testing.T) {
	requireDocker(t)
	ex := newTestDockerExecutor(t)

	res, err := ex.Run(context.Background(), RunRequest{
		Command: "echo hello-from-container",
		Timeout: 60 * time.Second,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr=%q)", res.ExitCode, res.Stderr)
	}
	if strings.TrimSpace(res.Stdout) != "hello-from-container" {
		t.Fatalf("unexpected stdout: %q", res.Stdout)
	}
}

func TestDockerExecutorNonZeroExitIsNotAnError(t *testing.T) {
	requireDocker(t)
	ex := newTestDockerExecutor(t)

	res, err := ex.Run(context.Background(), RunRequest{
		Command: "exit 7",
		Timeout: 60 * time.Second,
	})
	if err != nil {
		t.Fatalf("Run should not return a Go error for a non-zero exit: %v", err)
	}
	if res.ExitCode != 7 {
		t.Fatalf("expected exit code 7, got %d", res.ExitCode)
	}
}

func TestDockerExecutorPersistsEnvironmentAcrossCalls(t *testing.T) {
	requireDocker(t)
	ex := newTestDockerExecutor(t)
	opts := &DockerRunOptions{EnvironmentID: "persist-test"}

	if _, err := ex.Run(context.Background(), RunRequest{
		Command: "echo marker-value > /tmp/marker.txt",
		Docker:  opts,
		Timeout: 60 * time.Second,
	}); err != nil {
		t.Fatalf("Run (write marker): %v", err)
	}

	res, err := ex.Run(context.Background(), RunRequest{
		Command: "cat /tmp/marker.txt",
		Docker:  opts,
		Timeout: 60 * time.Second,
	})
	if err != nil {
		t.Fatalf("Run (read marker): %v", err)
	}
	if strings.TrimSpace(res.Stdout) != "marker-value" {
		t.Fatalf("expected the second call to see state left by the first — got stdout %q", res.Stdout)
	}

	ex.mu.Lock()
	envCount := len(ex.envs)
	ex.mu.Unlock()
	if envCount != 1 {
		t.Fatalf("expected exactly 1 tracked environment (container reused), got %d", envCount)
	}
}

func TestDockerExecutorDifferentEnvironmentIDsGetSeparateContainers(t *testing.T) {
	requireDocker(t)
	ex := newTestDockerExecutor(t)

	if _, err := ex.Run(context.Background(), RunRequest{
		Command: "true",
		Docker:  &DockerRunOptions{EnvironmentID: "env-a"},
		Timeout: 60 * time.Second,
	}); err != nil {
		t.Fatalf("Run env-a: %v", err)
	}
	if _, err := ex.Run(context.Background(), RunRequest{
		Command: "true",
		Docker:  &DockerRunOptions{EnvironmentID: "env-b"},
		Timeout: 60 * time.Second,
	}); err != nil {
		t.Fatalf("Run env-b: %v", err)
	}

	ex.mu.Lock()
	envCount := len(ex.envs)
	ex.mu.Unlock()
	if envCount != 2 {
		t.Fatalf("expected 2 tracked environments, got %d", envCount)
	}
}

func TestDockerExecutorFilesystemIsolatedFromHost(t *testing.T) {
	requireDocker(t)
	ex := newTestDockerExecutor(t)

	// A file created on the host in this test process must not be visible
	// inside the container unless explicitly bind-mounted — proves the
	// container boundary is real, not a no-op.
	res, err := ex.Run(context.Background(), RunRequest{
		Command: "test -f /etc/hostname && echo container-etc-hostname-exists; test -d " + t.TempDir() + " && echo LEAKED-HOST-PATH-VISIBLE || echo host-tempdir-not-visible",
		Timeout: 60 * time.Second,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(res.Stdout, "LEAKED-HOST-PATH-VISIBLE") {
		t.Fatalf("host path was visible inside the container without a bind mount: %q", res.Stdout)
	}
	if !strings.Contains(res.Stdout, "host-tempdir-not-visible") {
		t.Fatalf("expected confirmation that the host temp dir is not visible, got %q", res.Stdout)
	}
}

func TestDockerExecutorNetworkAccessDisabled(t *testing.T) {
	requireDocker(t)
	cfg := DefaultDockerConfig()
	cfg.NetworkAccess = false
	ex, err := newDockerExecutor(cfg)
	if err != nil {
		t.Fatalf("newDockerExecutor: %v", err)
	}
	t.Cleanup(func() { _ = ex.Close() })

	res, err := ex.Run(context.Background(), RunRequest{
		Command: "getent hosts example.com",
		Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode == 0 {
		t.Fatalf("expected DNS resolution to fail with --network=none, but it succeeded: stdout=%q", res.Stdout)
	}
}

func TestDockerExecutorNetworkAccessEnabledByDefault(t *testing.T) {
	requireDocker(t)
	ex := newTestDockerExecutor(t)

	res, err := ex.Run(context.Background(), RunRequest{
		Command: "getent hosts example.com",
		Timeout: 30 * time.Second,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected DNS resolution to succeed with network access enabled, got exit %d stderr=%q", res.ExitCode, res.Stderr)
	}
}

func TestDockerExecutorWorkDirBindMount(t *testing.T) {
	requireDocker(t)
	ex := newTestDockerExecutor(t)
	hostDir := t.TempDir()

	res, err := ex.Run(context.Background(), RunRequest{
		Command: "echo from-host-mount > seen.txt && cat seen.txt",
		WorkDir: hostDir,
		Timeout: 60 * time.Second,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.TrimSpace(res.Stdout) != "from-host-mount" {
		t.Fatalf("unexpected stdout: %q", res.Stdout)
	}
}

func TestDockerExecutorCloseRemovesContainers(t *testing.T) {
	requireDocker(t)
	cfg := DefaultDockerConfig()
	ex, err := newDockerExecutor(cfg)
	if err != nil {
		t.Fatalf("newDockerExecutor: %v", err)
	}

	if _, err := ex.Run(context.Background(), RunRequest{Command: "true", Timeout: 60 * time.Second}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	ex.mu.Lock()
	var containerID string
	for _, env := range ex.envs {
		containerID = env.containerID
	}
	ex.mu.Unlock()
	if containerID == "" {
		t.Fatal("expected a tracked container after Run")
	}

	if err := ex.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, "docker", "inspect", containerID).Run(); err == nil {
		t.Fatalf("expected container %s to be removed after Close, but docker inspect succeeded", containerID)
	}
}

// TestDockerExecutorDropsNetRawCapability verifies the --cap-drop hardening
// by reading /proc/self/status's CapEff bitmask directly inside the
// container, rather than relying on a specific tool like `ping` (which
// needs CAP_NET_RAW but isn't installed by default in the base image) — bit
// 13 is CAP_NET_RAW per capabilities(7); it must be 0.
func TestDockerExecutorDropsNetRawCapability(t *testing.T) {
	requireDocker(t)
	ex := newTestDockerExecutor(t)

	res, err := ex.Run(context.Background(), RunRequest{
		Command: "grep CapEff /proc/self/status",
		Timeout: 60 * time.Second,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	fields := strings.Fields(res.Stdout)
	if len(fields) != 2 {
		t.Fatalf("unexpected CapEff line: %q", res.Stdout)
	}
	capEff, err := strconv.ParseUint(fields[1], 16, 64)
	if err != nil {
		t.Fatalf("parse CapEff %q: %v", fields[1], err)
	}
	const capNetRaw = 1 << 13
	if capEff&capNetRaw != 0 {
		t.Fatalf("expected CAP_NET_RAW to be dropped, but CapEff=%x has it set", capEff)
	}
}

// TestDockerExecutorReapsOrphanFromPreviousProcess simulates a container
// left running by a crashed previous process instance (same sandbox label,
// different process-id label — this test binary's real DockerExecutor
// instances all share this process's real processInstanceID, so a manually
// created container with a different one is indistinguishable from a true
// orphan) and verifies a freshly constructed DockerExecutor's reaper removes
// it on construction, before ever calling Run.
func TestDockerExecutorReapsOrphanFromPreviousProcess(t *testing.T) {
	requireDocker(t)

	orphanName := "seshat-sbx-test-orphan-" + hex.EncodeToString(mustRandomBytes(t, 4))
	createCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	createArgs := []string{
		"run", "-d", "--rm", "--name", orphanName,
		"--label", labelSandbox + "=true",
		"--label", labelEnvironmentID + "=default",
		"--label", labelProcessID + "=not-this-process-" + hex.EncodeToString(mustRandomBytes(t, 4)),
		"--label", labelCreatedAt + "=" + time.Now().UTC().Format(time.RFC3339),
		"ubuntu:24.04", "sleep", "infinity",
	}
	if err := exec.CommandContext(createCtx, "docker", createArgs...).Run(); err != nil {
		t.Fatalf("failed to create simulated orphan container: %v", err)
	}
	// Safety net in case the reaper doesn't clean it up (test failure path).
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", orphanName).Run()
	})

	cfg := DefaultDockerConfig()
	ex, err := newDockerExecutor(cfg)
	if err != nil {
		t.Fatalf("newDockerExecutor: %v", err)
	}
	t.Cleanup(func() { _ = ex.Close() })

	inspectCtx, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel2()
	if err := exec.CommandContext(inspectCtx, "docker", "inspect", orphanName).Run(); err == nil {
		t.Fatal("expected the orphaned container to be reaped by newDockerExecutor, but docker inspect still succeeds")
	}
}

func mustRandomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	return b
}

// TestDockerExecutorDoesNotLeakHostEnvironment is a regression test for a
// security property DockerExecutor already has, not a gap: unlike
// NoopExecutor.Run (which inherits the full host process environment via
// cmd.Environ()), a sandboxed command must NOT see host-side environment
// variables the seshat process holds (API keys, tokens, …) unless the
// caller explicitly passes them via RunRequest.Env.
func TestDockerExecutorDoesNotLeakHostEnvironment(t *testing.T) {
	requireDocker(t)
	t.Setenv("SESHAT_TEST_HOST_SECRET", "this-must-not-reach-the-sandbox")

	ex := newTestDockerExecutor(t)
	res, err := ex.Run(context.Background(), RunRequest{
		Command: "env",
		Timeout: 60 * time.Second,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.Contains(res.Stdout, "SESHAT_TEST_HOST_SECRET") {
		t.Fatalf("host environment variable leaked into the sandbox: %q", res.Stdout)
	}
}

// TestDockerExecutorForwardsOnlyExplicitEnv confirms the flip side: a
// variable explicitly passed via RunRequest.Env DOES reach the sandbox —
// the previous test only proves the default is closed, this proves the
// explicit path still works.
func TestDockerExecutorForwardsOnlyExplicitEnv(t *testing.T) {
	requireDocker(t)
	ex := newTestDockerExecutor(t)

	res, err := ex.Run(context.Background(), RunRequest{
		Command: "echo $SESHAT_TEST_EXPLICIT_VAR",
		Env:     map[string]string{"SESHAT_TEST_EXPLICIT_VAR": "reached-the-sandbox"},
		Timeout: 60 * time.Second,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.TrimSpace(res.Stdout) != "reached-the-sandbox" {
		t.Fatalf("expected explicitly-passed env var to reach the sandbox, got stdout %q", res.Stdout)
	}
}
