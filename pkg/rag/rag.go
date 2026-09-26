package rag

import (
	internalrag "github.com/KPO-Tech/seshat/internal/rag"
	publicreader "github.com/KPO-Tech/seshat/pkg/documentreader"
	publicstorage "github.com/KPO-Tech/seshat/pkg/storage"
	publicvector "github.com/KPO-Tech/seshat/pkg/vector"
)

type (
	Chunk                    = internalrag.Chunk
	ChunkCache               = internalrag.ChunkCache
	ChunkCacheKeyProvider    = internalrag.ChunkCacheKeyProvider
	ChunkProfile             = internalrag.ChunkProfile
	ChunkProfileName         = internalrag.ChunkProfileName
	Chunker                  = internalrag.Chunker
	CachedDocumentChunker    = internalrag.CachedDocumentChunker
	Document                 = internalrag.Document
	DocumentChunker          = internalrag.DocumentChunker
	HybridDocumentChunker    = internalrag.HybridDocumentChunker
	Embedder                 = internalrag.Embedder
	Enricher                 = internalrag.Enricher
	EnrichmentCache          = internalrag.EnrichmentCache
	EnricherCacheKeyProvider = internalrag.EnricherCacheKeyProvider
	CachedEnricher           = internalrag.CachedEnricher
	ArtifactEnrichmentCache  = internalrag.ArtifactEnrichmentCache
	MemoryEnrichmentCache    = internalrag.MemoryEnrichmentCache
	IngestRequest            = internalrag.IngestRequest
	IngestResult             = internalrag.IngestResult
	Reranker                 = internalrag.Reranker
	SearchRequest            = internalrag.SearchRequest
	SearchResponse           = internalrag.SearchResponse
	SearchResult             = internalrag.SearchResult
	Service                  = internalrag.Service
	VectorStore              = publicvector.Store
	ArtifactChunkCache       = internalrag.ArtifactChunkCache
	MemoryChunkCache         = internalrag.MemoryChunkCache

	// ParagraphChunker splits on blank lines with a hard character cap per
	// chunk. The default Chunker (see DefaultChunker) when none is given.
	ParagraphChunker = internalrag.ParagraphChunker

	// SemanticChunker groups sentences into chunks by embedding-similarity
	// instead of blind paragraph splitting - meaningfully better retrieval
	// quality, at the cost of one extra embedding call per sentence during
	// ingest. Falls back to ParagraphChunker behavior if the embedder errors.
	SemanticChunker = internalrag.SemanticChunker

	// TableChunker keeps GFM/Markdown table rows intact as their own
	// chunk(s), splitting an oversized table by data row while repeating
	// the header row on every piece, instead of a generic splitter cutting
	// through the middle of a table. Falls back to HeadingChunker behavior
	// for a document with no table.
	TableChunker = internalrag.TableChunker

	// QAChunker splits a question/answer-formatted document (FAQs, support
	// docs) into one chunk per Q/A pair, detected either from explicit
	// "Q:"/"A:" labels or from Markdown headings ending in "?". Falls back
	// to HeadingChunker behavior for a document with no QA structure.
	QAChunker = internalrag.QAChunker
)

const (
	ChunkProfileSmall      = internalrag.ChunkProfileSmall
	ChunkProfileMedium     = internalrag.ChunkProfileMedium
	ChunkProfileLarge      = internalrag.ChunkProfileLarge
	ChunkProfileStructured = internalrag.ChunkProfileStructured
	ChunkProfileTable      = internalrag.ChunkProfileTable
	ChunkProfileQA         = internalrag.ChunkProfileQA
	ChunkProfileCustom     = internalrag.ChunkProfileCustom
)

func NewService(artifacts publicstorage.ArtifactStore, vectors publicvector.Store, embedder Embedder, chunker Chunker) *Service {
	return internalrag.NewService(artifacts, vectors, embedder, chunker)
}

// DefaultChunker returns a ParagraphChunker with sensible defaults - what
// NewService uses internally when chunker is nil.
func DefaultChunker() Chunker {
	return internalrag.DefaultChunker()
}

