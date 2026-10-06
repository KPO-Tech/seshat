package runtimepath

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestEverythingOfASessionLivesInItsWorkspaceDirectory(t *testing.T) {
	root := t.TempDir()
	dir := SessionDir(root, "abc")
	if dir != filepath.Join(root, "workspaces", "abc") {
		t.Fatalf("session dir = %s", dir)
	}
	for name, got := range map[string]string{
		"plans":       SessionPlansDir(root, "abc"),
		"pastes":      SessionPastesDir(root, "abc"),
		"tools":       SessionToolsDir(root, "abc"),
		"artifacts":   SessionArtifactsDir(root, "abc"),
		"screenshots": SessionScreenshotsDir(root, "abc"),
		"log":         SessionLogPath(root, "abc"),
	} {
		if rel, err := filepath.Rel(dir, got); err != nil || filepath.IsAbs(rel) || rel == ".." || len(rel) > 1 && rel[:2] == ".." {
			t.Errorf("%s (%s) is not inside the session directory", name, got)
		}
	}
	if SessionsDir(root) != WorkspacesDir(root) {
		t.Error("the sessions directory is the workspaces directory")
	}
}

func TestPermissionGrantsAreOutsideTheDirectoryTheAgentCanWrite(t *testing.T) {
	root := t.TempDir()
	grants := SessionPermissionsPath(root, "abc")
	if rel, err := filepath.Rel(SessionDir(root, "abc"), grants); err == nil && len(rel) > 0 && rel[0] != '.' {
		t.Fatalf("grants %s are inside the session directory", grants)
	}
	if grants != filepath.Join(root, "data", "permissions", "abc.json") {
		t.Fatalf("grants path = %s", grants)
	}
}

func TestRemoveSessionDataDeletesTheDirectoryAndTheGrants(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(SessionPlansDir(root, "abc"), "plan.md"), "plan")
	write(t, filepath.Join(SessionDir(root, "abc"), "uploads", "a.pdf"), "pdf")
	write(t, SessionPermissionsPath(root, "abc"), "{}")
	write(t, filepath.Join(SessionDir(root, "other"), "keep.txt"), "keep")

	if err := RemoveSessionData(root, "abc"); err != nil {
		t.Fatalf("RemoveSessionData: %v", err)
	}
	if exists(SessionDir(root, "abc")) || exists(SessionPermissionsPath(root, "abc")) {
		t.Fatal("the session data should be gone")
	}
	if !exists(filepath.Join(SessionDir(root, "other"), "keep.txt")) {
		t.Fatal("another session must be left alone")
	}
	if err := RemoveSessionData(root, "abc"); err != nil {
		t.Fatalf("removing what is already gone is not an error: %v", err)
	}
}

func TestRemoveSessionDataRefusesAnIdThatIsAPath(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "data", "precious.txt"), "precious")
	for _, id := range []string{"", ".", "..", "../data", `..\data`, "a/b", "/abs"} {
		if err := RemoveSessionData(root, id); err == nil {
			t.Errorf("%q must be refused", id)
		}
	}
	if !exists(filepath.Join(root, "data", "precious.txt")) {
		t.Fatal("nothing outside a session directory may be deleted")
	}
}

func TestMigrationMovesALegacyDirectoryAndItsGrants(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(LegacySessionsDir(root), "abc", "plans", "plan.md"), "plan")
	write(t, filepath.Join(LegacySessionsDir(root), "abc", "permissions.json"), `{"Bash":true}`)

	moved, err := MigrateLegacySessionDirs(root)
	if err != nil || moved != 1 {
		t.Fatalf("moved=%d err=%v", moved, err)
	}
	if read(t, filepath.Join(SessionPlansDir(root, "abc"), "plan.md")) != "plan" {
		t.Fatal("the plan should be in the new directory")
	}
	if read(t, SessionPermissionsPath(root, "abc")) != `{"Bash":true}` {
		t.Fatal("the grants should be in their own place")
	}
	if exists(filepath.Join(SessionDir(root, "abc"), "permissions.json")) {
		t.Fatal("grants must not be left inside the agent's directory")
	}
	if exists(LegacySessionsDir(root)) {
		t.Fatal("the legacy directory should be gone once it is empty")
	}
}

func TestMigrationMergesWithoutOverwriting(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(LegacySessionsDir(root), "abc", "plans", "old.md"), "old")
	write(t, filepath.Join(LegacySessionsDir(root), "abc", "plans", "same.md"), "legacy")
	write(t, filepath.Join(SessionPlansDir(root, "abc"), "same.md"), "current")
	write(t, SessionPermissionsPath(root, "abc"), `{"current":true}`)
	write(t, filepath.Join(LegacySessionsDir(root), "abc", "permissions.json"), `{"legacy":true}`)

	if _, err := MigrateLegacySessionDirs(root); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if read(t, filepath.Join(SessionPlansDir(root, "abc"), "same.md")) != "current" {
		t.Fatal("an existing file must not be overwritten")
	}
	if read(t, filepath.Join(SessionPlansDir(root, "abc"), "old.md")) != "old" {
		t.Fatal("a file that was not there should arrive")
	}
	if read(t, SessionPermissionsPath(root, "abc")) != `{"current":true}` {
		t.Fatal("existing grants must not be overwritten")
	}
	if !exists(filepath.Join(LegacySessionsDir(root), "abc", "plans", "same.md")) {
		t.Fatal("what could not be moved stays where it was")
	}
}

func TestMigrationIsSafeToRunAgainAndWithNothingToMigrate(t *testing.T) {
	root := t.TempDir()
	if moved, err := MigrateLegacySessionDirs(root); err != nil || moved != 0 {
		t.Fatalf("nothing to migrate: moved=%d err=%v", moved, err)
	}
	write(t, filepath.Join(LegacySessionsDir(root), "abc", "plans", "plan.md"), "plan")
	if _, err := MigrateLegacySessionDirs(root); err != nil {
		t.Fatal(err)
	}
	if moved, err := MigrateLegacySessionDirs(root); err != nil || moved != 0 {
		t.Fatalf("second run: moved=%d err=%v", moved, err)
	}
	if read(t, filepath.Join(SessionPlansDir(root, "abc"), "plan.md")) != "plan" {
		t.Fatal("the data must still be there")
	}
}

func TestMigrationLeavesFilesAndOddNamesAlone(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(LegacySessionsDir(root), "stray.txt"), "stray")
	if _, err := MigrateLegacySessionDirs(root); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(LegacySessionsDir(root), "stray.txt")) {
		t.Fatal("a file in the legacy directory is not a session and must stay")
	}
}
