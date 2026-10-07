package vector

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// clusteredUnitVectors makes n vectors around `clusters` centres: closer to what an embedding model gives than isotropic noise.
func clusteredUnitVectors(rng *rand.Rand, n, dim, clusters int) [][]float32 {
	centres := make([][]float32, clusters)
	for i := range centres {
		centres[i] = randomUnit(rng, dim, nil, 1)
	}
	out := make([][]float32, n)
	for i := range out {
		out[i] = randomUnit(rng, dim, centres[rng.Intn(clusters)], 0.8)
	}
	return out
}

// exactNearest is the brute force answer: the keys of the k nearest of the vectors.
func exactNearest(vecs [][]float32, q []float32, k int) map[string]bool {
	type scored struct {
		key  string
		dist float32
	}
	all := make([]scored, len(vecs))
	qInv := invNorm(q)
	for i, v := range vecs {
		all[i] = scored{fmt.Sprintf("k%05d", i), 1 - dot(q, v)*qInv*invNorm(v)}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].dist < all[j].dist })
	out := map[string]bool{}
	for _, s := range all[:k] {
		out[s.key] = true
	}
	return out
}

// TestHNSWIndexRecall is the check the graph of github.com/coder/hnsw failed (8% of the true nearest neighbours at 2,000 random
// vectors, 2% at 20,000): the answers of the graph against an exact search.
func TestHNSWIndexRecall(t *testing.T) {
	const dim, k, queries = 64, 10, 60
	for _, tc := range []struct {
		name      string
		n         int
		clustered bool
		minRecall float64
	}{
		{"clustered 3000", 3000, true, 0.95},
		{"random 3000", 3000, false, 0.80}, // random vectors have no structure to follow: the hardest case
	} {
		t.Run(tc.name, func(t *testing.T) {
			rng := rand.New(rand.NewSource(7))
			var vecs [][]float32
			if tc.clustered {
				vecs = clusteredUnitVectors(rng, tc.n, dim, 40)
			} else {
				vecs = make([][]float32, tc.n)
				for i := range vecs {
					vecs[i] = randomUnit(rng, dim, nil, 1)
				}
			}
			x := newHNSWIndex()
			for i, v := range vecs {
				if err := x.Add(fmt.Sprintf("k%05d", i), v); err != nil {
					t.Fatal(err)
				}
			}
			hits := 0
			for q := 0; q < queries; q++ {
				query := randomUnit(rng, dim, vecs[rng.Intn(tc.n)], 0.5)
				truth := exactNearest(vecs, query, k)
				got, err := x.Search(query, k, 0)
				if err != nil {
					t.Fatal(err)
				}
				if len(got) != k {
					t.Fatalf("got %d results, want %d", len(got), k)
				}
				for i, h := range got {
					if truth[h.Key] {
						hits++
					}
					if i > 0 && h.Dist < got[i-1].Dist {
						t.Fatalf("results not best first: %v", got)
					}
				}
			}
			recall := float64(hits) / float64(queries*k)
			t.Logf("recall@%d = %.3f", k, recall)
			if recall < tc.minRecall {
				t.Fatalf("recall %.3f, want at least %.2f", recall, tc.minRecall)
			}
		})
	}
}