// NewHybridDocumentChunker creates a document-aware chunker backed by the given
// reader.
func NewHybridDocumentChunker(client publicreader.HybridChunker, opts publicreader.ChunkOptions) *HybridDocumentChunker {
	return internalrag.NewHybridDocumentChunker(client, opts)
}

func NewHybridDocumentChunkerForProfile(client publicreader.HybridChunker, profile ChunkProfile, opts publicreader.ChunkOptions) *HybridDocumentChunker {
	return internalrag.NewHybridDocumentChunkerForProfile(client, profile, opts)
}

func DefaultChunkProfile() ChunkProfile {
	return internalrag.DefaultChunkProfile()
}

func RecommendedChunkProfile(name ChunkProfileName) (ChunkProfile, bool) {
	return internalrag.RecommendedChunkProfile(name)
}

func NewCustomChunkProfile(maxTokens, overlapTokens int) (ChunkProfile, error) {
	return internalrag.NewCustomChunkProfile(maxTokens, overlapTokens)
}

func DocumentReaderChunkOptionsForProfile(profile ChunkProfile, opts publicreader.ChunkOptions) publicreader.ChunkOptions {
	return internalrag.DocumentReaderChunkOptionsForProfile(profile, opts)
}

func NewCachedDocumentChunker(chunker DocumentChunker, cache ChunkCache) *CachedDocumentChunker {
	return internalrag.NewCachedDocumentChunker(chunker, cache)
}

func NewArtifactChunkCache(store publicstorage.ArtifactStore) *ArtifactChunkCache {
	return internalrag.NewArtifactChunkCache(store)
}

func NewMemoryChunkCache() *MemoryChunkCache {
	return internalrag.NewMemoryChunkCache()
}

func DocumentChunkCacheKey(doc Document, chunkerKey string) string {
	return internalrag.DocumentChunkCacheKey(doc, chunkerKey)
}

// NewCachedEnricher wraps enricher with a deterministic cache (see
// internal/rag.CachedEnricher's doc comment) so a re-ingest of unchanged
// chunks doesn't re-bill the LLM for enrichment.
func NewCachedEnricher(enricher Enricher, cache EnrichmentCache) *CachedEnricher {
	return internalrag.NewCachedEnricher(enricher, cache)
}

func NewArtifactEnrichmentCache(store publicstorage.ArtifactStore) *ArtifactEnrichmentCache {
	return internalrag.NewArtifactEnrichmentCache(store)
}

func NewMemoryEnrichmentCache() *MemoryEnrichmentCache {
	return internalrag.NewMemoryEnrichmentCache()
}

// ChunkEnrichmentCacheKey builds the deterministic cache key for a chunk's
// enrichment result, matching what CachedEnricher uses internally - useful
// for callers that want to pre-warm or invalidate the cache directly.
func ChunkEnrichmentCacheKey(chunkText, enricherKey string) string {
	return internalrag.ChunkEnrichmentCacheKey(chunkText, enricherKey)
}

// NewSemanticChunker creates a SemanticChunker. threshold <= 0 uses the
// default (0.3) - see SemanticChunker's doc for the tradeoff it makes.
func NewSemanticChunker(embedder Embedder, threshold float32) *SemanticChunker {
	return internalrag.NewSemanticChunker(embedder, threshold)
}

// NewTableChunker creates a table-aware chunker using one of Seshat's
// recommended chunk profiles - see TableChunker's own doc comment.
func NewTableChunker(profile ChunkProfile) *TableChunker {
	return internalrag.NewTableChunker(profile)
}

// NewQAChunker creates a QA-pair-aware chunker using one of Seshat's
// recommended chunk profiles - see QAChunker's own doc comment.
func NewQAChunker(profile ChunkProfile) *QAChunker {
	return internalrag.NewQAChunker(profile)
}

// ArtifactKey builds the deterministic artifact key for a file within a
// corpus (format: "rag/{corpusID}/{fileID}"), matching what Ingest uses
// internally when IngestRequest.FileID is set. Useful for a caller building
// their own delete-by-file tooling on top of Service.DeleteFileChunks.
func ArtifactKey(corpusID, fileID string) string {
	return internalrag.ArtifactKey(corpusID, fileID)
}
