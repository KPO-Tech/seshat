package fetch

import (
	"time"

	webcore "github.com/EngineerProjects/nexus-engine/internal/web"
)

// CacheEntry represents a cached URL response.
type CacheEntry struct {
	Content            string
	Bytes              int
	Code               int
	CodeText           string
	ContentType        string
	FinalURL           string
	PersistedPath      string
	PersistedSize      int
	BrowserRecommended bool
}

// Cache is a fetch-specific view over the shared web TTL cache.
type Cache = webcore.TTLCache[CacheEntry]

// DefaultCache creates the default fetch cache on top of the shared web cache implementation.
func DefaultCache() *Cache {
	return webcore.NewTTLCache[CacheEntry](webcore.TTLCacheConfig{
		TTL:      15 * time.Minute,
		MaxItems: 100,
	})
}
