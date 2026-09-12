package reranker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPReranker_IsConfigured(t *testing.T) {
	if (&HTTPReranker{}).IsConfigured() {
		t.Error("expected zero-value reranker to be unconfigured")
	}
	if !New(Config{BaseURL: "http://localhost:8080/rerank"}).IsConfigured() {
		t.Error("expected a reranker with a BaseURL to be configured")
	}
	var nilReranker *HTTPReranker
	if nilReranker.IsConfigured() {
		t.Error("expected a nil *HTTPReranker to report unconfigured, not panic")
	}
}

func TestHTTPReranker_Rerank_RequiresConfiguration(t *testing.T) {
	r := New(Config{})
	if _, _, err := r.Rerank(context.Background(), "q", []string{"a"}, 0); err == nil {
		t.Error("expected an error when no BaseURL is configured")
	}
}

func TestHTTPReranker_Rerank_SendsBothDocumentsAndTextsFields(t *testing.T) {
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_ = json.NewDecoder(req.Body).Decode(&gotPayload)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"index":0,"relevance_score":0.9}]}`))
	}))
	defer server.Close()

	r := New(Config{BaseURL: server.URL, Model: "test-model"})
	_, _, err := r.Rerank(context.Background(), "query text", []string{"doc a", "doc b"}, 0)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}

	docs, ok := gotPayload["documents"].([]any)
	if !ok || len(docs) != 2 {
		t.Errorf("expected a 'documents' field with 2 entries, got %v", gotPayload["documents"])
	}
	texts, ok := gotPayload["texts"].([]any)
	if !ok || len(texts) != 2 {
		t.Errorf("expected a 'texts' field with 2 entries (TEI compatibility), got %v", gotPayload["texts"])
	}
	if gotPayload["model"] != "test-model" {
		t.Errorf("expected model field to be sent, got %v", gotPayload["model"])
	}
}

func TestHTTPReranker_Rerank_OmitsModelWhenEmpty(t *testing.T) {
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_ = json.NewDecoder(req.Body).Decode(&gotPayload)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"index":0,"score":0.5}]`))
	}))
	defer server.Close()

	r := New(Config{BaseURL: server.URL}) // no Model - matches a single-model TEI server
	if _, _, err := r.Rerank(context.Background(), "q", []string{"a"}, 0); err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if _, present := gotPayload["model"]; present {
		t.Errorf("expected no 'model' field when Config.Model is empty, got %v", gotPayload["model"])
	}
}

func TestHTTPReranker_Rerank_ParsesCohereEnvelopeShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"results":[{"index":1,"relevance_score":0.95},{"index":0,"relevance_score":0.2}]}`))
	}))
	defer server.Close()

	r := New(Config{BaseURL: server.URL})
	indices, scores, err := r.Rerank(context.Background(), "q", []string{"a", "b"}, 0)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if len(indices) != 2 || indices[0] != 1 || indices[1] != 0 {
		t.Fatalf("expected indices [1 0], got %v", indices)
	}
	if scores[0] != 0.95 {
		t.Errorf("expected first score 0.95, got %v", scores[0])
	}
}

func TestHTTPReranker_Rerank_ParsesTEIBareArrayShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"index":1,"score":0.88},{"index":0,"score":0.1}]`))
	}))
	defer server.Close()

	r := New(Config{BaseURL: server.URL})
	indices, scores, err := r.Rerank(context.Background(), "q", []string{"a", "b"}, 0)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if len(indices) != 2 || indices[0] != 1 {
		t.Fatalf("expected first index 1, got %v", indices)
	}
	if scores[0] != 0.88 {
		t.Errorf("expected first score 0.88, got %v", scores[0])
	}
}

func TestHTTPReranker_Rerank_SendsAuthHeaderOnlyWhenAPIKeySet(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		gotAuth = req.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"index":0,"score":1}]`))
	}))
	defer server.Close()

	// No API key - self-hosted server without auth.
	r := New(Config{BaseURL: server.URL})
	if _, _, err := r.Rerank(context.Background(), "q", []string{"a"}, 0); err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("expected no Authorization header without an API key, got %q", gotAuth)
	}

	// With an API key.
	r2 := New(Config{BaseURL: server.URL, APIKey: "secret"})
	if _, _, err := r2.Rerank(context.Background(), "q", []string{"a"}, 0); err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("expected 'Bearer secret' Authorization header, got %q", gotAuth)
	}
}

func TestHTTPReranker_Rerank_ErrorStatusCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid key"}`))
	}))
	defer server.Close()

	r := New(Config{BaseURL: server.URL, APIKey: "bad"})
	if _, _, err := r.Rerank(context.Background(), "q", []string{"a"}, 0); err == nil {
		t.Error("expected an error for a 401 response")
	}
}

