package read

import (
	"os"
	"sync"
	"time"
)

// FileReadState represents the state of a previously read file
type FileReadState struct {
	// Content is the cached content
	Content string `json:"content"`

	// Timestamp is the file modification time when cached
	Timestamp int64 `json:"timestamp"`

	// Offset is the line offset used for text files (0-based)
	Offset int `json:"offset,omitempty"`

	// Limit is the number of lines read
	Limit int `json:"limit,omitempty"`

	// IsPartialView indicates if this is a partial view (offset/limit used)
	IsPartialView bool `json:"is_partial_view,omitempty"`

	// CachedAt is when this entry was created
	CachedAt time.Time `json:"cached_at"`
}

// FileReadCache manages deduplication of file reads
type FileReadCache struct {
	mu    sync.RWMutex
	cache map[string]*FileReadState

	// MaxEntries is the maximum number of entries to cache
	MaxEntries int

	// MaxAge is the maximum age of cache entries
	MaxAge time.Duration
}

// NewFileReadCache creates a new file read cache
func NewFileReadCache() *FileReadCache {
	return &FileReadCache{
		cache:      make(map[string]*FileReadState),
		MaxEntries: 100,              // Cache up to 100 files
		MaxAge:     30 * time.Minute, // Expire after 30 minutes
	}
}

// Get retrieves a cached file read state if it exists and is still valid
func (c *FileReadCache) Get(filePath string, offset, limit int) (*FileReadState, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	state, exists := c.getValidatedState(filePath)
	if !exists {
		return nil, false
	}

	if state.Offset != offset || state.Limit != limit {
		return nil, false
	}

	return state, true
}

// GetLatest retrieves the latest cached state for a file if it is still valid.
func (c *FileReadCache) GetLatest(filePath string) (*FileReadState, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.getValidatedState(filePath)
}

func (c *FileReadCache) getValidatedState(filePath string) (*FileReadState, bool) {
	state, exists := c.cache[filePath]
	if !exists {
		return nil, false
	}

	if time.Since(state.CachedAt) > c.MaxAge {
		return nil, false
	}

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, false
	}

	if fileInfo.ModTime().Unix() != state.Timestamp {
		return nil, false
	}

	stateCopy := *state
	return &stateCopy, true
}

// Set stores a file read state in the cache
func (c *FileReadCache) Set(filePath string, content string, fileInfo os.FileInfo, offset, limit int, isPartialView bool) {
	c.SetState(filePath, &FileReadState{
		Content:       content,
		Timestamp:     fileInfo.ModTime().Unix(),
		Offset:        offset,
		Limit:         limit,
		IsPartialView: isPartialView,
		CachedAt:      time.Now(),
	})
}

// SetState stores a precomputed file read state in the cache.
func (c *FileReadCache) SetState(filePath string, state *FileReadState) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.cache) >= c.MaxEntries {
		c.evictOldest()
	}

	stateCopy := *state
	if stateCopy.CachedAt.IsZero() {
		stateCopy.CachedAt = time.Now()
	}
	c.cache[filePath] = &stateCopy
}

// Invalidate removes a file from the cache
func (c *FileReadCache) Invalidate(filePath string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.cache, filePath)
}

// Clear removes all entries from the cache
func (c *FileReadCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*FileReadState)
}

// Size returns the number of entries in the cache
func (c *FileReadCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.cache)
}

// evictOldest removes the oldest entry from the cache
func (c *FileReadCache) evictOldest() {
	var oldestPath string
	var oldestTime time.Time

	for path, state := range c.cache {
		if oldestPath == "" || state.CachedAt.Before(oldestTime) {
			oldestPath = path
			oldestTime = state.CachedAt
		}
	}

	if oldestPath != "" {
		delete(c.cache, oldestPath)
	}
}

// Cleanup removes expired entries from the cache
func (c *FileReadCache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for path, state := range c.cache {
		if now.Sub(state.CachedAt) > c.MaxAge {
			delete(c.cache, path)
		}
	}
}

// Global file read cache instance
var globalCache *FileReadCache
var globalCacheOnce sync.Once

// GetGlobalCache returns the global file read cache (singleton)
func GetGlobalCache() *FileReadCache {
	globalCacheOnce.Do(func() {
		globalCache = NewFileReadCache()
	})
	return globalCache
}

// CheckFileUnchanged checks if a file has the same content as cached
func CheckFileUnchanged(filePath string, offset, limit int) (*FileReadState, bool) {
	cache := GetGlobalCache()
	return cache.Get(filePath, offset, limit)
}

// CacheFileRead stores a file read in the cache
func CacheFileRead(filePath string, content string, fileInfo os.FileInfo, offset, limit int, isPartialView bool) {
	cache := GetGlobalCache()
	cache.Set(filePath, content, fileInfo, offset, limit, isPartialView)
}

// GetLastReadState returns the most recent cached read state for a file.
func GetLastReadState(filePath string) (*FileReadState, bool) {
	cache := GetGlobalCache()
	return cache.GetLatest(filePath)
}

// RecordExternalRead stores externally-observed read state in the shared cache.
func RecordExternalRead(filePath string, modTime time.Time, content string, isFullRead bool) {
	cache := GetGlobalCache()
	cache.SetState(filePath, &FileReadState{
		Content:       content,
		Timestamp:     modTime.Unix(),
		Offset:        0,
		Limit:         0,
		IsPartialView: !isFullRead,
		CachedAt:      time.Now(),
	})
}

// InvalidateFileCache removes a file from the cache
func InvalidateFileCache(filePath string) {
	cache := GetGlobalCache()
	cache.Invalidate(filePath)
}
