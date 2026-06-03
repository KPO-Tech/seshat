package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// ---------------------------------------------------------------------------
// Tool Results Cache System
// ---------------------------------------------------------------------------

// ToolResultKey represents a cache key for a tool call
type ToolResultKey struct {
	ToolName string `json:"toolName"`
	Hash     string `json:"hash"` // Hash of parameters for uniqueness
}

// CachedToolResult represents a cached tool result
type CachedToolResult struct {
	// Result is the cached tool result
	Result *types.ToolResult `json:"result"`

	// CreatedAt is when this cache entry was created
	CreatedAt time.Time `json:"createdAt"`

	// LastAccessed is when this cache entry was last accessed
	LastAccessed time.Time `json:"lastAccessed"`

	// AccessCount is the number of times this entry has been accessed
	AccessCount int `json:"accessCount"`

	// Size is the approximate size of the cached result (in bytes)
	Size int `json:"size"`
}

// ToolResultCacheConfig configures the tool results cache behavior
type ToolResultCacheConfig struct {
	// MaxEntries is the maximum number of cache entries (0 = unlimited)
	MaxEntries int `json:"maxEntries"`

	// MaxSize is the maximum total cache size in bytes (0 = unlimited)
	MaxSize int `json:"maxSize"`

	// TTL is the time-to-live for cache entries (0 = no expiration)
	TTL time.Duration `json:"ttl"`

	// EnableCompression enables result compression (future feature)
	EnableCompression bool `json:"enableCompression"`
}

// DefaultToolResultCacheConfig returns default cache configuration
func DefaultToolResultCacheConfig() *ToolResultCacheConfig {
	return &ToolResultCacheConfig{
		MaxEntries:        1000,
		MaxSize:           50 * 1024 * 1024, // 50 MB
		TTL:               30 * time.Minute,
		EnableCompression: false,
	}
}

// ToolResultCacheStats provides cache statistics
type ToolResultCacheStats struct {
	// Hits is the number of cache hits
	Hits int64 `json:"hits"`

	// Misses is the number of cache misses
	Misses int64 `json:"misses"`

	// Evictions is the number of cache evictions
	Evictions int64 `json:"evictions"`

	// Entries is the current number of cache entries
	Entries int `json:"entries"`

	// Size is the current cache size in bytes
	Size int64 `json:"size"`

	// HitRate is the cache hit rate (0.0 to 1.0)
	HitRate float64 `json:"hitRate"`
}

// ToolResultCache provides thread-safe caching for tool results
type ToolResultCache struct {
	mu       sync.RWMutex
	cache    map[ToolResultKey]*CachedToolResult
	access   map[string]time.Time // Track access times for LRU
	config   *ToolResultCacheConfig
	stats    ToolResultCacheStats
	stopChan chan struct{}
}

// NewToolResultCache creates a new tool results cache with default config
func NewToolResultCache() *ToolResultCache {
	return NewToolResultCacheWithConfig(DefaultToolResultCacheConfig())
}

// NewToolResultCacheWithConfig creates a new tool results cache with custom config
func NewToolResultCacheWithConfig(config *ToolResultCacheConfig) *ToolResultCache {
	cache := &ToolResultCache{
		cache:    make(map[ToolResultKey]*CachedToolResult),
		access:   make(map[string]time.Time),
		config:   config,
		stopChan: make(chan struct{}),
	}

	// Start cleanup goroutine
	go cache.cleanupExpiredEntries()

	return cache
}

