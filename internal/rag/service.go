package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/KPO-Tech/seshat/internal/storage"
	"github.com/KPO-Tech/seshat/internal/vector"
)

type Service struct {
	artifacts storage.ArtifactStore // optional — nil = skip rag-doc blob storage
	vectors   vector.Store
	embedder  Embedder
	chunker   Chunker
	reranker  Reranker // optional — nil = return vector results as-is
}

func NewService(artifacts storage.ArtifactStore, vectors vector.Store, embedder Embedder, chunker Chunker) *Service {
	if chunker == nil {
		chunker = DefaultChunker()
	}
	return &Service{
		artifacts: artifacts,
		vectors:   vectors,
		embedder:  embedder,
		chunker:   chunker,
	}
}

// SetReranker installs a second-pass reranker applied after vector retrieval.
// Pass nil to disable reranking.
func (s *Service) SetReranker(r Reranker) {
	if s == nil {
		return
	}
	s.reranker = r
}

// Vectors returns the underlying vector store, e.g. for callers that need to
// inspect raw records directly (tests, admin tooling).
func (s *Service) Vectors() vector.Store {
	if s == nil {
		return nil
	}
	return s.vectors
}

// DeleteNamespace removes all vector records for the given corpus namespace.
func (s *Service) DeleteNamespace(ctx context.Context, namespace string) error {
	if s == nil || s.vectors == nil {
		return nil
	}
	return s.vectors.DeleteNamespace(ctx, namespace)
}

// DeleteFileChunks removes stale chunk records that exceed the new chunk count.
// Called after re-ingesting a file that now has fewer chunks than its previous run,
// to avoid leaving orphaned vectors from the old (longer) version.
func (s *Service) DeleteFileChunks(ctx context.Context, namespace, artifactKey string, fromChunk, toChunk int) error {
	if s == nil || s.vectors == nil || fromChunk >= toChunk {
		return nil
	}
	keys := make([]string, 0, toChunk-fromChunk)
	for i := fromChunk; i < toChunk; i++ {
		keys = append(keys, fmt.Sprintf("%s#chunk-%d", artifactKey, i))
	}
	return s.vectors.DeleteKeys(ctx, namespace, keys)
}

// ArtifactKey builds the deterministic artifact key for a file within a
// corpus, matching what Ingest uses internally when FileID is set. Exported
// so callers (e.g. a delete-by-file tool) can reconstruct it without
// duplicating the format.
func ArtifactKey(corpusID, fileID string) string {
	return fmt.Sprintf("rag/%s/%s", corpusID, fileID)
}

