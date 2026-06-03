package embedder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Provider selects which embedding API format to use.
type Provider string

const (
	ProviderOpenAI Provider = "openai" // OpenAI-compatible: POST /embeddings
	ProviderOllama Provider = "ollama" // Ollama: POST /api/embed
)

// Config holds the configuration for a provider-backed embedder.
type Config struct {
	BaseURL  string
	APIKey   string
	Model    string
	Provider Provider
	Timeout  time.Duration
}

// FromEnv reads embedding configuration from environment variables:
//
//	RAG_EMBEDDING_URL      — base URL (e.g. https://api.openai.com/v1 or http://localhost:11434)
//	RAG_EMBEDDING_MODEL    — model name (e.g. text-embedding-3-small or nomic-embed-text)
//	RAG_EMBEDDING_API_KEY  — API key (optional for Ollama)
//	RAG_EMBEDDING_PROVIDER — "openai" or "ollama" (auto-detected if empty)
func FromEnv() *Config {
	c := &Config{
		BaseURL:  strings.TrimRight(os.Getenv("RAG_EMBEDDING_URL"), "/"),
		APIKey:   os.Getenv("RAG_EMBEDDING_API_KEY"),
		Model:    os.Getenv("RAG_EMBEDDING_MODEL"),
		Provider: Provider(strings.ToLower(os.Getenv("RAG_EMBEDDING_PROVIDER"))),
		Timeout:  30 * time.Second,
	}
	if c.Provider == "" {
		c.Provider = detectProvider(c.BaseURL)
	}
	return c
}

// IsConfigured reports whether all required fields are present.
func (c *Config) IsConfigured() bool {
	return c != nil && c.BaseURL != "" && c.Model != ""
}

// Embedder calls a remote embedding API to produce dense float vectors.
type Embedder struct {
	cfg    *Config
	client *http.Client
}

// New creates an Embedder from an explicit Config.
func New(cfg *Config) *Embedder {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &Embedder{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}
}

// NewFromEnv creates an Embedder from environment variables. Returns nil if
// the required variables are not set (so callers can guard with IsConfigured).
func NewFromEnv() *Embedder {
	cfg := FromEnv()
	if !cfg.IsConfigured() {
		return nil
	}
	return New(cfg)
}

// EmbedTexts implements rag.Embedder.
func (e *Embedder) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	switch e.cfg.Provider {
	case ProviderOllama:
		return e.embedOllama(ctx, texts)
	default:
		return e.embedOpenAI(ctx, texts)
	}
}

// embedOpenAI calls POST {baseURL}/embeddings.
// Request:  {"input": texts, "model": model}
// Response: {"data": [{"embedding": [...]}, ...]}
func (e *Embedder) embedOpenAI(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(map[string]any{
		"input": texts,
		"model": e.cfg.Model,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.BaseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.cfg.APIKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding API returned %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode embedding response: %w", err)
	}
	if len(result.Data) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(result.Data))
	}

	out := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		out[i] = d.Embedding
	}
	return out, nil
}

// embedOllama calls POST {baseURL}/api/embed.
// Request:  {"input": texts, "model": model, "truncate": true}
// Response: {"embeddings": [[...], ...]}
func (e *Embedder) embedOllama(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(map[string]any{
		"input":    texts,
		"model":    e.cfg.Model,
		"truncate": true,
	})
	if err != nil {
		return nil, err
	}

	url := e.cfg.BaseURL
	if !strings.HasSuffix(url, "/api/embed") {
		// strip any /v1 suffix that might have been added and use Ollama's path
		url = strings.TrimSuffix(url, "/v1") + "/api/embed"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if e.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+e.cfg.APIKey)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		b, _ := io.ReadAll(resp.Body)
		_ = json.Unmarshal(b, &errResp)
		msg := errResp.Error
		if msg == "" {
			msg = string(b)
		}
		return nil, fmt.Errorf("ollama embed error (%d): %s", resp.StatusCode, msg)
	}

	var result struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode ollama embedding response: %w", err)
	}
	if len(result.Embeddings) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings from ollama, got %d", len(texts), len(result.Embeddings))
	}
	return result.Embeddings, nil
}

// detectProvider guesses the provider from the base URL.
// Ollama default port is 11434 and its paths start with /api.
func detectProvider(baseURL string) Provider {
	lower := strings.ToLower(baseURL)
	if strings.Contains(lower, ":11434") || strings.HasSuffix(lower, "/api") {
		return ProviderOllama
	}
	return ProviderOpenAI
}

// DetectProviderPublic is the exported version of detectProvider for use outside this package.
func DetectProviderPublic(baseURL string) Provider {
	return detectProvider(baseURL)
}
