package rag

import (
	"context"
	"regexp"
	"strings"

	"github.com/KPO-Tech/seshat/internal/utils"
)

// HeadingChunker cuts a markdown document along its own structure: a heading hierarchy (markdown headings, numbered
// outline, or structural words like "Chapter"/"Article"/"Capítulo"/"Artigo"), paragraphs, lists, tables and fenced code.
// Each chunk carries the path of headings it sits under ("Part One > Article 5 > Clause 5.2"), in its text (so
// embedding and BM25 relevance benefit from the section context) and in Metadata["heading_path"] (for citation and
// display without re-parsing the text).
//
// A block is never cut in the middle of what it is (a paragraph is cut between sentences, a table between rows with
// its header repeated, code between lines, a list between items), small sections are joined to their neighbours with
// their headings written in the text so no chunk is a title and one line, and a paragraph that has to be cut
// overlaps with the piece before it by the profile's OverlapTokens. See markdownChunker for the details.
//
// Inspired by RAGFlow's bullets_category/tree_merge (rag/nlp/__init__.py:370-387,1120-1164), not a port of it.
//
// A document with no heading at all is simply packed block by block.
type HeadingChunker struct {
	Profile ChunkProfile
}

// NewHeadingChunker creates a heading-aware chunker using one of Seshat's
// recommended chunk profiles (same defaulting guard as
// NewHybridDocumentChunkerForProfile).
func NewHeadingChunker(profile ChunkProfile) *HeadingChunker {
	if profile.MaxTokens <= 0 {
		profile = DefaultChunkProfile()
	}
	return &HeadingChunker{Profile: profile}
}

type headingStackEntry struct {
	level int
	title string
}

// headingPattern, matchHeading and splitBodyByTokenBudget below are the older line-based heading detection, still used
// by QAChunker. HeadingChunker itself reads blocks (see markdown_chunker.go).
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

func (c *HeadingChunker) Split(_ context.Context, text string) ([]Chunk, error) {
	return newMarkdownChunker(c.Profile, false).split(text), nil
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