func (s *Service) Ingest(ctx context.Context, request IngestRequest) (IngestResult, error) {
	if s == nil || s.vectors == nil {
		return IngestResult{}, fmt.Errorf("rag service is not fully configured (a vector store is required)")
	}
	if strings.TrimSpace(request.CorpusID) == "" {
		return IngestResult{}, fmt.Errorf("corpus id is required")
	}
	if strings.TrimSpace(request.Filename) == "" {
		return IngestResult{}, fmt.Errorf("filename is required")
	}
	if strings.TrimSpace(request.Text) == "" && len(request.Data) == 0 {
		return IngestResult{}, fmt.Errorf("text or data is required")
	}
	artifactPayload := []byte(request.Text)
	contentType := "text/plain"
	if len(artifactPayload) == 0 {
		artifactPayload = request.Data
		contentType = storage.DetectContentType(request.Filename)
	}

	// Build the artifact key. When FileID is provided we use a deterministic key so that
	// re-ingesting the same file produces identical chunk keys and vector Upsert replaces
	// in-place instead of creating orphaned duplicate records.
	var artifact storage.ArtifactRef
	if request.FileID != "" {
		deterministicKey := ArtifactKey(request.CorpusID, request.FileID)
		if s.artifacts != nil {
			if _, err := s.artifacts.Put(ctx, deterministicKey, artifactPayload, contentType); err != nil {
				return IngestResult{}, err
			}
		}
		artifact = storage.ArtifactRef{
			Key:  deterministicKey,
			Size: int64(len(artifactPayload)),
		}
	} else if s.artifacts != nil {
		// Fallback for callers that don't supply FileID: timestamp-based key via blob store.
		var err error
		artifact, err = storage.StoreRAGDocumentRef(ctx, s.artifacts, artifactPayload, request.Filename)
		if err != nil {
			return IngestResult{}, err
		}
	} else {
		// Synthetic ref: stable key derived from corpus + filename.
		artifact = storage.ArtifactRef{
			Key:  ArtifactKey(request.CorpusID, request.Filename),
			Size: int64(len(artifactPayload)),
		}
	}

	chunks, err := s.splitChunks(ctx, request)
	if err != nil {
		return IngestResult{}, fmt.Errorf("chunker: %w", err)
	}
	if len(chunks) == 0 {
		return IngestResult{Artifact: artifact}, nil
	}
	// vectorsOut stays nil when there's no embedder configured - records get
	// stored without a vector (vectorless/BM25-only), see vector.Record.Vector.
	var vectorsOut [][]float32
	if s.embedder != nil {
		texts := make([]string, 0, len(chunks))
		for _, chunk := range chunks {
			texts = append(texts, chunk.Text)
		}
		vectorsOut, err = s.embedder.EmbedTexts(ctx, texts)
		if err != nil {
			return IngestResult{}, err
		}
		if len(vectorsOut) != len(chunks) {
			return IngestResult{}, fmt.Errorf("embedder returned %d vectors for %d chunks", len(vectorsOut), len(chunks))
		}
	}
	records := make([]vector.Record, 0, len(chunks))
	for i, chunk := range chunks {
		key := fmt.Sprintf("%s#chunk-%d", artifact.Key, chunk.Position)
		chunk.Key = key
		var vec []float32
		if vectorsOut != nil {
			vec = vectorsOut[i]
		}
		metadata := map[string]string{
			"artifact_key": artifact.Key,
			"filename":     request.Filename,
			"position":     fmt.Sprintf("%d", chunk.Position),
		}
		for k, v := range chunk.Metadata {
			if strings.TrimSpace(k) == "" {
				continue
			}
			metadata[k] = v
		}
		if len(request.ScopeIDs) > 0 {
			// Encoded as a JSON array string - matchesFilter/pgFilterClause
			// both detect and unpack this representation transparently.
			if encoded, err := json.Marshal(request.ScopeIDs); err == nil {
				metadata["scope_id"] = string(encoded)
			}
		} else if request.ScopeID != "" {
			metadata["scope_id"] = request.ScopeID
		}
		records = append(records, vector.Record{
			Namespace: request.CorpusID,
			Key:       key,
			Text:      chunk.Text,
			Vector:    vec,
			Metadata:  metadata,
		})
	}
	if err := s.vectors.Upsert(ctx, records); err != nil {
		return IngestResult{}, err
	}

	// Re-ingesting a file that shrank (fewer chunks than its previous
	// version) would otherwise leave the old version's trailing chunks
	// behind - Upsert only replaces keys present in the new set, it can't
	// know a key from the old ingest no longer exists in the new one.
	// DeleteKeys silently no-ops on keys that were never written, so this
	// blind range-delete is safe even when there was no previous version
	// (the common case) or the new version is the same size or longer.
	if request.FileID != "" {
		if err := s.DeleteFileChunks(ctx, request.CorpusID, artifact.Key, len(records), len(records)+staleChunkCleanupCeiling); err != nil {
			return IngestResult{}, fmt.Errorf("cleanup stale chunks: %w", err)
		}
	}

	return IngestResult{
		Artifact: artifact,
		Chunks:   len(records),
	}, nil
}

func (s *Service) splitChunks(ctx context.Context, request IngestRequest) ([]Chunk, error) {
	if documentChunker, ok := s.chunker.(DocumentChunker); ok && len(request.Data) > 0 {
		return documentChunker.SplitDocument(ctx, Document{
			Filename: request.Filename,
			Text:     request.Text,
			Data:     request.Data,
		})
	}
	return s.chunker.Split(ctx, request.Text)
}

