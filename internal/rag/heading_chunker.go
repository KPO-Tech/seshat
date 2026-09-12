package rag

import (
	"context"
	"regexp"
	"strings"

	"github.com/KPO-Tech/seshat/internal/utils"
)

// HeadingChunker splits a plain-text document along its own heading
// hierarchy (Markdown headings, numbered outline notation, or EN/FR
// legal-structural keywords like "Article"/"Chapitre"), instead of a flat
// paragraph/character split - each chunk carries the full ancestor-heading
// path ("Part One > Article 5 > Clause 5.2") both in its text (so
// embedding/BM25 relevance benefits from the section context) and in
// Metadata["heading_path"] (for citation/UI display without re-parsing the
// text).
//
// Inspired by RAGFlow's bullets_category/tree_merge
// (rag/nlp/__init__.py:370-387,1120-1164), not a port of it: that
// implementation is built around Chinese legal numbering and PDF
// layout-position metadata from a separate upstream layout-recognition
// step that seshat's chunkers don't have. Here, every candidate pattern
// already carries an inherent semantic level (a Markdown "###" is always
// deeper than "##"; "Article" always nests under "Chapter"), so there's no
// need for RAGFlow's "vote across candidate families, then pick one" step.
//
// A document with no recognizable heading at all delegates entirely to
// ParagraphChunker (via DefaultChunker) - HeadingChunker is additive, never
// worse than today's baseline for an unstructured document.
type HeadingChunker struct {
	Profile ChunkProfile
}

// NewHeadingChunker creates a heading-aware chunker using one of Seshat's
// recommended chunk profiles (same defaulting guard as
// NewDoclingChunkerForProfile).
func NewHeadingChunker(profile ChunkProfile) *HeadingChunker {
	if profile.MaxTokens <= 0 {
		profile = DefaultChunkProfile()
	}
	return &HeadingChunker{Profile: profile}
}

// headingPattern matches a heading line and reports its nesting level.
type headingPattern struct {
	re    *regexp.Regexp
	level func(match []string) int
}

// maxHeadingLineRunes guards against matching an ordinary sentence that
// merely starts with a heading-shaped prefix ("Section 8 of the agreement
// states that...") - a real heading is short; RAGFlow's own not_title/
// not_bullet heuristics apply a similar length guard for the same reason.
const maxHeadingLineRunes = 200

var headingPatterns = []headingPattern{
	// Markdown ATX headings - level = number of '#'.
	{
		re:    regexp.MustCompile(`^(#{1,6})\s+\S`),
		level: func(m []string) int { return len(m[1]) },
	},
	// Numbered outline ("1.", "1.2", "1.2.3", ...) - level = number of
	// dot-separated numeric segments.
	{
		re:    regexp.MustCompile(`^((?:\d+\.)+\d*)\s+\S`),
		level: func(m []string) int { return strings.Count(strings.TrimSuffix(m[1], "."), ".") + 1 },
	},
	// EN/FR legal-structural keywords - fixed canonical levels so a
	// document mixing schemes ("Part One" ... later "Article 5") still
	// nests sensibly instead of every keyword competing at the same depth.
	{re: regexp.MustCompile(`(?i)^(part|partie)\s+\S`), level: func([]string) int { return 1 }},
	{re: regexp.MustCompile(`(?i)^(chapter|chapitre)\s+\S`), level: func([]string) int { return 2 }},
	{re: regexp.MustCompile(`(?i)^section\s+\S`), level: func([]string) int { return 3 }},
	{re: regexp.MustCompile(`(?i)^article\s+\S`), level: func([]string) int { return 4 }},
	{re: regexp.MustCompile(`(?i)^(clause|annex|annexe)\s+\S`), level: func([]string) int { return 5 }},
}

// matchHeading returns (level, ok) for a single line, already trimmed.
func matchHeading(line string) (int, bool) {
	if line == "" || len([]rune(line)) > maxHeadingLineRunes {
		return 0, false
	}
	for _, p := range headingPatterns {
		if m := p.re.FindStringSubmatch(line); m != nil {
			return p.level(m), true
		}
	}
	return 0, false
}

type headingStackEntry struct {
	level int
	title string
}

func (c *HeadingChunker) Split(ctx context.Context, text string) ([]Chunk, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")

	hasHeading := false
	for _, line := range lines {
		if _, ok := matchHeading(strings.TrimSpace(line)); ok {
			hasHeading = true
			break
		}
	}
	if !hasHeading {
		return DefaultChunker().Split(ctx, text)
	}

	maxTokens := c.Profile.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultChunkProfile().MaxTokens
	}

	var (
		chunks   []Chunk
		position int
		stack    []headingStackEntry
		body     strings.Builder
	)

	ancestorPath := func() string {
		titles := make([]string, len(stack))
		for i, e := range stack {
			titles[i] = e.title
		}
		return strings.Join(titles, " > ")
	}

	flush := func() {
		sectionBody := strings.TrimSpace(body.String())
		body.Reset()
		if sectionBody == "" {
			return
		}
		path := ancestorPath()
		for _, piece := range splitBodyByTokenBudget(sectionBody, maxTokens) {
			chunkText := piece
			if path != "" {
				chunkText = path + "\n\n" + piece
			}
			meta := map[string]string{}
			if path != "" {
				meta["heading_path"] = path
			}
			chunks = append(chunks, Chunk{Text: chunkText, Position: position, Metadata: meta})
			position++
		}
	}

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		level, ok := matchHeading(line)
		if !ok {
			if line != "" {
				body.WriteString(line)
				body.WriteString("\n")
			}
			continue
		}
		// A new heading closes every open section at the same depth or
		// deeper - flush whatever body accumulated under the section this
		// heading interrupts, using that section's own ancestor path
		// (computed from the stack as it stood *before* this heading is
		// pushed).
		flush()
		for len(stack) > 0 && stack[len(stack)-1].level >= level {
			stack = stack[:len(stack)-1]
		}
		stack = append(stack, headingStackEntry{level: level, title: line})
	}
	flush()

	return chunks, nil
}

// SplitDocument satisfies DocumentChunker by ignoring the original bytes -
// HeadingChunker works on already-extracted text/patterns, not layout, the
// same as ParagraphChunker/SemanticChunker.
func (c *HeadingChunker) SplitDocument(ctx context.Context, doc Document) ([]Chunk, error) {
	return c.Split(ctx, doc.Text)
}

// splitBodyByTokenBudget hard-caps a single section's body at maxTokens
// (estimated via internal/utils, ~4 chars/token), the same rune-safe
// slicing ParagraphChunker uses - except every piece is later re-prefixed
// with the same ancestor path by flush() above, so a section too long for
// one chunk doesn't lose its heading context after the first piece.
func splitBodyByTokenBudget(text string, maxTokens int) []string {
	maxChars := maxTokens * 4
	if maxChars <= 0 || utils.CountTokensInText(text) <= maxTokens {
		return []string{text}
	}
	runes := []rune(text)
	pieces := make([]string, 0, len(runes)/maxChars+1)
	for len(runes) > maxChars {
		pieces = append(pieces, strings.TrimSpace(string(runes[:maxChars])))
		runes = []rune(strings.TrimSpace(string(runes[maxChars:])))
	}
	if len(runes) > 0 {
		pieces = append(pieces, string(runes))
	}
	return pieces
}
