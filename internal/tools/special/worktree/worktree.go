package worktree

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// WorktreeConfig is configuration for worktree tools
type WorktreeConfig struct {
	// UseGitWorktree uses git worktree (true) or custom hooks (false)
	UseGitWorktree bool

	// CreateBranch creates a new branch (true) or uses existing (false)
	CreateBranch bool

	// DeleteWorktree allows deleting worktrees
	DeleteWorktree bool

	// WorktreeBaseDir is the base directory for worktrees
	WorktreeBaseDir string
}

// WorktreeSession represents an active worktree session
type WorktreeSession struct {
	// WorktreePath is the worktree directory path
	WorktreePath string

	// WorktreeBranch is the branch name
	WorktreeBranch string

	// OriginalCwd is the original working directory
	OriginalCwd string

	// OriginalHeadCommit is the commit hash when worktree was created
	OriginalHeadCommit string

	// TmuxSessionName is the tmux session name (if any)
	TmuxSessionName string
}

// Per-session worktree state — keyed by SessionID, protected by sessionMu.
// Using a map prevents two concurrent sessions from colliding on a single
// global pointer while still being safe under the race detector.
var (
	sessionRegistry = map[types.SessionID]*WorktreeSession{}
	sessionMu       sync.RWMutex
)

// GetSession returns the active WorktreeSession for the given session ID,
// or nil if none exists.
func GetSession(id types.SessionID) *WorktreeSession {
	sessionMu.RLock()
	defer sessionMu.RUnlock()
	return sessionRegistry[id]
}

// SetSession stores (or removes, when s == nil) the WorktreeSession for id.
func SetSession(id types.SessionID, s *WorktreeSession) {
	sessionMu.Lock()
	defer sessionMu.Unlock()
	if s == nil {
		delete(sessionRegistry, id)
	} else {
		sessionRegistry[id] = s
	}
}

// WorktreeManager manages worktree sessions
type WorktreeManager struct {
	config WorktreeConfig
}

// NewWorktreeManager creates a new worktree manager
func NewWorktreeManager(config WorktreeConfig) *WorktreeManager {
	if config.WorktreeBaseDir == "" {
		config.WorktreeBaseDir = ".worktrees"
	}
	return &WorktreeManager{
		config: config,
	}
}

// FindGitRoot finds the canonical git root directory
func FindGitRoot(cwd string) (string, error) {
	// Walk up to find .git
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("not in a git repository")
}

// GenerateWorktreeName generates a worktree name from a slug
func GenerateWorktreeName(slug string) string {
	// Sanitize slug for directory name
	name := strings.ReplaceAll(slug, "/", "-")
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")

	// Truncate if too long
	if len(name) > 64 {
		name = name[:64]
	}

	return WorktreeDirPrefix + name
}

// CreateWorktree creates a new worktree
func (m *WorktreeManager) CreateWorktree(
	ctx context.Context,
	slug string,
	branch string,
	originalCwd string,
) (*WorktreeSession, error) {
	// Find git root
	gitRoot, err := FindGitRoot(originalCwd)
	if err != nil {
		return nil, fmt.Errorf("not in a git repository: %w", err)
	}

	// Generate worktree name
	worktreeName := GenerateWorktreeName(slug)
	worktreePath := filepath.Join(gitRoot, m.config.WorktreeBaseDir, worktreeName)

	// Get current head commit
	headCommit, err := m.getCurrentHeadCommit(gitRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to get current commit: %w", err)
	}

	// Determine branch name
	worktreeBranch := branch
	if worktreeBranch == "" {
		worktreeBranch = "worktree-" + slug
	}

	// Create the worktree using git
	if err := m.createGitWorktree(gitRoot, worktreePath, worktreeBranch); err != nil {
		return nil, fmt.Errorf("failed to create worktree: %w", err)
	}

	session := &WorktreeSession{
		WorktreePath:       worktreePath,
		WorktreeBranch:     worktreeBranch,
		OriginalCwd:        originalCwd,
		OriginalHeadCommit: headCommit,
	}

	return session, nil
}

