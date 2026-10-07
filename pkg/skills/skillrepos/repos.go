package skillrepos

import (
	"context"

	internalrepos "github.com/KPO-Tech/seshat/internal/tools/system/skills/skillrepos"
)

// Repo describes a remote git repository that contains skills.
type Repo = internalrepos.Repo

// RepoFromURL creates a Repo from a raw git URL, deriving the name from the
// last path segment (e.g. "https://github.com/foo/paperasse" → name "paperasse").
func RepoFromURL(url string) Repo {
	return internalrepos.RepoFromURL(url)
}

// ParseRepos parses a comma-separated list of git URLs into Repos.
func ParseRepos(csv string) []Repo {
	return internalrepos.ParseRepos(csv)
}

// EnsureCloned clones repos that do not yet exist and pulls those that do,
// throttled to at most once per pullCooldown to avoid redundant network calls.
// Each repo is placed at destDir/<repo.Name>/.
// Errors are logged but not fatal — a partially updated repo is still useful.
func EnsureCloned(ctx context.Context, destDir string, repos []Repo) []string {
	return internalrepos.EnsureCloned(ctx, destDir, repos)
}
