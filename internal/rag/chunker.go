package rag

import (
	"context"
	"github.com/KPO-Tech/seshat/internal/pagemark"
	"strings"
)

// ParagraphChunker splits on blank lines and hard-caps each paragraph at
// MaxChunkChars characters. It does not call any remote service.
type ParagraphChunker struct {
	MaxChunkChars int
}

// defaultMaxChunkChars was 800 until 2026-08-15 - too small for how far
// embedding models and downstream LLM context windows have moved: it cut
// natural-language paragraphs mid-thought more than necessary, and (found
// via a real end-to-end Google Drive connector test) made any large,
// unusually-structured document balloon into thousands of chunks that then
// dominated search results by sheer volume rather than relevance (see
// seshat-ai/helps/roadmap.md Phase 1). 2000 keeps chunks focused enough for
// precise retrieval while meaningfully reducing both problems.
const defaultMaxChunkChars = 2000

// DefaultChunker returns a ParagraphChunker with sensible defaults.
func DefaultChunker() Chunker {
	return ParagraphChunker{MaxChunkChars: defaultMaxChunkChars}
}

func (c ParagraphChunker) Split(_ context.Context, text string) ([]Chunk, error) {
	text = pagemark.Strip(text)
	maxChars := c.MaxChunkChars
	if maxChars <= 0 {
		maxChars = defaultMaxChunkChars
	}
	parts := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n")
	chunks := make([]Chunk, 0, len(parts))
	position := 0
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// Slice by rune, not byte: a byte-index cut can land inside a
		// multi-byte UTF-8 character (e.g. accented French text) and
		// corrupt it at the chunk boundary.
		runes := []rune(part)
		for len(runes) > maxChars {
			chunks = append(chunks, Chunk{
				Text:     strings.TrimSpace(string(runes[:maxChars])),
				Position: position,
			})
			position++
			runes = []rune(strings.TrimSpace(string(runes[maxChars:])))
		}
		if len(runes) > 0 {
			chunks = append(chunks, Chunk{
				Text:     string(runes),
				Position: position,
			})
			position++
		}
	}
	return chunks, nil
}
