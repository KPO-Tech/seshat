// Package reranker implements RAG's optional second-pass reranking step: an
// HTTP client speaking the Cohere-compatible rerank API shape
// ({query, documents, top_n} -> {results: [{index, relevance_score}]}),
// which LangSearch's hosted API, vLLM's built-in reranker endpoint, and
// Cohere itself all speak. HuggingFace Text Embeddings Inference (TEI) uses
// a slightly different shape ({query, texts} -> a bare array of
// {index, score}) - HTTPReranker sends both request field names and parses
// both response shapes, so the same client works against either without
// needing to know in advance which server is on the other end.
//
// This means a fully free, self-hosted alternative to LangSearch's hosted
// API is just a BaseURL away: point RAG_RERANK_URL at a local TEI or vLLM
// instance serving BAAI/bge-reranker-v2-m3 (Apache 2.0, runs on CPU) and no
// API key or external network call is needed at all.
package reranker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	langSearchBaseURL = "https://api.langsearch.com/v1/rerank"
	langSearchModel   = "langsearch-reranker-v1"

	defaultTimeout = 15 * time.Second
)

// Config configures an HTTPReranker.
type Config struct {
	// BaseURL is the full rerank endpoint URL, e.g.
	// "https://api.langsearch.com/v1/rerank" or "http://localhost:8080/rerank"
	// for a self-hosted TEI/vLLM instance. Empty means unconfigured -
	// IsConfigured() reports false and Rerank refuses to run.
	BaseURL string

	// APIKey is sent as "Authorization: Bearer <key>" when non-empty.
	// Self-hosted servers (TEI, a local vLLM instance) typically don't need
	// one; LangSearch and Cohere do.
	APIKey string

	// Model is sent as the "model" request field when non-empty. Ignored by
	// single-model servers like TEI, required by LangSearch/Cohere/vLLM.
	Model string

	// Style selects the request shape of a hosted provider (StyleCohere,
	// StyleVoyage); empty sends the fields of every known server, which is
	// what a self-hosted endpoint wants.
	Style string

	Timeout time.Duration
}

// FromEnv reads reranker configuration from environment variables:
//
//	RAG_RERANK_PROVIDER — a provider of the catalog (cohere, voyage, openrouter, langsearch): its endpoint,
//	                      request shape, default model and key variable (COHERE_API_KEY, VOYAGE_API_KEY,
//	                      OPENROUTER_API_KEY, LANGSEARCH_API_KEY) are used
//	RAG_RERANK_URL      — full rerank endpoint URL (self-hosted TEI/vLLM, or any Cohere-compatible service);
//	                      wins over the provider's endpoint
//	RAG_RERANK_MODEL    — model name (ignored by single-model servers such as TEI)
//	RAG_RERANK_API_KEY  — optional; omit for a self-hosted server that doesn't require auth
//
// When RAG_RERANK_URL isn't set, falls back to LangSearch's hosted endpoint
// if LANGSEARCH_API_KEY is set - the previous zero-config default, kept for
// backward compatibility. Returns a zero Config (IsConfigured() == false)
// when neither is configured.
func FromEnv() Config {
	if id := strings.TrimSpace(os.Getenv("RAG_RERANK_PROVIDER")); id != "" {
		if cfg, ok := fromProvider(id); ok {
			return cfg
		}
	}
	url := strings.TrimSpace(os.Getenv("RAG_RERANK_URL"))
	if url != "" {
		return Config{
			BaseURL: url,
			APIKey:  strings.TrimSpace(os.Getenv("RAG_RERANK_API_KEY")),
			Model:   strings.TrimSpace(os.Getenv("RAG_RERANK_MODEL")),
		}
	}
	if lsKey := strings.TrimSpace(os.Getenv("LANGSEARCH_API_KEY")); lsKey != "" {
		return Config{BaseURL: langSearchBaseURL, APIKey: lsKey, Model: langSearchModel}
	}
	return Config{}
}

// fromProvider builds the configuration of a catalog provider. The explicit
// RAG_RERANK_* variables still override the endpoint, the model and the key.
// ok is false for an unknown provider, so the caller falls back to the other
// variables instead of silently running without a reranker.
func fromProvider(id string) (Config, bool) {
	profile, ok := ProfileFor(id)
	if !ok {
		return Config{}, false
	}
	cfg := Config{BaseURL: profile.URL, Model: profile.DefaultModel(), Style: profile.Style}
	if v := strings.TrimSpace(os.Getenv("RAG_RERANK_URL")); v != "" {
		cfg.BaseURL = v
	}
	if v := strings.TrimSpace(os.Getenv("RAG_RERANK_MODEL")); v != "" {
		cfg.Model = v
	}
	cfg.APIKey = strings.TrimSpace(os.Getenv("RAG_RERANK_API_KEY"))
	for _, name := range profile.EnvVars {
		if cfg.APIKey != "" {
			break
		}
		cfg.APIKey = strings.TrimSpace(os.Getenv(name))
	}
	if cfg.APIKey == "" {
		return Config{}, false // a hosted provider without a key cannot answer
	}
	return cfg, true
}

// HTTPReranker reranks documents via a Cohere-compatible (or TEI-compatible)
// HTTP rerank endpoint.
type HTTPReranker struct {
	cfg        Config
	httpClient *http.Client
}

// New creates an HTTPReranker from an explicit Config.
func New(cfg Config) *HTTPReranker {
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}
	return &HTTPReranker{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: cfg.Timeout},
	}
}

// NewFromEnv creates an HTTPReranker from environment variables (see FromEnv).
func NewFromEnv() *HTTPReranker {
	return New(FromEnv())
}