// Get retrieves a cached tool result
func (c *ToolResultCache) Get(toolName string, params map[string]any) (*types.ToolResult, bool) {
	key, err := c.generateKey(toolName, params)
	if err != nil {
		c.mu.Lock()
		c.stats.Misses++
		c.updateHitRate()
		c.mu.Unlock()
		return nil, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.cache[key]
	if !exists {
		c.stats.Misses++
		c.updateHitRate()
		return nil, false
	}

	// Check if entry has expired
	if c.config.TTL > 0 && time.Since(entry.CreatedAt) > c.config.TTL {
		delete(c.cache, key)
		delete(c.access, key.ToolName)
		c.stats.Evictions++
		c.stats.Entries = len(c.cache)
		c.updateHitRate()
		return nil, false
	}

	// Update access time and count
	entry.LastAccessed = time.Now()
	entry.AccessCount++
	c.access[key.ToolName] = entry.LastAccessed

	// Update stats
	c.stats.Hits++
	c.updateHitRate()

	// Return a copy of the result to avoid mutations
	resultCopy := *entry.Result
	return &resultCopy, true
}

// Set stores a tool result in the cache
func (c *ToolResultCache) Set(toolName string, params map[string]any, result *types.ToolResult) error {
	key, err := c.generateKey(toolName, params)
	if err != nil {
		return fmt.Errorf("failed to generate cache key: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Calculate size of the result
	size, err := c.calculateResultSize(result)
	if err != nil {
		return fmt.Errorf("failed to calculate result size: %w", err)
	}

	// Check if entry already exists
	if existingEntry, exists := c.cache[key]; exists {
		// Update existing entry
		existingEntry.Result = result
		existingEntry.Size = size
		existingEntry.LastAccessed = time.Now()
		c.access[key.ToolName] = existingEntry.LastAccessed
		return nil
	}

	// Check cache limits
	if c.config.MaxEntries > 0 && len(c.cache) >= c.config.MaxEntries {
		c.evictLRU()
	}

	currentTotalSize := c.calculateTotalSize()
	if c.config.MaxSize > 0 && currentTotalSize+int64(size) > int64(c.config.MaxSize) {
		c.evictToMakeSpace(int64(size))
	}

	// Create new cache entry
	entry := &CachedToolResult{
		Result:       result,
		CreatedAt:    time.Now(),
		LastAccessed: time.Now(),
		AccessCount:  1,
		Size:         size,
	}

	// Store in cache
	c.cache[key] = entry
	c.access[key.ToolName] = entry.LastAccessed

	// Update stats
	c.stats.Entries = len(c.cache)
	c.stats.Size = c.calculateTotalSize()

	return nil
}

// Invalidate removes specific tool results from cache
func (c *ToolResultCache) Invalidate(toolName string, params map[string]any) error {
	key, err := c.generateKey(toolName, params)
	if err != nil {
		return fmt.Errorf("failed to generate cache key: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.cache[key]; exists {
		delete(c.cache, key)
		delete(c.access, key.ToolName)
		c.stats.Entries = len(c.cache)
		c.stats.Size = c.calculateTotalSize()
		c.stats.Evictions++
	}

	return nil
}

// InvalidateByTool removes all results for a specific tool
func (c *ToolResultCache) InvalidateByTool(toolName string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	keysToDelete := []ToolResultKey{}
	for key := range c.cache {
		if key.ToolName == toolName {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		delete(c.cache, key)
		delete(c.access, key.ToolName)
	}

	c.stats.Entries = len(c.cache)
	c.stats.Size = c.calculateTotalSize()
	c.stats.Evictions += int64(len(keysToDelete))
}

// Clear removes all entries from the cache
func (c *ToolResultCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[ToolResultKey]*CachedToolResult)
	c.access = make(map[string]time.Time)
	c.stats.Entries = 0
	c.stats.Size = 0
	c.stats.Evictions++
}

// GetStats returns current cache statistics
func (c *ToolResultCache) GetStats() ToolResultCacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.stats
}

// Shutdown gracefully shuts down the cache
func (c *ToolResultCache) Shutdown() {
	close(c.stopChan)
}

// generateKey creates a unique cache key for a tool call
func (c *ToolResultCache) generateKey(toolName string, params map[string]any) (ToolResultKey, error) {
	// Create a hash of the parameters
	hash, err := c.hashParameters(params)
	if err != nil {
		return ToolResultKey{}, err
	}

	return ToolResultKey{
		ToolName: toolName,
		Hash:     hash,
	}, nil
}

// hashParameters creates a hash of the parameters for cache key generation
func (c *ToolResultCache) hashParameters(params map[string]any) (string, error) {
	if len(params) == 0 {
		return "nil", nil
	}

	// Normalize parameters for consistent hashing
	normalized, err := c.normalizeParameters(params)
	if err != nil {
		return "", err
	}

	// Create hash
	jsonBytes, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("failed to marshal parameters: %w", err)
	}

	hasher := sha256.New()
	hasher.Write(jsonBytes)
	hashBytes := hasher.Sum(nil)

	return hex.EncodeToString(hashBytes), nil
}

// normalizeParameters normalizes parameters for consistent hashing
func (c *ToolResultCache) normalizeParameters(params map[string]any) (map[string]any, error) {
	// Sort keys for consistent ordering
	normalized := make(map[string]any)
	for k, v := range params {
		normalized[k] = c.normalizeValue(v)
	}

	return normalized, nil
}

// normalizeValue normalizes a value for consistent hashing
func (c *ToolResultCache) normalizeValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		// Recursively normalize maps
		return c.normalizeMap(v)
	case []any:
		// Recursively normalize slices
		return c.normalizeSlice(v)
	default:
		return v
	}
}

