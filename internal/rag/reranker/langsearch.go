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

const langSearchRerankURL = "https://api.langsearch.com/v1/rerank"
const langSearchRerankModel = "langsearch-reranker-v1"

// LangSearchReranker reranks a set of documents by semantic relevance using
// the LangSearch free rerank API (langsearch.com — no credit card required).
type LangSearchReranker struct {
	apiKey     string
	httpClient *http.Client
}

// NewLangSearchReranker reads LANGSEARCH_API_KEY from the environment.
func NewLangSearchReranker() *LangSearchReranker {
	return NewLangSearchRerankerWithKey(os.Getenv("LANGSEARCH_API_KEY"))
}

func NewLangSearchRerankerWithKey(apiKey string) *LangSearchReranker {
	return &LangSearchReranker{
		apiKey:     strings.TrimSpace(apiKey),
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// IsConfigured reports whether an API key is set.
func (r *LangSearchReranker) IsConfigured() bool {
	return r.apiKey != ""
}

// Rerank sends docs to LangSearch and returns their indices sorted by
// descending relevance score, along with the score for each position.
// topN caps the returned list; pass 0 to return all.
func (r *LangSearchReranker) Rerank(ctx context.Context, query string, docs []string, topN int) ([]int, []float32, error) {
	if !r.IsConfigured() {
		return nil, nil, fmt.Errorf("langsearch reranker: API key not configured (set LANGSEARCH_API_KEY)")
	}
	if len(docs) == 0 {
		return nil, nil, nil
	}

	payload := map[string]any{
		"model":     langSearchRerankModel,
		"query":     query,
		"documents": docs,
	}
	if topN > 0 {
		payload["top_n"] = topN
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("langsearch reranker: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, langSearchRerankURL, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "NexusAI-RAG/1.0")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("langsearch reranker: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, nil, fmt.Errorf("langsearch reranker: API returned status %d", resp.StatusCode)
	}

	var result struct {
		Code    int `json:"code"`
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil, fmt.Errorf("langsearch reranker: decode response: %w", err)
	}

	indices := make([]int, 0, len(result.Results))
	scores := make([]float32, 0, len(result.Results))
	for _, item := range result.Results {
		indices = append(indices, item.Index)
		scores = append(scores, float32(item.RelevanceScore))
	}
	return indices, scores, nil
}
