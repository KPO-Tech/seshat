package rag

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/KPO-Tech/seshat/internal/storage"
)

const (
	enrichmentCacheVersion       = "rag-enrich-v1"
	defaultEnrichmentCachePrefix = "rag/cache/enrichment"
)

// EnrichmentCache stores per-chunk enrichment results (synthetic questions)
// under a deterministic key - the same content-hash-keying idea as
// ChunkCache, so a re-ingest of unchanged chunks doesn't re-bill the LLM.
type EnrichmentCache interface {
	GetEnrichment(ctx context.Context, key string) ([]string, bool, error)
	PutEnrichment(ctx context.Context, key string, questions []string) error
}

// EnricherCacheKeyProvider lets an Enricher include its own configuration
// (model, questions-per-chunk, prompt version) in cache keys, so changing
// that configuration doesn't silently reuse stale enrichment results - the
// same purpose ChunkCacheKeyProvider serves for chunkers.
type EnricherCacheKeyProvider interface {
	EnricherCacheKey() string
}

// CachedEnricher wraps an Enricher with a deterministic cache, keyed by
// chunk text plus the wrapped enricher's option fingerprint when available.
type CachedEnricher struct {
	Enricher Enricher
	Cache    EnrichmentCache
}

func NewCachedEnricher(enricher Enricher, cache EnrichmentCache) *CachedEnricher {
	return &CachedEnricher{Enricher: enricher, Cache: cache}
}

func (c *CachedEnricher) EnrichChunks(ctx context.Context, texts []string) ([][]string, error) {
	if c == nil || c.Enricher == nil {
		return make([][]string, len(texts)), nil
	}
	if c.Cache == nil {
		return c.Enricher.EnrichChunks(ctx, texts)
	}

	enricherKey := enricherCacheKey(c.Enricher)
	results := make([][]string, len(texts))
	var missIdx []int
	var missTexts []string
	for i, text := range texts {
		key := ChunkEnrichmentCacheKey(text, enricherKey)
		questions, ok, err := c.Cache.GetEnrichment(ctx, key)
		if err != nil {
			return nil, err
		}
		if ok {
			results[i] = questions
			continue
		}
		missIdx = append(missIdx, i)
		missTexts = append(missTexts, text)
	}
	if len(missTexts) == 0 {
		return results, nil
	}

	fresh, err := c.Enricher.EnrichChunks(ctx, missTexts)
	if err != nil {
		return nil, err
	}
	if len(fresh) != len(missTexts) {
		return nil, fmt.Errorf("enricher returned %d results for %d texts", len(fresh), len(missTexts))
	}
	for j, idx := range missIdx {
		results[idx] = fresh[j]
		key := ChunkEnrichmentCacheKey(missTexts[j], enricherKey)
		if err := c.Cache.PutEnrichment(ctx, key, fresh[j]); err != nil {
			return nil, err
		}
	}
	return results, nil
}

// ChunkEnrichmentCacheKey builds the deterministic cache key for a chunk's
// enrichment result, mirroring DocumentChunkCacheKey's shape.
func ChunkEnrichmentCacheKey(chunkText, enricherKey string) string {
	h := sha256.New()
	writeCachePart(h, enrichmentCacheVersion)
	writeCachePart(h, enricherKey)
	_, _ = h.Write([]byte(chunkText))
	return hex.EncodeToString(h.Sum(nil))
}

func enricherCacheKey(enricher Enricher) string {
	if provider, ok := enricher.(EnricherCacheKeyProvider); ok {
		return provider.EnricherCacheKey()
	}
	return fmt.Sprintf("%T", enricher)
}

type enrichmentCacheEnvelope struct {
	Version   string   `json:"version"`
	Questions []string `json:"questions"`
}

// ArtifactEnrichmentCache persists cached enrichment results in the
// configured artifact store.
type ArtifactEnrichmentCache struct {
	Store  storage.ArtifactStore
	Prefix string
}

func NewArtifactEnrichmentCache(store storage.ArtifactStore) *ArtifactEnrichmentCache {
	return &ArtifactEnrichmentCache{Store: store, Prefix: defaultEnrichmentCachePrefix}
}

func (c *ArtifactEnrichmentCache) GetEnrichment(ctx context.Context, key string) ([]string, bool, error) {
	if c == nil || c.Store == nil {
		return nil, false, nil
	}
	cacheKey := c.storageKey(key)
	exists, err := c.Store.Exists(ctx, cacheKey)
	if err != nil {
		return nil, false, err
	}
	if !exists {
		return nil, false, nil
	}
	data, err := c.Store.Get(ctx, cacheKey)
	if err != nil {
		return nil, false, err
	}
	var envelope enrichmentCacheEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, false, err
	}
	if envelope.Version != enrichmentCacheVersion {
		return nil, false, nil
	}
	return envelope.Questions, true, nil
}

func (c *ArtifactEnrichmentCache) PutEnrichment(ctx context.Context, key string, questions []string) error {
	if c == nil || c.Store == nil {
		return nil
	}
	data, err := json.Marshal(enrichmentCacheEnvelope{
		Version:   enrichmentCacheVersion,
		Questions: questions,
	})
	if err != nil {
		return err
	}
	_, err = c.Store.Put(ctx, c.storageKey(key), data, "application/json")
	return err
}

func (c *ArtifactEnrichmentCache) storageKey(key string) string {
	prefix := strings.Trim(strings.TrimSpace(c.Prefix), "/")
	if prefix == "" {
		prefix = defaultEnrichmentCachePrefix
	}
	return path.Join(prefix, strings.TrimSpace(key)+".json")
}

// MemoryEnrichmentCache is useful for tests and short-lived local runs.
type MemoryEnrichmentCache struct {
	mu        sync.RWMutex
	questions map[string][]string
}

func NewMemoryEnrichmentCache() *MemoryEnrichmentCache {
	return &MemoryEnrichmentCache{questions: make(map[string][]string)}
}

func (c *MemoryEnrichmentCache) GetEnrichment(_ context.Context, key string) ([]string, bool, error) {
	if c == nil {
		return nil, false, nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	questions, ok := c.questions[key]
	if !ok {
		return nil, false, nil
	}
	return cloneStrings(questions), true, nil
}

func (c *MemoryEnrichmentCache) PutEnrichment(_ context.Context, key string, questions []string) error {
	if c == nil {
		return errors.New("memory enrichment cache is nil")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.questions == nil {
		c.questions = make(map[string][]string)
	}
	c.questions[key] = cloneStrings(questions)
	return nil
}

func cloneStrings(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
}

var _ Enricher = (*CachedEnricher)(nil)
var _ EnrichmentCache = (*ArtifactEnrichmentCache)(nil)
var _ EnrichmentCache = (*MemoryEnrichmentCache)(nil)
