package repomap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tool "github.com/KPO-Tech/seshat-runtime/internal/tools/registry"
)

func TestToolCallReturnsRenderedRepoMap(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "main.go", `package main

func Run() {}
`)

	tl := NewTool(root)
	result, err := tl.Call(context.Background(), tool.CallInput{
		Raw: `{"tokens":256}`,
	}, nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if !strings.Contains(result.Content, "func Run") {
		t.Fatalf("expected repo map content, got %q", result.Content)
	}
	if result.ContentType == "" {
		t.Fatalf("expected content type to be set")
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
