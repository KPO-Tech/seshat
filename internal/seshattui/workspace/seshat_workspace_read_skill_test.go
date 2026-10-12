package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/internal/seshattui/config"
)

// TestReadSkillReturnsRealContent guards H8 in TODO.md: SeshatWorkspace.ReadSkill
// was a complete stub that always returned nil/empty, so the terminal
// interface's "attach skill" action silently attached nothing. It must now
// return the real file content for the same catalog entry ListSkills shows.
func TestReadSkillReturnsRealContent(t *testing.T) {
	workDir := t.TempDir()
	skillsDir := filepath.Join(workDir, "skills")
	skillDir := filepath.Join(skillsDir, "greet")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := "---\nname: greet\ndescription: say hello\nuser-invocable: true\n---\n\nHello, world!\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	w := NewSeshatWorkspace(nil, workDir, "")
	w.cfg = &config.Config{Options: &config.Options{SkillsPaths: []string{skillsDir}}}

	entries, err := w.ListSkills(context.Background())
	if err != nil {
		t.Fatalf("ListSkills: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 catalog entry, got %d: %#v", len(entries), entries)
	}

	data, result, err := w.ReadSkill(context.Background(), entries[0].ID)
	if err != nil {
		t.Fatalf("ReadSkill: %v", err)
	}
	if result.Name != "greet" {
		t.Errorf("expected result.Name=greet, got %q", result.Name)
	}
	if string(data) != content {
		t.Errorf("expected ReadSkill to return the real file content, got %q", string(data))
	}
}

func TestReadSkillUnknownIDReturnsError(t *testing.T) {
	workDir := t.TempDir()
	w := NewSeshatWorkspace(nil, workDir, "")
	w.cfg = &config.Config{Options: &config.Options{}}

	if _, _, err := w.ReadSkill(context.Background(), "/does/not/exist/SKILL.md"); err == nil {
		t.Fatal("expected an error for an unknown skill ID")
	}
}
