package vector

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
)

type MemoryStore struct {
	mu      sync.RWMutex
	records map[string]map[string]Record
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		records: make(map[string]map[string]Record),
	}
}

func (s *MemoryStore) Upsert(_ context.Context, records []Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range records {
		if record.Namespace == "" {
			return fmt.Errorf("vector namespace is required")
		}
		if record.Key == "" {
			return fmt.Errorf("vector key is required")
		}
		if len(record.Vector) == 0 {
			return fmt.Errorf("vector values are required")
		}
		if _, ok := s.records[record.Namespace]; !ok {
			s.records[record.Namespace] = make(map[string]Record)
		}
		s.records[record.Namespace][record.Key] = cloneRecord(record)
	}
	return nil
}

func (s *MemoryStore) Search(_ context.Context, query Query) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
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
	namespaceRecords := s.records[query.Namespace]
	results := make([]SearchResult, 0, len(namespaceRecords))
	for _, record := range namespaceRecords {
		if len(query.Filter) > 0 && !matchesFilter(record, query.Filter) {
			continue
		}
		score := cosineSimilarity(query.Vector, record.Vector)
		results = append(results, SearchResult{
			Record: cloneRecord(record),
			Score:  score,
		})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > topK {
		results = results[:topK]
	}
	return results, nil
}

func (s *MemoryStore) Get(_ context.Context, namespace string, keys []string) ([]Record, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ns := s.records[namespace]
	if len(ns) == 0 {
		return nil, nil
	}
	if len(keys) == 0 {
		all := make([]Record, 0, len(ns))
		for _, r := range ns {
			all = append(all, cloneRecord(r))
		}
		return all, nil
	}
	results := make([]Record, 0, len(keys))
	for _, k := range keys {
		if r, ok := ns[k]; ok {
			results = append(results, cloneRecord(r))
		}
	}
	return results, nil
}

func (s *MemoryStore) HasNamespace(_ context.Context, namespace string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.records[namespace]) > 0, nil
}

func (s *MemoryStore) DeleteNamespace(_ context.Context, namespace string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, namespace)
	return nil
}

func (s *MemoryStore) DeleteKeys(_ context.Context, namespace string, keys []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, key := range keys {
		delete(s.records[namespace], key)
	}
	if len(s.records[namespace]) == 0 {
		delete(s.records, namespace)
	}
	return nil
}

func cloneRecord(record Record) Record {
	cloned := record
	cloned.Vector = append([]float32(nil), record.Vector...)
	if record.Metadata != nil {
		cloned.Metadata = make(map[string]string, len(record.Metadata))
		for key, value := range record.Metadata {
			cloned.Metadata[key] = value
		}
	}
	return cloned
}

func cosineSimilarity(a, b []float32) float32 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		av := float64(a[i])
		bv := float64(b[i])
		dot += av * bv
		normA += av * av
		normB += bv * bv
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}