func TestHNSWIndexReplaceDeleteAndCompact(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	const dim = 16
	vecs := clusteredUnitVectors(rng, 400, dim, 8)
	x := newHNSWIndex()
	for i, v := range vecs {
		if err := x.Add(fmt.Sprintf("k%05d", i), v); err != nil {
			t.Fatal(err)
		}
	}
	// replace: the same key, another vector
	target := randomUnit(rng, dim, nil, 1)
	if err := x.Add("k00000", target); err != nil {
		t.Fatal(err)
	}
	if x.Len() != 400 {
		t.Fatalf("Len = %d after a replace, want 400", x.Len())
	}
	hits, _ := x.Search(target, 1, 0)
	if len(hits) != 1 || hits[0].Key != "k00000" || hits[0].Dist > 1e-4 {
		t.Fatalf("the replaced vector should be its own nearest: %+v", hits)
	}
	// delete most of the graph: tombstones first, then a rebuild
	for i := 1; i < 350; i++ {
		if !x.Delete(fmt.Sprintf("k%05d", i)) {
			t.Fatalf("key %d not found", i)
		}
	}
	if x.Delete("k00001") {
		t.Fatal("a deleted key must not be deleted twice")
	}
	if x.Len() != 51 {
		t.Fatalf("Len = %d, want 51", x.Len())
	}
	if len(x.nodes) > 2*x.Len()+64 {
		t.Fatalf("the graph kept %d nodes for %d live vectors: it should have been rebuilt", len(x.nodes), x.Len())
	}
	got, err := x.Search(vecs[360], 5, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 || got[0].Key != "k00360" {
		t.Fatalf("search after the deletes: %+v", got)
	}
	for _, h := range got {
		var i int
		_, _ = fmt.Sscanf(h.Key, "k%05d", &i)
		if i >= 1 && i < 350 {
			t.Fatalf("a deleted key is in the results: %s", h.Key)
		}
	}
}

func TestHNSWIndexSearchEdgeCases(t *testing.T) {
	x := newHNSWIndex()
	if hits, err := x.Search([]float32{1, 0}, 3, 0); err != nil || hits != nil {
		t.Fatalf("empty index: %v %v", hits, err)
	}
	if err := x.Add("a", []float32{1, 0}); err != nil {
		t.Fatal(err)
	}
	if err := x.Add("b", []float32{1, 0, 0}); err == nil {
		t.Fatal("a vector of another dimension must be refused")
	}
	if err := x.Add("c", nil); err == nil {
		t.Fatal("an empty vector must be refused")
	}
	if _, err := x.Search([]float32{1, 0, 0}, 1, 0); err == nil {
		t.Fatal("a query of another dimension must be refused")
	}
	if err := x.Add("zero", []float32{0, 0}); err != nil {
		t.Fatal(err)
	}
	hits, err := x.Search([]float32{1, 0}, 5, 0)
	if err != nil || len(hits) != 2 || hits[0].Key != "a" {
		t.Fatalf("hits %+v err %v", hits, err)
	}
}

func TestHNSWIndexSaveAndLoad(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	vecs := clusteredUnitVectors(rng, 500, 24, 10)
	x := newHNSWIndex()
	for i, v := range vecs {
		_ = x.Add(fmt.Sprintf("k%05d", i), v)
	}
	x.Delete("k00007")
	path := filepath.Join(t.TempDir(), "ns.hnsw")
	if err := x.Save(path); err != nil {
		t.Fatal(err)
	}
	y, err := loadHNSWIndex(path)
	if err != nil {
		t.Fatal(err)
	}
	if y.Len() != x.Len() || y.Has("k00007") {
		t.Fatalf("loaded Len %d (want %d), has the deleted key: %v", y.Len(), x.Len(), y.Has("k00007"))
	}
	for q := 0; q < 20; q++ {
		query := randomUnit(rng, 24, vecs[rng.Intn(500)], 0.4)
		a, _ := x.Search(query, 10, 0)
		b, _ := y.Search(query, 10, 0)
		if fmt.Sprint(a) != fmt.Sprint(b) {
			t.Fatalf("the loaded index answers differently:\n%v\n%v", a, b)
		}
	}
	// the loaded index can still be written to
	if err := y.Add("new", vecs[0]); err != nil {
		t.Fatal(err)
	}

	// a damaged file is an error, not a crash or a huge allocation
	data, _ := os.ReadFile(path)
	for _, cut := range []int{10, 40, len(data) / 2, len(data) - 3} {
		bad := filepath.Join(t.TempDir(), "bad.hnsw")
		_ = os.WriteFile(bad, data[:cut], 0o600)
		if _, err := loadHNSWIndex(bad); err == nil {
			t.Fatalf("a file cut at %d bytes loaded", cut)
		}
	}
	notOurs := filepath.Join(t.TempDir(), "legacy.hnsw")
	_ = os.WriteFile(notOurs, []byte("anything else"), 0o600)
	if _, err := loadHNSWIndex(notOurs); !errors.Is(err, errNotHNSWIndex) {
		t.Fatalf("err = %v, want errNotHNSWIndex", err)
	}
}
