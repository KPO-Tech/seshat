package vector

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestOpenSearchIndexNameIsStableAndSafe(t *testing.T) {
	got := openSearchIndexName("Seshat RAG", "Entreprise A / Knowledge")
	again := openSearchIndexName("Seshat RAG", "Entreprise A / Knowledge")
	if got != again {
		t.Fatalf("expected stable index name, got %q then %q", got, again)
	}
	if got != "seshat-rag-entreprise-a-knowledge-237b82f1" {
		t.Fatalf("unexpected index name: %q", got)
	}
}

func TestOpenSearchDocumentPreservesMultiValuedMetadata(t *testing.T) {
	doc := openSearchDocumentFromRecord(Record{
		Namespace: "kb",
		Key:       "doc-1",
		Text:      "hello",
		Metadata: map[string]string{
			"scope_id": `["org:1","team:legal"]`,
			"source":   "sharepoint",
		},
	})
	if got := doc.Metadata["scope_id"]; !reflect.DeepEqual(got, []string{"org:1", "team:legal"}) {
		t.Fatalf("expected scope_id as []string, got %#v", got)
	}
	record := doc.toRecord()
	if record.Metadata["scope_id"] != `["org:1","team:legal"]` {
		t.Fatalf("expected scope_id JSON array to round-trip, got %q", record.Metadata["scope_id"])
	}
	if record.Metadata["source"] != "sharepoint" {
		t.Fatalf("expected source metadata to round-trip, got %q", record.Metadata["source"])
	}
}

func TestOpenSearchBulkBody(t *testing.T) {
	body, err := openSearchBulkBody("seshat-rag-kb", []Record{
		{
			Namespace: "kb",
			Key:       "<doc&1>",
			Text:      "hello",
			Metadata:  map[string]string{"source": "sharepoint"},
		},
		{
			Namespace: "kb",
			Key:       "doc-2",
			Text:      "world",
			Metadata:  map[string]string{"scope_id": `["org:1","team:legal"]`},
		},
	})
	if err != nil {
		t.Fatalf("openSearchBulkBody: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 NDJSON lines, got %d:\n%s", len(lines), string(body))
	}
	if strings.Contains(string(body), `\u003c`) || strings.Contains(string(body), `\u0026`) {
		t.Fatalf("bulk body should not HTML-escape ids:\n%s", string(body))
	}
	var action map[string]map[string]string
	if err := json.Unmarshal([]byte(lines[0]), &action); err != nil {
		t.Fatalf("unmarshal bulk action: %v", err)
	}
	if action["index"]["_index"] != "seshat-rag-kb" || action["index"]["_id"] != "<doc&1>" {
		t.Fatalf("unexpected bulk action: %#v", action)
	}
	var doc openSearchDocument
	if err := json.Unmarshal([]byte(lines[3]), &doc); err != nil {
		t.Fatalf("unmarshal bulk document: %v", err)
	}
	if !reflect.DeepEqual(doc.Metadata["scope_id"], []any{"org:1", "team:legal"}) {
		t.Fatalf("expected scope_id array metadata, got %#v", doc.Metadata["scope_id"])
	}
}

func TestOpenSearchUpsertUsesBulkAPI(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`{"version":{"number":"2.19.0"},"tagline":"The OpenSearch Project: https://opensearch.org/"}`))
			return
		case "/_nodes/http":
			_, _ = w.Write([]byte(`{"nodes":{}}`))
			return
		case "/_bulk":
		default:
			t.Fatalf("expected bulk path, got %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("refresh"); got != "wait_for" {
			t.Fatalf("expected refresh=wait_for, got %q", got)
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		requests = append(requests, string(data))
		_, _ = w.Write([]byte(`{"errors":false,"items":[{"index":{"_index":"seshat-rag-kb","_id":"doc-1","status":201}}],"took":1}`))
	}))
	defer server.Close()

	store, err := NewOpenSearchStore(context.Background(), OpenSearchConfig{
		Addresses:   []string{server.URL},
		CreateIndex: false,
		KNN:         false,
		BulkSize:    10,
	})
	if err != nil {
		t.Fatalf("NewOpenSearchStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	err = store.Upsert(context.Background(), []Record{{
		Namespace: "kb",
		Key:       "doc-1",
		Text:      "hello",
	}})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if len(requests) != 1 {
		t.Fatalf("expected one bulk request, got %d", len(requests))
	}
	if !strings.Contains(requests[0], `"_index":"seshat-rag-kb-46e6e95a"`) {
		t.Fatalf("expected namespace index in bulk body, got:\n%s", requests[0])
	}
}

func TestOpenSearchFilterClauses(t *testing.T) {
	clauses := openSearchFilterClauses(map[string]any{
		"source":   "sharepoint",
		"scope_id": map[string]any{"$in": []string{"org:1", "team:legal"}},
	})
	data, err := json.Marshal(clauses)
	if err != nil {
		t.Fatalf("marshal clauses: %v", err)
	}
	got := string(data)
	want := `[{"terms":{"metadata.scope_id":["org:1","team:legal"]}},{"term":{"metadata.source":"sharepoint"}}]`
	if got != want {
		t.Fatalf("unexpected clauses:\nwant %s\n got %s", want, got)
	}
}

func TestBlendOpenSearchResults(t *testing.T) {
	results := blendOpenSearchResults(
		[]SearchResult{
			{Record: Record{Key: "semantic"}, Score: 0.8},
			{Record: Record{Key: "both"}, Score: 0.4},
		},
		[]SearchResult{
			{Record: Record{Key: "both"}, Score: 12},
			{Record: Record{Key: "keyword"}, Score: 6},
		},
		0.25,
		2,
	)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Record.Key != "semantic" {
		t.Fatalf("expected semantic result first, got %q", results[0].Record.Key)
	}
	if results[1].Record.Key != "both" {
		t.Fatalf("expected blended result second, got %q", results[1].Record.Key)
	}
	if results[1].Score <= 0.5 {
		t.Fatalf("expected blended score to include keyword contribution, got %f", results[1].Score)
	}
}
