package vector

import (
	"context"
	"encoding/json"
	"fmt"
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

// TestOpenSearchDeleteKeysToleratesNotFound is a regression guard: deleting a
// key that was never written (or already removed by a prior cleanup pass)
// must no-op, matching every other VectorStore backend (Qdrant/Chroma
// explicitly swallow their own "not found" shape; pgvector/sqlite/hnsw/memory
// are naturally idempotent) and the contract DeleteFileChunks documents in
// service.go. A real OpenSearch 404 delete response was previously surfaced
// as a hard error by the opensearch-go client, aborting the whole batch.
// DeleteKeys goes through the _bulk API (not one DELETE per key - see
// TestOpenSearchDeleteKeysUsesBulkAPI), so this exercises the per-item 404
// tolerance in a bulk response instead of a standalone DELETE's status code.
func TestOpenSearchDeleteKeysToleratesNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/":
			_, _ = w.Write([]byte(`{"version":{"number":"2.19.0"},"tagline":"The OpenSearch Project: https://opensearch.org/"}`))
		case r.URL.Path == "/_nodes/http":
			_, _ = w.Write([]byte(`{"nodes":{}}`))
		case r.URL.Path == "/_bulk":
			_, _ = w.Write([]byte(`{"errors":true,"took":1,"items":[` +
				`{"delete":{"_index":"seshat-rag-kb","_id":"missing-chunk","status":404,"result":"not_found"}},` +
				`{"delete":{"_index":"seshat-rag-kb","_id":"real-chunk","status":200,"result":"deleted"}}` +
				`]}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	store, err := NewOpenSearchStore(context.Background(), OpenSearchConfig{
		Addresses:   []string{server.URL},
		CreateIndex: false,
		KNN:         false,
	})
	if err != nil {
		t.Fatalf("NewOpenSearchStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.DeleteKeys(context.Background(), "kb", []string{"missing-chunk", "real-chunk"}); err != nil {
		t.Fatalf("expected DeleteKeys to no-op on a 404/not_found key, got: %v", err)
	}
}

// TestOpenSearchDeleteKeysStillFailsOnRealErrors is the flip side of the
// tolerate-404 fix above: a genuine per-item failure (not a 404) must still
// abort and surface an error, not be swallowed alongside the benign
// not-found case.
func TestOpenSearchDeleteKeysStillFailsOnRealErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/":
			_, _ = w.Write([]byte(`{"version":{"number":"2.19.0"},"tagline":"The OpenSearch Project: https://opensearch.org/"}`))
		case r.URL.Path == "/_nodes/http":
			_, _ = w.Write([]byte(`{"nodes":{}}`))
		case r.URL.Path == "/_bulk":
			_, _ = w.Write([]byte(`{"errors":true,"took":1,"items":[` +
				`{"delete":{"_index":"seshat-rag-kb","_id":"some-chunk","status":500,"error":{"type":"internal_server_error","reason":"boom"}}}` +
				`]}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	store, err := NewOpenSearchStore(context.Background(), OpenSearchConfig{
		Addresses:   []string{server.URL},
		CreateIndex: false,
		KNN:         false,
	})
	if err != nil {
		t.Fatalf("NewOpenSearchStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.DeleteKeys(context.Background(), "kb", []string{"some-chunk"}); err == nil {
		t.Fatal("expected a genuine server error to still be returned, got nil")
	}
}

// TestOpenSearchDeleteKeysUsesBulkAPI is the P1 regression test: deleting
// many keys used to make one DELETE request per key, painfully slow for the
// thousands of chunks a stale-chunk cleanup or whole-file delete can
// involve. It now goes through the same _bulk endpoint Upsert already uses.
func TestOpenSearchDeleteKeysUsesBulkAPI(t *testing.T) {
	var bulkRequests int
	var lastBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`{"version":{"number":"2.19.0"},"tagline":"The OpenSearch Project: https://opensearch.org/"}`))
		case "/_nodes/http":
			_, _ = w.Write([]byte(`{"nodes":{}}`))
		case "/_bulk":
			bulkRequests++
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			lastBody = string(data)
			_, _ = w.Write([]byte(`{"errors":false,"took":1,"items":[` +
				`{"delete":{"_index":"seshat-rag-kb","_id":"c1","status":200,"result":"deleted"}},` +
				`{"delete":{"_index":"seshat-rag-kb","_id":"c2","status":200,"result":"deleted"}}` +
				`]}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
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

	if err := store.DeleteKeys(context.Background(), "kb", []string{"c1", "c2"}); err != nil {
		t.Fatalf("DeleteKeys: %v", err)
	}
	if bulkRequests != 1 {
		t.Fatalf("expected exactly one bulk request for 2 keys under BulkSize=10, got %d", bulkRequests)
	}
	if !strings.Contains(lastBody, `"delete"`) || !strings.Contains(lastBody, `"_id":"c1"`) || !strings.Contains(lastBody, `"_id":"c2"`) {
		t.Fatalf("expected both keys as delete actions in the bulk body, got:\n%s", lastBody)
	}
}

// TestOpenSearchEnsureIndexToleratesConcurrentCreateRace is the P1
// regression test: two ingests racing to create the same not-yet-existing
// namespace's index both see HasNamespace() == false, both call
// Indices.Create, and the loser gets "resource_already_exists_exception"
// back from OpenSearch - previously surfaced as a hard Upsert failure
// instead of the idempotent success it actually is (the index exists,
// which is all ensureIndex's caller asked for).
func TestOpenSearchEnsureIndexToleratesConcurrentCreateRace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/":
			_, _ = w.Write([]byte(`{"version":{"number":"2.19.0"},"tagline":"The OpenSearch Project: https://opensearch.org/"}`))
		case r.URL.Path == "/_nodes/http":
			_, _ = w.Write([]byte(`{"nodes":{}}`))
		case r.Method == http.MethodHead:
			// Indices.Exists -> index not found yet.
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodPut:
			// Indices.Create -> lost the race, index was just created by
			// another process/goroutine.
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"type":"resource_already_exists_exception","reason":"index [seshat-rag-kb/abc] already exists"},"status":400}`))
		case r.URL.Path == "/_bulk":
			_, _ = w.Write([]byte(`{"errors":false,"took":1,"items":[{"index":{"_index":"seshat-rag-kb","_id":"doc-1","status":201}}]}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	store, err := NewOpenSearchStore(context.Background(), OpenSearchConfig{
		Addresses:   []string{server.URL},
		CreateIndex: true,
		KNN:         false,
		BulkSize:    10,
	})
	if err != nil {
		t.Fatalf("NewOpenSearchStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	err = store.Upsert(context.Background(), []Record{{Namespace: "kb", Key: "doc-1", Text: "hello"}})
	if err != nil {
		t.Fatalf("expected Upsert to tolerate a concurrent index-create race, got: %v", err)
	}
}

// TestOpenSearchGetAllRecordsPaginatesBeyondPageSize is the P1 regression
// test: Get(ctx, namespace, nil) ("all records") used to issue a single
// query capped at size:10000 - a namespace with more chunks than that
// silently lost the rest, with no error to signal the truncation, breaking
// the Store interface's own documented "all records in the namespace are
// returned" contract. It now pages with search_after until a page comes
// back short of a full page.
func TestOpenSearchGetAllRecordsPaginatesBeyondPageSize(t *testing.T) {
	makeHit := func(id string) string {
		doc := openSearchDocument{Namespace: "kb", Key: id, Text: "t"}
		data, err := json.Marshal(doc)
		if err != nil {
			t.Fatalf("marshal doc: %v", err)
		}
		return fmt.Sprintf(`{"_id":%q,"_score":1,"_source":%s}`, id, data)
	}

	var pageRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/":
			_, _ = w.Write([]byte(`{"version":{"number":"2.19.0"},"tagline":"The OpenSearch Project: https://opensearch.org/"}`))
		case r.URL.Path == "/_nodes/http":
			_, _ = w.Write([]byte(`{"nodes":{}}`))
		case strings.HasSuffix(r.URL.Path, "/_search"):
			pageRequests++
			var hits []string
			if pageRequests == 1 {
				// A full page - triggers a second request.
				for i := 0; i < getAllRecordsPageSize; i++ {
					hits = append(hits, makeHit(fmt.Sprintf("chunk-%05d", i)))
				}
			} else {
				// A short page - the loop must stop after this one.
				hits = []string{makeHit("chunk-last-1"), makeHit("chunk-last-2")}
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`{"took":1,"hits":{"total":{"value":0},"hits":[%s]}}`, strings.Join(hits, ","))))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	store, err := NewOpenSearchStore(context.Background(), OpenSearchConfig{
		Addresses:   []string{server.URL},
		CreateIndex: false,
		KNN:         false,
	})
	if err != nil {
		t.Fatalf("NewOpenSearchStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	records, err := store.Get(context.Background(), "kb", nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if pageRequests != 2 {
		t.Fatalf("expected exactly 2 pages fetched, got %d", pageRequests)
	}
	want := getAllRecordsPageSize + 2
	if len(records) != want {
		t.Fatalf("expected %d records across both pages, got %d", want, len(records))
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
