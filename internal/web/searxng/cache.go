package searxng

import (
	"os"
	"strconv"
	"sync"
	"time"
)

// cacheEntry holds the fetched content of a URL.
// Mirrors the CacheEntry interface in cache.ts.
type cacheEntry struct {
	htmlContent     string
	markdownContent string
	timestamp       time.Time
	hitCount        int
}

const (
	defaultCacheTTL        = 24 * time.Hour
	defaultCacheMaxEntries = 500
	defaultCleanupInterval = 60 * time.Second
)

// URLCache is a TTL + LFU (least-frequently-used on tie) in-memory cache.
// Mirrors SimpleCache from cache.ts.
type URLCache struct {
	mu          sync.Mutex
	entries     map[string]*cacheEntry
	ttl         time.Duration
	maxEntries  int
	stopCleanup chan struct{}
}

// NewURLCache creates a cache with TTL and max size read from env vars,
// matching the MCP's CACHE_TTL_MS and CACHE_MAX_ENTRIES.
func NewURLCache() *URLCache {
	ttl := parseDurationEnv("CACHE_TTL_MS", defaultCacheTTL, time.Millisecond)
	max := parseIntEnv("CACHE_MAX_ENTRIES", defaultCacheMaxEntries)
	c := &URLCache{
		entries:     make(map[string]*cacheEntry, max),
		ttl:         ttl,
		maxEntries:  max,
		stopCleanup: make(chan struct{}),
	}
	go c.periodicCleanup(defaultCleanupInterval)
	return c
}

// Get retrieves cached markdown content for url. Returns ("", false) on miss or expiry.
func (c *URLCache) Get(url string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[url]
	if !ok {
		return "", false
	}
	if time.Since(e.timestamp) > c.ttl {
		delete(c.entries, url)
		return "", false
	}
	e.hitCount++
	return e.markdownContent, true
}

// Set stores the HTML and its markdown conversion for url.
func (c *URLCache) Set(url, htmlContent, markdownContent string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[url] = &cacheEntry{
		htmlContent:     htmlContent,
		markdownContent: markdownContent,
		timestamp:       time.Now(),
	}
	c.evictLocked()
}

// Close stops the background cleanup goroutine.
func (c *URLCache) Close() {
	close(c.stopCleanup)
}

func (c *URLCache) periodicCleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.mu.Lock()
			c.cleanupExpiredLocked()
			c.mu.Unlock()
		case <-c.stopCleanup:
			return
		}
	}
}

func (c *URLCache) cleanupExpiredLocked() {
	now := time.Now()
	for url, e := range c.entries {
		if now.Sub(e.timestamp) > c.ttl {
			delete(c.entries, url)
		}
	}
}

// evictLocked removes entries until size <= maxEntries, preferring to evict
// the entry with the lowest hitCount (LFU), breaking ties by oldest timestamp.
func (c *URLCache) evictLocked() {
	c.cleanupExpiredLocked()
	for len(c.entries) > c.maxEntries {
		var victim string
		var victimEntry *cacheEntry
		for url, e := range c.entries {
			if victimEntry == nil ||
				e.hitCount < victimEntry.hitCount ||
				(e.hitCount == victimEntry.hitCount && e.timestamp.Before(victimEntry.timestamp)) {
				victim = url
				victimEntry = e
			}
		}
		if victim == "" {
			break
		}
		delete(c.entries, victim)
	}
}

func parseDurationEnv(key string, fallback time.Duration, unit time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return fallback
	}
	return time.Duration(n) * unit
}

func parseIntEnv(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

// DefaultURLCache is the process-wide singleton, matching the MCP's exported urlCache.
var DefaultURLCache = NewURLCache()
