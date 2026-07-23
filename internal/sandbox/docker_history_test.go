package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// requireGit skips the test when git isn't on PATH — these tests exercise
// the real git CLI against real files, not a mock.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH — skipping file-change tracking tests")
	}
}

func newTrackingDockerExecutor(t *testing.T) *DockerExecutor {
	t.Helper()
	cfg := DefaultDockerConfig()
	cfg.TrackFileChanges = true
	cfg.HistoryDir = t.TempDir() // isolated per test — never shares history with other tests
	ex, err := newDockerExecutor(cfg)
	if err != nil {
		t.Fatalf("newDockerExecutor: %v", err)
	}
	t.Cleanup(func() { _ = ex.Close() })
	return ex
}

func TestDockerExecutorHealthyFailsWhenGitMissingButTrackingRequested(t *testing.T) {
	requireDocker(t)
	cfg := DefaultDockerConfig()
	cfg.TrackFileChanges = true
	cfg.GitBinary = "seshat-definitely-not-a-real-git-binary"
	ex, err := newDockerExecutor(cfg)
	if err != nil {
		t.Fatalf("newDockerExecutor: %v", err)
	}
	if err := ex.Healthy(context.Background()); err == nil {
		t.Fatal("expected Healthy to fail when TrackFileChanges is set but the git binary can't be found")
	}
}

func gitLog(t *testing.T, gitDir string) []string {
	t.Helper()
	out, err := exec.Command("git", "--git-dir", gitDir, "log", "--format=%s").Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

func TestDockerExecutorTracksFileChangesAcrossCalls(t *testing.T) {
	requireDocker(t)
	requireGit(t)
	ex := newTrackingDockerExecutor(t)
	hostDir := t.TempDir()

	// First call: creates the environment, takes the initial snapshot
	// (empty dir → no commit), then this command's own change.
	res, err := ex.Run(context.Background(), RunRequest{
		Command: "echo one > a.txt",
		WorkDir: hostDir,
		Timeout: 60 * time.Second,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	ex.mu.Lock()
	var gitDir string
	for _, env := range ex.envs {
		gitDir = env.historyGitDir
	}
	ex.mu.Unlock()
	if gitDir == "" {
		t.Fatal("expected the environment to have a historyGitDir")
	}

	commits := gitLog(t, gitDir)
	if len(commits) != 1 || !strings.Contains(commits[0], "echo one > a.txt") {
		t.Fatalf("expected exactly 1 commit referencing the command, got %v", commits)
	}

	// Second call, same environment, another change.
	if _, err := ex.Run(context.Background(), RunRequest{
		Command: "echo two > b.txt",
		WorkDir: hostDir,
		Timeout: 60 * time.Second,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	commits = gitLog(t, gitDir)
	if len(commits) != 2 {
		t.Fatalf("expected 2 commits after a second changing call, got %v", commits)
	}

	// Confirm the tracked content is genuinely retrievable from the shadow
	// history, not just that commits exist.
	show, err := exec.Command("git", "--git-dir", gitDir, "show", "HEAD:a.txt").Output()
	if err != nil {
		t.Fatalf("git show HEAD:a.txt: %v", err)
	}
	if strings.TrimSpace(string(show)) != "one" {
		t.Fatalf("unexpected tracked content for a.txt: %q", show)
	}

	_ = res // exit code / stdout not the focus of this test
}

func TestDockerExecutorTrackingSkipsNoOpCommits(t *testing.T) {
	requireDocker(t)
	requireGit(t)
	ex := newTrackingDockerExecutor(t)
	hostDir := t.TempDir()

	if _, err := ex.Run(context.Background(), RunRequest{
		Command: "echo content > f.txt",
		WorkDir: hostDir,
		Timeout: 60 * time.Second,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	ex.mu.Lock()
	var gitDir string
	for _, env := range ex.envs {
		gitDir = env.historyGitDir
	}
	ex.mu.Unlock()
	before := gitLog(t, gitDir)

	// A command that reads but changes nothing must not add a commit.
	if _, err := ex.Run(context.Background(), RunRequest{
		Command: "cat f.txt",
		WorkDir: hostDir,
		Timeout: 60 * time.Second,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	after := gitLog(t, gitDir)
	if len(after) != len(before) {
		t.Fatalf("expected no new commit for a no-op command: before=%v after=%v", before, after)
	}
}

func TestDockerExecutorTrackingExcludesProjectGitDir(t *testing.T) {
	requireDocker(t)
	requireGit(t)
	ex := newTrackingDockerExecutor(t)
	hostDir := t.TempDir()

	// Simulate the project already being a real git repo — the shadow
	// tracker must never snapshot its .git contents.
	projectGitDir := filepath.Join(hostDir, ".git")
	if err := os.MkdirAll(projectGitDir, 0o755); err != nil {
		t.Fatalf("mkdir project .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectGitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("write project .git/HEAD: %v", err)
	}

	if _, err := ex.Run(context.Background(), RunRequest{
		Command: "echo tracked > tracked.txt",
		WorkDir: hostDir,
		Timeout: 60 * time.Second,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	ex.mu.Lock()
	var gitDir string
	for _, env := range ex.envs {
		gitDir = env.historyGitDir
	}
	ex.mu.Unlock()

	out, err := exec.Command("git", "--git-dir", gitDir, "ls-tree", "-r", "--name-only", "HEAD").Output()
	if err != nil {
		t.Fatalf("git ls-tree: %v", err)
	}
	if strings.Contains(string(out), ".git/") {
		t.Fatalf("shadow history must never track the project's own .git contents, got tree:\n%s", out)
	}
	if !strings.Contains(string(out), "tracked.txt") {
		t.Fatalf("expected tracked.txt to be in the shadow history tree, got:\n%s", out)
	}
}

func TestDockerExecutorHistoryPersistsAcrossExecutorInstances(t *testing.T) {
	requireDocker(t)
	requireGit(t)
	hostDir := t.TempDir()
	sharedHistoryDir := t.TempDir()

	cfg1 := DefaultDockerConfig()
	cfg1.TrackFileChanges = true
	cfg1.HistoryDir = sharedHistoryDir
	ex1, err := newDockerExecutor(cfg1)
	if err != nil {
		t.Fatalf("newDockerExecutor: %v", err)
	}
	if _, err := ex1.Run(context.Background(), RunRequest{
		Command: "echo first > x.txt", WorkDir: hostDir, Timeout: 60 * time.Second,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if err := ex1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// A brand new executor instance, same HistoryDir, same project directory
	// — must continue the same shadow history, not start a fresh one.
	cfg2 := DefaultDockerConfig()
	cfg2.TrackFileChanges = true
	cfg2.HistoryDir = sharedHistoryDir
	ex2, err := newDockerExecutor(cfg2)
	if err != nil {
		t.Fatalf("newDockerExecutor: %v", err)
	}
	t.Cleanup(func() { _ = ex2.Close() })
	if _, err := ex2.Run(context.Background(), RunRequest{
		Command: "echo second > y.txt", WorkDir: hostDir, Timeout: 60 * time.Second,
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	ex2.mu.Lock()
	var gitDir string
	for _, env := range ex2.envs {
		gitDir = env.historyGitDir
	}
	ex2.mu.Unlock()

	commits := gitLog(t, gitDir)
	joined := strings.Join(commits, "\n")
	if !strings.Contains(joined, "x.txt") || !strings.Contains(joined, "y.txt") {
		t.Fatalf("expected history from both executor instances to be present, got commits: %v", commits)
	}
}
