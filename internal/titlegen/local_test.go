package titlegen

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTitleOutputUsesLastUsefulLine(t *testing.T) {
	output := "Generate one title\n\nTitle: Analyse CV data IA\n"
	if got := parseTitleOutput(output); got != "Analyse CV data IA" {
		t.Fatalf("unexpected title: %q", got)
	}
}

func TestLocalGeneratorRequiresExecutable(t *testing.T) {
	_, err := NewLocalGenerator(LocalConfig{
		Enabled:        true,
		ExecutablePath: filepath.Join(t.TempDir(), "missing-llama-cli"),
		ModelPath:      filepath.Join(t.TempDir(), "model.gguf"),
	})
	if err == nil || !strings.Contains(err.Error(), "executable not found") {
		t.Fatalf("expected executable error, got %v", err)
	}
}

func TestBuildPromptTruncatesInput(t *testing.T) {
	long := strings.Repeat("a", defaultMaxInputRunes+20)
	prompt := buildPrompt(long)
	if strings.Contains(prompt, strings.Repeat("a", defaultMaxInputRunes+1)) {
		t.Fatalf("prompt still contains more than the allowed input runes")
	}
}

func TestLocalGeneratorRequiresModelSource(t *testing.T) {
	g := &LocalGenerator{config: LocalConfig{Enabled: true}}
	_, err := g.ensureModel(context.Background())
	if err == nil || !strings.Contains(err.Error(), "model_path or hf_repo") {
		t.Fatalf("expected model source error, got %v", err)
	}
}
