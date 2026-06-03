package shared

import "context"

// GetFileReadIgnorePatterns returns patterns to ignore during file reads.
func GetFileReadIgnorePatterns(_ context.Context) []string {
	return []string{
		"node_modules",
		".next",
		".nuxt",
		"dist",
		"build",
		"target",
		"bin",
		"obj",
		".venv",
		"venv",
		"__pycache__",
		"*.pyc",
	}
}

// GetVCSDirectoriesToExclude returns VCS directories to exclude from searches.
func GetVCSDirectoriesToExclude() []string {
	return []string{".git", ".svn", ".hg", ".bzr", ".jj", ".sl"}
}