// staleChunkCleanupCeiling bounds how many chunk-position keys beyond the
// new chunk count are speculatively deleted after a FileID-based re-ingest,
// to catch a shrunk document's leftover trailing chunks without needing an
// expensive full-namespace scan to find the previous chunk count. Generous
// enough to cover any realistic single-document shrink.
const staleChunkCleanupCeiling = 500

func (s *Service) Search(ctx context.Context, request SearchRequest) (SearchResponse, error) {
	if s == nil || s.vectors == nil {
		return SearchResponse{}, fmt.Errorf("rag service is not fully configured (a vector store is required)")
	}
	if strings.TrimSpace(request.CorpusID) == "" {
		return SearchResponse{}, fmt.Errorf("corpus id is required")
	}
	if strings.TrimSpace(request.Query) == "" {
		return SearchResponse{}, fmt.Errorf("query is required")
	}

	topK := request.TopK
	if topK <= 0 {
		topK = 10
	}

	// When a reranker is active, retrieve a larger candidate pool first so the
	// reranker has more material to work with, then trim to the requested TopK.
	fetchK := topK
	useReranker := s.reranker != nil && s.reranker.IsConfigured()
	if useReranker {
		fetchK = topK * 3
		if fetchK < 20 {
			fetchK = 20
		}
	}

	// Vectorless (pure keyword/BM25) when no embedder is configured, or the
	// caller explicitly asked for pure keyword scoring - skips the embedding
	// call entirely instead of computing a query vector and discarding it.
	// Reranking still applies on top either way: it scores raw text against
	// the query, it doesn't need embeddings.
	var queryVector []float32
	if s.embedder != nil && request.HybridWeight < 1 {
		embeddings, err := s.embedder.EmbedTexts(ctx, []string{request.Query})
		if err != nil {
			return SearchResponse{}, err
		}
		if len(embeddings) != 1 {
			return SearchResponse{}, fmt.Errorf("embedder returned %d vectors for query", len(embeddings))
		}
		queryVector = embeddings[0]
	}
	results, err := s.vectors.Search(ctx, vector.Query{
		Namespace:    request.CorpusID,
		Vector:       queryVector,
		TopK:         fetchK,
		Filter:       request.Filter,
		HybridWeight: request.HybridWeight,
		QueryText:    request.Query,
	})
	if err != nil {
		return SearchResponse{}, err
	}

	// Apply reranker when configured. On failure, fall back to vector order silently.
	if useReranker && len(results) > 0 {
		texts := make([]string, 0, len(results))
		for _, r := range results {
			texts = append(texts, r.Record.Text)
		}
		indices, scores, rerankErr := s.reranker.Rerank(ctx, request.Query, texts, topK)
		if rerankErr == nil && len(indices) > 0 {
			reranked := make([]SearchResult, 0, len(indices))
			for i, idx := range indices {
				if idx < 0 || idx >= len(results) {
					continue
				}
				score := float32(0)
				if i < len(scores) {
					score = scores[i]
				}
				reranked = append(reranked, SearchResult{
					Key:      results[idx].Record.Key,
					Text:     results[idx].Record.Text,
					Score:    score,
					Metadata: results[idx].Record.Metadata,
				})
			}
			return SearchResponse{CorpusID: request.CorpusID, Results: reranked}, nil
		}
		// reranker failed — fall through to vector order below
	}

	// No reranker or reranker failed: return vector results trimmed to topK.
	limit := topK
	if limit > len(results) {
		limit = len(results)
	}
	response := SearchResponse{
		CorpusID: request.CorpusID,
		Results:  make([]SearchResult, 0, limit),
	}
	for _, result := range results[:limit] {
		response.Results = append(response.Results, SearchResult{
			Key:      result.Record.Key,
			Text:     result.Record.Text,
			Score:    result.Score,
			Metadata: result.Record.Metadata,
		})
	}
	return response, nil
}
