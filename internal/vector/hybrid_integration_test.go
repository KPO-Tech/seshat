package vector

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

// hybridDim is the dimension of the vectors of the hybrid integration tests.
const hybridDim = 8

func axisVector(axis int) []float32 {
	v := make([]float32, hybridDim)
	v[axis%hybridDim] = 1
	return v
}

// checkHybridSearch is what a hybrid search must do in any store: find a chunk that only the words match (its vector is far
// from the query), and apply the metadata filter to the keyword side as well as to the vector side.
func checkHybridSearch(t *testing.T, store Store, namespace string) {
	t.Helper()
	ctx := context.Background()
	_ = store.DeleteNamespace(ctx, namespace)
	t.Cleanup(func() { _ = store.DeleteNamespace(context.Background(), namespace) })

	var records []Record
	for i := 0; i < 30; i++ {
		records = append(records, Record{
			Namespace: namespace, Key: fmt.Sprintf("filler-%02d", i), Text: "ordinary text about the weather",
			Vector: axisVector(i % 4), Metadata: map[string]string{"scope": "a"},
		})
	}
	records = append(records,
		Record{Namespace: namespace, Key: "needle", Text: "The clause says the Supplier owes a quarterly Zebracode.", Vector: axisVector(6), Metadata: map[string]string{"scope": "a"}},
		Record{Namespace: namespace, Key: "hidden", Text: "Another zebracode, in a scope that is not allowed.", Vector: axisVector(7), Metadata: map[string]string{"scope": "b"}},
	)
	if err := store.Upsert(ctx, records); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	vectorOnly, err := store.Search(ctx, Query{Namespace: namespace, Vector: axisVector(0), TopK: 3})
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	for _, r := range vectorOnly {
		if r.Record.Key == "needle" {
			t.Fatalf("the needle should not be among the nearest vectors: %+v", vectorOnly)
		}
	}

	query := Query{Namespace: namespace, Vector: axisVector(0), QueryText: "zebracode", TopK: 3, HybridWeight: 0.7}
	results, err := store.Search(ctx, query)
	if err != nil {
		t.Fatalf("hybrid search: %v", err)
	}
	found := map[string]bool{}
	for _, r := range results {
		found[r.Record.Key] = true
	}
	if !found["needle"] || !found["hidden"] {
		t.Fatalf("a hybrid search should find the chunks only the word matches (any case), got %+v", results)
	}
	for _, r := range results {
		if r.Record.Key == "needle" && !strings.Contains(r.Record.Text, "Supplier") {
			t.Fatalf("the text of a keyword hit is missing: %+v", r)
		}
	}

	query.Filter = map[string]any{"scope": "a"}
	results, err = store.Search(ctx, query)
	if err != nil {
		t.Fatalf("hybrid search with a filter: %v", err)
	}
	foundNeedle := false
	for _, r := range results {
		if r.Record.Key == "hidden" {
			t.Fatalf("the filter must apply to the keyword side too: %+v", results)
		}
		foundNeedle = foundNeedle || r.Record.Key == "needle"
	}
	if !foundNeedle {
		t.Fatalf("the needle is in the scope and should be found: %+v", results)
	}

	// The same search with no weight is a vector search again.
	query.Filter, query.HybridWeight = nil, 0
	results, err = store.Search(ctx, query)
	if err != nil {
		t.Fatalf("vector search through Search: %v", err)
	}
	for _, r := range results {
		if r.Record.Key == "needle" || r.Record.Key == "hidden" {
			t.Fatalf("with no hybrid weight only the vectors count: %+v", results)
		}
	}
}

func TestQdrantStoreHybridIntegration(t *testing.T) {
	address := os.Getenv("QDRANT_INTEGRATION_ADDR") // host:grpcPort
	if address == "" {
		t.Skip("set QDRANT_INTEGRATION_ADDR (host:port of the gRPC API) to run the Qdrant integration test")
	}
	host, portText, _ := strings.Cut(address, ":")
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("QDRANT_INTEGRATION_ADDR %q: %v", address, err)
	}
	store, err := NewQdrantStore(context.Background(), QdrantConfig{Host: host, Port: port, CollPrefix: "seshat_test_", DefaultDim: hybridDim})
	if err != nil {
		t.Fatalf("NewQdrantStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	checkHybridSearch(t, store, "hybrid")
}

func TestChromaStoreHybridIntegration(t *testing.T) {
	url := os.Getenv("CHROMA_INTEGRATION_URL")
	if url == "" {
		t.Skip("set CHROMA_INTEGRATION_URL to run the Chroma integration test")
	}
	checkHybridSearch(t, NewChromaStore(ChromaConfig{BaseURL: url}), "seshat-test-hybrid")
}
