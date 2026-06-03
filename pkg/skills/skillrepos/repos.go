package skillrepos

import (
	"context"

	internalrepos "github.com/EngineerProjects/nexus-engine/internal/tools/system/skills/skillrepos"
)

type Repo = internalrepos.Repo

func RepoFromURL(url string) Repo {
	return internalrepos.RepoFromURL(url)
}

func ParseRepos(csv string) []Repo {
	return internalrepos.ParseRepos(csv)
}

func EnsureCloned(ctx context.Context, destDir string, repos []Repo) []string {
	return internalrepos.EnsureCloned(ctx, destDir, repos)
}
