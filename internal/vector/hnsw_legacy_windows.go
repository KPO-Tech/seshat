//go:build windows

package vector

import "errors"

// migrateLegacyHNSW cannot read the graph files of the HNSW store from before it had its own graph: the library that wrote
// them has no Windows support, and no such file was ever written on Windows.
func migrateLegacyHNSW(string, map[string]hnswMeta) (*hnswIndex, error) {
	return nil, errors.New("an hnsw graph file of an older format can only be converted on Linux or macOS; ingest the corpus again")
}
