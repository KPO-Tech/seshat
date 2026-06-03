package rag

import (
	"context"

	"github.com/EngineerProjects/nexus-engine/internal/storage"
	"github.com/EngineerProjects/nexus-engine/internal/vector"
)

type Embedder interface {
	EmbedTexts(ctx context.Context, texts []string) ([][]float32, error)
}

type Chunk struct {
	Key      string
	Text     string
	Position int
}

type IngestRequest struct {
	CorpusID string
	// FileID, when non-empty, produces a deterministic artifact key ("rag/{CorpusID}/{FileID}").
	// This makes Upsert idempotent: re-ingesting the same file replaces its vectors in-place
	// rather than creating orphaned duplicates alongside the old ones.
	FileID   string
	Filename string
	Text     string
}

type IngestResult struct {
	Artifact storage.ArtifactRef
	Chunks   int
}

type SearchRequest struct {
	CorpusID string
	Query    string
	TopK     int
	// HybridWeight blends vector similarity with BM25 keyword search.
	// 0 (default) = pure vector; 1 = pure BM25; intermediate = linear blend.
	HybridWeight float32
	// Filter restricts results by metadata key-value predicates.
	// Passed through to vector.Query.Filter unchanged.
	Filter map[string]any
}

type SearchResult struct {
	Key      string            `json:"key"`
	Text     string            `json:"text"`
	Score    float32           `json:"score"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type SearchResponse struct {
	CorpusID string         `json:"corpus_id"`
	Results  []SearchResult `json:"results"`
}

// Chunker splits a text into indexable chunks.
// The context allows implementations that call remote services (e.g. SemanticChunker).
type Chunker interface {
	Split(ctx context.Context, text string) ([]Chunk, error)
}

type VectorStore = vector.Store

// Reranker reorders a candidate set of documents by semantic relevance.
// Implementations are optional — when nil the RAG service returns vector
// results in score order without a second-pass rerank.
//
// Rerank returns (indices, scores) where indices[i] is the position of the
// i-th most relevant document in the original docs slice. Pass topN=0 to
// return all results.
type Reranker interface {
	Rerank(ctx context.Context, query string, docs []string, topN int) (indices []int, scores []float32, err error)
	IsConfigured() bool
}
