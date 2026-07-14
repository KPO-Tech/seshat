//go:build windows

package vector

import "fmt"

// NewHNSWStore is a build-time stub on Windows — see hnsw_store.go's build
// constraint doc comment: the real implementation depends on
// github.com/coder/hnsw, which in turn depends on github.com/google/renameio
// for atomic file writes, and renameio has no Windows support.
func NewHNSWStore(dir string) (Store, error) {
	return nil, fmt.Errorf("the hnsw vector backend is not available on Windows (github.com/coder/hnsw's atomic-write dependency doesn't support this OS) — use vector.BackendSQLite or vector.BackendMemory instead")
}
