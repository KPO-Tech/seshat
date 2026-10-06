package vector

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestHybridCandidatesReadMoreThanTopK(t *testing.T) {
	t.Parallel()
	cases := map[int]int{0: 100, 1: 100, 5: 100, 10: 100, 20: 200, 50: 500, 500: 500}
	for topK, want := range cases {
		if got := hybridCandidates(topK); got != want {
			t.Errorf("hybridCandidates(%d) = %d, want %d", topK, got, want)
		}
	}
}

func TestBlendHybridGivesAHitInBothListsBothScores(t *testing.T) {
	t.Parallel()
	results := blendHybrid(
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

func TestBlendHybridWeightsTheListsAndDoesNotNeedThemSorted(t *testing.T) {
	t.Parallel()
	vector := []SearchResult{{Record: Record{Key: "a"}, Score: 0.2}, {Record: Record{Key: "b"}, Score: 0.9}}
	keyword := []SearchResult{{Record: Record{Key: "a"}, Score: 3}, {Record: Record{Key: "c"}, Score: 1}}
	byKey := func(results []SearchResult) map[string]float32 {
		out := map[string]float32{}
		for _, r := range results {
			out[r.Record.Key] = r.Score
		}
		return out
	}
	// Only the vector list counts.
	only := byKey(blendHybrid(vector, keyword, 0, 10))
	if only["b"] != 1 || only["c"] != 0 {
		t.Errorf("weight 0 should be the vector list divided by its best: %v", only)
	}
	// Only the keyword list counts.
	words := byKey(blendHybrid(vector, keyword, 1, 10))
	if words["a"] != 1 || words["b"] != 0 || words["c"] <= 0.33 || words["c"] >= 0.34 {
		t.Errorf("weight 1 should be the keyword list divided by its best: %v", words)
	}
}

func TestBlendHybridIsDeterministicOnTies(t *testing.T) {
	t.Parallel()
	vector := []SearchResult{{Record: Record{Key: "z"}, Score: 1}, {Record: Record{Key: "a"}, Score: 1}, {Record: Record{Key: "m"}, Score: 1}}
	for i := 0; i < 20; i++ {
		got := blendHybrid(vector, nil, 0.5, 3)
		if got[0].Record.Key != "a" || got[1].Record.Key != "m" || got[2].Record.Key != "z" {
			t.Fatalf("ties must go to the smaller key: %v", got)
		}
	}
}

func TestBlendHybridWithAListWithNothingToGive(t *testing.T) {
	t.Parallel()
	got := blendHybrid([]SearchResult{{Record: Record{Key: "a"}, Score: 0.5}}, []SearchResult{{Record: Record{Key: "b"}, Score: 0}}, 0.5, 5)
	if len(got) != 2 || got[0].Record.Key != "a" {
		t.Errorf("a keyword list of zeros adds nothing: %v", got)
	}
	if got := blendHybrid(nil, nil, 0.5, 5); len(got) != 0 {
		t.Errorf("nothing in, nothing out: %v", got)
	}
}

// A chunk the vectors rank far down and the words match must be found even when topK is small: the keyword list is read to the
// candidates of the search, not to topK, and the vector list is not cut to topK either.
func TestMemoryHybridSearchFindsWhatOnlyTheWordsMatch(t *testing.T) {
	t.Parallel()
	store := NewMemoryStore()
	ctx := context.Background()
	records := []Record{{Namespace: "kb", Key: "answer", Text: "the quasar redshift is 2.3", Vector: []float32{0, 1}}}
	for i := 0; i < 30; i++ {
		// thirty chunks the vector search prefers
		records = append(records, Record{Namespace: "kb", Key: fmt.Sprintf("noise-%02d", i), Text: "unrelated words about weather", Vector: []float32{1, 0.01 * float32(i)}})
	}
	if err := store.Upsert(ctx, records); err != nil {
		t.Fatal(err)
	}
	query := Query{Namespace: "kb", Vector: []float32{1, 0}, TopK: 3, QueryText: "quasar redshift"}

	if got, _ := store.Search(ctx, query); containsKey(got, "answer") {
		t.Fatalf("a pure vector search should not find it: %v", got)
	}
	query.HybridWeight = 0.5
	got, err := store.Search(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if !containsKey(got, "answer") {
		t.Errorf("a hybrid search finds the chunk its words match, whatever the vectors say: %v", got)
	}
}

func TestSQLiteHybridSearchFindsWhatOnlyTheWordsMatch(t *testing.T) {
	t.Parallel()
	store := openTestSQLiteStore(t)
	ctx := context.Background()
	records := []Record{{Namespace: "kb", Key: "answer", Text: "the quasar redshift is 2.3", Vector: []float32{0, 1}}}
	for i := 0; i < 30; i++ {
		records = append(records, Record{Namespace: "kb", Key: fmt.Sprintf("noise-%02d", i), Text: "unrelated words about weather", Vector: []float32{1, 0.01 * float32(i)}})
	}
	if err := store.Upsert(ctx, records); err != nil {
		t.Fatal(err)
	}
	got, err := store.Search(ctx, Query{Namespace: "kb", Vector: []float32{1, 0}, TopK: 3, HybridWeight: 0.5, QueryText: "quasar redshift"})
	if err != nil {
		t.Fatal(err)
	}
	if !containsKey(got, "answer") {
		t.Errorf("a hybrid search finds the chunk its words match, whatever the vectors say: %v", got)
	}
}

func TestPgTextSearchQueryIsAListOfQuotedAlternatives(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"deep learning":                     "'deep' | 'learning'",
		"What is the DOCLAYNET dataset?":    "'what' | 'the' | 'doclaynet' | 'dataset'",
		"réseaux de neurones":               "'réseaux' | 'neurones'",
		"to be or not to be":                "'not'",
		"a'; DROP TABLE vector_chunks; --":  "'drop' | 'table' | 'vector' | 'chunks'",
		"repeat repeat repeat":              "'repeat'",
		"!!! ???":                           "",
		"":                                  "",
		"Привет мир, это тест":              "'привет' | 'мир' | 'это' | 'тест'",
		"AES-256 and (x0, y0, x1, y1) 2024": "'aes' | '256' | 'and' | '2024'",
	}
	for in, want := range cases {
		if got := pgTextSearchQuery(in); got != want {
			t.Errorf("pgTextSearchQuery(%q) = %q, want %q", in, got, want)
		}
	}
	long := strings.Repeat("word1 word2 word3 word4 word5 word6 word7 word8 ", 10) + strings.Repeat("extra", 1)
	if n := strings.Count(pgTextSearchQuery(long), "|") + 1; n > maxQueryWords {
		t.Errorf("a query has at most %d words, got %d", maxQueryWords, n)
	}
}

func TestPgVectorTextSearchConfigIsAnIdentifier(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{"": "simple", "english": "english", "french": "french", "x'); DROP TABLE a; --": "simple", "English": "simple", "pg_catalog.simple": "simple"} {
		if got := normalizePgVectorOptions(PgVectorOptions{TextSearchConfig: in}).TextSearchConfig; got != want {
			t.Errorf("TextSearchConfig %q -> %q, want %q", in, got, want)
		}
	}
}

func containsKey(results []SearchResult, key string) bool {
	for _, r := range results {
		if r.Record.Key == key {
			return true
		}
	}
	return false
}

// A hybrid search on a real pgvector database: the vector side and the keyword side (the text_search column and its
// index) are both SQL, which the unit tests above cannot reach.
func TestPgVectorStore_HybridSearch(t *testing.T) {
	database := openPgVectorTestDB(t)
	ctx := context.Background()
	store, err := NewPgVectorStore(ctx, database, PgVectorOptions{Dim: 1536, CreateExtension: true, IndexMethod: "hnsw", TextSearchConfig: "simple"})
	if err != nil {
		t.Fatalf("NewPgVectorStore: %v", err)
	}
	namespace := "itest_pgvector_hybrid"
	_ = store.DeleteNamespace(ctx, namespace)
	t.Cleanup(func() { _ = store.DeleteNamespace(ctx, namespace) })

	// "needle" is far from the query vector (a different axis) but is the only chunk that holds the word.
	var records []Record
	for i := 0; i < 30; i++ {
		records = append(records, Record{
			Namespace: namespace, Key: fmt.Sprintf("filler-%02d", i), Text: "ordinary text about the weather",
			Vector: sparseVector1536(i % 5), Metadata: map[string]string{"scope": "a"},
		})
	}
	records = append(records,
		Record{Namespace: namespace, Key: "needle", Text: "the clause says the Supplier owes a quarterly zebracode", Vector: sparseVector1536(100), Metadata: map[string]string{"scope": "a"}},
		Record{Namespace: namespace, Key: "hidden", Text: "another zebracode in a scope that is not allowed", Vector: sparseVector1536(101), Metadata: map[string]string{"scope": "b"}},
	)
	if err := store.Upsert(ctx, records); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	vectorOnly, err := store.Search(ctx, Query{Namespace: namespace, Vector: sparseVector1536(0), TopK: 3})
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	for _, r := range vectorOnly {
		if r.Record.Key == "needle" {
			t.Fatalf("the needle should not be among the nearest vectors: %+v", vectorOnly)
		}
	}

	query := Query{Namespace: namespace, Vector: sparseVector1536(0), QueryText: "zebracode", TopK: 3, HybridWeight: 0.7}
	results, err := store.Search(ctx, query)
	if err != nil {
		t.Fatalf("hybrid search: %v", err)
	}
	found := map[string]bool{}
	for _, r := range results {
		found[r.Record.Key] = true
	}
	if !found["needle"] || !found["hidden"] {
		t.Fatalf("hybrid search should find the chunks only the word matches, got %+v", results)
	}

	query.Filter = map[string]any{"scope": "a"}
	results, err = store.Search(ctx, query)
	if err != nil {
		t.Fatalf("hybrid search with a filter: %v", err)
	}
	for _, r := range results {
		if r.Record.Key == "hidden" {
			t.Fatalf("the filter must apply to the keyword side too: %+v", results)
		}
	}
	if len(results) == 0 || results[0].Record.Key != "needle" && results[0].Record.Key != "filler-00" {
		t.Fatalf("unexpected hybrid result with a filter: %+v", results)
	}

	// The column is stored and indexed: the planner can answer the word match from the GIN index (on a table this small it
	// would rather scan, so the scan is switched off), and no to_tsvector is evaluated at query time.
	var plan string
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
		t.Fatalf("set: %v", err)
	}
	explain, err := tx.QueryContext(ctx, `EXPLAIN SELECT id FROM vector_chunks WHERE text_search @@ to_tsquery('simple', $1)`, "'zebracode'")
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer explain.Close()
	for explain.Next() {
		var line string
		_ = explain.Scan(&line)
		plan += line + "\n"
	}
	if !strings.Contains(plan, "idx_vector_chunks_text_search") {
		t.Fatalf("the keyword query does not use the text search index:\n%s", plan)
	}
}
