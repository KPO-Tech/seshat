package utils

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const gitRootNotFound = "GIT_ROOT_NOT_FOUND"

type gitRootResult struct {
	root  string
	found bool
}

var (
	gitRootCache       = newLRUCache(50)
	canonicalRootCache = newLRUCache(50)
	gitDirCache        = newLRUCache(50)
	stateCache         = newCache()
)

type LRUCache struct {
	mu     sync.RWMutex
	data   map[string]*gitRootResult
	maxLen int
}

func newLRUCache(maxLen int) *LRUCache {
	return &LRUCache{
		data:   make(map[string]*gitRootResult),
		maxLen: maxLen,
	}
}

func (c *LRUCache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if val, ok := c.data[key]; ok {
		if val.root == gitRootNotFound {
			return "", false
		}
		return val.root, val.found
	}
	return "", false
}

func (c *LRUCache) Set(key string, root string, found bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.data) >= c.maxLen {
		for k := range c.data {
			delete(c.data, k)
			break
		}
	}

	if !found {
		c.data[key] = &gitRootResult{root: gitRootNotFound, found: false}
	} else {
		c.data[key] = &gitRootResult{root: root, found: true}
	}
}

type cacheEntry struct {
	mu            sync.RWMutex
	branch        string
	head          string
	defaultBranch string
	remoteUrl     string
	lastCheck     time.Time
}

type cache struct {
	mu    sync.RWMutex
	cache map[string]*cacheEntry
}

func newCache() *cache {
	return &cache{
		cache: make(map[string]*cacheEntry),
	}
}

func (c *cache) get(root string) *cacheEntry {
	c.mu.RLock()
	if state, ok := c.cache[root]; ok {
		c.mu.RUnlock()
		return state
	}
	c.mu.RUnlock()

	c.mu.Lock()
	if state, ok := c.cache[root]; ok {
		c.mu.Unlock()
		return state
	}
	state := &cacheEntry{}
	c.cache[root] = state
	c.mu.Unlock()
	return state
}

type Context struct {
	Root       string
	Branch     string
	Head       string
	IsGit      bool
	IsWorktree bool
}

func Detect(dir string) *Context {
	gitRoot := FindGitRoot(dir)
	if gitRoot == "" {
		return &Context{IsGit: false}
	}

	canonicalRoot := FindCanonicalGitRoot(dir)
	branch := GetCachedBranch(canonicalRoot)
	head := GetCachedHead(canonicalRoot)
	isWorktree := isWorktree(gitRoot)

	return &Context{
		Root:       canonicalRoot,
		Branch:     branch,
		Head:       head,
		IsGit:      true,
		IsWorktree: isWorktree,
	}
}

func FindGitRoot(startDir string) string {
	if cached, found := gitRootCache.Get(startDir); found {
		return cached
	}

	root := findGitRootImpl(startDir)
	gitRootCache.Set(startDir, root, root != "")
	return root
}