// getCurrentHeadCommit gets the current HEAD commit hash
func (m *WorktreeManager) getCurrentHeadCommit(gitRoot string) (string, error) {
	cmd := exec.Command("git", "-C", gitRoot, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// createGitWorktree creates a git worktree
func (m *WorktreeManager) createGitWorktree(gitRoot, worktreePath, branch string) error {
	// Create parent directory
	parentDir := filepath.Dir(worktreePath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Create the worktree
	cmd := exec.Command("git", "-C", gitRoot, "worktree", "add", "-b", branch, worktreePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		// If branch already exists, try checking it out
		if strings.Contains(string(out), "branch already exists") {
			return m.checkoutWorktreeBranch(gitRoot, worktreePath, branch)
		}
		return fmt.Errorf("git worktree add failed: %w, output: %s", err, string(out))
	}

	return nil
}

// checkoutWorktreeBranch checks out an existing branch in a worktree path
func (m *WorktreeManager) checkoutWorktreeBranch(gitRoot, worktreePath, branch string) error {
	// Remove the directory if it exists (git will fail otherwise)
	if _, err := os.Stat(worktreePath); err == nil {
		if err := os.RemoveAll(worktreePath); err != nil {
			return fmt.Errorf("failed to remove existing path: %w", err)
		}
	}

	cmd := exec.Command("git", "-C", gitRoot, "worktree", "add", worktreePath, branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add failed: %w, output: %s", err, string(out))
	}

	return nil
}

// RemoveWorktree removes the given worktree from disk.
// The caller is responsible for clearing the session registry entry.
func (m *WorktreeManager) RemoveWorktree(session *WorktreeSession, force bool) error {
	if session == nil {
		return fmt.Errorf("no active worktree session")
	}

	// Check for uncommitted changes
	if !force {
		hasChanges, err := m.hasUncommittedChanges(session.WorktreePath)
		if err != nil {
			return err
		}
		if hasChanges {
			return fmt.Errorf("worktree has uncommitted changes, use discard_changes: true to remove")
		}
	}

	// Get git root
	gitRoot, err := FindGitRoot(session.WorktreePath)
	if err != nil {
		return err
	}

	// Remove the worktree
	cmd := exec.Command("git", "-C", gitRoot, "worktree", "remove", session.WorktreePath, "--force")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to remove worktree: %w, output: %s", err, string(out))
	}

	return nil
}

// hasUncommittedChanges checks if worktree has uncommitted changes
func (m *WorktreeManager) hasUncommittedChanges(worktreePath string) (bool, error) {
	cmd := exec.Command("git", "-C", worktreePath, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("git status failed: %w", err)
	}
	return len(strings.TrimSpace(string(out))) > 0, nil
}

// CountWorktreeChanges counts changed files and commits
func (m *WorktreeManager) CountWorktreeChanges(worktreePath, originalCommit string) (int, int, error) {
	// Count uncommitted files
	cmd := exec.Command("git", "-C", worktreePath, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("git status failed: %w", err)
	}
	changedFiles := len(strings.Split(strings.TrimSpace(string(out)), "\n"))
	if changedFiles > 0 && strings.TrimSpace(string(out)) == "" {
		changedFiles = 0
	}

	// Count commits ahead of original
	commits := 0
	if originalCommit != "" {
		cmd = exec.Command("git", "-C", worktreePath, "rev-list", "--count", originalCommit+"..HEAD")
		out, err = cmd.Output()
		if err == nil {
			lines := strings.TrimSpace(string(out))
			if lines != "" {
				fmt.Sscanf(lines, "%d", &commits)
			}
		}
	}

	return changedFiles, commits, nil
}

// ValidateWorktreeSlug validates a worktree name/slug
func ValidateWorktreeSlug(slug string) error {
	if len(slug) > 64 {
		return fmt.Errorf("slug too long (max 64 characters)")
	}

	// Check for invalid characters
	validChars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-/"
	for _, c := range slug {
		if !strings.ContainsRune(validChars, c) {
			return fmt.Errorf("invalid character %q - only letters, digits, dots, underscores, dashes, and slashes allowed", c)
		}
	}

	return nil
}
