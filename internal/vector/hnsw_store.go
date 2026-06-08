package vector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/coder/hnsw"
)

// HNSWStore is an embedded, persistent vector store backed by HNSW graphs.
//
// Each namespace gets two files in dir:
//   - <slug>.hnsw  — binary HNSW index (vectors + graph topology)
//   - <slug>.meta.json — text and metadata per key
//
// Search is O(log n) via HNSW. No external service or CGO required.
// Designed as the default RAG backend for the Nexus CLI.
type HNSWStore struct {
	dir string
	mu  sync.RWMutex
	ns  map[string]*hnswNamespace
}

type hnswNamespace struct {
	mu       sync.RWMutex
	graph    *hnsw.SavedGraph[string]
	meta     map[string]hnswMeta
	metaPath string
}

// hnswMeta stores everything except the vector itself (the graph holds that).
type hnswMeta struct {
	Text     string            `json:"t"`
	Metadata map[string]string `json:"m,omitempty"`
}

// NewHNSWStore creates an HNSWStore that persists files in dir.
// The directory is created if it does not exist.
func NewHNSWStore(dir string) (*HNSWStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create hnsw store dir: %w", err)
	}
	return &HNSWStore{
		dir: dir,
		ns:  make(map[string]*hnswNamespace),
	}, nil
}

// Close releases in-memory graphs. All data is already persisted to disk.
func (s *HNSWStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ns = make(map[string]*hnswNamespace)
	return nil
}

// namespace returns (or lazily loads) the hnswNamespace for name.
func (s *HNSWStore) namespace(name string) (*hnswNamespace, error) {
	s.mu.RLock()
	n, ok := s.ns[name]
	s.mu.RUnlock()
	if ok {
		return n, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if n, ok = s.ns[name]; ok {
		return n, nil
	}

	slug := sanitizeNamespace(name)
	graphPath := filepath.Join(s.dir, slug+".hnsw")
	metaPath := filepath.Join(s.dir, slug+".meta.json")

	g, err := hnsw.LoadSavedGraph[string](graphPath)
	if err != nil {
		return nil, fmt.Errorf("load hnsw graph %q: %w", name, err)
	}
	// Higher efSearch for better recall on CLI-scale corpora.
	if g.Len() == 0 {
		g.EfSearch = 50
	}

	meta := make(map[string]hnswMeta)
	if data, err := os.ReadFile(metaPath); err == nil {
		if unmarshalErr := json.Unmarshal(data, &meta); unmarshalErr != nil {
			log.Printf("[vector/hnsw] metadata unmarshal warning for namespace %q: %v", name, unmarshalErr)
		}
	}

	n = &hnswNamespace{graph: g, meta: meta, metaPath: metaPath}
	s.ns[name] = n
	return n, nil
}

func (n *hnswNamespace) saveMeta() error {
	data, err := json.Marshal(n.meta)
	if err != nil {
		return err
	}
	tmp := n.metaPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, n.metaPath)
}

// Upsert inserts or replaces records. Saves index and metadata atomically after each namespace batch.
func (s *HNSWStore) Upsert(ctx context.Context, records []Record) error {
	byNS := make(map[string][]Record, 1)
	for _, r := range records {
		if r.Namespace == "" {
			return fmt.Errorf("vector namespace is required")
		}
		if r.Key == "" {
			return fmt.Errorf("vector key is required")
		}
		if len(r.Vector) == 0 {
			return fmt.Errorf("vector values are required for key %q", r.Key)
		}
		byNS[r.Namespace] = append(byNS[r.Namespace], r)
	}

	for nsName, recs := range byNS {
		n, err := s.namespace(nsName)
		if err != nil {
			return err
		}
		n.mu.Lock()
		for _, r := range recs {
			n.graph.Add(hnsw.MakeNode(r.Key, r.Vector))
			n.meta[r.Key] = hnswMeta{Text: r.Text, Metadata: r.Metadata}
		}
		saveErr := n.graph.Save()
		metaErr := n.saveMeta()
		n.mu.Unlock()
		if err := errors.Join(saveErr, metaErr); err != nil {
			return fmt.Errorf("persist hnsw namespace %q: %w", nsName, err)
		}
	}
	return nil
}

