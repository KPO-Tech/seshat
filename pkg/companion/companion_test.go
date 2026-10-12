package companion

import (
	"os"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/pkg/runtimepath"
)

func TestLoadMissingReturnsDisabledDefault(t *testing.T) {
	t.Setenv(runtimepath.EnvRuntimeRoot, t.TempDir())
	profile, err := Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if profile.Enabled {
		t.Fatal("missing companion file should not enable prompt injection")
	}
	if profile.Name != "Seshat" {
		t.Fatalf("unexpected default name %q", profile.Name)
	}
}

func TestSaveLoadAndSystemPrompt(t *testing.T) {
	t.Setenv(runtimepath.EnvRuntimeRoot, t.TempDir())
	profile := DefaultProfile()
	profile.Name = "Nora"
	profile.Traits = []string{"warm", "warm", "precise"}
	profile.Instructions = "Prefer short answers."
	if err := Save("", profile); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	loaded, err := Load("")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Name != "Nora" {
		t.Fatalf("expected name Nora, got %q", loaded.Name)
	}
	if len(loaded.Traits) != 2 {
		t.Fatalf("expected traits to be compacted, got %#v", loaded.Traits)
	}
	prompt := SystemPrompt(loaded)
	for _, want := range []string{"<companion>", "Nora", "Prefer short answers.", "</companion>"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q, got:\n%s", want, prompt)
		}
	}
}

func TestSystemPromptDisabled(t *testing.T) {
	profile := DefaultProfile()
	profile.Enabled = false
	if got := SystemPrompt(profile); got != "" {
		t.Fatalf("expected disabled companion prompt to be empty, got %q", got)
	}
}

func TestSaveWritesAtomicallyLeavingNoTempFiles(t *testing.T) {
	root := t.TempDir()
	t.Setenv(runtimepath.EnvRuntimeRoot, root)
	profile := DefaultProfile()
	profile.Name = "Atomic"
	if err := Save("", profile); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("expected no leftover temp file after Save, found %q", e.Name())
		}
	}
	if _, err := os.Stat(runtimepath.CompanionPath("")); err != nil {
		t.Fatalf("expected companion file to exist: %v", err)
	}
}
