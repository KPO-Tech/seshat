package rag

import (
	"context"

	"github.com/KPO-Tech/seshat/internal/storage"
	"github.com/KPO-Tech/seshat/internal/vector"
)

type Embedder interface {
	EmbedTexts(ctx context.Context, texts []string) ([][]float32, error)
}

type Chunk struct {
	Key      string
	Text     string
	Position int
	Metadata map[string]string
}

type IngestRequest struct {
	CorpusID string
	// FileID, when non-empty, produces a deterministic artifact key ("rag/{CorpusID}/{FileID}").
	// This makes Upsert idempotent: re-ingesting the same file replaces its vectors in-place
	// rather than creating orphaned duplicates alongside the old ones.
	FileID   string
	Filename string
	Text     string
	// Data carries the original document bytes when ingestion can provide
	// them. Chunkers that understand document structure can use it instead of
	// splitting the already-extracted Text representation.
	Data []byte
	// ScopeID tags every chunk produced from this ingest with a permission
	// scope (a workspace ID for shared content, or an owning user ID for
	// personal content). Empty means unscoped — no scope_id metadata is
	// written, and the chunk is invisible to any search that filters on it.
	// Mutually exclusive with ScopeIDs - set at most one of the two.
	ScopeID string
	// ScopeIDs tags every chunk with multiple permission scopes at once
	// (e.g. a connector-synced document visible to several identities:
	// specific users, groups, a domain). Takes precedence over ScopeID when
	// both would apply from a caller's perspective - callers should set
	// only one. Encoded as a JSON array in the scope_id metadata field, so
	// a single "$in" filter transparently matches both the scalar (ScopeID)
	// and multi-valued (ScopeIDs) representations - see vector.matchesFilter
	// and pgvector_store.go's pgFilterClause.
	ScopeIDs []string
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

// Document is the original source material plus its text representation.
// Plain text chunkers can ignore Data. Document-aware chunkers can prefer Data
// to preserve layout, headings, page numbers, tables, and captions.
type Document struct {
	Filename string
	Text     string
	Data     []byte
}

// Chunker splits a text into indexable chunks.
// The context allows implementations that call remote services (e.g. SemanticChunker).
type Chunker interface {
	Split(ctx context.Context, text string) ([]Chunk, error)
}

// DocumentChunker is implemented by chunkers that can split the original
// document, not just extracted plain text.
type DocumentChunker interface {
	Chunker
	SplitDocument(ctx context.Context, doc Document) ([]Chunk, error)
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
