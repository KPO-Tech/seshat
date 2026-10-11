package skills

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func mkSkillDir(t *testing.T, root string, rel string) {
	t.Helper()
	dir := filepath.Join(root, rel, ".claude", "skills")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
}

func TestDiscoverNestedSkillDirs_FindsSubdirectorySkills(t *testing.T) {
	root := t.TempDir()
	mkSkillDir(t, root, "packages/foo")
	mkSkillDir(t, root, "packages/bar")

	got := discoverNestedSkillDirs(root)
	sort.Strings(got)

	want := []string{
		filepath.Join(root, "packages", "bar", ".claude", "skills"),
		filepath.Join(root, "packages", "foo", ".claude", "skills"),
	}
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, got)
		}
	}
}

func TestDiscoverNestedSkillDirs_IgnoresTopLevel(t *testing.T) {
	root := t.TempDir()
	mkSkillDir(t, root, "") // root/.claude/skills - covered by GetProjectSkillsPath already

	got := discoverNestedSkillDirs(root)
	if len(got) != 0 {
		t.Fatalf("expected the top-level .claude/skills to be excluded (covered elsewhere), got %v", got)
	}
}

func TestDiscoverNestedSkillDirs_SkipsIgnoredDirectories(t *testing.T) {
	root := t.TempDir()
	mkSkillDir(t, root, "node_modules/some-package")
	mkSkillDir(t, root, "vendor/some-lib")
	mkSkillDir(t, root, ".git/hooks")
	mkSkillDir(t, root, "real-package")

	got := discoverNestedSkillDirs(root)
	want := filepath.Join(root, "real-package", ".claude", "skills")
	if len(got) != 1 || got[0] != want {
		t.Fatalf("expected only %q, got %v", want, got)
	}
}

func TestDiscoverNestedSkillDirs_RespectsDepthLimit(t *testing.T) {
	root := t.TempDir()
	deep := ""
	for i := 0; i < maxNestedSkillDirDepth+3; i++ {
		deep = filepath.Join(deep, "d")
	}
	mkSkillDir(t, root, deep)

	got := discoverNestedSkillDirs(root)
	if len(got) != 0 {
		t.Fatalf("expected a directory beyond the depth limit to be excluded, got %v", got)
	}
}

func TestDiscoverNestedSkillDirs_EmptyCwd(t *testing.T) {
	if got := discoverNestedSkillDirs(""); got != nil {
		t.Fatalf("expected nil for an empty cwd, got %v", got)
	}
}

func TestCachedNestedSkillDirs_ServesFromCacheWithinTTL(t *testing.T) {
	root := t.TempDir()
	mkSkillDir(t, root, "packages/foo")

	first := cachedNestedSkillDirs(root)
	if len(first) != 1 {
		t.Fatalf("expected 1 nested dir, got %v", first)
	}

	// Add a second directory after the first call: within the TTL, the
	// cached result (without it) should still be served.
	mkSkillDir(t, root, "packages/bar")
	second := cachedNestedSkillDirs(root)
	if len(second) != 1 {
		t.Fatalf("expected the cache to still serve the stale 1-entry result within the TTL, got %v", second)
	}
}

func TestGetSkillDirCommandsForUser_IncludesNestedSkills(t *testing.T) {
	t.Setenv("SESHAT_RUNTIME_ROOT", t.TempDir())

	root := t.TempDir()
	skillDir := filepath.Join(root, "packages", "foo", ".claude", "skills", "nested-greet")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := "---\nname: \"nested-greet\"\ndescription: \"a nested skill\"\nuser-invocable: true\n---\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "skill.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill.md: %v", err)
	}
	// Force a fresh walk regardless of another test's cache entry for a
	// different temp root (TTL-based, keyed by cwd, so this is already
	// isolated, but a stale entry for the same path across sub-tests would
	// otherwise be possible if t.TempDir() ever reused a path).
	nestedSkillDirsCache.mu.Lock()
	nestedSkillDirsCache.cwd = ""
	nestedSkillDirsCache.mu.Unlock()

	loader := NewFileSkillLoader("")
	found, err := loader.GetSkillDirCommandsForUser(root, "")
	if err != nil {
		t.Fatalf("GetSkillDirCommandsForUser: %v", err)
	}
	for _, sk := range found {
		if sk.Name == "nested-greet" {
			return
		}
	}
	t.Fatalf("expected to find the nested skill 'nested-greet', got %d skills: %#v", len(found), found)
}
