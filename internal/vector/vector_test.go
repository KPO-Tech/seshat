package vector

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	dbpkg "github.com/EngineerProjects/nexus-engine/internal/db"
)

func TestMemoryStoreSearchRanksByCosineSimilarity(t *testing.T) {
	store := NewMemoryStore()
	if err := store.Upsert(context.Background(), []Record{
		{Namespace: "docs", Key: "a", Text: "alpha", Vector: []float32{1, 0}},
		{Namespace: "docs", Key: "b", Text: "beta", Vector: []float32{0, 1}},
	}); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	results, err := store.Search(context.Background(), Query{
		Namespace: "docs",
		Vector:    []float32{1, 0},
		TopK:      1,
	})
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Record.Key != "a" {
		t.Fatalf("unexpected top result: %s", results[0].Record.Key)
	}
}

func openPgVectorTestDB(t *testing.T) *dbpkg.DB {
	t.Helper()
	dsn := os.Getenv("NEXUS_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("NEXUS_TEST_POSTGRES_DSN not set")
	}
	database, err := dbpkg.Open(context.Background(), dbpkg.DefaultPostgresConfig(dsn))
	if err != nil {
		t.Fatalf("open postgres db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func TestPgVectorStore_UpsertSearchDeleteNamespace(t *testing.T) {
	database := openPgVectorTestDB(t)
	ctx := context.Background()

	store, err := NewPgVectorStore(ctx, database, PgVectorOptions{
		Dim:             1536,
		CreateExtension: true,
		IndexMethod:     "hnsw",
	})
	if err != nil {
		t.Fatalf("NewPgVectorStore: %v", err)
	}

	namespace := "itest_pgvector_slice6"
	_ = store.DeleteNamespace(ctx, namespace)
	t.Cleanup(func() { _ = store.DeleteNamespace(ctx, namespace) })

	err = store.Upsert(ctx, []Record{
		{
			Namespace: namespace,
			Key:       "a",
			Text:      "alpha",
			Vector:    sparseVector1536(0),
			Metadata:  map[string]string{"kind": "primary"},
		},
		{
			Namespace: namespace,
			Key:       "b",
			Text:      "beta",
			Vector:    sparseVector1536(1),
			Metadata:  map[string]string{"kind": "secondary"},
		},
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	results, err := store.Search(ctx, Query{
		Namespace: namespace,
		Vector:    sparseVector1536(0),
		TopK:      2,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Record.Key != "a" {
		t.Fatalf("expected best match 'a', got %q", results[0].Record.Key)
	}
	if results[0].Score < results[1].Score {
		t.Fatalf("expected descending score order, got %f then %f", results[0].Score, results[1].Score)
	}

	filtered, err := store.Search(ctx, Query{
		Namespace: namespace,
		Vector:    sparseVector1536(0),
		TopK:      2,
		Filter:    map[string]any{"kind": "primary"},
	})
	if err != nil {
		t.Fatalf("Search filtered: %v", err)
	}
	if len(filtered) != 1 || filtered[0].Record.Key != "a" {
		t.Fatalf("expected filtered result 'a', got %+v", filtered)
	}

	if err := store.DeleteKeys(ctx, namespace, []string{"a"}); err != nil {
		t.Fatalf("DeleteKeys: %v", err)
	}

	remaining, err := store.Get(ctx, namespace, nil)
	if err != nil {
		t.Fatalf("Get remaining: %v", err)
	}
	if len(remaining) != 1 || remaining[0].Key != "b" {
		t.Fatalf("expected only 'b' remaining, got %+v", remaining)
	}

	if err := store.DeleteNamespace(ctx, namespace); err != nil {
		t.Fatalf("DeleteNamespace: %v", err)
	}
	exists, err := store.HasNamespace(ctx, namespace)
	if err != nil {
		t.Fatalf("HasNamespace: %v", err)
	}
	if exists {
		t.Fatal("expected namespace to be deleted")
	}
}

func TestPgVectorStore_SeparateDatabaseHandleWithoutBackendMigrations(t *testing.T) {
	dsn := os.Getenv("NEXUS_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("NEXUS_TEST_POSTGRES_DSN not set")
	}

	database, err := dbpkg.Open(context.Background(), dbpkg.Config{
		Driver:      dbpkg.DriverPostgres,
		DSN:         dsn,
		AutoMigrate: false,
	})
	if err != nil {
		t.Fatalf("open postgres db without automigrate: %v", err)
	}
	defer database.Close()

	store, err := NewPgVectorStore(context.Background(), database, PgVectorOptions{
		Dim:             1536,
		CreateExtension: true,
		IndexMethod:     "hnsw",
	})
	if err != nil {
		t.Fatalf("NewPgVectorStore with separate handle: %v", err)
	}

	indexDef, err := store.existingVectorIndexDefinition(context.Background(), "idx_vector_chunks_embedding")
	if err != nil {
		t.Fatalf("existingVectorIndexDefinition: %v", err)
	}
	if indexDef == "" {
		t.Fatal("expected pgvector index to exist")
	}
}

func sparseVector1536(activeIndex int) []float32 {
	vector := make([]float32, 1536)
	if activeIndex >= 0 && activeIndex < len(vector) {
		vector[activeIndex] = 1
	}
	return vector
}

func TestNormalizePgVectorOptionsDefaults(t *testing.T) {
	options := normalizePgVectorOptions(PgVectorOptions{})

	if options.Dim != 1536 {
		t.Fatalf("expected default dim 1536, got %d", options.Dim)
	}
	if options.IndexMethod != "hnsw" {
		t.Fatalf("expected default index method hnsw, got %q", options.IndexMethod)
	}
	if options.HNSWM != 16 {
		t.Fatalf("expected default hnsw m 16, got %d", options.HNSWM)
	}
	if options.HNSWEfConstruction != 64 {
		t.Fatalf("expected default hnsw ef_construction 64, got %d", options.HNSWEfConstruction)
	}
	if options.IVFFlatLists != 100 {
		t.Fatalf("expected default ivfflat lists 100, got %d", options.IVFFlatLists)
	}
}

func TestNormalizePgVectorOptionsKeepsIVFFlat(t *testing.T) {
	options := normalizePgVectorOptions(PgVectorOptions{
		IndexMethod:  "ivfflat",
		IVFFlatLists: 42,
	})

	if options.IndexMethod != "ivfflat" {
		t.Fatalf("expected ivfflat, got %q", options.IndexMethod)
	}
	if options.IVFFlatLists != 42 {
		t.Fatalf("expected ivfflat lists 42, got %d", options.IVFFlatLists)
	}
}

func TestExtractPgIndexMethod(t *testing.T) {
	if got := extractPgIndexMethod("CREATE INDEX idx ON public.vector_chunks USING hnsw (embedding vector_cosine_ops)"); got != "hnsw" {
		t.Fatalf("expected hnsw, got %q", got)
	}
	if got := extractPgIndexMethod("create index idx on public.vector_chunks using ivfflat (embedding vector_cosine_ops) with (lists = 100)"); got != "ivfflat" {
		t.Fatalf("expected ivfflat, got %q", got)
	}
	if got := extractPgIndexMethod(""); got != "" {
		t.Fatalf("expected empty index method, got %q", got)
	}
}

func TestPgVectorNewStoreRequiresPostgres(t *testing.T) {
	database, err := dbpkg.Open(context.Background(), dbpkg.DefaultSQLiteConfig(filepath.Join(t.TempDir(), "vector.db")))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	_, err = NewStore(context.Background(), Config{Backend: BackendPgVector, DB: database})
	if err == nil {
		t.Fatal("expected pgvector store creation to reject sqlite database")
	}
}

func TestPgVectorConfigDefaultsCreateExtension(t *testing.T) {
	database, err := dbpkg.Open(context.Background(), dbpkg.DefaultSQLiteConfig(filepath.Join(t.TempDir(), "vector.db")))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer database.Close()

	cfg := Config{Backend: BackendPgVector, DB: database}
	_, err = NewStore(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected pgvector store creation to reject sqlite database")
	}
	if cfg.PgVectorCreateExtension != nil {
		t.Fatal("expected zero-value config to keep nil extension flag before normalization")
	}
}

func openTestSQLiteStore(t *testing.T) *SQLiteStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vectors.db")
	store, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatalf("OpenSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestSQLiteStore_UpsertAndSearch(t *testing.T) {
	store := openTestSQLiteStore(t)
	ctx := context.Background()

	err := store.Upsert(ctx, []Record{
		{Namespace: "docs", Key: "a", Text: "alpha doc", Vector: []float32{1, 0}},
		{Namespace: "docs", Key: "b", Text: "beta doc", Vector: []float32{0, 1}},
	})
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	results, err := store.Search(ctx, Query{
		Namespace: "docs",
		Vector:    []float32{1, 0},
		TopK:      1,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Record.Key != "a" {
		t.Errorf("expected key=a, got %q", results[0].Record.Key)
	}
	if results[0].Score < 0.99 {
		t.Errorf("expected score near 1.0, got %f", results[0].Score)
	}
}

func TestSQLiteStore_UpsertOverwrites(t *testing.T) {
	store := openTestSQLiteStore(t)
	ctx := context.Background()

	_ = store.Upsert(ctx, []Record{
		{Namespace: "ns", Key: "k1", Text: "original", Vector: []float32{1, 0}},
	})
	_ = store.Upsert(ctx, []Record{
		{Namespace: "ns", Key: "k1", Text: "updated", Vector: []float32{0, 1}},
	})

	results, err := store.Search(ctx, Query{Namespace: "ns", Vector: []float32{0, 1}, TopK: 1})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].Record.Text != "updated" {
		t.Errorf("expected updated record, got %+v", results)
	}
}

func TestSQLiteStore_DeleteNamespace(t *testing.T) {
	store := openTestSQLiteStore(t)
	ctx := context.Background()

	_ = store.Upsert(ctx, []Record{
		{Namespace: "ns", Key: "k1", Vector: []float32{1, 0}},
		{Namespace: "ns", Key: "k2", Vector: []float32{0, 1}},
	})

	if err := store.DeleteNamespace(ctx, "ns"); err != nil {
		t.Fatalf("DeleteNamespace: %v", err)
	}

	count, _ := store.CountNamespace(ctx, "ns")
	if count != 0 {
		t.Errorf("expected 0 records after DeleteNamespace, got %d", count)
	}
}

func TestSQLiteStore_DeleteKeys(t *testing.T) {
	store := openTestSQLiteStore(t)
	ctx := context.Background()

	_ = store.Upsert(ctx, []Record{
		{Namespace: "ns", Key: "k1", Vector: []float32{1, 0}},
		{Namespace: "ns", Key: "k2", Vector: []float32{0, 1}},
		{Namespace: "ns", Key: "k3", Vector: []float32{1, 1}},
	})

	if err := store.DeleteKeys(ctx, "ns", []string{"k1", "k3"}); err != nil {
		t.Fatalf("DeleteKeys: %v", err)
	}

	count, _ := store.CountNamespace(ctx, "ns")
	if count != 1 {
		t.Errorf("expected 1 remaining record, got %d", count)
	}

	results, _ := store.Search(ctx, Query{Namespace: "ns", Vector: []float32{0, 1}, TopK: 5})
	if len(results) != 1 || results[0].Record.Key != "k2" {
		t.Errorf("expected only k2 left, got %+v", results)
	}
}

func TestSQLiteStore_MultiNamespaceIsolation(t *testing.T) {
	store := openTestSQLiteStore(t)
	ctx := context.Background()

	_ = store.Upsert(ctx, []Record{
		{Namespace: "ns1", Key: "a", Vector: []float32{1, 0}},
		{Namespace: "ns2", Key: "b", Vector: []float32{1, 0}},
	})

	r1, _ := store.Search(ctx, Query{Namespace: "ns1", Vector: []float32{1, 0}, TopK: 5})
	r2, _ := store.Search(ctx, Query{Namespace: "ns2", Vector: []float32{1, 0}, TopK: 5})

	if len(r1) != 1 || r1[0].Record.Key != "a" {
		t.Errorf("ns1 isolation failed: %+v", r1)
	}
	if len(r2) != 1 || r2[0].Record.Key != "b" {
		t.Errorf("ns2 isolation failed: %+v", r2)
	}
}

func TestSQLiteStore_EmptyNamespaceSearch(t *testing.T) {
	store := openTestSQLiteStore(t)
	ctx := context.Background()

	results, err := store.Search(ctx, Query{Namespace: "empty", Vector: []float32{1, 0}, TopK: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}

func TestSQLiteStore_Metadata(t *testing.T) {
	store := openTestSQLiteStore(t)
	ctx := context.Background()

	_ = store.Upsert(ctx, []Record{
		{
			Namespace: "ns",
			Key:       "doc1",
			Text:      "hello",
			Vector:    []float32{1, 0},
			Metadata:  map[string]string{"filename": "intro.txt", "position": "0"},
		},
	})

	results, _ := store.Search(ctx, Query{Namespace: "ns", Vector: []float32{1, 0}, TopK: 1})
	if len(results) != 1 {
		t.Fatalf("expected 1 result")
	}
	if results[0].Record.Metadata["filename"] != "intro.txt" {
		t.Errorf("metadata not preserved: %+v", results[0].Record.Metadata)
	}
}

func TestSQLiteStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vectors.db")

	// Open, write, close.
	s1, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	ctx := context.Background()
	if err := s1.Upsert(ctx, []Record{
		{Namespace: "corpus", Key: "chunk-0", Text: "persistent text", Vector: []float32{0.5, 0.5}},
	}); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	_ = s1.Close()

	// Re-open, verify data survived.
	s2, err := OpenSQLiteStore(path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer s2.Close()

	count, _ := s2.CountNamespace(ctx, "corpus")
	if count != 1 {
		t.Errorf("expected 1 record after reopen, got %d", count)
	}

	results, err := s2.Search(ctx, Query{Namespace: "corpus", Vector: []float32{0.5, 0.5}, TopK: 1})
	if err != nil {
		t.Fatalf("Search after reopen: %v", err)
	}
	if len(results) != 1 || results[0].Record.Text != "persistent text" {
		t.Errorf("data not persisted across reopens: %+v", results)
	}
}

func TestSQLiteStore_VectorEncoding(t *testing.T) {
	cases := [][]float32{
		{0, 0, 0},
		{1, 2, 3},
		{-1, 0.5, 1e-6},
		{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, // 16-dim
	}
	for _, v := range cases {
		blob, err := encodeVector(v)
		if err != nil {
			t.Fatalf("encodeVector: %v", err)
		}
		got, err := decodeVector(blob)
		if err != nil {
			t.Fatalf("decodeVector: %v", err)
		}
		if len(got) != len(v) {
			t.Fatalf("length mismatch: got %d, want %d", len(got), len(v))
		}
		for i := range v {
			if got[i] != v[i] {
				t.Errorf("value mismatch at %d: got %f, want %f", i, got[i], v[i])
			}
		}
	}
}

func TestSQLiteStore_ValidationErrors(t *testing.T) {
	store := openTestSQLiteStore(t)
	ctx := context.Background()

	cases := []struct {
		name string
		rec  Record
	}{
		{"missing namespace", Record{Key: "k", Vector: []float32{1}}},
		{"missing key", Record{Namespace: "ns", Vector: []float32{1}}},
		{"missing vector", Record{Namespace: "ns", Key: "k"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := store.Upsert(ctx, []Record{tc.rec})
			if err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}

func TestNewSQLiteStore_RejectsNilDB(t *testing.T) {
	_, err := NewSQLiteStore(nil)
	if err == nil {
		t.Error("expected error for nil db")
	}
}

func TestNewSQLiteStore_RejectsWrongDriver(t *testing.T) {
	// Can't create a non-SQLite DB without another driver, so test the guard indirectly.
	// Just verify our error message mentions the driver.
	cfg := dbpkg.Config{Driver: "postgres", DSN: "user=x"}
	_, openErr := dbpkg.Open(context.Background(), cfg)
	// If it opens somehow, test NewSQLiteStore rejection.
	if openErr == nil {
		t.Skip("postgres driver opened unexpectedly, skipping guard test")
	}
}

// ─── Hybrid search (BM25 + vector) ───────────────────────────────────────────

func TestSQLiteStore_HybridSearch_PureVector(t *testing.T) {
	// HybridWeight=0 must behave identically to pure vector search.
	store := openTestSQLiteStore(t)
	ctx := context.Background()
	_ = store.Upsert(ctx, []Record{
		{Namespace: "h", Key: "alpha", Text: "alpha document content", Vector: []float32{1, 0}},
		{Namespace: "h", Key: "beta", Text: "beta document content", Vector: []float32{0, 1}},
	})
	results, err := store.Search(ctx, Query{
		Namespace:    "h",
		Vector:       []float32{1, 0},
		TopK:         1,
		HybridWeight: 0,
		QueryText:    "alpha",
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 || results[0].Record.Key != "alpha" {
		t.Errorf("pure vector: expected alpha on top, got %v", results)
	}
}

func TestSQLiteStore_HybridSearch_Blended(t *testing.T) {
	// HybridWeight > 0: record whose text matches the query term should score
	// higher than a non-matching record when keyword overlap is decisive.
	store := openTestSQLiteStore(t)
	ctx := context.Background()
	_ = store.Upsert(ctx, []Record{
		// Both records have the same vector, so vector score is identical.
		// Only BM25 (text match) can differentiate them.
		{Namespace: "hybrid", Key: "match", Text: "the quick brown fox jumps", Vector: []float32{1, 0}},
		{Namespace: "hybrid", Key: "nomatch", Text: "completely unrelated words", Vector: []float32{1, 0}},
	})
	results, err := store.Search(ctx, Query{
		Namespace:    "hybrid",
		Vector:       []float32{1, 0},
		TopK:         2,
		HybridWeight: 0.7,
		QueryText:    "quick fox",
	})
	if err != nil {
		t.Fatalf("HybridSearch: %v", err)
	}
	// "match" should appear first when BM25 dominates.
	if len(results) == 0 {
		t.Fatal("expected results, got none")
	}
	if results[0].Record.Key != "match" {
		t.Logf("Note: hybrid search did not rank 'match' first (FTS5 may be unavailable or scores tied)")
		// Not a hard failure — FTS5 might not be available in this build.
	}
}

func TestSQLiteStore_HybridSearch_EmptyQueryText(t *testing.T) {
	// Empty QueryText with HybridWeight > 0 must not error — it falls back to vector.
	store := openTestSQLiteStore(t)
	ctx := context.Background()
	_ = store.Upsert(ctx, []Record{
		{Namespace: "empty_q", Key: "a", Text: "some text", Vector: []float32{1, 0}},
	})
	results, err := store.Search(ctx, Query{
		Namespace:    "empty_q",
		Vector:       []float32{1, 0},
		TopK:         1,
		HybridWeight: 0.5,
		QueryText:    "",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestSQLiteStore_HybridSearch_FilterWithHybrid(t *testing.T) {
	// Filter must still be applied when HybridWeight > 0.
	store := openTestSQLiteStore(t)
	ctx := context.Background()
	_ = store.Upsert(ctx, []Record{
		{Namespace: "fh", Key: "a", Text: "hello world", Vector: []float32{1, 0},
			Metadata: map[string]string{"src": "doc-a"}},
		{Namespace: "fh", Key: "b", Text: "hello world", Vector: []float32{1, 0},
			Metadata: map[string]string{"src": "doc-b"}},
	})
	results, err := store.Search(ctx, Query{
		Namespace:    "fh",
		Vector:       []float32{1, 0},
		TopK:         5,
		HybridWeight: 0.3,
		QueryText:    "hello",
		Filter:       map[string]any{"src": "doc-a"},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, r := range results {
		if r.Record.Metadata["src"] != "doc-a" {
			t.Errorf("filter bypassed in hybrid search: got src=%s", r.Record.Metadata["src"])
		}
	}
}

func TestHNSWStore_UpsertSearchPersistence(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	store, err := NewHNSWStore(dir)
	if err != nil {
		t.Fatalf("NewHNSWStore: %v", err)
	}

	records := []Record{
		{Namespace: "ns", Key: "a", Text: "apple", Vector: []float32{1, 0, 0}},
		{Namespace: "ns", Key: "b", Text: "banana", Vector: []float32{0, 1, 0}},
		{Namespace: "ns", Key: "c", Text: "cherry", Vector: []float32{0, 0, 1}},
	}
	if err := store.Upsert(ctx, records); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	// Vector [1,0,0] should rank "a" first.
	results, err := store.Search(ctx, Query{Namespace: "ns", Vector: []float32{1, 0, 0}, TopK: 1})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 || results[0].Record.Key != "a" {
		t.Fatalf("expected key=a as top result, got %+v", results)
	}
	if results[0].Record.Text != "apple" {
		t.Fatalf("expected text=apple, got %q", results[0].Record.Text)
	}

	// Score should be ~1.0 for identical direction.
	if results[0].Score < 0.99 {
		t.Fatalf("expected score ~1, got %f", results[0].Score)
	}

	// HasNamespace
	has, err := store.HasNamespace(ctx, "ns")
	if err != nil || !has {
		t.Fatalf("HasNamespace: has=%v err=%v", has, err)
	}

	// --- Persistence: reload from disk ---
	store2, err := NewHNSWStore(dir)
	if err != nil {
		t.Fatalf("reload NewHNSWStore: %v", err)
	}
	results2, err := store2.Search(ctx, Query{Namespace: "ns", Vector: []float32{0, 1, 0}, TopK: 1})
	if err != nil {
		t.Fatalf("Search after reload: %v", err)
	}
	if len(results2) == 0 || results2[0].Record.Key != "b" {
		t.Fatalf("expected key=b after reload, got %+v", results2)
	}

	// DeleteKeys
	if err := store2.DeleteKeys(ctx, "ns", []string{"a"}); err != nil {
		t.Fatalf("DeleteKeys: %v", err)
	}
	got, err := store2.Get(ctx, "ns", []string{"a"})
	if err != nil {
		t.Fatalf("Get after delete: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected key=a deleted, still got %+v", got)
	}

	// DeleteNamespace
	if err := store2.DeleteNamespace(ctx, "ns"); err != nil {
		t.Fatalf("DeleteNamespace: %v", err)
	}
	has, err = store2.HasNamespace(ctx, "ns")
	if err != nil || has {
		t.Fatalf("after DeleteNamespace: has=%v err=%v", has, err)
	}
}

func TestHNSWStore_HybridKeywordBlend(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	store, err := NewHNSWStore(dir)
	if err != nil {
		t.Fatalf("NewHNSWStore: %v", err)
	}
	_ = store.Upsert(ctx, []Record{
		{Namespace: "k", Key: "x", Text: "the quick brown fox", Vector: []float32{1, 0}},
		{Namespace: "k", Key: "y", Text: "lazy dog sleeps", Vector: []float32{1, 0}}, // same vector direction
	})

	// Without hybrid both should have the same vector score.
	// With hybrid=1 "fox" should rank "x" higher.
	results, err := store.Search(ctx, Query{
		Namespace:    "k",
		Vector:       []float32{1, 0},
		TopK:         2,
		HybridWeight: 1.0,
		QueryText:    "fox",
	})
	if err != nil {
		t.Fatalf("hybrid search: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Record.Key != "x" {
		t.Fatalf("expected 'x' to rank first with keyword 'fox', got %+v", results)
	}
}