// IsConfigured reports whether a rerank endpoint is set.
func (r *HTTPReranker) IsConfigured() bool {
	return r != nil && strings.TrimSpace(r.cfg.BaseURL) != ""
}

// Rerank sends docs to the configured endpoint and returns their indices
// sorted by descending relevance, along with the score for each position.
// topN caps the returned list; pass 0 to return all.
func (r *HTTPReranker) Rerank(ctx context.Context, query string, docs []string, topN int) ([]int, []float32, error) {
	if !r.IsConfigured() {
		return nil, nil, fmt.Errorf("reranker: no rerank endpoint configured (set RAG_RERANK_PROVIDER, RAG_RERANK_URL or LANGSEARCH_API_KEY)")
	}
	if len(docs) == 0 {
		return nil, nil, nil
	}

	// Both "documents" (Cohere/LangSearch/vLLM) and "texts" (TEI) point at
	// the same array so one request works against either server shape -
	// each side ignores the field name it doesn't recognize.
	payload := r.payload(query, docs, topN)

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("reranker: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.cfg.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "SeshatAI-RAG/1.0")
	if r.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+r.cfg.APIKey)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("reranker: request failed: %w", err)
	}
	defer resp.Body.Close()

	var rawBody bytes.Buffer
	if _, err := rawBody.ReadFrom(resp.Body); err != nil {
		return nil, nil, fmt.Errorf("reranker: read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, nil, fmt.Errorf("reranker: endpoint returned status %d: %s", resp.StatusCode, rawBody.String())
	}

	items, err := parseRerankResponse(rawBody.Bytes())
	if err != nil {
		return nil, nil, err
	}

	indices := make([]int, 0, len(items))
	scores := make([]float32, 0, len(items))
	for _, item := range items {
		indices = append(indices, item.Index)
		scores = append(scores, item.Score)
	}
	return indices, scores, nil
}

// payload builds the request body: exactly the fields of a hosted provider that
// validates its schema, or the union of every known shape for an unknown server.
func (r *HTTPReranker) payload(query string, docs []string, topN int) map[string]any {
	payload := map[string]any{"query": query, "documents": docs}
	if r.cfg.Model != "" {
		payload["model"] = r.cfg.Model
	}
	switch r.cfg.Style {
	case StyleVoyage:
		if topN > 0 {
			payload["top_k"] = topN
		}
	case StyleCohere:
		if topN > 0 {
			payload["top_n"] = topN
		}
	default:
		payload["texts"] = docs
		if topN > 0 {
			payload["top_n"] = topN
		}
	}
	return payload
}

type rerankItem struct {
	Index int
	Score float32
}

// rawRerankResult covers both "relevance_score" (Cohere/LangSearch/vLLM)
// and "score" (TEI) field names for a single result item.
type rawRerankResult struct {
	Index          int      `json:"index"`
	RelevanceScore *float64 `json:"relevance_score"`
	Score          *float64 `json:"score"`
}

func (r rawRerankResult) score() float32 {
	if r.RelevanceScore != nil {
		return float32(*r.RelevanceScore)
	}
	if r.Score != nil {
		return float32(*r.Score)
	}
	return 0
}

// parseRerankResponse accepts either the Cohere/LangSearch/vLLM envelope
// ({"results": [...]}) or TEI's bare top-level array ([...]) - whichever
// the configured endpoint actually returned.
func parseRerankResponse(raw []byte) ([]rerankItem, error) {
	var wrapped struct {
		Results []rawRerankResult `json:"results"`
		Data    []rawRerankResult `json:"data"` // Voyage
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil {
		if len(wrapped.Results) > 0 {
			return toRerankItems(wrapped.Results), nil
		}
		if len(wrapped.Data) > 0 {
			return toRerankItems(wrapped.Data), nil
		}
	}

	var bare []rawRerankResult
	if err := json.Unmarshal(raw, &bare); err == nil && len(bare) > 0 {
		return toRerankItems(bare), nil
	}

	return nil, fmt.Errorf("reranker: decode response: unrecognized shape")
}

func toRerankItems(results []rawRerankResult) []rerankItem {
	items := make([]rerankItem, 0, len(results))
	for _, res := range results {
		items = append(items, rerankItem{Index: res.Index, Score: res.score()})
	}
	return items
}

// NormalizeScores min-max scales scores into [0,1] so a rerank score can be
// safely blended with a differently-scaled retrieval score (e.g. cosine
// similarity), regardless of which provider produced it - Cohere/LangSearch
// return an already-calibrated [0,1] relevance_score, but some
// cross-encoder endpoints (raw logits) don't. Mirrors RAGFlow's
// Base._normalize_rank: scores already inside [0,1] are left untouched
// rather than needlessly rescaled; only an out-of-range batch is rescaled.
//
// A spreadless batch (every score identical, so max == min) would divide by
// zero - returns 0.5 for every entry instead. That constant doesn't bias
// relative order: added the same amount to every item in a blend, the
// blended ranking for that batch just falls back to whatever the other
// blend term (the original retrieval score) already ordered.
func NormalizeScores(scores []float32) []float32 {
	if len(scores) == 0 {
		return scores
	}
	min, max := scores[0], scores[0]
	for _, s := range scores[1:] {
		if s < min {
			min = s
		}
		if s > max {
			max = s
		}
	}
	if min >= 0 && max <= 1 {
		return scores
	}
	out := make([]float32, len(scores))
	if max == min {
		for i := range out {
			out[i] = 0.5
		}
		return out
	}
	spread := max - min
	for i, s := range scores {
		out[i] = (s - min) / spread
	}
	return out
}
