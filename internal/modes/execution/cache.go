package execution

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/EngineerProjects/seshat/internal/types"
)

// WordList is used to generate human-readable mode file slugs.
var WordList = []string{
	"algorithm", "cascade", "discovery", "element", "focus", "galaxy", "horizon", "insight",
	"journey", "kernel", "launch", "momentum", "seshat", "orbit", "prism", "quest",
	"resonance", "spectrum", "tempo", "unity", "vertex", "wavelength", "zenith", "apex",
	"bridge", "chronicle", "dynamics", "eclipse", "fusion", "genesis", "harmonic", "infinity",
	"junction", "kinesis", "lucid", "meridian", "novel", "origins", "paradigm", "quantum",
	"radian", "symmetry", "threshold", "unity", "vortex", "wanderer", "xenon", "yearning",
}

// ModeCache manages per-session state, slug, and file artifact for a single mode.
// S is the mode-specific state type.
type ModeCache[S any] struct {
	states   map[types.SessionID]*S
	statesMu sync.RWMutex

	slugs   map[types.SessionID]string
	slugsMu sync.RWMutex

	dir    string
	dirMu  sync.RWMutex
	subDir string

	newState func() *S
}

// NewModeCache creates a new ModeCache.
// subDir is the path relative to cwd (e.g. ".seshat/plans").
// newState returns a fresh zero state for new sessions.
func NewModeCache[S any](subDir string, newState func() *S) *ModeCache[S] {
	return &ModeCache[S]{
		states:   make(map[types.SessionID]*S),
		slugs:    make(map[types.SessionID]string),
		subDir:   subDir,
		newState: newState,
	}
}

// GetState returns the state for a session, creating it if it does not exist.
func (c *ModeCache[S]) GetState(sessionID types.SessionID) *S {
	c.statesMu.RLock()
	s, ok := c.states[sessionID]
	c.statesMu.RUnlock()
	if ok {
		return s
	}
	c.statesMu.Lock()
	defer c.statesMu.Unlock()
	if s, ok = c.states[sessionID]; ok {
		return s
	}
	s = c.newState()
	c.states[sessionID] = s
	return s
}

// SetState explicitly sets the state for a session.
func (c *ModeCache[S]) SetState(sessionID types.SessionID, state *S) {
	c.statesMu.Lock()
	c.states[sessionID] = state
	c.statesMu.Unlock()
}

// ClearState removes the state for a session.
func (c *ModeCache[S]) ClearState(sessionID types.SessionID) {
	c.statesMu.Lock()
	delete(c.states, sessionID)
	c.statesMu.Unlock()
}

// ClearAllStates removes all session states.
func (c *ModeCache[S]) ClearAllStates() {
	c.statesMu.Lock()
	c.states = make(map[types.SessionID]*S)
	c.statesMu.Unlock()
}

// GetDirectory returns the mode's file directory, creating it lazily on first call.
func (c *ModeCache[S]) GetDirectory() string {
	c.dirMu.RLock()
	if c.dir != "" {
		d := c.dir
		c.dirMu.RUnlock()
		return d
	}
	c.dirMu.RUnlock()

	c.dirMu.Lock()
	defer c.dirMu.Unlock()
	if c.dir != "" {
		return c.dir
	}
	var path string
	if filepath.IsAbs(c.subDir) {
		path = c.subDir
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			cwd = "."
		}
		path = filepath.Join(cwd, c.subDir)
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		path = filepath.Join(os.TempDir(), "seshat", filepath.Base(c.subDir))
		_ = os.MkdirAll(path, 0755)
	}
	c.dir = path
	return c.dir
}

// SetDirectory sets a custom directory for mode files.
func (c *ModeCache[S]) SetDirectory(dir string) error {
	absPath, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("failed to resolve directory: %w", err)
	}
	if err := os.MkdirAll(absPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	c.dirMu.Lock()
	c.dir = absPath
	c.dirMu.Unlock()
	return nil
}

// GetSlug returns the slug for a session, generating one if it does not exist.
func (c *ModeCache[S]) GetSlug(sessionID types.SessionID) string {
	c.slugsMu.RLock()
	slug, ok := c.slugs[sessionID]
	c.slugsMu.RUnlock()
	if ok {
		return slug
	}
	c.slugsMu.Lock()
	defer c.slugsMu.Unlock()
	if slug, ok = c.slugs[sessionID]; ok {
		return slug
	}
	slug = generateSlug()
	c.slugs[sessionID] = slug
	return slug
}

// SetSlug sets the slug for a session.
func (c *ModeCache[S]) SetSlug(sessionID types.SessionID, slug string) {
	c.slugsMu.Lock()
	c.slugs[sessionID] = slug
	c.slugsMu.Unlock()
}

// ClearSlug removes the slug for a session.
func (c *ModeCache[S]) ClearSlug(sessionID types.SessionID) {
	c.slugsMu.Lock()
	delete(c.slugs, sessionID)
	c.slugsMu.Unlock()
}

// ClearAllSlugs removes all slugs.
func (c *ModeCache[S]) ClearAllSlugs() {
	c.slugsMu.Lock()
	c.slugs = make(map[types.SessionID]string)
	c.slugsMu.Unlock()
}

// GetFilePath returns the full path to the mode file for a session.
func (c *ModeCache[S]) GetFilePath(sessionID types.SessionID) string {
	return filepath.Join(c.GetDirectory(), c.GetSlug(sessionID)+".md")
}

// GetDisplayPath returns a user-friendly path relative to the current working directory.
func (c *ModeCache[S]) GetDisplayPath(filePath string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return filePath
	}
	rel, err := filepath.Rel(cwd, filePath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return filePath
	}
	return rel
}

// FileExists checks if the mode file exists for a session.
func (c *ModeCache[S]) FileExists(sessionID types.SessionID) bool {
	_, err := os.Stat(c.GetFilePath(sessionID))
	return err == nil
}

// ReadFile reads the mode file content for a session.
func (c *ModeCache[S]) ReadFile(sessionID types.SessionID) (string, error) {
	content, err := os.ReadFile(c.GetFilePath(sessionID))
	if err != nil {
		return "", fmt.Errorf("failed to read mode file: %w", err)
	}
	return string(content), nil
}

// WriteFile writes content to the mode file for a session.
func (c *ModeCache[S]) WriteFile(sessionID types.SessionID, content string) error {
	path := c.GetFilePath(sessionID)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create mode directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write mode file: %w", err)
	}
	return nil
}

// DeleteFile deletes the mode file for a session.
func (c *ModeCache[S]) DeleteFile(sessionID types.SessionID) error {
	if err := os.Remove(c.GetFilePath(sessionID)); err != nil {
		return fmt.Errorf("failed to delete mode file: %w", err)
	}
	return nil
}

// generateSlug generates a random two-word slug (e.g. "quantum-bridge").
func generateSlug() string {
	return generateWord() + "-" + generateWord()
}

// generateWord picks a random word from WordList.
func generateWord() string {
	idx := make([]byte, 1)
	if _, err := rand.Read(idx); err != nil {
		return WordList[0]
	}
	return WordList[int(idx[0])%len(WordList)]
}

// ExtractSlugFromFilePath extracts the slug from a mode file path.
// E.g. "/path/to/quantum-bridge.md" → "quantum-bridge".
func ExtractSlugFromFilePath(filePath string) string {
	base := filepath.Base(filePath)
	if ext := filepath.Ext(base); ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	return base
}
