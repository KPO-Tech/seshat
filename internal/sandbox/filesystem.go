package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FilesystemPolicy centralizes common filesystem access checks.
type FilesystemPolicy struct {
	readDeniedPrefixes  []string
	writeDeniedPrefixes []string
}

func NewDefaultFilesystemPolicy() *FilesystemPolicy {
	return &FilesystemPolicy{
		readDeniedPrefixes: []string{
			"/boot",
			"/sys",
			"/proc/sys",
		},
		writeDeniedPrefixes: []string{
			"/boot",
			"/sys",
			"/proc/sys",
			"/etc",
			"/usr/bin",
			"/usr/sbin",
			"/bin",
			"/sbin",
		},
	}
}

func (p *FilesystemPolicy) EvaluatePath(ctx Context, path string, access AccessKind) (PathDecision, error) {
	if p == nil {
		p = NewDefaultFilesystemPolicy()
	}

	resolvedPath, err := ctx.ResolvePath(path)
	if err != nil {
		return PathDecision{}, err
	}

	switch access {
	case AccessRead:
		if err := requireExistingPath(resolvedPath); err != nil {
			return PathDecision{}, err
		}
		if prefix := matchingPrefix(resolvedPath, p.readDeniedPrefixes); prefix != "" {
			return denyPath(resolvedPath, fmt.Sprintf("read access denied for protected path prefix %q", prefix)), nil
		}
	case AccessSearch:
		info, err := os.Stat(resolvedPath)
		if err != nil {
			return PathDecision{}, err
		}
		if !info.IsDir() {
			return PathDecision{}, fmt.Errorf("not a directory: %s", resolvedPath)
		}
		if prefix := matchingPrefix(resolvedPath, p.readDeniedPrefixes); prefix != "" {
			return denyPath(resolvedPath, fmt.Sprintf("search access denied for protected path prefix %q", prefix)), nil
		}
	case AccessWrite, AccessCreate, AccessDelete:
		if prefix := matchingPrefix(resolvedPath, p.writeDeniedPrefixes); prefix != "" {
			return denyPath(resolvedPath, fmt.Sprintf("write access denied for protected path prefix %q", prefix)), nil
		}
	default:
		return PathDecision{}, fmt.Errorf("unsupported filesystem access kind: %s", access)
	}

	return PathDecision{
		DecisionResult: DecisionResult{
			Decision: DecisionAllow,
			Reason:   "path allowed by filesystem policy",
		},
		ResolvedPath: resolvedPath,
	}, nil
}

func requireExistingPath(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	return nil
}

func denyPath(path string, reason string) PathDecision {
	return PathDecision{
		DecisionResult: DecisionResult{
			Decision: DecisionDeny,
			Reason:   reason,
		},
		ResolvedPath: path,
	}
}

func matchingPrefix(path string, prefixes []string) string {
	for _, prefix := range prefixes {
		if hasPathPrefix(path, prefix) {
			return prefix
		}
	}
	return ""
}

func hasPathPrefix(path string, prefix string) bool {
	path = filepath.Clean(path)
	prefix = filepath.Clean(prefix)
	if path == prefix {
		return true
	}
	return strings.HasPrefix(path, prefix+string(filepath.Separator))
}
