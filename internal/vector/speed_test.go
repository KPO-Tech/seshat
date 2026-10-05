package vector

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	dbpkg "github.com/KPO-Tech/seshat/internal/db"
)

// TestStoreSpeed measures every store on the same synthetic collection: how long the ingestion takes, and the latency of a
// vector search and of a hybrid search (median and 95th percentile), plus how often the chunk a query was made from is among
// the first five results. It is a measure, not a check: it runs only when VECTOR_SPEED is set, and each remote store only
// when its address is:
//
//	VECTOR_SPEED=1 VECTOR_SPEED_CHUNKS=20000 \
//	SESHAT_TEST_POSTGRES_DSN=... QDRANT_INTEGRATION_ADDR=host:6334 CHROMA_INTEGRATION_URL=http://host:8000 \
//	OPENSEARCH_INTEGRATION_URL=http://host:9200 VECTOR_SPEED_ONLY=qdrant,memory go test ./internal/vector -run TestStoreSpeed -v -timeout 1h
//
// The data: vectors of 384 dimensions, texts of 60 words from a Zipf vocabulary plus two words of a pool where each word is
// held by about ten chunks; a query is the vector of a chunk with noise, and those two words of the chunk.
func TestStoreSpeed(t *testing.T) {
	if os.Getenv("VECTOR_SPEED") == "" {
		t.Skip("set VECTOR_SPEED=1 to measure the stores")
	}
	chunks := envInt("VECTOR_SPEED_CHUNKS", 20000)
	queries := envInt("VECTOR_SPEED_QUERIES", 200)
	const dim = 384

	rng := rand.New(rand.NewSource(1))
	vocab := make([]string, 3000)
	for i := range vocab {
		vocab[i] = fmt.Sprintf("w%dx", i)
	}
	zipf := rand.NewZipf(rng, 1.2, 4, uint64(len(vocab)-1))
	pool := max(chunks/5, 10)

	type chunk struct {
		record Record
		rare   [2]string
	}
	data := make([]chunk, chunks)
	vectors := make([][]float32, chunks)
	for i := range data {
		var words []string
		for j := 0; j < 60; j++ {
			words = append(words, vocab[zipf.Uint64()])
		}
		rare := [2]string{fmt.Sprintf("rare%dq", rng.Intn(pool)), fmt.Sprintf("rare%dq", rng.Intn(pool))}
		words = append(words, rare[0], rare[1])
		rng.Shuffle(len(words), func(a, b int) { words[a], words[b] = words[b], words[a] })
		vectors[i] = randomUnit(rng, dim, nil, 1)
		data[i] = chunk{
			record: Record{Namespace: "speed", Key: fmt.Sprintf("c%06d", i), Text: strings.Join(words, " "), Vector: vectors[i], Metadata: map[string]string{"scope": strconv.Itoa(i % 4)}},
			rare:   rare,
		}
	}
	type probe struct {
		target int
		query  Query
	}
	probes := make([]probe, queries)
	for i := range probes {
		target := rng.Intn(chunks)
		probes[i] = probe{target: target, query: Query{
			Namespace: "speed", Vector: randomUnit(rng, dim, vectors[target], 4.5), TopK: 10,
			QueryText: data[target].rare[0] + " " + data[target].rare[1],
		}}
	}

	ctx := context.Background()
	stores := map[string]func(t *testing.T) Store{
		"memory": func(t *testing.T) Store { return NewMemoryStore() },
		"sqlite": func(t *testing.T) Store {
			database, err := dbpkg.Open(ctx, dbpkg.DefaultSQLiteConfig(filepath.Join(t.TempDir(), "speed.db")))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = database.Close() })
			s, err := NewSQLiteStore(database)
			if err != nil {
				t.Fatal(err)
			}
			return s
		},
		"hnsw": func(t *testing.T) Store {
			s, err := NewHNSWStore(t.TempDir())
			if err != nil {
				t.Skipf("hnsw: %v", err)
			}
			return s
		},
	}
	if dsn := os.Getenv("SESHAT_TEST_POSTGRES_DSN"); dsn != "" {
		stores["pgvector"] = func(t *testing.T) Store {
			database, err := dbpkg.Open(ctx, dbpkg.DefaultPostgresConfig(dsn))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = database.Close() })
			s, err := NewPgVectorStore(ctx, database, PgVectorOptions{Dim: dim, CreateExtension: true, IndexMethod: "hnsw"})
			if err != nil {
				t.Fatal(err)
			}
			return s
		}
	}
	if address := os.Getenv("QDRANT_INTEGRATION_ADDR"); address != "" {
		stores["qdrant"] = func(t *testing.T) Store {
			host, portText, _ := strings.Cut(address, ":")
			port, _ := strconv.Atoi(portText)
			s, err := NewQdrantStore(ctx, QdrantConfig{Host: host, Port: port, CollPrefix: "speed_", DefaultDim: dim})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			return s
		}
	}
	if address := os.Getenv("CHROMA_INTEGRATION_URL"); address != "" {
		stores["chroma"] = func(t *testing.T) Store { return NewChromaStore(ChromaConfig{BaseURL: address}) }
	}
	if address := os.Getenv("OPENSEARCH_INTEGRATION_URL"); address != "" {
		stores["opensearch"] = func(t *testing.T) Store {
			s, err := NewOpenSearchStore(ctx, OpenSearchConfig{Addresses: []string{address}, IndexPrefix: "speed", DefaultDim: dim, CreateIndex: true, KNN: true, RequestTimeout: time.Minute})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = s.Close() })
			return s
		}
	}

	names := make([]string, 0, len(stores))
	for name := range stores {
		if only := os.Getenv("VECTOR_SPEED_ONLY"); only != "" && !strings.Contains(","+only+",", ","+name+",") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	var table []string
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			store := stores[name](t)
			_ = store.DeleteNamespace(ctx, "speed")
			t.Cleanup(func() { _ = store.DeleteNamespace(ctx, "speed") })

			start := time.Now()
			for from := 0; from < chunks; from += 500 {
				to := min(from+500, chunks)
				batch := make([]Record, 0, to-from)
				for _, c := range data[from:to] {
					batch = append(batch, c.record)
				}
				if err := store.Upsert(ctx, batch); err != nil {
					t.Fatalf("Upsert: %v", err)
				}
			}
			ingest := time.Since(start)
			time.Sleep(2 * time.Second) // let an index that refreshes in the background catch up

			measure := func(weight float32) (p50, p95 time.Duration, hit float64) {
				var durations []time.Duration
				hits := 0
				for _, p := range probes {
					q := p.query
					q.HybridWeight = weight
					begin := time.Now()
					results, err := store.Search(ctx, q)
					durations = append(durations, time.Since(begin))
					if err != nil {
						t.Fatalf("Search: %v", err)
					}
					for i, r := range results {
						if i < 5 && r.Record.Key == data[p.target].record.Key {
							hits++
							break
						}
					}
				}
				sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
				return durations[len(durations)/2], durations[len(durations)*95/100], float64(hits) / float64(len(probes))
			}
			// the first searches warm caches and connections
			for _, p := range probes[:min(10, len(probes))] {
				_, _ = store.Search(ctx, p.query)
			}
			vp50, vp95, vhit := measure(0)
			hp50, hp95, hhit := measure(0.3)
			row := fmt.Sprintf("| %-10s | %7.1f s | %8.1f | %8.1f | %5.2f | %8.1f | %8.1f | %5.2f |",
				name, ingest.Seconds(), ms(vp50), ms(vp95), vhit, ms(hp50), ms(hp95), hhit)
			table = append(table, row)
			t.Log(row)
		})
	}
	t.Logf("\n%d chunks, %d queries, top 10, weight 0.3 for the hybrid search\n| store      | ingest     | vec p50 ms | vec p95 ms | hit@5 | hyb p50 ms | hyb p95 ms | hit@5 |\n%s",
		chunks, queries, strings.Join(table, "\n"))
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func envInt(name string, fallback int) int {
	if n, err := strconv.Atoi(os.Getenv(name)); err == nil && n > 0 {
		return n
	}
	return fallback
}

// randomUnit returns a unit vector: random, or base plus noise (noise times a random unit vector) when base is given.
func randomUnit(rng *rand.Rand, dim int, base []float32, noise float64) []float32 {
	v := make([]float64, dim)
	var norm float64
	for i := range v {
		v[i] = rng.NormFloat64()
		norm += v[i] * v[i]
	}
	norm = math.Sqrt(norm)
	out := make([]float32, dim)
	var outNorm float64
	for i := range v {
		x := v[i] / norm * noise
		if base != nil {
			x += float64(base[i])
		}
		out[i] = float32(x)
		outNorm += x * x
	}
	outNorm = math.Sqrt(outNorm)
	for i := range out {
		out[i] = float32(float64(out[i]) / outNorm)
	}
	return out
}