func findGitRootImpl(startDir string) string {
	absPath, err := filepath.Abs(startDir)
	if err != nil {
		return ""
	}

	absPath = filepath.Clean(absPath)
	root := filepath.VolumeName(absPath)
	if root == "" {
		root = string(filepath.Separator)
	}

	current := absPath
	for {
		gitPath := filepath.Join(current, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			if isGitDirOrFile(gitPath) {
				return filepath.Clean(current)
			}
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	rootPath := filepath.Join(root, ".git")
	if _, err := os.Stat(rootPath); err == nil {
		if isGitDirOrFile(rootPath) {
			return filepath.Clean(root)
		}
	}

	return ""
}

func isGitDirOrFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir() || info.Mode().IsRegular()
}

func FindCanonicalGitRoot(startDir string) string {
	if cached, found := canonicalRootCache.Get(startDir); found {
		return cached
	}

	gitRoot := FindGitRoot(startDir)
	if gitRoot == "" {
		canonicalRootCache.Set(startDir, "", false)
		return ""
	}

	canonical := resolveCanonicalRoot(gitRoot)
	canonicalRootCache.Set(startDir, canonical, canonical != "")
	return canonical
}

func resolveCanonicalRoot(gitRoot string) string {
	gitPath := filepath.Join(gitRoot, ".git")

	if _, err := os.Stat(gitPath); err != nil {
		return gitRoot
	}

	if isFile(gitPath) {
		content, err := os.ReadFile(gitPath)
		if err != nil {
			return gitRoot
		}

		trimmed := strings.TrimSpace(string(content))
		if !strings.HasPrefix(trimmed, "gitdir:") {
			return gitRoot
		}

		worktreeGitDir := filepath.Clean(filepath.Join(gitRoot, strings.TrimPrefix(trimmed, "gitdir:")))

		commondirPath := filepath.Join(worktreeGitDir, "commondir")
		commondirContent, err := os.ReadFile(commondirPath)
		if err != nil {
			return gitRoot
		}

		commonDir := filepath.Clean(filepath.Join(worktreeGitDir, strings.TrimSpace(string(commondirContent))))

		if !isSecureWorktreeLink(gitRoot, worktreeGitDir, commonDir) {
			return gitRoot
		}

		if filepath.Base(commonDir) != ".git" {
			return commonDir
		}
		return filepath.Dir(commonDir)
	}

	return gitRoot
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular()
}

func isSecureWorktreeLink(gitRoot, worktreeGitDir, commonDir string) bool {
	worktreeParent := filepath.Dir(worktreeGitDir)
	expectedWorktreesDir := filepath.Join(filepath.Dir(commonDir), "worktrees")

	if filepath.Clean(worktreeParent) != filepath.Clean(expectedWorktreesDir) {
		return false
	}

	gitdirPath := filepath.Join(worktreeGitDir, "gitdir")
	backlinkContent, err := os.ReadFile(gitdirPath)
	if err != nil {
		return false
	}

	backlink := strings.TrimSpace(string(backlinkContent))
	if !strings.HasPrefix(backlink, "gitdir:") {
		return false
	}

	actualBacklink := filepath.Clean(filepath.Join(gitRoot, strings.TrimPrefix(backlink, "gitdir:")))
	expectedBacklink := filepath.Join(gitRoot, ".git")

	return filepath.Clean(actualBacklink) == filepath.Clean(expectedBacklink)
}

func isWorktree(gitRoot string) bool {
	gitPath := filepath.Join(gitRoot, ".git")
	if info, err := os.Stat(gitPath); err == nil {
		return info.Mode().IsRegular()
	}
	return false
}

func ResolveGitDir(startDir string) string {
	if cached, found := gitDirCache.Get(startDir); found {
		return cached
	}

	gitRoot := FindGitRoot(startDir)
	if gitRoot == "" {
		gitDirCache.Set(startDir, "", false)
		return ""
	}

	gitPath := filepath.Join(gitRoot, ".git")
	if info, err := os.Stat(gitPath); err == nil && !info.IsDir() {
		content, err := os.ReadFile(gitPath)
		if err == nil {
			trimmed := strings.TrimSpace(string(content))
			if strings.HasPrefix(trimmed, "gitdir:") {
				rawDir := strings.TrimPrefix(trimmed, "gitdir:")
				resolved := filepath.Clean(filepath.Join(startDir, strings.TrimSpace(rawDir)))
				gitDirCache.Set(startDir, resolved, true)
				return resolved
			}
		}
	}

	gitDirCache.Set(startDir, gitPath, true)
	return gitPath
}

func GetIsGit(dir string) bool {
	return FindGitRoot(dir) != ""
}

func ReadGitHead(gitDir string) (string, bool) {
	headPath := filepath.Join(gitDir, "HEAD")
	content, err := os.ReadFile(headPath)
	if err != nil {
		return "", false
	}

	trimmed := strings.TrimSpace(string(content))

	if strings.HasPrefix(trimmed, "ref: ") {
		ref := strings.TrimPrefix(trimmed, "ref: ")
		ref = strings.TrimSpace(ref)

		refFile := filepath.Join(gitDir, ref)
		if content, err := os.ReadFile(refFile); err == nil {
			return strings.TrimSpace(string(content)), true
		}

		packedRefsPath := filepath.Join(gitDir, "packed-refs")
		if content, err := os.ReadFile(packedRefsPath); err == nil {
			for _, line := range strings.Split(string(content), "\n") {
				if strings.HasSuffix(line, " "+ref) {
					parts := strings.SplitN(line, " ", 2)
					if len(parts) == 2 && isValidGitSha(parts[0]) {
						return parts[0], true
					}
				}
			}
		}

		return "", false
	}

	if isValidGitSha(trimmed) {
		return trimmed, true
	}

	return "", false
}

func GetCachedBranch(root string) string {
	if root == "" {
		return ""
	}

	entry := stateCache.get(root)
	if branch, _ := entry.getBranch(); branch != "" {
		return branch
	}

	gitDir := ResolveGitDir(root)
	if gitDir == "" {
		return ""
	}

	branch, found := ReadGitHead(gitDir)
	if found && branch != "" {
		entry.setBranch(extractBranchName(gitDir, branch))
		return entry.branch
	}

	return "HEAD"
}

func GetCachedHead(root string) string {
	if root == "" {
		return ""
	}

	entry := stateCache.get(root)
	if head, _ := entry.getHead(); head != "" {
		return head
	}

	gitDir := ResolveGitDir(root)
	if gitDir == "" {
		return ""
	}

	head, found := ReadGitHead(gitDir)
	if found {
		entry.setHead(head)
	}

	return head
}

func extractBranchName(gitDir, shaOrRef string) string {
	if isValidGitSha(shaOrRef) {
		return shaOrRef[:7]
	}

	refsPath := filepath.Join(gitDir, shaOrRef)
	content, err := os.ReadFile(refsPath)
	if err != nil {
		return shaOrRef[:7]
	}

	return strings.TrimSpace(string(content))[:7]
}

func GetCachedDefaultBranch(root string) string {
	if root == "" {
		return ""
	}

	entry := stateCache.get(root)

	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "origin/HEAD")
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err == nil {
		defaultBranch := strings.TrimSpace(stdout.String())
		defaultBranch = strings.TrimPrefix(defaultBranch, "origin/")
		entry.defaultBranch = defaultBranch
		return defaultBranch
	}

	return entry.branch
}

