package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	internalrag "github.com/KPO-Tech/seshat/internal/rag"
	engineconfig "github.com/KPO-Tech/seshat/pkg/config"
	"github.com/KPO-Tech/seshat/pkg/sdk"
)

func TestResolveModelKeepsExplicitProviderPrefix(t *testing.T) {
	model := resolveModel(engineconfig.Config{Model: "openai:gpt-5.5"})
	if model.Provider != sdk.APIProviderOpenAI {
		t.Fatalf("unexpected provider: got %q", model.Provider)
	}
	if model.Model != "gpt-5.5" {
		t.Fatalf("unexpected model: got %q", model.Model)
	}
}

func TestResolveModelInfersOllamaFromRawModelID(t *testing.T) {
	model := resolveModel(engineconfig.Config{Model: "qwen2.5-coder:7b"})
	if model.Provider != sdk.APIProviderOllama {
		t.Fatalf("unexpected provider: got %q", model.Provider)
	}
	if model.Model != "qwen2.5-coder:7b" {
		t.Fatalf("unexpected model: got %q", model.Model)
	}
}

// TestBuildRAGService_FallsBackToSQLiteWhenHNSWUnavailable forces the HNSW
// store to fail regardless of platform (by pointing it at a directory whose
// parent is a file, so os.MkdirAll can't create it) rather than relying on
// actually running on Windows - github.com/coder/hnsw's atomic-write
// dependency (google/renameio) doesn't support Windows, so this same
// fallback path is what makes RAG work at all on Windows; forcing the
// failure here exercises it deterministically in CI too.
func TestBuildRAGService_FallsBackToSQLiteWhenHNSWUnavailable(t *testing.T) {
	t.Setenv("RAG_EMBEDDING_URL", "http://127.0.0.1:1/v1")
	t.Setenv("RAG_EMBEDDING_MODEL", "test-model")

	dir := t.TempDir()
	blockerFile := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blockerFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	hnswDir := filepath.Join(blockerFile, "hnsw") // parent is a file: MkdirAll must fail
	sqlitePath := filepath.Join(dir, "rag.sqlite3")

	svc := buildRAGService(engineconfig.Config{}, hnswDir, sqlitePath)
	if svc == nil {
		t.Fatal("expected a RAG service backed by the sqlite fallback, got nil")
	}
	// The sqlite backend holds the file open; on Windows TempDir's cleanup
	// can't remove an open file, so close it explicitly before the test ends.
	if closer, ok := svc.Vectors().(io.Closer); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}

	if _, err := os.Stat(sqlitePath); err != nil {
		t.Errorf("expected the sqlite fallback file to be created at %s: %v", sqlitePath, err)
	}
}

func TestBuildRAGService_VectorlessWhenEmbeddingNotConfigured(t *testing.T) {
	t.Setenv("RAG_EMBEDDING_URL", "")
	t.Setenv("RAG_EMBEDDING_MODEL", "")

	dir := t.TempDir()
	svc := buildRAGService(engineconfig.Config{}, filepath.Join(dir, "hnsw"), filepath.Join(dir, "rag.sqlite3"))
	if svc == nil {
		t.Fatal("expected a vectorless (BM25-only) RAG service even without an embedding provider configured")
	}
	if closer, ok := svc.Vectors().(io.Closer); ok {
		t.Cleanup(func() { _ = closer.Close() })
	}

	ctx := context.Background()
	if _, err := svc.Ingest(ctx, internalrag.IngestRequest{
		CorpusID: "kb", Filename: "doc.txt", Text: "the quick brown fox jumps over the lazy dog",
	}); err != nil {
		t.Fatalf("Ingest without an embedder should succeed in vectorless mode: %v", err)
	}
	resp, err := svc.Search(ctx, internalrag.SearchRequest{CorpusID: "kb", Query: "fox", TopK: 5})
	if err != nil {
		t.Fatalf("Search without an embedder should succeed in vectorless mode: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Error("expected a BM25 match for 'fox' in the ingested text")
	}
}

func TestParsePermissionModeRejectsPlan(t *testing.T) {
	_, err := parsePermissionMode("plan")
	if err == nil {
		t.Fatal("expected plan permission mode to be rejected")
	}
	if !strings.Contains(err.Error(), "execution mode") {
		t.Fatalf("expected execution mode guidance, got %q", err)
	}
}

func TestParsePermissionModeCaseInsensitive(t *testing.T) {
	tests := []struct {
		input string
		want  sdk.PermissionMode
	}{
		{"onRequest", sdk.PermissionModeOnRequest},
		{"onrequest", sdk.PermissionModeOnRequest},
		{"ONREQUEST", sdk.PermissionModeOnRequest},
		{"auto", sdk.PermissionModeAuto},
		{"AUTO", sdk.PermissionModeAuto},
		{"acceptEdits", sdk.PermissionMode("acceptEdits")},
		{"acceptedits", sdk.PermissionMode("acceptEdits")},
		{"ACCEPTEDITS", sdk.PermissionMode("acceptEdits")},
		{"bypass", sdk.PermissionModeBypass},
		{"BYPASS", sdk.PermissionModeBypass},
		{"never", sdk.PermissionModeNever},
		{"NEVER", sdk.PermissionModeNever},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := parsePermissionMode(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}