// normalizeMap normalizes a map value for consistent hashing
func (c *ToolResultCache) normalizeMap(m map[string]any) map[string]any {
	normalized := make(map[string]any)
	for k, v := range m {
		normalized[k] = c.normalizeValue(v)
	}
	return normalized
}

// normalizeSlice normalizes a slice value for consistent hashing
func (c *ToolResultCache) normalizeSlice(s []any) []any {
	normalized := make([]any, len(s))
	for i, v := range s {
		normalized[i] = c.normalizeValue(v)
	}
	return normalized
}

// calculateResultSize calculates the approximate size of a tool result
func (c *ToolResultCache) calculateResultSize(result *types.ToolResult) (int, error) {
	if result == nil {
		return 0, nil
	}

	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal result: %w", err)
	}

	return len(jsonBytes), nil
}

// calculateTotalSize calculates the total size of all cached entries
func (c *ToolResultCache) calculateTotalSize() int64 {
	var total int64
	for _, entry := range c.cache {
		total += int64(entry.Size)
	}
	return total
}

// updateHitRate updates the cache hit rate based on current stats
func (c *ToolResultCache) updateHitRate() {
	total := c.stats.Hits + c.stats.Misses
	if total > 0 {
		c.stats.HitRate = float64(c.stats.Hits) / float64(total)
	}
}

// evictLRU evicts the least recently used entry
func (c *ToolResultCache) evictLRU() {
	// Find the least recently used entry
	var lruKey ToolResultKey
	var lruTime time.Time
	found := false

	for key, entry := range c.cache {
		if !found || entry.LastAccessed.Before(lruTime) {
			lruKey = key
			lruTime = entry.LastAccessed
			found = true
		}
	}

	if found {
		delete(c.cache, lruKey)
		delete(c.access, lruKey.ToolName)
		c.stats.Evictions++
		c.stats.Entries = len(c.cache)
		c.stats.Size = c.calculateTotalSize()
	}
}

// evictToMakeSpace evicts entries to make room for a new entry
func (c *ToolResultCache) evictToMakeSpace(requiredSize int64) {
	// Keep evicting until we have enough space
	for c.calculateTotalSize()+requiredSize > int64(c.config.MaxSize) {
		if len(c.cache) == 0 {
			return
		}
		before := len(c.cache)
		c.evictLRU()
		if len(c.cache) == before {
			return
		}
	}
}

// cleanupExpiredEntries periodically removes expired cache entries
func (c *ToolResultCache) cleanupExpiredEntries() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.performCleanup()
		case <-c.stopChan:
			return
		}
	}
}

// performCleanup removes expired entries from the cache
func (c *ToolResultCache) performCleanup() {
	if c.config.TTL <= 0 {
		return // No TTL configured
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	keysToDelete := []ToolResultKey{}

	for key, entry := range c.cache {
		if now.Sub(entry.CreatedAt) > c.config.TTL {
			keysToDelete = append(keysToDelete, key)
		}
	}

	for _, key := range keysToDelete {
		delete(c.cache, key)
		delete(c.access, key.ToolName)
	}

	if len(keysToDelete) > 0 {
		c.stats.Evictions += int64(len(keysToDelete))
		c.stats.Entries = len(c.cache)
		c.stats.Size = c.calculateTotalSize()
	}
}

// ShouldCache determines if a tool result should be cached
func (c *ToolResultCache) ShouldCache(toolName string, result *types.ToolResult) bool {
	// Don't cache failed results
	if result == nil || result.Error != nil {
		return false
	}

	// Don't cache results from non-idempotent tools
	// (tools that have side effects or depend on external state)
	nonIdempotentTools := map[string]bool{
		"bash":       true,
		"write_file": true,
		"edit_file":  true,
	}

	return !nonIdempotentTools[toolName]
}
