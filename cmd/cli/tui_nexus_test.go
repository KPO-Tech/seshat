package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/EngineerProjects/nexus-engine/pkg/runtimepath"
)

func TestEnsureNexusTUIRuntimeRootSetsDefaultWhenUnset(t *testing.T) {
	t.Setenv(runtimepath.EnvRuntimeRoot, "")
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Fatalf("resolve user home: %v", err)
	}

	ensureNexusTUIRuntimeRoot()

	want := filepath.Join(home, ".config", "nexus-tui")
	if got := os.Getenv(runtimepath.EnvRuntimeRoot); got != want {
		t.Fatalf("expected runtime root %q, got %q", want, got)
	}
}

func TestEnsureNexusTUIRuntimeRootPreservesExistingValue(t *testing.T) {
	t.Setenv(runtimepath.EnvRuntimeRoot, "/tmp/custom-nexus-root")

	ensureNexusTUIRuntimeRoot()

	if got := os.Getenv(runtimepath.EnvRuntimeRoot); got != "/tmp/custom-nexus-root" {
		t.Fatalf("expected existing runtime root to be preserved, got %q", got)
	}
}
