package rag

import (
	"context"
	"fmt"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/storage"
	"github.com/EngineerProjects/nexus-engine/internal/vector"
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

func (s *Service) Ingest(ctx context.Context, request IngestRequest) (IngestResult, error) {
	if s == nil || s.vectors == nil || s.embedder == nil {
		return IngestResult{}, fmt.Errorf("rag service is not fully configured (vectors and embedder required)")
	}
	if strings.TrimSpace(request.CorpusID) == "" {
		return IngestResult{}, fmt.Errorf("corpus id is required")
	}
	if strings.TrimSpace(request.Filename) == "" {
		return IngestResult{}, fmt.Errorf("filename is required")
	}
	if strings.TrimSpace(request.Text) == "" {
		return IngestResult{}, fmt.Errorf("text is required")
	}

	// Build the artifact key. When FileID is provided we use a deterministic key so that
	// re-ingesting the same file produces identical chunk keys and vector Upsert replaces
	// in-place instead of creating orphaned duplicate records.
	var artifact storage.ArtifactRef
	if request.FileID != "" {
		deterministicKey := fmt.Sprintf("rag/%s/%s", request.CorpusID, request.FileID)
		if s.artifacts != nil {
			if _, err := s.artifacts.Put(ctx, deterministicKey, []byte(request.Text), "text/plain"); err != nil {
				return IngestResult{}, err
			}
		}
		artifact = storage.ArtifactRef{
			Key:  deterministicKey,
			Size: int64(len(request.Text)),
		}
	} else if s.artifacts != nil {
		// Fallback for callers that don't supply FileID: timestamp-based key via blob store.
		var err error
		artifact, err = storage.StoreRAGDocumentRef(ctx, s.artifacts, []byte(request.Text), request.Filename)
		if err != nil {
			return IngestResult{}, err
		}
	} else {
		// Synthetic ref: stable key derived from corpus + filename.
		artifact = storage.ArtifactRef{
			Key:  fmt.Sprintf("rag/%s/%s", request.CorpusID, request.Filename),
			Size: int64(len(request.Text)),
		}
	}

	chunks, err := s.chunker.Split(ctx, request.Text)
	if err != nil {
		return IngestResult{}, fmt.Errorf("chunker: %w", err)
	}
	if len(chunks) == 0 {
		return IngestResult{Artifact: artifact}, nil
	}
	texts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		texts = append(texts, chunk.Text)
	}
	vectorsOut, err := s.embedder.EmbedTexts(ctx, texts)
	if err != nil {
		return IngestResult{}, err
	}
	if len(vectorsOut) != len(chunks) {
		return IngestResult{}, fmt.Errorf("embedder returned %d vectors for %d chunks", len(vectorsOut), len(chunks))
	}
	records := make([]vector.Record, 0, len(chunks))
	for i, chunk := range chunks {
		key := fmt.Sprintf("%s#chunk-%d", artifact.Key, chunk.Position)
		chunk.Key = key
		records = append(records, vector.Record{
			Namespace: request.CorpusID,
			Key:       key,
			Text:      chunk.Text,
			Vector:    vectorsOut[i],
			Metadata: map[string]string{
				"artifact_key": artifact.Key,
				"filename":     request.Filename,
				"position":     fmt.Sprintf("%d", chunk.Position),
			},
		})
	}
	if err := s.vectors.Upsert(ctx, records); err != nil {
		return IngestResult{}, err
	}
	return IngestResult{
		Artifact: artifact,
		Chunks:   len(records),
	}, nil
}

func (s *Service) Search(ctx context.Context, request SearchRequest) (SearchResponse, error) {
	if s == nil || s.vectors == nil || s.embedder == nil {
		return SearchResponse{}, fmt.Errorf("rag service is not fully configured")
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

	embeddings, err := s.embedder.EmbedTexts(ctx, []string{request.Query})
	if err != nil {
		return SearchResponse{}, err
	}
	if len(embeddings) != 1 {
		return SearchResponse{}, fmt.Errorf("embedder returned %d vectors for query", len(embeddings))
	}
	results, err := s.vectors.Search(ctx, vector.Query{
		Namespace:    request.CorpusID,
		Vector:       embeddings[0],
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
