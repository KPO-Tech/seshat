package vector

import "context"

// Record is one vectorized chunk stored in a vector backend.
type Record struct {
	Namespace string            `json:"namespace"`
	Key       string            `json:"key"`
	Text      string            `json:"text,omitempty"`
	Vector    []float32         `json:"vector"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// Query describes a vector similarity search.
// Filter applies optional metadata predicates server-side (where supported)
// or client-side (SQLite / in-memory backends).
//
// Simple equality:  {"source_file": "doc.txt"}
// IN operator:      {"source_file": {"$in": ["a.txt", "b.txt"]}}
type Query struct {
	Namespace string
	Vector    []float32
	TopK      int
	Filter    map[string]any // optional; nil = no filter

	// HybridWeight blends BM25 keyword search with vector similarity.
	//   0   (default) → pure vector
	//   1             → pure BM25
	//   0 < w < 1    → linear blend: (1-w)*vector + w*bm25
	// Only the SQLite and pgvector backends implement BM25; others ignore it.
	HybridWeight float32

	// QueryText is the raw query string used for BM25 scoring.
	// Must be non-empty when HybridWeight > 0.
	QueryText string
}

// SearchResult is one ranked vector search hit.
type SearchResult struct {
	Record Record  `json:"record"`
	Score  float32 `json:"score"`
}

// Store abstracts the vector backend used by RAG.
// Concrete implementations target SQLite, pgvector, Qdrant, Chroma, or any
// future provider — the rag.Service and knowledge.Service call only this interface.
type Store interface {
	// Upsert inserts or replaces records (namespace + key = primary key).
	Upsert(ctx context.Context, records []Record) error

	// Search performs vector similarity search within a namespace.
	// Query.Filter is applied as an AND predicate on record metadata.
	Search(ctx context.Context, query Query) ([]SearchResult, error)

	// Get retrieves records by key. If keys is nil or empty, all records in
	// the namespace are returned.
	Get(ctx context.Context, namespace string, keys []string) ([]Record, error)

	// HasNamespace reports whether the namespace contains at least one record.
	HasNamespace(ctx context.Context, namespace string) (bool, error)

	// DeleteNamespace removes all records for a namespace.
	DeleteNamespace(ctx context.Context, namespace string) error

	// DeleteKeys removes specific records within a namespace.
	DeleteKeys(ctx context.Context, namespace string, keys []string) error
}
