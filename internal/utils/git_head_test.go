package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func writeGitDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// A HEAD file comes with the repository: the ref it names is looked up in the git directory, never outside of it.
func TestReadGitHead(t *testing.T) {
	t.Parallel()
	sha := "0123456789abcdef0123456789abcdef01234567"
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("not a commit, a secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	relToOutside := func(gitDir string) string {
		rel, err := filepath.Rel(gitDir, outside)
		if err != nil {
			t.Fatal(err)
		}
		return filepath.ToSlash(rel)
	}
	backslash := string(rune(92))

	cases := []struct {
		name   string
		head   func(gitDir string) string
		files  map[string]string
		want   string
		wantOK bool
	}{
		{"a branch", func(string) string { return "ref: refs/heads/main\n" }, map[string]string{"refs/heads/main": sha + "\n"}, sha, true},
		{"detached", func(string) string { return sha + "\n" }, nil, sha, true},
		{"a ref in packed-refs", func(string) string { return "ref: refs/heads/dev\n" }, map[string]string{"packed-refs": "# pack-refs\n" + sha + " refs/heads/dev\n"}, sha, true},
		{"a ref that leaves the git directory", func(gitDir string) string { return "ref: refs/heads/../../" + relToOutside(gitDir) + "\n" }, nil, "", false},
		{"a relative path that is not under refs/", func(gitDir string) string { return "ref: " + relToOutside(gitDir) + "\n" }, nil, "", false},
		{"an absolute path", func(string) string { return "ref: " + filepath.ToSlash(outside) + "\n" }, nil, "", false},
		{"a backslash", func(string) string { return "ref: refs" + backslash + "heads" + backslash + "main\n" }, nil, "", false},
		{"a drive letter", func(string) string { return "ref: refs/C:/x\n" }, nil, "", false},
	}
	for _, tc := range cases {
		dir := writeGitDir(t, tc.files)
		if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte(tc.head(dir)), 0o600); err != nil {
			t.Fatal(err)
		}
		got, ok := ReadGitHead(dir)
		if ok != tc.wantOK || got != tc.want {
			t.Errorf("%s: ReadGitHead = (%q, %v), want (%q, %v)", tc.name, got, ok, tc.want, tc.wantOK)
		}
	}
}
