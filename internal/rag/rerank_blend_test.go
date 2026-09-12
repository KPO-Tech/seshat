package rag

import (
	"context"
	"testing"

	"github.com/KPO-Tech/seshat/internal/storage"
	"github.com/KPO-Tech/seshat/internal/vector"
)

// blendFixtureEmbedder maps three known documents ("doc-a"/"doc-b"/"doc-c")
// to 2D vectors chosen so their cosine similarity against the query vector
// (1,0) is exactly 0.9, 0.4 and 0.3 respectively (cosine similarity is scale
// invariant, so these don't need to be unit vectors) - giving
// Service.Search's pre-rerank results[i].Score known, controlled values to
// blend against, without depending on MemoryStore's cosine math being
// reverse-engineered from arbitrary embeddings.
type blendFixtureEmbedder struct{}

func (blendFixtureEmbedder) EmbedTexts(_ context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for _, text := range texts {
		switch text {
		case "doc-a":
			out = append(out, []float32{0.9, 0.43589})
		case "doc-b":
			out = append(out, []float32{0.4, 0.91652})
		case "doc-c":
			out = append(out, []float32{0.3, 0.95394})
		default: // the query itself
			out = append(out, []float32{1, 0})
		}
	}
	return out, nil
}

// scriptedReranker returns caller-supplied (index, score) pairs verbatim -
// unlike stubReranker (service_test.go), which only ever reverses input
// order, this lets a test specify an arbitrary rerank ranking/score set
// independent of the number or order of input docs.
type scriptedReranker struct {
	indices []int
	scores  []float32
}

func (r scriptedReranker) IsConfigured() bool { return true }

func (r scriptedReranker) Rerank(_ context.Context, _ string, _ []string, _ int) ([]int, []float32, error) {
	return r.indices, r.scores, nil
}

func newBlendFixtureService(t *testing.T) *Service {
	t.Helper()
	tmpDir := t.TempDir()
	storage.SetConfig(storage.Config{Provider: storage.ProviderLocal, LocalPath: tmpDir})
	t.Cleanup(storage.ResetProvider)
	artifacts, err := storage.DefaultArtifactStore()
	if err != nil {
		t.Fatalf("DefaultArtifactStore: %v", err)
	}
	svc := NewService(artifacts, vector.NewMemoryStore(), blendFixtureEmbedder{}, nil)
	ctx := context.Background()
	// Ingested as their own text so blendFixtureEmbedder recognizes them by
	// exact content; production text/chunk pipelines aren't under test here.
	for _, doc := range []string{"doc-a", "doc-b", "doc-c"} {
		if _, err := svc.Ingest(ctx, IngestRequest{CorpusID: "kb", Filename: doc + ".txt", Text: doc}); err != nil {
			t.Fatalf("ingest %s: %v", doc, err)
		}
	}
	return svc
}

// keyOrder returns each result's originating document, inferring it from the
// chunk key/text rather than assuming a specific key format.
func keyOrder(t *testing.T, results []SearchResult) []string {
	t.Helper()
	order := make([]string, len(results))
	for i, r := range results {
		order[i] = r.Text
	}
	return order
}

func TestServiceSearch_RerankWeightOneMatchesPureRerankOrder(t *testing.T) {
	svc := newBlendFixtureService(t)
	// Rerank says doc-c is most relevant, then doc-b, then doc-a - the
	// reverse of the vector order (a:0.9 > b:0.4 > c:0.3). Scores already
	// in [0,1], so NormalizeScores passes them through unchanged.
	svc.SetReranker(scriptedReranker{indices: []int{2, 1, 0}, scores: []float32{0.9, 0.6, 0.2}})

	resp, err := svc.Search(context.Background(), SearchRequest{
		CorpusID: "kb", Query: "search-query", TopK: 3, RerankWeight: 1,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	got := keyOrder(t, resp.Results)
	want := []string{"doc-c", "doc-b", "doc-a"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("RerankWeight=1 should reproduce the reranker's own order, got %v want %v", got, want)
	}
}

func TestServiceSearch_DefaultWeightAppliesRealBlendNotVectorOrder(t *testing.T) {
	svc := newBlendFixtureService(t)
	svc.SetReranker(scriptedReranker{indices: []int{2, 1, 0}, scores: []float32{0.9, 0.6, 0.2}})

	// RerankWeight left unset (0) - must fall back to the service default
	// (0.7), not silently skip blending and return pure vector order
	// (a > b > c).
	resp, err := svc.Search(context.Background(), SearchRequest{CorpusID: "kb", Query: "search-query", TopK: 3})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	got := keyOrder(t, resp.Results)
	if got[0] == "doc-a" {
		t.Fatalf("expected the default rerank weight to move doc-c/doc-b ahead of the top vector match, got order %v", got)
	}
	// At weight 0.7 the reranker's own order dominates for this fixture:
	// blended(a)=0.3*0.9+0.7*0.2=0.41, blended(b)=0.3*0.4+0.7*0.6=0.54, blended(c)=0.3*0.3+0.7*0.9=0.72
	want := []string{"doc-c", "doc-b", "doc-a"}
	if got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestServiceSearch_BlendProducesOrderDistinctFromBothPureExtremes(t *testing.T) {
	svc := newBlendFixtureService(t)
	svc.SetReranker(scriptedReranker{indices: []int{2, 1, 0}, scores: []float32{0.9, 0.6, 0.2}})

	// At weight 0.5: blended(a)=0.5*0.9+0.5*0.2=0.55, blended(b)=0.5*0.4+0.5*0.6=0.50,
	// blended(c)=0.5*0.3+0.5*0.9=0.60 -> order c, a, b: distinct from both
	// the pure-vector order (a, b, c) and the pure-rerank order (c, b, a).
	resp, err := svc.Search(context.Background(), SearchRequest{
		CorpusID: "kb", Query: "search-query", TopK: 3, RerankWeight: 0.5,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	got := keyOrder(t, resp.Results)
	want := []string{"doc-c", "doc-a", "doc-b"}
	pureVectorOrder := []string{"doc-a", "doc-b", "doc-c"}
	pureRerankOrder := []string{"doc-c", "doc-b", "doc-a"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("expected the 0.5-weighted blend to produce %v, got %v", want, got)
	}
	if got[0] == pureVectorOrder[0] && got[1] == pureVectorOrder[1] && got[2] == pureVectorOrder[2] {
		t.Fatal("blended order should not equal the pure vector order")
	}
	if got[0] == pureRerankOrder[0] && got[1] == pureRerankOrder[1] && got[2] == pureRerankOrder[2] {
		t.Fatal("blended order should not equal the pure rerank order")
	}
}
