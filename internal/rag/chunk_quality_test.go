package rag

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/KPO-Tech/seshat/internal/utils"
)

// The documents in testdata/chunking are what the readers write for real files (a Portuguese statute as DOCX, a
// slide deck, a spreadsheet, PDFs with code and with financial tables, a paper, a web page). The chunkers are held to
// the same measures on all of them. Set RAG_CHUNK_CORPUS to a directory of .md files to run the same checks on a
// bigger corpus (the thresholds are the same).

type chunkReport struct {
	doc                           string
	chunks, tiny, over, cutTables int
	badFences, midSentence        int
	missingWords, sourceWords     int
	p50, p90, max                 int
	elapsed                       time.Duration
}

var wordPattern = regexp.MustCompile(`[\p{L}\p{N}]{3,}`)

func measure(doc, source string, chunks []Chunk, maxTokens int, took time.Duration) chunkReport {
	r := chunkReport{doc: doc, chunks: len(chunks), elapsed: took}
	var sizes []int
	var all strings.Builder
	for _, c := range chunks {
		all.WriteString(c.Text)
		all.WriteByte('\n')
		tk := utils.CountTokensInText(c.Text)
		sizes = append(sizes, tk)
		if tk < 40 {
			r.tiny++
		}
		if tk > maxTokens && c.Metadata["chunk_type"] == "" {
			r.over++
		}
		lines := strings.Split(c.Text, "\n")
		hasRows, hasSeparator := false, false
		for _, l := range lines {
			if isTableRow(l) {
				hasRows = true
			}
			if isTableSeparatorRow(l) {
				hasSeparator = true
			}
		}
		if hasRows && !hasSeparator {
			r.cutTables++ // a piece of a table without its header
		}
		if strings.Count(c.Text, "```")%2 == 1 {
			r.badFences++
		}
		last := strings.TrimSpace(lines[len(lines)-1])
		if last != "" && !strings.HasPrefix(last, "|") && !strings.HasPrefix(last, "- ") && !strings.HasPrefix(last, "```") && c.Metadata["chunk_type"] == "" {
			switch []rune(last)[len([]rune(last))-1] {
			case '.', '!', '?', ':', ';', ')', '"', ']', '`', '”', '»', '*':
			default:
				r.midSentence++
			}
		}
	}
	text := all.String()
	seen := map[string]bool{}
	for _, w := range wordPattern.FindAllString(source, -1) {
		if seen[w] {
			continue
		}
		seen[w] = true
		r.sourceWords++
		if !strings.Contains(text, w) {
			r.missingWords++
		}
	}
	sort.Ints(sizes)
	if len(sizes) > 0 {
		r.p50, r.p90, r.max = sizes[len(sizes)/2], sizes[len(sizes)*9/10], sizes[len(sizes)-1]
	}
	return r
}

func chunkCorpus(t *testing.T) []string {
	dir := os.Getenv("RAG_CHUNK_CORPUS")
	if dir == "" {
		dir = filepath.Join("testdata", "chunking")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	if len(files) == 0 {
		t.Fatalf("no documents in %s", dir)
	}
	sort.Strings(files)
	return files
}

func TestChunkQuality(t *testing.T) {
	structured, _ := RecommendedChunkProfile(ChunkProfileStructured)
	chunkers := map[string]Chunker{
		"heading": NewHeadingChunker(structured),
		"table":   NewTableChunker(structured),
	}
	for name, chunker := range chunkers {
		var total chunkReport
		for _, f := range chunkCorpus(t) {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			chunks, err := chunker.Split(t.Context(), string(raw))
			if err != nil {
				t.Fatal(err)
			}
			r := measure(filepath.Base(f), string(raw), chunks, structured.MaxTokens, time.Since(start))
			t.Logf("%-8s %-44s chunks=%-4d tokens p50/p90/max=%d/%d/%d tiny=%d over=%d cutTables=%d badFences=%d midSentence=%d missingWords=%d/%d %v",
				name, r.doc, r.chunks, r.p50, r.p90, r.max, r.tiny, r.over, r.cutTables, r.badFences, r.midSentence, r.missingWords, r.sourceWords, r.elapsed.Round(time.Microsecond))

			if r.chunks == 0 {
				t.Errorf("%s/%s: no chunk at all", name, r.doc)
			}
			if r.missingWords > 0 {
				t.Errorf("%s/%s: %d of %d distinct words of the source are in no chunk", name, r.doc, r.missingWords, r.sourceWords)
			}
			if r.cutTables > 0 {
				t.Errorf("%s/%s: %d chunks hold rows of a table without its header", name, r.doc, r.cutTables)
			}
			if r.badFences > 0 {
				t.Errorf("%s/%s: %d chunks have an unbalanced code fence", name, r.doc, r.badFences)
			}
			if r.over > 0 {
				t.Errorf("%s/%s: %d chunks are over %d tokens", name, r.doc, r.over, structured.MaxTokens)
			}
			total.chunks += r.chunks
			total.tiny += r.tiny
			total.midSentence += r.midSentence
		}
		// Small chunks make poor embeddings: they must be the exception. (Tables kept apart, and the end of a
		// section before a big one, are the only legitimate ones.)
		if limit := total.chunks / 8; total.tiny > limit {
			t.Errorf("%s: %d of %d chunks have under 40 tokens (limit %d)", name, total.tiny, total.chunks, limit)
		}
		t.Logf("%s: %d chunks in all, %d tiny, %d end in the middle of a sentence", name, total.chunks, total.tiny, total.midSentence)
		_ = fmt.Sprint
	}
}
