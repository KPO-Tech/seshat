package rag

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// evaluationChunk is a line of the file the retrieval benchmark of seshat-intelligence reads
// (seshat-intelligence/benchmarks/chunk_bench in SeshatOS).
type evaluationChunk struct {
	Strategy string `json:"strategy"`
	Doc      string `json:"doc"`
	Index    int    `json:"index"`
	Text     string `json:"text"`
}

// TestDumpChunksForEvaluation chunks markdown documents with the strategies named in STRATS (heading512, heading1024,
// paragraph) and writes the chunks, one JSON object per line, for the retrieval benchmark. It does nothing unless
// MD_DIR (a directory with <doc>.md files), DOCS (the names, comma separated) and OUT are set.
func TestDumpChunksForEvaluation(t *testing.T) {
	dir, out, docs, strategies := os.Getenv("MD_DIR"), os.Getenv("OUT"), os.Getenv("DOCS"), os.Getenv("STRATS")
	if dir == "" || out == "" {
		t.Skip()
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, name := range strings.Split(strategies, ",") {
		var c Chunker
		switch name {
		case "heading512":
			p, _ := RecommendedChunkProfile(ChunkProfileStructured)
			c = NewHeadingChunker(p)
		case "heading1024":
			c = NewHeadingChunker(ChunkProfile{Name: ChunkProfileStructured, MaxTokens: 1024, OverlapTokens: 128})
		case "paragraph":
			c = DefaultChunker()
		default:
			t.Fatalf("unknown strategy %s", name)
		}
		for _, doc := range strings.Split(docs, ",") {
			raw, err := os.ReadFile(dir + "/" + doc + ".md")
			if err != nil {
				t.Fatal(err)
			}
			chunks, err := c.Split(t.Context(), string(raw))
			if err != nil {
				t.Fatal(err)
			}
			for i, ch := range chunks {
				_ = enc.Encode(evaluationChunk{Strategy: name, Doc: doc, Index: i, Text: ch.Text})
			}
		}
	}
}
