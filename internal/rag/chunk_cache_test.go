package rag

import (
	"context"
	"testing"

	"github.com/KPO-Tech/seshat/internal/storage"
)

type countingDocumentChunker struct {
	calls int
	key   string
}

func (c *countingDocumentChunker) Split(ctx context.Context, text string) ([]Chunk, error) {
	return []Chunk{{Text: text, Position: 0}}, nil
}

func (c *countingDocumentChunker) SplitDocument(ctx context.Context, doc Document) ([]Chunk, error) {
	c.calls++
	return []Chunk{{
		Text:     "chunked " + doc.Filename,
		Position: 0,
		Metadata: map[string]string{
			"source": "test",
		},
	}}, nil
}

func (c *countingDocumentChunker) ChunkCacheKey() string {
	return c.key
}

func TestCachedDocumentChunkerUsesCache(t *testing.T) {
	ctx := context.Background()
	inner := &countingDocumentChunker{key: "counting:v1"}
	chunker := NewCachedDocumentChunker(inner, NewMemoryChunkCache())
	doc := Document{
		Filename: "policy.pdf",
		Text:     "fallback text",
		Data:     []byte("%PDF"),
	}

	first, err := chunker.SplitDocument(ctx, doc)
	if err != nil {
		t.Fatalf("first SplitDocument: %v", err)
	}
	second, err := chunker.SplitDocument(ctx, doc)
	if err != nil {
		t.Fatalf("second SplitDocument: %v", err)
	}
	if inner.calls != 1 {
		t.Fatalf("expected inner chunker once, got %d calls", inner.calls)
	}
	if second[0].Text != first[0].Text {
		t.Fatalf("expected cached chunks, got %#v then %#v", first, second)
	}
	second[0].Metadata["source"] = "mutated"
	third, err := chunker.SplitDocument(ctx, doc)
	if err != nil {
		t.Fatalf("third SplitDocument: %v", err)
	}
	if third[0].Metadata["source"] != "test" {
		t.Fatalf("cache should clone chunks, got metadata %#v", third[0].Metadata)
	}
}

func TestDocumentChunkCacheKeyIncludesDocumentAndChunkerOptions(t *testing.T) {
	doc := Document{Filename: "policy.pdf", Data: []byte("a")}
	base := DocumentChunkCacheKey(doc, "docling:{max_tokens:512}")
	if base == DocumentChunkCacheKey(Document{Filename: "policy.pdf", Data: []byte("b")}, "docling:{max_tokens:512}") {
		t.Fatal("expected document content to affect cache key")
	}
	if base == DocumentChunkCacheKey(doc, "docling:{max_tokens:1024}") {
		t.Fatal("expected chunker options to affect cache key")
	}
	if base == DocumentChunkCacheKey(Document{Filename: "other.pdf", Data: []byte("a")}, "docling:{max_tokens:512}") {
		t.Fatal("expected filename to affect cache key")
	}
}

func TestArtifactChunkCachePersistsChunks(t *testing.T) {
	ctx := context.Background()
	provider, err := storage.NewProviderFromConfig(storage.Config{
		Provider:  storage.ProviderLocal,
		LocalPath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewProviderFromConfig: %v", err)
	}
	store := storage.NewArtifactStore(provider)
	cache := NewArtifactChunkCache(store)

	err = cache.PutChunks(ctx, "abc123", []Chunk{{
		Text:     "cached",
		Position: 2,
		Metadata: map[string]string{
			"page_numbers": "[3]",
		},
	}})
	if err != nil {
		t.Fatalf("PutChunks: %v", err)
	}
	chunks, ok, err := cache.GetChunks(ctx, "abc123")
	if err != nil {
		t.Fatalf("GetChunks: %v", err)
	}
	if !ok {
		t.Fatal("expected cache hit")
	}
	if len(chunks) != 1 || chunks[0].Text != "cached" || chunks[0].Metadata["page_numbers"] != "[3]" {
		t.Fatalf("unexpected cached chunks: %#v", chunks)
	}
}
