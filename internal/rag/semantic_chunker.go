package rag

import (
	"context"
	"math"
	"strings"
	"unicode"
)

// SemanticChunker groups sentences into chunks based on cosine similarity
// between consecutive sentence embeddings. A new chunk is started whenever
// similarity falls below Threshold. Inspired by the Max-Min algorithm
// (Springer 2025) as implemented in the mcp-local-rag reference.
//
// Falls back to ParagraphChunker behaviour when the embedder returns an error,
// so ingest never fails due to a temporary embedding outage.
type SemanticChunker struct {
	embedder Embedder
	// Threshold below which two consecutive sentences are considered
	// semantically distinct and a new chunk is started. Default: 0.3.
	Threshold float32
	// MaxChunkChars is the hard character cap per chunk. Default: 2000.
	MaxChunkChars int
	// MinChunkChars is the minimum size a chunk must reach before it can be
	// split off. Chunks below this are merged with the next. Default: 100.
	MinChunkChars int
}

// NewSemanticChunker creates a SemanticChunker with sensible defaults.
// threshold ≤ 0 → use default 0.3.
func NewSemanticChunker(embedder Embedder, threshold float32) *SemanticChunker {
	if threshold <= 0 {
		threshold = 0.3
	}
	return &SemanticChunker{
		embedder:      embedder,
		Threshold:     threshold,
		MaxChunkChars: 2000,
		MinChunkChars: 100,
	}
}

// Split embeds each sentence and groups them into chunks by similarity.
// On embedder error it falls back to ParagraphChunker.
func (c *SemanticChunker) Split(ctx context.Context, text string) ([]Chunk, error) {
	sentences := splitSentences(text)
	if len(sentences) == 0 {
		return nil, nil
	}
	// Single sentence → one chunk.
	if len(sentences) == 1 {
		return []Chunk{{Text: sentences[0], Position: 0}}, nil
	}

	embeddings, err := c.embedder.EmbedTexts(ctx, sentences)
	if err != nil || len(embeddings) != len(sentences) {
		// Fall back to paragraph splitting so ingest still succeeds.
		return ParagraphChunker{MaxChunkChars: c.MaxChunkChars}.Split(ctx, text)
	}

	return c.group(sentences, embeddings), nil
}

// group assembles sentences into chunks based on similarity thresholds.
func (c *SemanticChunker) group(sentences []string, embeddings [][]float32) []Chunk {
	maxChars := c.MaxChunkChars
	if maxChars <= 0 {
		maxChars = 2000
	}
	minChars := c.MinChunkChars
	if minChars <= 0 {
		minChars = 100
	}

	var chunks []Chunk
	position := 0
	var buf []string // current chunk's sentences

	flush := func() {
		if len(buf) == 0 {
			return
		}
		text := strings.Join(buf, " ")
		chunks = append(chunks, Chunk{Text: text, Position: position})
		position++
		buf = buf[:0]
	}

	buf = append(buf, sentences[0])
	for i := 1; i < len(sentences); i++ {
		sim := cosine32(embeddings[i-1], embeddings[i])
		curLen := charsInBuf(buf)
		nextLen := len(sentences[i])

		// Start a new chunk when semantically distinct OR hard cap exceeded.
		if (sim < c.Threshold && curLen >= minChars) || curLen+nextLen > maxChars {
			flush()
		}
		buf = append(buf, sentences[i])
	}
	flush()

	// Hard-split any remaining oversized chunks (rare, happens when a single
	// sentence exceeds maxChunkChars).
	return hardSplitOversized(chunks, maxChars, position)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// splitSentences heuristically splits text into sentences.
// Splits on ". ", "! ", "? ", ".\n", "!\n", "?\n", and "\n\n".
func splitSentences(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	var sentences []string
	var current strings.Builder

	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		current.WriteRune(r)

		switch r {
		case '.', '!', '?':
			// Sentence-ending punctuation followed by whitespace or end.
			if i+1 < len(runes) && (runes[i+1] == ' ' || runes[i+1] == '\n') {
				s := strings.TrimSpace(current.String())
				if s != "" {
					sentences = append(sentences, s)
				}
				current.Reset()
				// Skip the trailing space.
				if i+1 < len(runes) && runes[i+1] == ' ' {
					i++
				}
			}
		case '\n':
			// Double newline = paragraph break → always split.
			if i+1 < len(runes) && runes[i+1] == '\n' {
				s := strings.TrimSpace(current.String())
				if s != "" {
					sentences = append(sentences, s)
				}
				current.Reset()
				i++ // skip second newline
			}
		}
	}
	if s := strings.TrimSpace(current.String()); s != "" {
		sentences = append(sentences, s)
	}

	// Filter short/empty after trim.
	result := sentences[:0]
	for _, s := range sentences {
		clean := strings.Map(func(r rune) rune {
			if unicode.IsPunct(r) || unicode.IsSpace(r) {
				return -1
			}
			return r
		}, s)
		if len(clean) >= 3 {
			result = append(result, s)
		}
	}
	return result
}

func charsInBuf(buf []string) int {
	n := 0
	for _, s := range buf {
		n += len(s)
	}
	return n + len(buf) - 1 // spaces between sentences
}

// hardSplitOversized breaks any chunk that exceeds maxChars at word boundaries.
func hardSplitOversized(chunks []Chunk, maxChars int, nextPos int) []Chunk {
	out := make([]Chunk, 0, len(chunks))
	for _, c := range chunks {
		if len(c.Text) <= maxChars {
			out = append(out, c)
			continue
		}
		// Split at word boundaries.
		words := strings.Fields(c.Text)
		var part strings.Builder
		for _, w := range words {
			if part.Len()+len(w)+1 > maxChars && part.Len() > 0 {
				out = append(out, Chunk{Text: strings.TrimSpace(part.String()), Position: nextPos})
				nextPos++
				part.Reset()
			}
			if part.Len() > 0 {
				part.WriteByte(' ')
			}
			part.WriteString(w)
		}
		if part.Len() > 0 {
			out = append(out, Chunk{Text: strings.TrimSpace(part.String()), Position: nextPos})
			nextPos++
		}
	}
	return out
}

// cosine32 computes cosine similarity between two float32 vectors.
func cosine32(a, b []float32) float32 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		av, bv := float64(a[i]), float64(b[i])
		dot += av * bv
		normA += av * av
		normB += bv * bv
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return float32(dot / (math.Sqrt(normA) * math.Sqrt(normB)))
}
