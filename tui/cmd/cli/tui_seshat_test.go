package main

import (
	"os"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/pkg/runtimepath"
)

func TestEnsureSeshatTUIRuntimeRootSetsDefaultWhenUnset(t *testing.T) {
	t.Setenv(runtimepath.EnvRuntimeRoot, "")

	ensureSeshatTUIRuntimeRoot()

	want := runtimepath.DefaultConfigDir("seshat-tui")
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
