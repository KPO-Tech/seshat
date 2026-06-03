package skillrepos

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// pullCooldown is the minimum interval between automatic git pulls for a given
// skill repo. Mirrors the gstack throttle (1× per hour).
const pullCooldown = time.Hour

// Repo describes a remote git repository that contains skills.
type Repo struct {
	// URL is the git clone URL.
	URL string
	// Name is the local directory name under the dest root (derived from URL if empty).
	Name string
}

// RepoFromURL creates a Repo from a raw git URL, deriving the name from the
// last path segment (e.g. "https://github.com/foo/paperasse" → name "paperasse").
func RepoFromURL(url string) Repo {
	url = strings.TrimSpace(url)
	name := filepath.Base(strings.TrimSuffix(url, ".git"))
	return Repo{URL: url, Name: name}
}

// ParseRepos parses a comma-separated list of git URLs into Repos.
func ParseRepos(csv string) []Repo {
	var repos []Repo
	for _, raw := range strings.Split(csv, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		repos = append(repos, RepoFromURL(raw))
	}
	return repos
}

// EnsureCloned clones repos that do not yet exist and pulls those that do,
// throttled to at most once per pullCooldown to avoid redundant network calls.
// Each repo is placed at destDir/<repo.Name>/.
// Errors are logged but not fatal — a partially updated repo is still useful.
func EnsureCloned(ctx context.Context, destDir string, repos []Repo) []string {
	var cloned []string
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		slog.Error("skillrepos: cannot create directory", "dir", destDir, "err", err)
		return nil
	}

	stampsDir := filepath.Join(destDir, ".pull-stamps")

	for _, repo := range repos {
		target := filepath.Join(destDir, repo.Name)
		if _, err := os.Stat(filepath.Join(target, ".git")); os.IsNotExist(err) {
			if err := cloneRepo(ctx, repo.URL, target); err != nil {
				slog.Warn("skillrepos: clone failed", "url", repo.URL, "err", err)
				continue
			}
			slog.Info("skillrepos: cloned", "url", repo.URL, "target", target)
			markPulled(stampsDir, repo.Name)
		} else {
			if shouldPull(stampsDir, repo.Name) {
				if err := pullRepo(ctx, target); err != nil {
					slog.Warn("skillrepos: pull failed", "target", target, "err", err)
					// keep going — stale but usable
				} else {
					slog.Info("skillrepos: updated", "target", target)
					markPulled(stampsDir, repo.Name)
				}
			}
		}
		cloned = append(cloned, target)
	}
	return cloned
}

// shouldPull returns true when the repo has not been pulled within pullCooldown.
func shouldPull(stampsDir, repoName string) bool {
	info, err := os.Stat(filepath.Join(stampsDir, repoName))
	if err != nil {
		return true
	}
	return time.Since(info.ModTime()) >= pullCooldown
}

// markPulled records the current time as the last successful pull for repoName.
func markPulled(stampsDir, repoName string) {
	if err := os.MkdirAll(stampsDir, 0o755); err != nil {
		return
	}
	p := filepath.Join(stampsDir, repoName)
	f, err := os.Create(p)
	if err != nil {
		return
	}
	f.Close()
}

func cloneRepo(ctx context.Context, url, dest string) error {
	cmd := exec.CommandContext(ctx, "git", "clone", "--depth=1", url, dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func pullRepo(ctx context.Context, dir string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "pull", "--ff-only", "--quiet")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
