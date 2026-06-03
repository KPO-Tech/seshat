package prompt

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"time"

	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// ================================================================================
// Prompt Cache System (Slice 1: State Kernel + Prompt Caching)
// Based on OpenClaude's memoized system prompt sections
// ================================================================================

// PromptCache gère la mémoïsation des sections de prompt
type PromptCache struct {
	mu    sync.RWMutex
	store map[string]*CacheEntry
}

// CacheEntry represents a cached prompt section
type CacheEntry struct {
	Content      string
	ComputedAt   time.Time
	SectionNames []string
	ToolHash     string
	ModelConfig  string
	ValidUntil   *time.Time // nil = valide jusqu'à invalidation explicite
}

// CacheHitResult represents a cache hit with metadata
type CacheHitResult struct {
	Content  string
	CachedAt time.Time
	ToolHash string
	IsStale  bool
}

// NewPromptCache creates a new prompt cache
func NewPromptCache() *PromptCache {
	return &PromptCache{
		store: make(map[string]*CacheEntry),
	}
}

// GetSectionCache tente de récupérer une section cacheée
func (c *PromptCache) GetSectionCache(
	sectionNames []string,
	tools map[string]tool.Tool,
	model types.ModelIdentifier,
	env map[string]string,
) (*CacheHitResult, bool) {
	key := buildCacheKey(sectionNames, tools, model, env)
	hashKey := key.computeHash()

	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.store[hashKey]
	if !exists {
		return nil, false
	}

	// Vérifier validité
	isStale := false
	if entry.ValidUntil != nil && time.Now().After(*entry.ValidUntil) {
		isStale = true
	}

	return &CacheHitResult{
		Content:  entry.Content,
		CachedAt: entry.ComputedAt,
		ToolHash: entry.ToolHash,
		IsStale:  isStale,
	}, true
}

// SetSectionCache met en cache une section
func (c *PromptCache) SetSectionCache(
	sectionNames []string,
	content string,
	tools map[string]tool.Tool,
	model types.ModelIdentifier,
	env map[string]string,
	ttl *time.Duration,
) {
	key := buildCacheKey(sectionNames, tools, model, env)
	hashKey := key.computeHash()

	var validUntil *time.Time
	if ttl != nil {
		expiry := time.Now().Add(*ttl)
		validUntil = &expiry
	}

	entry := &CacheEntry{
		Content:      content,
		ComputedAt:   time.Now(),
		SectionNames: sectionNames,
		ToolHash:     key.ToolHash,
		ModelConfig:  key.ModelConfig,
		ValidUntil:   validUntil,
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.store[hashKey] = entry
}

// InvalidateSectionCache invalide des sections spécifiques
func (c *PromptCache) InvalidateSectionCache(sectionNames []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Supprimer toutes les entrées contenant ces sections
	for key, entry := range c.store {
		for _, sectionName := range sectionNames {
			for _, cachedSection := range entry.SectionNames {
				if cachedSection == sectionName {
					delete(c.store, key)
					break
				}
			}
		}
	}
}

// InvalidateByToolHash invalide toutes les entrées dépendant d'un hash d'outils
func (c *PromptCache) InvalidateByToolHash(toolHash string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, entry := range c.store {
		if entry.ToolHash == toolHash {
			delete(c.store, key)
		}
	}
}

// InvalidateAll invalide tout le cache
func (c *PromptCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.store = make(map[string]*CacheEntry)
}

// Size returns the number of cached entries
func (c *PromptCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.store)
}

// ClearStaleEntries removes expired entries from the cache
func (c *PromptCache) ClearStaleEntries() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	removed := 0
	now := time.Now()

	for key, entry := range c.store {
		if entry.ValidUntil != nil && now.After(*entry.ValidUntil) {
			delete(c.store, key)
			removed++
		}
	}

	return removed
}

// ================================================================================
// Cache Key Generation (Deterministic)
// ================================================================================

// cacheKey represents a deterministic cache key
type cacheKey struct {
	SectionNames []string
	ToolHash     string
	ModelConfig  string
	Environment  map[string]string
}

// computeHash returns a string hash of the cache key
func (k cacheKey) computeHash() string {
	// Build a deterministic string representation
	var builder strings.Builder

	// Section names (already sorted)
	builder.WriteString("sections:")
	for _, name := range k.SectionNames {
		builder.WriteString(name)
		builder.WriteString(",")
	}
	builder.WriteString(";")

	// Tool hash
	builder.WriteString("tools:")
	builder.WriteString(k.ToolHash)
	builder.WriteString(";")

	// Model config
	builder.WriteString("model:")
	builder.WriteString(k.ModelConfig)
	builder.WriteString(";")

	// Environment (sorted)
	if k.Environment != nil {
		envKeys := make([]string, 0, len(k.Environment))
		for key := range k.Environment {
			envKeys = append(envKeys, key)
		}
		sort.Strings(envKeys)

		builder.WriteString("env:")
		for _, key := range envKeys {
			builder.WriteString(key)
			builder.WriteString("=")
			builder.WriteString(k.Environment[key])
			builder.WriteString(",")
		}
	}

	// Hash the final string
	hash := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(hash[:])
}

// buildCacheKey construit une clé de cache déterministe
func buildCacheKey(
	sectionNames []string,
	tools map[string]tool.Tool,
	model types.ModelIdentifier,
	env map[string]string,
) cacheKey {
	// Tri pour déterminisme
	sortedNames := make([]string, len(sectionNames))
	copy(sortedNames, sectionNames)
	sort.Strings(sortedNames)

	// Hash des outils
	toolHash := hashTools(tools)

	// Config modèle
	modelConfig := model.String()

	return cacheKey{
		SectionNames: sortedNames,
		ToolHash:     toolHash,
		ModelConfig:  modelConfig,
		Environment:  env,
	}
}

// hashTools construit un hash déterministe des outils
func hashTools(tools map[string]tool.Tool) string {
	if len(tools) == 0 {
		return ""
	}

	toolNames := make([]string, 0, len(tools))
	for name := range tools {
		toolNames = append(toolNames, name)
	}
	sort.Strings(toolNames)

	combined := strings.Join(toolNames, "|")
	hash := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(hash[:])
}

// HashTools exported for testing
func HashTools(tools map[string]tool.Tool) string {
	return hashTools(tools)
}
