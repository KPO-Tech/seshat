package workspace

import internalworkspace "github.com/KPO-Tech/seshat/internal/workspace"

const (
	SubdirUploads   = internalworkspace.SubdirUploads
	SubdirImages    = internalworkspace.SubdirImages
	SubdirDocuments = internalworkspace.SubdirDocuments
	SubdirOther     = internalworkspace.SubdirOther
	SubdirPlans     = internalworkspace.SubdirPlans
	SubdirArtifacts = internalworkspace.SubdirArtifacts
)

// Context represents the filesystem boundary for one agent session.
type Context = internalworkspace.Context

// New resolves, cleans, and creates a workspace root.
func New(root string) (*Context, error) {
	return internalworkspace.New(root)
}

// DefaultPath returns the default workspace path for a session without creating it.
func DefaultPath(sessionID string) (string, error) {
	return internalworkspace.DefaultPath(sessionID)
}

// EnsureDir creates the workspace directory if it doesn't exist.
func EnsureDir(path string) error {
	return internalworkspace.EnsureDir(path)
}
