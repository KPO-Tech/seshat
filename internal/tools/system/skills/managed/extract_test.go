package managed

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureExtractedIsNoopWithEmptyBuiltin(t *testing.T) {
	dest := t.TempDir()

	if err := EnsureExtracted(dest); err != nil {
		t.Fatalf("EnsureExtracted: %v", err)
	}

	// Version file must be written even when there is nothing to extract.
	vf := filepath.Join(dest, ".builtin-version")
	data, err := os.ReadFile(vf)
	if err != nil {
		t.Fatalf("version file missing: %v", err)
	}
	if string(data) != Version {
		t.Fatalf("version mismatch: got %q want %q", string(data), Version)
	}

	// Calling again must be a no-op (idempotent).
	if err := EnsureExtracted(dest); err != nil {
		t.Fatalf("second EnsureExtracted: %v", err)
	}
}

func TestEmbeddedBuiltinContainsOnlyReadme(t *testing.T) {
	// The builtin/ directory now contains only README.md — all skills were
	// moved to github.com/EngineerProjects/nexus-skills.
	readme, err := FS.ReadFile("builtin/README.md")
	if err != nil {
		t.Fatalf("README.md should be embedded: %v", err)
	}
	if len(readme) == 0 {
		t.Fatal("README.md is empty")
	}
}
