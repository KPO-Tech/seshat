package vector

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestOpenSearchStoreIntegration(t *testing.T) {
	address := os.Getenv("OPENSEARCH_INTEGRATION_URL")
	if address == "" {
		t.Skip("set OPENSEARCH_INTEGRATION_URL to run the OpenSearch integration test")
	}

	ctx := context.Background()
	store, err := NewOpenSearchStore(ctx, OpenSearchConfig{
		Addresses:   []string{address},
		IndexPrefix: "seshat-test",
		CreateIndex: true,
		KNN:         false,
		// The first request on a new index of a node that has just started can take longer than the default 10 seconds.
		RequestTimeout: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewOpenSearchStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	namespace := "integration-open-search"
	t.Cleanup(func() { _ = store.DeleteNamespace(context.Background(), namespace) })
	_ = store.DeleteNamespace(ctx, namespace)

	if err := store.Upsert(ctx, []Record{
		{
			Namespace: namespace,
			Key:       "sharepoint-policy",
			Text:      "The travel policy requires manager approval before booking international flights.",
			Metadata: map[string]string{
				"source":   "sharepoint",
				"scope_id": `["org:demo","team:finance"]`,
			},
		},
		{
			Namespace: namespace,
			Key:       "slack-note",
			Text:      "Lunch will be served at noon in the cafeteria.",
			Metadata: map[string]string{
				"source":   "slack",
				"scope_id": `["org:demo","team:people"]`,
			},
		},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	results, err := store.Search(ctx, Query{
		Namespace: namespace,
		QueryText: "international travel approval",
		TopK:      5,
		Filter: map[string]any{
			"scope_id": map[string]any{"$in": []string{"team:finance"}},
		},
		HybridWeight: 1,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one OpenSearch result")
	}
	if results[0].Record.Key != "sharepoint-policy" {
		t.Fatalf("expected sharepoint-policy first, got %q", results[0].Record.Key)
	}

	if err := store.DeleteNamespace(ctx, namespace); err != nil {
		t.Fatalf("DeleteNamespace: %v", err)
	}
	exists, err := store.HasNamespace(ctx, namespace)
	if err != nil {
		t.Fatalf("HasNamespace after delete: %v", err)
	}
	if exists {
		t.Fatal("expected namespace index to be deleted")
	}
}

// A hybrid search reads the best hits of the vector search and of the text search, and blends them (see hybrid.go): a chunk the
// vectors rank far down and the words match is found, and so is one the words do not match and the vectors rank first.
func TestOpenSearchStoreHybridIntegration(t *testing.T) {
	address := os.Getenv("OPENSEARCH_INTEGRATION_URL")
	if address == "" {
		t.Skip("set OPENSEARCH_INTEGRATION_URL to run the OpenSearch integration test")
	}

	ctx := context.Background()
	store, err := NewOpenSearchStore(ctx, OpenSearchConfig{
		Addresses:   []string{address},
		IndexPrefix: "seshat-test",
		CreateIndex: true,
		KNN:         true,
		DefaultDim:  2,
		// See the other test: a new index can be slow to answer its first request.
		RequestTimeout: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewOpenSearchStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	namespace := "integration-open-search-hybrid"
	t.Cleanup(func() { _ = store.DeleteNamespace(context.Background(), namespace) })
	_ = store.DeleteNamespace(ctx, namespace)

	records := []Record{{Namespace: namespace, Key: "answer", Text: "the quasar redshift is 2.3", Vector: []float32{0, 1}}}
	for i := 0; i < 30; i++ {
		records = append(records, Record{Namespace: namespace, Key: fmt.Sprintf("noise-%02d", i), Text: "unrelated words about weather", Vector: []float32{1, 0.01 * float32(i)}})
	}
	if err := store.Upsert(ctx, records); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	// The records are searchable once the index is refreshed.
	time.Sleep(2 * time.Second)

	query := Query{Namespace: namespace, Vector: []float32{1, 0}, TopK: 3, QueryText: "quasar redshift"}
	got, err := store.Search(ctx, query)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if containsKey(got, "answer") {
		t.Fatalf("a pure vector search should not find the chunk the words match: %v", got)
	}
	query.HybridWeight = 0.5
	got, err = store.Search(ctx, query)
	if err != nil {
		t.Fatalf("hybrid search: %v", err)
	}
	if !containsKey(got, "answer") {
		t.Errorf("a hybrid search finds the chunk its words match, whatever the vectors say: %v", got)
	}
	if len(got) != 3 {
		t.Errorf("a search gives at most TopK results, got %d", len(got))
	}
}
