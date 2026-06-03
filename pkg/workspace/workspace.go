package workspace

import internalworkspace "github.com/EngineerProjects/nexus-engine/internal/workspace"

const (
	SubdirUploads   = internalworkspace.SubdirUploads
	SubdirImages    = internalworkspace.SubdirImages
	SubdirDocuments = internalworkspace.SubdirDocuments
	SubdirOther     = internalworkspace.SubdirOther
	SubdirPlans     = internalworkspace.SubdirPlans
	SubdirArtifacts = internalworkspace.SubdirArtifacts
)

type Context = internalworkspace.Context

func New(root string) (*Context, error) {
	return internalworkspace.New(root)
}

func DefaultPath(sessionID string) (string, error) {
	return internalworkspace.DefaultPath(sessionID)
}

func EnsureDir(path string) error {
	return internalworkspace.EnsureDir(path)
}
