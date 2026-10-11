package skills

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGetSkillDirCommandsForUser_ReadsLegacySkillsDirectory guards H8 in
// TODO.md: NewFileSkillLoader("") defaults its Directory to
// DefaultSkillDirectory (".seshat/skills" - the folder the old documentation
// recommended), but GetSkillDirCommandsForUser never read it, so a skill
// placed there was invisible to the runtime (seshat_list_skills,
// seshat_read_skill, and the skill tool), even though the terminal
// interface's own separate catalog showed it.
func TestGetSkillDirCommandsForUser_ReadsLegacySkillsDirectory(t *testing.T) {
	t.Setenv("SESHAT_RUNTIME_ROOT", t.TempDir())

	cwd := t.TempDir()
	skillDir := filepath.Join(cwd, DefaultSkillDirectory, "legacy-greet")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := "---\nname: \"legacy-greet\"\ndescription: \"say hello from .seshat/skills\"\nuser-invocable: true\n---\n\nHello from the legacy skills dir.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "skill.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loader := NewFileSkillLoader("")
	loaded, err := loader.GetSkillDirCommandsForUser(cwd, "")
	if err != nil {
		t.Fatalf("GetSkillDirCommandsForUser: %v", err)
	}

	found := false
	for _, sk := range loaded {
		if sk.Name == "legacy-greet" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a skill from %s to be discovered, got %d skills: %#v", filepath.Join(cwd, DefaultSkillDirectory), len(loaded), loaded)
	}
}

// A loader built with a custom (non-default) directory must have that exact
// directory read too, not just the hardcoded default.
func TestGetSkillDirCommandsForUser_ReadsCustomLoaderDirectory(t *testing.T) {
	t.Setenv("SESHAT_RUNTIME_ROOT", t.TempDir())

	cwd := t.TempDir()
	customDir := filepath.Join(cwd, "custom-skills", "my-skill")
	if err := os.MkdirAll(customDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := "---\nname: \"custom-dir-skill\"\ndescription: \"from a custom loader directory\"\n---\n\nHi.\n"
	if err := os.WriteFile(filepath.Join(customDir, "skill.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	loader := NewFileSkillLoader(filepath.Join(cwd, "custom-skills"))
	loaded, err := loader.GetSkillDirCommandsForUser(cwd, "")
	if err != nil {
		t.Fatalf("GetSkillDirCommandsForUser: %v", err)
	}

	found := false
	for _, sk := range loaded {
		// Name is derived from the directory name ("my-skill"); DisplayName
		// comes from the frontmatter "name:" field and is what proves the
		// file was actually parsed, not just that the directory was seen.
		if sk.DisplayName == "custom-dir-skill" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the loader's own custom directory to be read, got %d skills: %#v", len(loaded), loaded)
	}
}
