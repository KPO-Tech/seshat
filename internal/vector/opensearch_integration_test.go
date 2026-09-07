package vector

import (
	"context"
	"os"
	"testing"
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
