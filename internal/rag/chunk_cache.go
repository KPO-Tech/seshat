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
	chunkCacheVersion       = "rag-chunks-v1"
	defaultChunkCachePrefix = "rag/cache/chunks"
)

// ChunkCache stores document-aware chunking outputs under a deterministic key.
type ChunkCache interface {
	GetChunks(ctx context.Context, key string) ([]Chunk, bool, error)
	PutChunks(ctx context.Context, key string, chunks []Chunk) error
}

// ChunkCacheKeyProvider lets chunkers include their own options in cache keys.
type ChunkCacheKeyProvider interface {
	ChunkCacheKey() string
}

// CachedDocumentChunker wraps a DocumentChunker with a deterministic cache.
//
// The cache key includes document bytes/text, filename, the cache schema
// version, and the wrapped chunker's option fingerprint when available.
type CachedDocumentChunker struct {
	Chunker DocumentChunker
	Cache   ChunkCache
}

func NewCachedDocumentChunker(chunker DocumentChunker, cache ChunkCache) *CachedDocumentChunker {
	return &CachedDocumentChunker{Chunker: chunker, Cache: cache}
}

func (c *CachedDocumentChunker) Split(ctx context.Context, text string) ([]Chunk, error) {
	if c == nil || c.Chunker == nil {
		return DefaultChunker().Split(ctx, text)
	}
	return c.Chunker.Split(ctx, text)
}

func (c *CachedDocumentChunker) SplitDocument(ctx context.Context, doc Document) ([]Chunk, error) {
	if c == nil || c.Chunker == nil {
		return DefaultChunker().Split(ctx, doc.Text)
	}
	if c.Cache == nil {
		return c.Chunker.SplitDocument(ctx, doc)
	}
	key := DocumentChunkCacheKey(doc, chunkerCacheKey(c.Chunker))
	if chunks, ok, err := c.Cache.GetChunks(ctx, key); err != nil {
		return nil, err
	} else if ok {
		return chunks, nil
	}
	chunks, err := c.Chunker.SplitDocument(ctx, doc)
	if err != nil {
		return nil, err
	}
	if err := c.Cache.PutChunks(ctx, key, chunks); err != nil {
		return nil, err
	}
	return chunks, nil
}

func DocumentChunkCacheKey(doc Document, chunkerKey string) string {
	h := sha256.New()
	writeCachePart(h, chunkCacheVersion)
	writeCachePart(h, strings.TrimSpace(doc.Filename))
	writeCachePart(h, chunkerKey)
	if len(doc.Data) > 0 {
		_, _ = h.Write(doc.Data)
	} else {
		_, _ = h.Write([]byte(doc.Text))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func chunkerCacheKey(chunker DocumentChunker) string {
	if provider, ok := chunker.(ChunkCacheKeyProvider); ok {
		return provider.ChunkCacheKey()
	}
	return fmt.Sprintf("%T", chunker)
}

func writeCachePart(h interface{ Write([]byte) (int, error) }, value string) {
	_, _ = h.Write([]byte(value))
	_, _ = h.Write([]byte{0})
}

type chunkCacheEnvelope struct {
	Version string  `json:"version"`
	Chunks  []Chunk `json:"chunks"`
}

// ArtifactChunkCache persists cached chunks in the configured artifact store.
type ArtifactChunkCache struct {
	Store  storage.ArtifactStore
	Prefix string
}

func NewArtifactChunkCache(store storage.ArtifactStore) *ArtifactChunkCache {
	return &ArtifactChunkCache{Store: store, Prefix: defaultChunkCachePrefix}
}

func (c *ArtifactChunkCache) GetChunks(ctx context.Context, key string) ([]Chunk, bool, error) {
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
	var envelope chunkCacheEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, false, err
	}
	if envelope.Version != chunkCacheVersion {
		return nil, false, nil
	}
	return envelope.Chunks, true, nil
}

func (c *ArtifactChunkCache) PutChunks(ctx context.Context, key string, chunks []Chunk) error {
	if c == nil || c.Store == nil {
		return nil
	}
	data, err := json.Marshal(chunkCacheEnvelope{
		Version: chunkCacheVersion,
		Chunks:  chunks,
	})
	if err != nil {
		return err
	}
	_, err = c.Store.Put(ctx, c.storageKey(key), data, "application/json")
	return err
}

func (c *ArtifactChunkCache) storageKey(key string) string {
	prefix := strings.Trim(strings.TrimSpace(c.Prefix), "/")
	if prefix == "" {
		prefix = defaultChunkCachePrefix
	}
	return path.Join(prefix, strings.TrimSpace(key)+".json")
}

// MemoryChunkCache is useful for tests and short-lived local runs.
type MemoryChunkCache struct {
	mu     sync.RWMutex
	chunks map[string][]Chunk
}

func NewMemoryChunkCache() *MemoryChunkCache {
	return &MemoryChunkCache{chunks: make(map[string][]Chunk)}
}

func (c *MemoryChunkCache) GetChunks(_ context.Context, key string) ([]Chunk, bool, error) {
	if c == nil {
		return nil, false, nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	chunks, ok := c.chunks[key]
	if !ok {
		return nil, false, nil
	}
	return cloneChunks(chunks), true, nil
}

func (c *MemoryChunkCache) PutChunks(_ context.Context, key string, chunks []Chunk) error {
	if c == nil {
		return errors.New("memory chunk cache is nil")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.chunks == nil {
		c.chunks = make(map[string][]Chunk)
	}
	c.chunks[key] = cloneChunks(chunks)
	return nil
}

func cloneChunks(chunks []Chunk) []Chunk {
	out := make([]Chunk, len(chunks))
	for i, chunk := range chunks {
		out[i] = chunk
		if chunk.Metadata != nil {
			out[i].Metadata = make(map[string]string, len(chunk.Metadata))
			for k, v := range chunk.Metadata {
				out[i].Metadata[k] = v
			}
		}
	}
	return out
}

var _ DocumentChunker = (*CachedDocumentChunker)(nil)
var _ ChunkCache = (*ArtifactChunkCache)(nil)
var _ ChunkCache = (*MemoryChunkCache)(nil)
