package managed

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// EnsureExtracted extracts builtin skills to destDir if they have not yet been
// extracted at the current Version.  It is safe to call on every boot — it is
// a no-op when the version stamp already matches.
func EnsureExtracted(destDir string) error {
	versionFile := filepath.Join(destDir, ".builtin-version")
	if data, err := os.ReadFile(versionFile); err == nil {
		if strings.TrimSpace(string(data)) == Version {
			return nil
		}
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("managed skills: create dest dir: %w", err)
	}

	if err := fs.WalkDir(FS, "builtin", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(path, "builtin/")
		if rel == "" || rel == "builtin" {
			return nil
		}
		dest := filepath.Join(destDir, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		content, err := FS.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, content, 0o644)
	}); err != nil {
		return fmt.Errorf("managed skills: extract: %w", err)
	}

	return os.WriteFile(versionFile, []byte(Version), 0o644)
}
