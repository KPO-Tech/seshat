package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/KPO-Tech/seshat/pkg/runtimepath"
)

func TestEnsureSeshatTUIRuntimeRootSetsDefaultWhenUnset(t *testing.T) {
	t.Setenv(runtimepath.EnvRuntimeRoot, "")
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Fatalf("resolve user home: %v", err)
	}

	ensureSeshatTUIRuntimeRoot()

	want := filepath.Join(home, ".config", "seshat-tui")
	if got := os.Getenv(runtimepath.EnvRuntimeRoot); got != want {
		t.Fatalf("expected runtime root %q, got %q", want, got)
	}
}

func TestEnsureSeshatTUIRuntimeRootPreservesExistingValue(t *testing.T) {
	t.Setenv(runtimepath.EnvRuntimeRoot, "/tmp/custom-seshat-root")

	ensureSeshatTUIRuntimeRoot()

	if got := os.Getenv(runtimepath.EnvRuntimeRoot); got != "/tmp/custom-seshat-root" {
		t.Fatalf("expected existing runtime root to be preserved, got %q", got)
	}
}
