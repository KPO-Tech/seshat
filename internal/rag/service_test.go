package rag

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/EngineerProjects/nexus-engine/internal/storage"
	"github.com/EngineerProjects/nexus-engine/internal/vector"
)

// fakeEmbedder returns deterministic embeddings based on keyword content.
type fakeEmbedder struct{}

func (fakeEmbedder) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for _, text := range texts {
		lower := strings.ToLower(text)
		switch {
		case strings.Contains(lower, "alpha"):
			out = append(out, []float32{1, 0})
		case strings.Contains(lower, "beta"):
			out = append(out, []float32{0, 1})
		default:
			out = append(out, []float32{1, 1})
		}
	}
	return out, nil
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	tmpDir := t.TempDir()
	storage.SetConfig(storage.Config{
		Provider:  storage.ProviderLocal,
		LocalPath: tmpDir,
	})
	t.Cleanup(storage.ResetProvider)
	artifacts, err := storage.DefaultArtifactStore()
	if err != nil {
		t.Fatalf("DefaultArtifactStore: %v", err)
	}
	return NewService(artifacts, vector.NewMemoryStore(), fakeEmbedder{}, nil)
}

func TestServiceIngestAndSearch(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	ingested, err := svc.Ingest(ctx, IngestRequest{
		CorpusID: "kb",
		Filename: "notes.txt",
		Text:     "alpha section\n\nbeta section",
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}
	if ingested.Chunks != 2 {
		t.Fatalf("expected 2 chunks, got %d", ingested.Chunks)
	}

	resp, err := svc.Search(ctx, SearchRequest{CorpusID: "kb", Query: "alpha", TopK: 1})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if !strings.Contains(strings.ToLower(resp.Results[0].Text), "alpha") {
		t.Fatalf("unexpected top match: %s", resp.Results[0].Text)
	}
}

func TestServiceSearch_WithFilter(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	_, err := svc.Ingest(ctx, IngestRequest{
		CorpusID: "filtered",
		FileID:   "doc-a",
		Filename: "doc-a.txt",
		Text:     "alpha content here",
	})
	if err != nil {
		t.Fatalf("Ingest A: %v", err)
	}
	_, err = svc.Ingest(ctx, IngestRequest{
		CorpusID: "filtered",
		FileID:   "doc-b",
		Filename: "doc-b.txt",
		Text:     "alpha content there",
	})
	if err != nil {
		t.Fatalf("Ingest B: %v", err)
	}

	// Filter to only doc-a by filename.
	resp, err := svc.Search(ctx, SearchRequest{
		CorpusID: "filtered",
		Query:    "alpha",
		TopK:     10,
		Filter:   map[string]any{"filename": "doc-a.txt"},
	})
	if err != nil {
		t.Fatalf("Search with filter: %v", err)
	}
	for _, r := range resp.Results {
		if r.Metadata["filename"] != "doc-a.txt" {
			t.Errorf("filter leaked result from %s", r.Metadata["filename"])
		}
	}
}

// ─── Chunker tests ────────────────────────────────────────────────────────────

func TestParagraphChunker_Split(t *testing.T) {
	ctx := context.Background()
	c := ParagraphChunker{MaxChunkChars: 50}
	chunks, err := c.Split(ctx, "First paragraph.\n\nSecond paragraph.\n\nThird.")
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if chunks[0].Position != 0 || chunks[1].Position != 1 {
		t.Fatalf("positions wrong: %v", chunks)
	}
}

func TestParagraphChunker_HardCap(t *testing.T) {
	ctx := context.Background()
	c := ParagraphChunker{MaxChunkChars: 10}
	chunks, err := c.Split(ctx, "abcdefghijklmnopqrstuvwxyz") // 26 chars, no newlines
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected at least 2 chunks for text exceeding MaxChunkChars, got %d", len(chunks))
	}
	for _, ch := range chunks {
		if len(ch.Text) > 10 {
			t.Errorf("chunk exceeds cap: %q (%d chars)", ch.Text, len(ch.Text))
		}
	}
}

func TestSemanticChunker_FallbackOnEmbedderError(t *testing.T) {
	ctx := context.Background()
	// An embedder that always fails.
	errEmbed := &errorEmbedder{}
	sc := NewSemanticChunker(errEmbed, 0.3)
	// Should fall back to ParagraphChunker without returning an error.
	chunks, err := sc.Split(ctx, "First paragraph.\n\nSecond paragraph.")
	if err != nil {
		t.Fatalf("expected graceful fallback, got error: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk on fallback")
	}
}

func TestSemanticChunker_GroupsBySimilarity(t *testing.T) {
	ctx := context.Background()
	// Embedder that returns identical vectors for "A" sentences and orthogonal
	// vectors for "B" sentences, so the chunker splits at the boundary.
	sc := NewSemanticChunker(semanticTestEmbedder{}, 0.5)
	sc.MinChunkChars = 1
	text := "A sentence one. A sentence two. B sentence three. B sentence four."
	chunks, err := sc.Split(ctx, text)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	// At least two chunks: the A group and the B group.
	if len(chunks) < 2 {
		t.Fatalf("expected ≥2 semantic chunks, got %d: %v", len(chunks), chunks)
	}
}

func TestSplitSentences_Basic(t *testing.T) {
	sentences := splitSentences("Hello world. How are you? Fine!")
	if len(sentences) < 2 {
		t.Fatalf("expected at least 2 sentences, got %d: %v", len(sentences), sentences)
	}
}

// ─── Test helpers ─────────────────────────────────────────────────────────────

type errorEmbedder struct{}

func (errorEmbedder) EmbedTexts(_ context.Context, _ []string) ([][]float32, error) {
	return nil, fmt.Errorf("embedding service down")
}

// semanticTestEmbedder returns [1,0] for sentences containing "A" and [0,1] for "B".
type semanticTestEmbedder struct{}

func (semanticTestEmbedder) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, t := range texts {
		if strings.Contains(t, "A") {
			out[i] = []float32{1, 0}
		} else {
			out[i] = []float32{0, 1}
		}
	}
	return out, nil
}