func TestHTTPReranker_Rerank_EmptyDocsReturnsNil(t *testing.T) {
	r := New(Config{BaseURL: "http://example.invalid/rerank"})
	indices, scores, err := r.Rerank(context.Background(), "q", nil, 0)
	if err != nil || indices != nil || scores != nil {
		t.Errorf("expected (nil, nil, nil) for empty docs, got (%v, %v, %v)", indices, scores, err)
	}
}

func TestFromEnv_PrefersExplicitRerankURL(t *testing.T) {
	t.Setenv("RAG_RERANK_URL", "http://localhost:8080/rerank")
	t.Setenv("RAG_RERANK_MODEL", "bge-reranker-v2-m3")
	t.Setenv("RAG_RERANK_API_KEY", "")
	t.Setenv("LANGSEARCH_API_KEY", "should-be-ignored")

	cfg := FromEnv()
	if cfg.BaseURL != "http://localhost:8080/rerank" {
		t.Errorf("expected self-hosted BaseURL to take priority, got %q", cfg.BaseURL)
	}
	if cfg.Model != "bge-reranker-v2-m3" {
		t.Errorf("expected RAG_RERANK_MODEL to be used, got %q", cfg.Model)
	}
	if cfg.APIKey != "" {
		t.Errorf("expected no API key, got %q", cfg.APIKey)
	}
}

func TestFromEnv_FallsBackToLangSearch(t *testing.T) {
	t.Setenv("RAG_RERANK_URL", "")
	t.Setenv("LANGSEARCH_API_KEY", "ls-key")

	cfg := FromEnv()
	if cfg.BaseURL != langSearchBaseURL {
		t.Errorf("expected LangSearch fallback BaseURL, got %q", cfg.BaseURL)
	}
	if cfg.APIKey != "ls-key" {
		t.Errorf("expected LangSearch API key, got %q", cfg.APIKey)
	}
	if cfg.Model != langSearchModel {
		t.Errorf("expected LangSearch model, got %q", cfg.Model)
	}
}

func TestFromEnv_UnconfiguredWhenNeitherSet(t *testing.T) {
	t.Setenv("RAG_RERANK_URL", "")
	t.Setenv("LANGSEARCH_API_KEY", "")

	cfg := FromEnv()
	if New(cfg).IsConfigured() {
		t.Errorf("expected an unconfigured reranker, got BaseURL=%q", cfg.BaseURL)
	}
}

func TestNewLangSearchRerankerWithKey_EmptyKeyIsUnconfigured(t *testing.T) {
	r := NewLangSearchRerankerWithKey("")
	if r.IsConfigured() {
		t.Error("expected an empty API key to leave the LangSearch reranker unconfigured, not pointed at LangSearch with no credentials")
	}
}

func TestNewLangSearchRerankerWithKey_NonEmptyKeyIsConfigured(t *testing.T) {
	r := NewLangSearchRerankerWithKey("a-real-key")
	if !r.IsConfigured() {
		t.Error("expected a non-empty API key to configure the LangSearch reranker")
	}
}

func TestNormalizeScores_AlreadyInRangeIsUntouched(t *testing.T) {
	in := []float32{0.95, 0.2, 0.0, 1.0}
	out := NormalizeScores(in)
	for i := range in {
		if out[i] != in[i] {
			t.Fatalf("expected already-in-[0,1] scores to pass through unchanged, got %v want %v", out, in)
		}
	}
}

func TestNormalizeScores_OutOfRangeIsMinMaxScaled(t *testing.T) {
	// Unbounded logits, e.g. a raw cross-encoder that doesn't calibrate to [0,1].
	out := NormalizeScores([]float32{5.2, 1.1, -0.3})
	want := []float32{1.0, (1.1 - -0.3) / (5.2 - -0.3), 0.0}
	for i := range want {
		if diff := out[i] - want[i]; diff > 1e-4 || diff < -1e-4 {
			t.Fatalf("index %d: got %v want %v (full: %v)", i, out[i], want[i], out)
		}
	}
}

func TestNormalizeScores_SpreadlessBatchAvoidsDivideByZero(t *testing.T) {
	out := NormalizeScores([]float32{3.0, 3.0, 3.0})
	for i, v := range out {
		if v != 0.5 {
			t.Fatalf("index %d: expected the spreadless-batch fallback 0.5, got %v (full: %v)", i, v, out)
		}
	}
}

func TestNormalizeScores_EmptyInput(t *testing.T) {
	if out := NormalizeScores(nil); len(out) != 0 {
		t.Fatalf("expected empty input to produce empty output, got %v", out)
	}
}
