package shared

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The git commands behind a diff run under the context of the call: a cancelled call does not start them.
func TestComputeGitDiffStopsWithItsContext(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, ok := ComputeGitDiff(context.Background(), file); !ok {
		t.Fatal("inside a repository the diff is computed")
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if diff, ok := ComputeGitDiff(cancelled, file); ok {
		t.Fatalf("a cancelled context must not produce a diff, got %+v", diff)
	}
}