// Search performs HNSW ANN search (O(log n)) over the namespace.
// When query.HybridWeight > 0 and query.QueryText is set, keyword scores
// are blended with vector scores using linear interpolation.
func (s *HNSWStore) Search(ctx context.Context, query Query) ([]SearchResult, error) {
	if query.Namespace == "" {
		return nil, fmt.Errorf("vector query namespace is required")
	}
	if len(query.Vector) == 0 {
		return nil, fmt.Errorf("vector query values are required")
	}
	topK := query.TopK
	if topK <= 0 {
		topK = 5
	}

	n, err := s.namespace(query.Namespace)
	if err != nil {
		return nil, err
	}

	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.graph.Len() == 0 {
		return nil, nil
	}

	nodes := n.graph.Search(query.Vector, topK)
	results := make([]SearchResult, 0, len(nodes))
	for _, node := range nodes {
		m := n.meta[node.Key]
		r := Record{
			Namespace: query.Namespace,
			Key:       node.Key,
			Text:      m.Text,
			Vector:    node.Value,
			Metadata:  m.Metadata,
		}
		if len(query.Filter) > 0 && !matchesFilter(r, query.Filter) {
			continue
		}
		// coder/hnsw uses cosine distance (0 = identical, 2 = opposite).
		// Convert to cosine similarity (1 = identical) to match the other backends.
		dist := hnsw.CosineDistance(query.Vector, node.Value)
		results = append(results, SearchResult{Record: r, Score: 1 - dist})
	}

	if query.HybridWeight > 0 && strings.TrimSpace(query.QueryText) != "" && len(results) > 0 {
		results = hnswBlendKeyword(results, query.QueryText, query.HybridWeight)
		// Re-sort after blending: keyword scores change the ranking.
		sort.Slice(results, func(i, j int) bool {
			return results[i].Score > results[j].Score
		})
	}

	return results, nil
}

// Get retrieves records by key (without their vectors — use Search for ANN retrieval).
// If keys is nil or empty, all records in the namespace are returned.
func (s *HNSWStore) Get(ctx context.Context, namespace string, keys []string) ([]Record, error) {
	n, err := s.namespace(namespace)
	if err != nil {
		return nil, err
	}

	n.mu.RLock()
	defer n.mu.RUnlock()

	if len(keys) == 0 {
		records := make([]Record, 0, len(n.meta))
		for key, m := range n.meta {
			records = append(records, Record{
				Namespace: namespace,
				Key:       key,
				Text:      m.Text,
				Metadata:  m.Metadata,
			})
		}
		return records, nil
	}

	records := make([]Record, 0, len(keys))
	for _, key := range keys {
		if m, ok := n.meta[key]; ok {
			records = append(records, Record{
				Namespace: namespace,
				Key:       key,
				Text:      m.Text,
				Metadata:  m.Metadata,
			})
		}
	}
	return records, nil
}

// HasNamespace reports whether the namespace contains at least one record.
func (s *HNSWStore) HasNamespace(ctx context.Context, namespace string) (bool, error) {
	n, err := s.namespace(namespace)
	if err != nil {
		return false, err
	}
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.graph.Len() > 0, nil
}

// DeleteNamespace removes all records for a namespace and its index files.
func (s *HNSWStore) DeleteNamespace(ctx context.Context, namespace string) error {
	s.mu.Lock()
	delete(s.ns, namespace)
	s.mu.Unlock()

	slug := sanitizeNamespace(namespace)
	_ = os.Remove(filepath.Join(s.dir, slug+".hnsw"))
	_ = os.Remove(filepath.Join(s.dir, slug+".meta.json"))
	return nil
}

// DeleteKeys removes specific records within a namespace.
func (s *HNSWStore) DeleteKeys(ctx context.Context, namespace string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	n, err := s.namespace(namespace)
	if err != nil {
		return err
	}

	n.mu.Lock()
	for _, key := range keys {
		n.graph.Delete(key)
		delete(n.meta, key)
	}
	saveErr := n.graph.Save()
	metaErr := n.saveMeta()
	n.mu.Unlock()

	if err := errors.Join(saveErr, metaErr); err != nil {
		return fmt.Errorf("persist hnsw namespace %q after delete: %w", namespace, err)
	}
	return nil
}

// hnswBlendKeyword blends HNSW cosine scores with a simple keyword presence score.
// This replaces FTS5 BM25 (unavailable in the standalone HNSW backend).
// Score per result = (1-hw)*vector_score + hw*keyword_score
// where keyword_score = fraction of query tokens found in the result text.
func hnswBlendKeyword(results []SearchResult, queryText string, hw float32) []SearchResult {
	tokens := strings.Fields(strings.ToLower(queryText))
	if len(tokens) == 0 {
		return results
	}
	for i := range results {
		text := strings.ToLower(results[i].Record.Text)
		var hits float32
		for _, tok := range tokens {
			if strings.Contains(text, tok) {
				hits++
			}
		}
		kwScore := hits / float32(len(tokens))
		results[i].Score = (1-hw)*results[i].Score + hw*kwScore
	}
	return results
}

// sanitizeNamespace converts a namespace string into a safe filename component.
func sanitizeNamespace(ns string) string {
	var sb strings.Builder
	for _, r := range ns {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-':
			sb.WriteRune(r)
		default:
			sb.WriteByte('_')
		}
	}
	s := sb.String()
	if len(s) > 200 {
		s = s[:200]
	}
	if s == "" {
		s = "default"
	}
	return s
}

// Ensure HNSWStore implements Store at compile time.
var _ Store = (*HNSWStore)(nil)
