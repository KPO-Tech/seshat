package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	SubdirUploads   = "uploads"
	SubdirImages    = "uploads/images"
	SubdirDocuments = "uploads/documents"
	SubdirOther     = "uploads/other"
	SubdirPlans     = "plans"
	SubdirArtifacts = "artifacts"
)

// Context represents the filesystem boundary for one agent session.
type Context struct {
	Root string `json:"root"`
}

// New resolves, cleans, and creates a workspace root.
func New(root string) (*Context, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("workspace root is required")
	}
	abs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}
	if err := EnsureDir(abs); err != nil {
		return nil, err
	}
	return &Context{Root: abs}, nil
}

// Resolve resolves rel against the workspace root and validates the result.
func (c *Context) Resolve(rel string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("workspace context is not configured")
	}
	return Resolve(rel, c.Root)
}

// EnsureSubdirs creates the standard subdirectory layout inside the workspace.
// Safe to call multiple times; existing directories are left untouched.
func (c *Context) EnsureSubdirs() error {
	if c == nil {
		return fmt.Errorf("workspace context is not configured")
	}
	for _, sub := range []string{SubdirImages, SubdirDocuments, SubdirOther, SubdirPlans, SubdirArtifacts} {
		if err := os.MkdirAll(filepath.Join(c.Root, sub), 0o700); err != nil {
			return fmt.Errorf("create workspace subdir %s: %w", sub, err)
		}
	}
	return nil
}

// UploadsPath returns the absolute path to the uploads sub-category directory.
// category should be one of "images", "documents", or "other".
func (c *Context) UploadsPath(category string) string {
	if category == "" {
		category = "other"
	}
	return filepath.Join(c.Root, SubdirUploads, filepath.Clean(category))
}

// PlansPath returns the absolute path to the plans directory.
func (c *Context) PlansPath() string { return filepath.Join(c.Root, SubdirPlans) }

// ArtifactsPath returns the absolute path to the artifacts directory.
func (c *Context) ArtifactsPath() string { return filepath.Join(c.Root, SubdirArtifacts) }

// Validate verifies that abs is within the workspace root.
func (c *Context) Validate(abs string) error {
	if c == nil {
		return fmt.Errorf("workspace context is not configured")
	}
	return Validate(abs, c.Root)
}

// DefaultPath returns the default workspace path for a session without creating it.
func DefaultPath(sessionID string) (string, error) {
	base, err := baseDir()
	if err != nil {
		return "", err
	}
	return filepath.Abs(filepath.Join(base, filepath.Clean(sessionID)))
}

// EnsureDir creates the workspace directory if it doesn't exist.
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0o700)
}

// Validate checks that absPath is within workspaceRoot.
// Returns an error if the path would escape the workspace via directory
// traversal or a symlink whose target lies outside the workspace.
func Validate(absPath, workspaceRoot string) error {
	clean, err := filepath.Abs(filepath.Clean(absPath))
	if err != nil {
		return fmt.Errorf("resolve path %q: %w", absPath, err)
	}
	root, err := filepath.Abs(filepath.Clean(workspaceRoot))
	if err != nil {
		return fmt.Errorf("resolve workspace %q: %w", workspaceRoot, err)
	}
	// Resolve symlinks on both sides so a symlinked component cannot smuggle the
	// path outside the workspace (e.g. a symlink created *inside* the workspace
	// pointing at /etc). The path is resolved leniently because it may not exist
	// yet (file creation). Resolving the root too means a symlinked root prefix
	// (e.g. macOS /var -> /private/var) cancels out on both sides.
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		rootResolved = root
	}
	cleanResolved, err := resolveSymlinksLenient(clean)
	if err != nil {
		return fmt.Errorf("resolve path %q: %w", absPath, err)
	}
	rel, err := filepath.Rel(rootResolved, cleanResolved)
	if err != nil {
		return fmt.Errorf("path %q is outside workspace %q", absPath, workspaceRoot)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q escapes workspace %q", absPath, workspaceRoot)
	}
	return nil
}

// resolveSymlinksLenient resolves symlinks on the longest existing prefix of
// path, then re-appends the remaining (not-yet-existing) components verbatim.
// This lets callers validate paths for files that do not exist yet (writes)
// while still catching a symlinked ancestor that escapes the workspace.
func resolveSymlinksLenient(path string) (string, error) {
	path = filepath.Clean(path)
	current := path
	remainder := ""
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			if remainder == "" {
				return resolved, nil
			}
			return filepath.Join(resolved, remainder), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			// Reached the filesystem root without an existing prefix to resolve.
			return path, nil
		}
		remainder = filepath.Join(filepath.Base(current), remainder)
		current = parent
	}
}

// Resolve resolves path against workspaceRoot.
// Relative paths are joined; absolute paths are validated.
// Returns an error if the resolved path escapes the workspace.
func Resolve(path, workspaceRoot string) (string, error) {
	root, err := filepath.Abs(filepath.Clean(workspaceRoot))
	if err != nil {
		return "", fmt.Errorf("resolve workspace %q: %w", workspaceRoot, err)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path, err = filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", path, err)
	}
	if err := Validate(path, root); err != nil {
		return "", err
	}
	return path, nil
}

func baseDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "nexus", "workspaces"), nil
}
