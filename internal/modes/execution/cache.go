package execution

import (
	"crypto/rand"
	"sync"

	"github.com/KPO-Tech/seshat-runtime/internal/types"
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

// ClearAllStates removes all session states.
func (c *ModeCache[S]) ClearAllStates() {
	c.statesMu.Lock()
	c.states = make(map[types.SessionID]*S)
	c.statesMu.Unlock()
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
