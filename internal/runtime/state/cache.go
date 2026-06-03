package state

import (
	"sync"
)

// FileStateCache provides thread-safe file state caching.
// This is a minimal implementation for ToolUseContext integration.
// OpenClaude's full FileStateCache includes image/file tracking,
// read-ahead, and memory-mapped file reading capabilities.
//
// For the ToolUseContext slice, we provide the minimal interface
// required by the context tests. The full implementation
// can be added incrementally when needed by other modules.

// FileState represents cached file state
type FileState struct {
	Path    string
	Size    int64
	ModTime int64
}

// FileStateCache provides thread-safe file state caching
type FileStateCache struct {
	mu    sync.RWMutex
	cache map[string]FileState
}

// NewFileStateCache creates a new file state cache
func NewFileStateCache() *FileStateCache {
	return &FileStateCache{
		cache: make(map[string]FileState),
	}
}

// Get retrieves cached file state
func (c *FileStateCache) Get(path string) (*FileState, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	state, ok := c.cache[path]
	if !ok {
		return nil, false
	}
	return &state, true
}

// Set stores file state in cache
func (c *FileStateCache) Set(path string, state FileState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[path] = state
}

// Delete removes file state from cache
func (c *FileStateCache) Delete(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cache, path)
}

// Clear removes all cached file state
func (c *FileStateCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]FileState)
}
