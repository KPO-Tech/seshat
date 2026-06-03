package utils

import (
	"os/exec"
	"testing"
)

func TestDetect_WithGitRepo(t *testing.T) {
	tempDir := t.TempDir()

	gitCtx := Detect(tempDir)
	if gitCtx == nil {
		t.Fatal("Detect should return a Context, not nil")
	}
	if gitCtx.IsGit {
		t.Fatal("directory should not be a git repo initially")
	}

	cmd := exec.Command("git", "init")
	cmd.Dir = tempDir
	err := cmd.Run()
	if err != nil {
		t.Fatalf("Failed to initialize git repo: %v", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tempDir
	err = cmd.Run()
	if err != nil {
		t.Fatalf("Failed to configure git user.name: %v", err)
	}

	gitCtx = Detect(tempDir)
	if gitCtx == nil {
		t.Fatal("git context should not be nil for a git repo")
	}
	if !gitCtx.IsGit {
		t.Fatal("should detect as git repo")
	}
	if gitCtx.Root == "" {
		t.Fatal("git root should not be empty for a git repo")
	}

	t.Logf("Detected git root: %s, branch: %s", gitCtx.Root, gitCtx.Branch)
}

func TestDetect_NoGitRepo(t *testing.T) {
	tempDir := t.TempDir()

	gitCtx := Detect(tempDir)
	if gitCtx == nil {
		t.Fatal("Detect should return a Context, not nil")
	}
	if gitCtx.IsGit {
		t.Fatal("directory should not be detected as git repo")
	}
	if gitCtx.Root != "" {
		t.Fatal("git root should be empty for non-git directory")
	}
}

func TestDetect_CustomBranch(t *testing.T) {
	tempDir := t.TempDir()

	cmd := exec.Command("git", "init")
	cmd.Dir = tempDir
	err := cmd.Run()
	if err != nil {
		t.Fatalf("Failed to initialize git repo: %v", err)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tempDir
	err = cmd.Run()
	if err != nil {
		t.Fatalf("Failed to configure git user.name: %v", err)
	}

	gitCtx := Detect(tempDir)
	if gitCtx == nil {
		t.Fatal("git context should not be nil for git repo")
	}
	if !gitCtx.IsGit {
		t.Fatal("should detect as git repo")
	}

	t.Logf("Detected git root: %s, branch: %s", gitCtx.Root, gitCtx.Branch)
}

func TestFindGitRoot(t *testing.T) {
	tempDir := t.TempDir()

	root := FindGitRoot(tempDir)
	if root != "" {
		t.Fatalf("expected empty root for non-git dir, got: %s", root)
	}

	cmd := exec.Command("git", "init")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	root = FindGitRoot(tempDir)
	if root == "" {
		t.Fatal("expected non-empty root for git repo")
	}
	t.Logf("Git root: %s", root)
}

func TestFindCanonicalGitRoot(t *testing.T) {
	tempDir := t.TempDir()

	canonical := FindCanonicalGitRoot(tempDir)
	if canonical != "" {
		t.Fatalf("expected empty canonical root for non-git dir, got: %s", canonical)
	}

	cmd := exec.Command("git", "init")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	canonical = FindCanonicalGitRoot(tempDir)
	if canonical == "" {
		t.Fatal("expected non-empty canonical root for git repo")
	}
	t.Logf("Canonical git root: %s", canonical)
}

func TestGetIsGit(t *testing.T) {
	tempDir := t.TempDir()

	if GetIsGit(tempDir) {
		t.Fatal("non-git directory should return false")
	}

	cmd := exec.Command("git", "init")
	cmd.Dir = tempDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to init git repo: %v", err)
	}

	if !GetIsGit(tempDir) {
		t.Fatal("git directory should return true")
	}
}

func TestIsValidGitSha(t *testing.T) {
	tests := []struct {
		s      string
		expect bool
	}{
		{"abc123def4567890123456789012345678901234", true},
		{"ABC123DEF4567890123456789012345678901234", true},
		{"0000000000000000000000000000000000000000", true},
		{"abc", false},
		{"abcdef", false},
		{"", false},
		{"ggggggg", false},
	}

	for _, tt := range tests {
		result := isValidGitSha(tt.s)
		if result != tt.expect {
			t.Errorf("isValidGitSha(%s) = %v, expected %v", tt.s, result, tt.expect)
		}
	}
}

func TestIsSafeRefName(t *testing.T) {
	tests := []struct {
		name   string
		expect bool
	}{
		{"main", true},
		{"feature/awesome", true},
		{"release-1.0.0", true},
		{"bug_fix", true},
		{"release+build", true},
		{"v1.2.3", true},
		{"../etc/passwd", false},
		{"-force", false},
		{"/absolute", false},
		{"", false},
		{"foo/./bar", false},
		{"foo//bar", false},
	}

	for _, tt := range tests {
		result := isSafeRefName(tt.name)
		if result != tt.expect {
			t.Errorf("isSafeRefName(%s) = %v, expected %v", tt.name, result, tt.expect)
		}
	}
}