func GetCachedRemoteUrl(root string) string {
	if root == "" {
		return ""
	}

	entry := stateCache.get(root)

	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err == nil {
		url := strings.TrimSpace(stdout.String())
		entry.remoteUrl = normalizeGitRemoteUrl(url)
		return entry.remoteUrl
	}

	return ""
}

func normalizeGitRemoteUrl(url string) string {
	trimmed := strings.TrimSpace(url)
	if trimmed == "" {
		return ""
	}

	sshMatch := regexp.MustCompile(`git@([^:]+):(.+?)(?:\.git)?$`).FindStringSubmatch(trimmed)
	if sshMatch != nil {
		return strings.ToLower(sshMatch[1] + "/" + sshMatch[2])
	}

	urlMatch := regexp.MustCompile(`^(?:https?|ssh)://(?:[^@]+@)?([^/]+)/(.+?)(?:\.git)?$`).FindStringSubmatch(trimmed)
	if urlMatch != nil {
		return strings.ToLower(urlMatch[1] + "/" + urlMatch[2])
	}

	return trimmed
}

func isSafeRefName(name string) bool {
	if name == "" || strings.HasPrefix(name, "-") || strings.HasPrefix(name, "/") {
		return false
	}

	if strings.Contains(name, "..") {
		return false
	}

	for _, part := range strings.Split(name, "/") {
		if part == "." || part == "" {
			return false
		}
	}

	matched, _ := regexp.MatchString(`^[a-zA-Z0-9/._+@-]+$`, name)
	return matched
}

func isValidGitSha(s string) bool {
	lower := strings.ToLower(s)
	if len(lower) == 40 {
		matched, _ := regexp.MatchString(`^[0-9a-f]{40}$`, lower)
		return matched
	}
	if len(lower) == 64 {
		matched, _ := regexp.MatchString(`^[0-9a-f]{64}$`, lower)
		return matched
	}
	return false
}

func DirIsInGitRepo(cwd string) bool {
	return FindGitRoot(cwd) != ""
}

func IsAtGitRoot(cwd string) bool {
	gitRoot := FindGitRoot(cwd)
	if gitRoot == "" {
		return false
	}

	resolvedCwd, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return cwd == gitRoot
	}

	resolvedGitRoot, err := filepath.EvalSymlinks(gitRoot)
	if err != nil {
		return cwd == gitRoot
	}

	return resolvedCwd == resolvedGitRoot
}

func GetGitExe() string {
	path, err := exec.LookPath("git")
	if err != nil {
		return "git"
	}
	return path
}

func (e *cacheEntry) getBranch() (string, time.Time) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.branch, e.lastCheck
}

func (e *cacheEntry) setBranch(branch string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.branch = branch
	e.lastCheck = time.Now()
}

func (e *cacheEntry) getHead() (string, time.Time) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.head, e.lastCheck
}

func (e *cacheEntry) setHead(head string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.head = head
	e.lastCheck = time.Now()
}
