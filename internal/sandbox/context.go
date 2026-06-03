package sandbox

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/workspace"
)

// Context carries the execution boundary relevant to sandbox decisions.
type Context struct {
	WorkingDirectory string
	WorkspaceRoot    string
	AdditionalRoots  []string
	Environment      EnvironmentKind
	SandboxEnabled   bool
}

// ResolvePath resolves a candidate path according to the current execution context.
func (c Context) ResolvePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is required")
	}

	if strings.TrimSpace(c.WorkspaceRoot) != "" {
		return workspace.Resolve(path, c.WorkspaceRoot)
	}

	if !filepath.IsAbs(path) {
		base := strings.TrimSpace(c.WorkingDirectory)
		if base == "" {
			abs, err := filepath.Abs(path)
			if err != nil {
				return "", fmt.Errorf("resolve path %q: %w", path, err)
			}
			path = abs
		} else {
			path = filepath.Join(base, path)
		}
	}

	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", path, err)
	}
	return abs, nil
}
