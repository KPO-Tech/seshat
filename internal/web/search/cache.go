package search

import (
	"time"

	webcore "github.com/EngineerProjects/nexus-engine/internal/web"
)

// Cache is a search-specific view over the shared web TTL cache.
type Cache = webcore.TTLCache[Output]

// DefaultCache returns the default shared cache used for repeated identical search queries.
func DefaultCache() *Cache {
	return webcore.NewTTLCache[Output](webcore.TTLCacheConfig{
		TTL:      5 * time.Minute,
		MaxItems: 128,
	})
}
