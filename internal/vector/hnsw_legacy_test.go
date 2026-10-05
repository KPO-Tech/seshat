//go:build !windows

package vector

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/coder/hnsw"
)

// A store directory written by the HNSW store of before 1.2.73 (graph of github.com/coder/hnsw) opens, answers, and is
// converted: the old file is kept beside the new one.
func TestHNSWStoreConvertsALegacyGraph(t *testing.T) {
	dir := t.TempDir()
	slug := sanitizeNamespace("docs")
	graphPath := filepath.Join(dir, slug+".hnsw")

	legacy, err := hnsw.LoadSavedGraph[string](graphPath)
	if err != nil {
		t.Fatal(err)
	}
	meta := map[string]hnswMeta{}
	for i, v := range [][]float32{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}} {
		key := string(rune('a' + i))
		legacy.Add(hnsw.MakeNode(key, v))
		meta[key] = hnswMeta{Text: "text " + key, Metadata: map[string]string{"n": key}}
	}
	meta["vectorless"] = hnswMeta{Text: "no vector"} // ingested with no embedder: in the metadata, not in the graph
	if err := legacy.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(dir, slug+".meta.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := NewHNSWStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	results, err := store.Search(context.Background(), Query{Namespace: "docs", Vector: []float32{0, 1, 0}, TopK: 1})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].Record.Key != "b" || results[0].Record.Text != "text b" {
		t.Fatalf("results = %+v", results)
	}
	if _, err := os.Stat(graphPath + ".legacy"); err != nil {
		t.Fatalf("the old graph file should be kept: %v", err)
	}
	// the converted file is of the current format and opens without conversion
	if _, err := loadHNSWIndex(graphPath); err != nil {
		t.Fatalf("converted file: %v", err)
	}
}
