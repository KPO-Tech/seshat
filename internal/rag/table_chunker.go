package rag

import (
	"context"
	"regexp"
	"strings"

	"github.com/KPO-Tech/seshat/internal/utils"
)

// TableChunker splits a Markdown/GFM document while keeping every table's
// rows intact as their own chunk(s), instead of letting a generic
// character/token splitter cut through the middle of a table (which
// destroys the header-to-cell association the table's meaning depends on).
// A table larger than the chunk profile's token budget is split by data
// row, but the header + separator row is repeated at the top of every
// resulting piece, so each chunk remains a valid, self-contained table
// rather than a headerless fragment.
//
// Text outside any detected table is delegated to Fallback (default
// HeadingChunker via NewTableChunker, so section/heading context is
// preserved for the surrounding prose too, not just the tables).
//
// A document with no GFM table at all delegates entirely to Fallback -
// TableChunker is additive, never worse than the fallback's own baseline
// for a document that has nothing for it to do, the same contract
// HeadingChunker already established for its own fallback.
type TableChunker struct {
	Profile  ChunkProfile
	Fallback Chunker
}

// NewTableChunker creates a table-aware chunker using one of Seshat's
// recommended chunk profiles (same defaulting guard as
// NewHeadingChunker/NewHybridDocumentChunkerForProfile). The fallback for
// non-table text is HeadingChunker, so a table-and-headings document gets
// both behaviors at once.
func NewTableChunker(profile ChunkProfile) *TableChunker {
	if profile.MaxTokens <= 0 {
		profile = DefaultChunkProfile()
	}
	return &TableChunker{Profile: profile, Fallback: NewHeadingChunker(profile)}
}

func (c *TableChunker) fallback() Chunker {
	if c != nil && c.Fallback != nil {
		return c.Fallback
	}
	return DefaultChunker()
}

func (c *TableChunker) maxTokens() int {
	if c != nil && c.Profile.MaxTokens > 0 {
		return c.Profile.MaxTokens
	}
	return DefaultChunkProfile().MaxTokens
}

// tableRowPattern matches a GFM table row: pipe-delimited, starting and
// ending with '|' after trimming. Matches both header/data rows and the
// separator row below the header - callers distinguish those with
// isTableSeparatorRow.
var tableRowPattern = regexp.MustCompile(`^\s*\|.*\|\s*$`)

// tableSeparatorPattern matches a GFM header separator row
// ("| --- | :--- | ---: |") - each cell is dashes with optional leading/
// trailing colons for alignment.
var tableSeparatorPattern = regexp.MustCompile(`^\s*\|(\s*:?-+:?\s*\|)+\s*$`)

func isTableRow(line string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed != "|" && tableRowPattern.MatchString(trimmed)
}

func isTableSeparatorRow(line string) bool {
	return tableSeparatorPattern.MatchString(strings.TrimSpace(line))
}

// tableBlock is one contiguous span of the document: either a detected GFM
// table (isTable) or ordinary surrounding text.
type tableBlock struct {
	isTable bool
	lines   []string
}

// splitIntoBlocks partitions text into alternating table/non-table spans. A
// table block starts at a row immediately followed by a valid separator
// row, and extends through every further contiguous pipe-delimited row.
func splitIntoBlocks(lines []string) []tableBlock {
	var blocks []tableBlock
	i := 0
	for i < len(lines) {
		if isTableRow(lines[i]) && i+1 < len(lines) && isTableSeparatorRow(lines[i+1]) {
			start := i
			i += 2
			for i < len(lines) && isTableRow(lines[i]) {
				i++
			}
			blocks = append(blocks, tableBlock{isTable: true, lines: lines[start:i]})
			continue
		}
		start := i
		for i < len(lines) && !(isTableRow(lines[i]) && i+1 < len(lines) && isTableSeparatorRow(lines[i+1])) {
			i++
		}
		blocks = append(blocks, tableBlock{isTable: false, lines: lines[start:i]})
	}
	return blocks
}

func (c *TableChunker) Split(ctx context.Context, text string) ([]Chunk, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	blocks := splitIntoBlocks(lines)

	hasTable := false
	for _, b := range blocks {
		if b.isTable {
			hasTable = true
			break
		}
	}
	if !hasTable {
		return c.fallback().Split(ctx, text)
	}

	var chunks []Chunk
	position := 0
	for _, b := range blocks {
		if !b.isTable {
			span := strings.TrimSpace(strings.Join(b.lines, "\n"))
			if span == "" {
				continue
			}
			sub, err := c.fallback().Split(ctx, span)
			if err != nil {
				return nil, err
			}
			for _, ch := range sub {
				ch.Position = position
				chunks = append(chunks, ch)
				position++
			}
			continue
		}
		for _, piece := range splitTableByRowBudget(b.lines, c.maxTokens()) {
			chunks = append(chunks, Chunk{
				Text:     piece,
				Position: position,
				Metadata: map[string]string{"chunk_type": "table"},
			})
			position++
		}
	}
	return chunks, nil
}

// SplitDocument satisfies DocumentChunker by ignoring the original bytes -
// TableChunker works on already-extracted Markdown text, the same as
// HeadingChunker/ParagraphChunker/SemanticChunker.
func (c *TableChunker) SplitDocument(ctx context.Context, doc Document) ([]Chunk, error) {
	return c.Split(ctx, doc.Text)
}

// splitTableByRowBudget returns tableLines (header row, separator row, then
// data rows) as one piece if it already fits within maxTokens, or as
// multiple pieces otherwise - each piece repeating the header + separator
// row so every resulting chunk is still a valid, self-contained table. A
// single data row that alone exceeds maxTokens (an unusually large cell) is
// not split further - cutting mid-cell would corrupt it, the same
// best-effort tradeoff splitBodyByTokenBudget already makes for prose.
func splitTableByRowBudget(tableLines []string, maxTokens int) []string {
	if len(tableLines) < 2 {
		return []string{strings.Join(tableLines, "\n")}
	}
	headerText := strings.Join(tableLines[:2], "\n")
	dataRows := tableLines[2:]

	full := strings.Join(tableLines, "\n")
	if len(dataRows) == 0 || utils.CountTokensInText(full) <= maxTokens {
		return []string{full}
	}

	var pieces []string
	var group []string
	flush := func() {
		if len(group) == 0 {
			return
		}
		pieces = append(pieces, headerText+"\n"+strings.Join(group, "\n"))
		group = nil
	}
	for _, row := range dataRows {
		candidateText := headerText + "\n" + strings.Join(append(append([]string{}, group...), row), "\n")
		if len(group) > 0 && utils.CountTokensInText(candidateText) > maxTokens {
			flush()
		}
		group = append(group, row)
	}
	flush()
	if len(pieces) == 0 {
		return []string{full}
	}
	return pieces
}

var _ DocumentChunker = (*TableChunker)(nil)
