package rag

import (
	"encoding/json"
	"os"
	"strconv"
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

// evaluationChunker builds the chunker a strategy name stands for: "profile" is the structured profile hosts get by
// default, "headingN" is the heading chunker with chunks of N tokens and an overlap of an eighth of that (the ratio of
// the structured profile), "headingNoM" is the same with an overlap of M tokens (heading512o0: none), and "paragraph" is the
// default chunker.
func evaluationChunker(name string) (Chunker, bool) {
	switch {
	case name == "profile":
		profile, ok := RecommendedChunkProfile(ChunkProfileStructured)
		return NewHeadingChunker(profile), ok
	case name == "paragraph":
		return DefaultChunker(), true
	case strings.HasPrefix(name, "heading"):
		sizeText, overlapText, hasOverlap := strings.Cut(strings.TrimPrefix(name, "heading"), "o")
		size, err := strconv.Atoi(sizeText)
		if err != nil {
			return nil, false
		}
		overlap := size / 8
		if hasOverlap {
			if overlap, err = strconv.Atoi(overlapText); err != nil {
				return nil, false
			}
		}
		profile, err := NewCustomChunkProfile(size, overlap)
		if err != nil {
			return nil, false
		}
		return NewHeadingChunker(profile), true
	}
	return nil, false
}

// TestDumpChunksForEvaluation chunks markdown documents with the strategies named in STRATS (comma separated: profile,
// paragraph, heading256, heading512, heading1024...) and writes the chunks, one JSON object per line, for the retrieval
// benchmark. It does nothing unless MD_DIR (a directory with <doc>.md files), DOCS (the names, comma separated), STRATS
// and OUT are set. LABEL, when set, is put before each strategy name, so that the chunks of an older checkout can be
// told from these.
func TestDumpChunksForEvaluation(t *testing.T) {
	dir, out, docs, strategies, label := os.Getenv("MD_DIR"), os.Getenv("OUT"), os.Getenv("DOCS"), os.Getenv("STRATS"), os.Getenv("LABEL")
	if dir == "" || out == "" || docs == "" || strategies == "" {
		t.Skip("set MD_DIR, DOCS, STRATS and OUT to dump chunks for the retrieval benchmark")
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, name := range strings.Split(strategies, ",") {
		chunker, ok := evaluationChunker(name)
		if !ok {
			t.Fatalf("unknown strategy %q", name)
		}
		for _, doc := range strings.Split(docs, ",") {
			raw, err := os.ReadFile(dir + "/" + doc + ".md")
			if err != nil {
				t.Fatal(err)
			}
			chunks, err := chunker.Split(t.Context(), string(raw))
			if err != nil {
				t.Fatalf("%s with %s: %v", doc, name, err)
			}
			for i, ch := range chunks {
				if err := enc.Encode(evaluationChunk{Strategy: label + name, Doc: doc, Index: i, Text: ch.Text}); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
