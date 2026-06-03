package workspace

import (
	"path/filepath"
	"testing"
)

func TestContextResolveKeepsRelativePathsInWorkspace(t *testing.T) {
	root := t.TempDir()
	ctx, err := New(root)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	got, err := ctx.Resolve("nested/file.txt")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := filepath.Join(root, "nested", "file.txt")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestContextRejectsTraversalOutsideWorkspace(t *testing.T) {
	ctx, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := ctx.Resolve("../outside.txt"); err == nil {
		t.Fatal("expected traversal outside workspace to fail")
	}
}

func TestContextRejectsAbsolutePathOutsideWorkspace(t *testing.T) {
	ctx, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := ctx.Validate(outside); err == nil {
		t.Fatal("expected absolute path outside workspace to fail")
	}
}
