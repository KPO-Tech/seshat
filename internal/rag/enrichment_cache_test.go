package rag

import (
	"context"
	"testing"

	"github.com/KPO-Tech/seshat/internal/storage"
)

// countingEnricher records how many texts it was actually asked to enrich
// (across all calls), and returns one deterministic question per text.
type countingEnricher struct {
	calls int
	seen  []string
	key   string
}

func (c *countingEnricher) EnrichChunks(_ context.Context, texts []string) ([][]string, error) {
	c.calls++
	c.seen = append(c.seen, texts...)
	out := make([][]string, len(texts))
	for i, text := range texts {
		out[i] = []string{"Q: " + text}
	}
	return out, nil
}

func (c *countingEnricher) EnricherCacheKey() string {
	return c.key
}

func TestCachedEnricherUsesCache(t *testing.T) {
	ctx := context.Background()
	inner := &countingEnricher{key: "counting:v1"}
	enricher := NewCachedEnricher(inner, NewMemoryEnrichmentCache())

	first, err := enricher.EnrichChunks(ctx, []string{"chunk a", "chunk b"})
	if err != nil {
		t.Fatalf("EnrichChunks: %v", err)
	}
	if inner.calls != 1 {
		t.Fatalf("expected 1 underlying call, got %d", inner.calls)
	}
	if len(first) != 2 || first[0][0] != "Q: chunk a" || first[1][0] != "Q: chunk b" {
		t.Fatalf("unexpected first results: %v", first)
	}

	// Re-running with the same texts should hit the cache entirely - no
	// further calls to the underlying enricher.
	second, err := enricher.EnrichChunks(ctx, []string{"chunk a", "chunk b"})
	if err != nil {
		t.Fatalf("EnrichChunks (cached): %v", err)
	}
	if inner.calls != 1 {
		t.Fatalf("expected cache hit to avoid a second call, got %d calls", inner.calls)
	}
	if len(second) != 2 || second[0][0] != "Q: chunk a" || second[1][0] != "Q: chunk b" {
		t.Fatalf("unexpected cached results: %v", second)
	}
}

func TestCachedEnricherOnlyCallsUnderlyingForMisses(t *testing.T) {
	ctx := context.Background()
	inner := &countingEnricher{key: "counting:v1"}
	enricher := NewCachedEnricher(inner, NewMemoryEnrichmentCache())

	if _, err := enricher.EnrichChunks(ctx, []string{"chunk a"}); err != nil {
		t.Fatalf("EnrichChunks (warm cache): %v", err)
	}
	inner.seen = nil

	// A batch with one cached text and one new text should only forward the
	// new one to the underlying enricher.
	results, err := enricher.EnrichChunks(ctx, []string{"chunk a", "chunk c"})
	if err != nil {
		t.Fatalf("EnrichChunks (mixed): %v", err)
	}
	if len(inner.seen) != 1 || inner.seen[0] != "chunk c" {
		t.Fatalf("expected underlying enricher to see only [\"chunk c\"], got %v", inner.seen)
	}
	if results[0][0] != "Q: chunk a" || results[1][0] != "Q: chunk c" {
		t.Fatalf("unexpected mixed results: %v", results)
	}
}

func TestCachedEnricherKeyChangeInvalidatesCache(t *testing.T) {
	ctx := context.Background()
	cache := NewMemoryEnrichmentCache()

	first := NewCachedEnricher(&countingEnricher{key: "v1"}, cache)
	if _, err := first.EnrichChunks(ctx, []string{"chunk a"}); err != nil {
		t.Fatalf("EnrichChunks (v1): %v", err)
	}

	// A different EnricherCacheKey (e.g. a config change) must not reuse v1's
	// cache entries.
	secondInner := &countingEnricher{key: "v2"}
	second := NewCachedEnricher(secondInner, cache)
	if _, err := second.EnrichChunks(ctx, []string{"chunk a"}); err != nil {
		t.Fatalf("EnrichChunks (v2): %v", err)
	}
	if secondInner.calls != 1 {
		t.Fatalf("expected v2 to miss v1's cache entry and call through, got %d calls", secondInner.calls)
	}
}

func TestArtifactEnrichmentCacheRoundTrip(t *testing.T) {
	ctx := context.Background()
	provider, err := storage.NewProviderFromConfig(storage.Config{
		Provider:  storage.ProviderLocal,
		LocalPath: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("NewProviderFromConfig: %v", err)
	}
	store := storage.NewArtifactStore(provider)
	cache := NewArtifactEnrichmentCache(store)

	key := ChunkEnrichmentCacheKey("some chunk text", "enricher:v1")
	if _, ok, err := cache.GetEnrichment(ctx, key); err != nil {
		t.Fatalf("GetEnrichment (miss): %v", err)
	} else if ok {
		t.Fatalf("expected cache miss before any Put")
	}

	want := []string{"What does this do?", "Why does it matter?"}
	if err := cache.PutEnrichment(ctx, key, want); err != nil {
		t.Fatalf("PutEnrichment: %v", err)
	}
	got, ok, err := cache.GetEnrichment(ctx, key)
	if err != nil {
		t.Fatalf("GetEnrichment (hit): %v", err)
	}
	if !ok {
		t.Fatalf("expected cache hit after Put")
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("GetEnrichment = %v, want %v", got, want)
	}
}
