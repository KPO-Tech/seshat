// Package web contains shared contracts and utilities for the reusable web stack.
package web

import (
	"sync"
	"time"
)

// TTLCacheConfig configures a lightweight in-memory TTL cache shared by web subsystems.
type TTLCacheConfig struct {
	TTL      time.Duration
	MaxItems int
}

type ttlEntry[V any] struct {
	Value     V
	ExpiresAt time.Time
}

// TTLCache is a generic in-memory cache with simple oldest-expiry eviction.
type TTLCache[V any] struct {
	mu       sync.RWMutex
	entries  map[string]ttlEntry[V]
	maxItems int
	ttl      time.Duration
}

// NewTTLCache creates a new shared cache with pragmatic defaults if values are omitted.
func NewTTLCache[V any](config TTLCacheConfig) *TTLCache[V] {
	if config.TTL <= 0 {
		config.TTL = 15 * time.Minute
	}
	if config.MaxItems <= 0 {
		config.MaxItems = 100
	}
	return &TTLCache[V]{
		entries:  make(map[string]ttlEntry[V]),
		maxItems: config.MaxItems,
		ttl:      config.TTL,
	}
}

// Get retrieves a cached value when it is still fresh.
func (c *TTLCache[V]) Get(key string) (V, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok || time.Now().After(entry.ExpiresAt) {
		var zero V
		return zero, false
	}
	return entry.Value, true
}

// Set stores a cached value and applies basic eviction if the cache is full.
func (c *TTLCache[V]) Set(key string, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.evictIfNeeded(len(c.entries) + 1)
	c.entries[key] = ttlEntry[V]{
		Value:     value,
		ExpiresAt: time.Now().Add(c.ttl),
	}
}

// Clear removes every entry from the cache.
func (c *TTLCache[V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]ttlEntry[V])
}

func (c *TTLCache[V]) evictIfNeeded(newCount int) {
	for newCount > c.maxItems && len(c.entries) > 0 {
		var oldestKey string
		var oldestTime time.Time
		for key, entry := range c.entries {
			if oldestKey == "" || entry.ExpiresAt.Before(oldestTime) {
				oldestKey = key
				oldestTime = entry.ExpiresAt
			}
		}
		if oldestKey != "" {
			delete(c.entries, oldestKey)
			newCount--
		}
	}
}
