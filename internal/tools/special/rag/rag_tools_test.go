package ragtool_test

import (
	"context"
	"strings"
	"testing"

	"github.com/EngineerProjects/nexus-engine/internal/rag"
	"github.com/EngineerProjects/nexus-engine/internal/storage"
	tool "github.com/EngineerProjects/nexus-engine/internal/tools/registry"
	ragtool "github.com/EngineerProjects/nexus-engine/internal/tools/special/rag"
	"github.com/EngineerProjects/nexus-engine/internal/vector"
)

// stubEmbedder returns the same single-dimension vector for every text.
type stubEmbedder struct{}

func (stubEmbedder) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = []float32{float32(i+1) * 0.1, 0.5}
	}
	return out, nil
}

func newTestService(t *testing.T) *rag.Service {
	t.Helper()
	tmpDir := t.TempDir()
	storage.SetConfig(storage.Config{
		Provider:  storage.ProviderLocal,
		LocalPath: tmpDir,
	})
	t.Cleanup(storage.ResetProvider)
	artifacts, err := storage.DefaultArtifactStore()
	if err != nil {
		t.Fatalf("DefaultArtifactStore: %v", err)
	}
	return rag.NewService(artifacts, vector.NewMemoryStore(), stubEmbedder{}, nil)
}

func callTool(t *testing.T, tl tool.Tool, input map[string]any) tool.CallResult {
	t.Helper()
	result, err := tl.Call(context.Background(), tool.CallInput{Parsed: input}, nil)
	if err != nil {
		t.Fatalf("Call returned unexpected error: %v", err)
	}
	return result
}

// --- SearchTool ---

func TestSearchTool_Definition(t *testing.T) {
	tl := ragtool.NewSearchTool(nil)
	def := tl.Definition()
	if def.Name != ragtool.ToolSearchName {
		t.Errorf("Name = %q", def.Name)
	}
	if !def.IsReadOnly {
		t.Error("expected IsReadOnly=true")
	}
}

func TestSearchTool_DisabledWhenNilService(t *testing.T) {
	tl := ragtool.NewSearchTool(nil)
	if tl.IsEnabled() {
		t.Error("expected IsEnabled=false when service is nil")
	}
}

func TestSearchTool_MissingCorpusID(t *testing.T) {
	tl := ragtool.NewSearchTool(newTestService(t))
	res := callTool(t, tl, map[string]any{"query": "hello"})
	if !res.IsError() {
		t.Error("expected error result when corpus_id missing")
	}
}

func TestSearchTool_MissingQuery(t *testing.T) {
	tl := ragtool.NewSearchTool(newTestService(t))
	res := callTool(t, tl, map[string]any{"corpus_id": "c1"})
	if !res.IsError() {
		t.Error("expected error result when query missing")
	}
}

func TestSearchTool_EmptyCorpus(t *testing.T) {
	svc := newTestService(t)
	tl := ragtool.NewSearchTool(svc)
	res := callTool(t, tl, map[string]any{"corpus_id": "empty", "query": "anything"})
	if res.IsError() {
		t.Errorf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "No results") && !strings.Contains(res.Content, "0 result") {
		t.Errorf("unexpected content: %s", res.Content)
	}
}

func TestSearchTool_AfterIngest(t *testing.T) {
	svc := newTestService(t)

	// Ingest via IngestTool
	ingestTl := ragtool.NewIngestTool(svc)
	ingestRes := callTool(t, ingestTl, map[string]any{
		"corpus_id": "docs",
		"filename":  "intro.txt",
		"text":      "The quick brown fox\n\njumps over the lazy dog",
	})
	if ingestRes.IsError() {
		t.Fatalf("ingest failed: %s", ingestRes.Content)
	}

	// Search
	searchTl := ragtool.NewSearchTool(svc)
	res := callTool(t, searchTl, map[string]any{
		"corpus_id": "docs",
		"query":     "fox",
		"top_k":     float64(2),
	})
	if res.IsError() {
		t.Fatalf("search failed: %s", res.Content)
	}
	if !strings.Contains(res.Content, "result") {
		t.Errorf("unexpected content: %s", res.Content)
	}
}

// --- IngestTool ---

func TestIngestTool_Definition(t *testing.T) {
	tl := ragtool.NewIngestTool(nil)
	def := tl.Definition()
	if def.Name != ragtool.ToolIngestName {
		t.Errorf("Name = %q", def.Name)
	}
	if def.IsReadOnly {
		t.Error("expected IsReadOnly=false for ingest tool")
	}
}

func TestIngestTool_DisabledWhenNilService(t *testing.T) {
	tl := ragtool.NewIngestTool(nil)
	if tl.IsEnabled() {
		t.Error("expected IsEnabled=false when service is nil")
	}
}

func TestIngestTool_MissingFields(t *testing.T) {
	svc := newTestService(t)
	tl := ragtool.NewIngestTool(svc)

	for _, input := range []map[string]any{
		{"filename": "f", "text": "t"},
		{"corpus_id": "c", "text": "t"},
		{"corpus_id": "c", "filename": "f"},
	} {
		res := callTool(t, tl, input)
		if !res.IsError() {
			t.Errorf("expected error for input %v, got: %s", input, res.Content)
		}
	}
}

func TestIngestTool_Success(t *testing.T) {
	svc := newTestService(t)
	tl := ragtool.NewIngestTool(svc)
	res := callTool(t, tl, map[string]any{
		"corpus_id": "kb",
		"filename":  "doc.txt",
		"text":      "paragraph one\n\nparagraph two\n\nparagraph three",
	})
	if res.IsError() {
		t.Fatalf("unexpected error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "3 chunk") {
		t.Errorf("expected 3 chunks in output, got: %s", res.Content)
	}
}
