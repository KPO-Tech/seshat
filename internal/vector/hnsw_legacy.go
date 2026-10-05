//go:build !windows

package vector

import (
	"fmt"
	"sort"

	"github.com/coder/hnsw"
)

// migrateLegacyHNSW reads a graph file written by github.com/coder/hnsw (the format of the HNSW store before the graph was
// written in this package) and builds an index of the current format from the vectors of the keys in meta. The old library
// has no Windows support, so on Windows such a file cannot be converted.
func migrateLegacyHNSW(path string, meta map[string]hnswMeta) (*hnswIndex, error) {
	legacy, err := hnsw.LoadSavedGraph[string](path)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(meta))
	for key := range meta {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	index := newHNSWIndex()
	for _, key := range keys {
		vec, ok := legacy.Lookup(key)
		if !ok {
			continue // a record without a vector (ingested with no embedder)
		}
		if err := index.Add(key, vec); err != nil {
			return nil, fmt.Errorf("record %q: %w", key, err)
		}
	}
	return index, nil
}
