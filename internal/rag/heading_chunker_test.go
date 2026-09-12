package rag

import (
	"context"
	"strings"
	"testing"
)

func TestHeadingChunker_MarkdownHeadings(t *testing.T) {
	c := NewHeadingChunker(ChunkProfile{Name: ChunkProfileStructured, MaxTokens: 1024})
	text := "# Employee Handbook\n\nIntro paragraph.\n\n## Leave Policy\n\nEmployees get 25 days per year.\n\n### Sick Leave\n\nSick leave is unlimited with a doctor's note.\n"

	chunks, err := c.Split(context.Background(), text)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks (Employee Handbook intro, Leave Policy, Sick Leave), got %d: %+v", len(chunks), chunks)
	}
	// The document opens directly with an H1, so there's no true preamble
	// before any heading - "Intro paragraph" is the H1's own body.
	if chunks[0].Metadata["heading_path"] != "# Employee Handbook" {
		t.Errorf("chunk 0 heading_path = %q, want %q", chunks[0].Metadata["heading_path"], "# Employee Handbook")
	}
	if !strings.Contains(chunks[0].Text, "Intro paragraph") {
		t.Errorf("expected chunk 0 to contain the H1's own body text, got %q", chunks[0].Text)
	}
	want1 := "# Employee Handbook > ## Leave Policy"
	if chunks[1].Metadata["heading_path"] != want1 {
		t.Errorf("chunk 1 heading_path = %q, want %q", chunks[1].Metadata["heading_path"], want1)
	}
	if !strings.Contains(chunks[1].Text, "25 days") {
		t.Errorf("expected chunk 1 to contain the leave policy body, got %q", chunks[1].Text)
	}
	want2 := "# Employee Handbook > ## Leave Policy > ### Sick Leave"
	if chunks[2].Metadata["heading_path"] != want2 {
		t.Errorf("chunk 2 heading_path = %q, want %q", chunks[2].Metadata["heading_path"], want2)
	}
	if !strings.Contains(chunks[2].Text, "doctor's note") {
		t.Errorf("expected chunk 2 to contain the sick leave body, got %q", chunks[2].Text)
	}
}

func TestHeadingChunker_PreambleBeforeFirstHeadingHasNoPath(t *testing.T) {
	c := NewHeadingChunker(DefaultChunkProfile())
	text := "Confidential - internal use only.\n\n# Employee Handbook\n\nIntro paragraph.\n"

	chunks, err := c.Split(context.Background(), text)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks (preamble, Employee Handbook), got %d: %+v", len(chunks), chunks)
	}
	if chunks[0].Metadata["heading_path"] != "" {
		t.Errorf("expected the true preamble chunk to have no heading_path, got %q", chunks[0].Metadata["heading_path"])
	}
	if !strings.Contains(chunks[0].Text, "Confidential") {
		t.Errorf("expected preamble text, got %q", chunks[0].Text)
	}
	if chunks[1].Metadata["heading_path"] != "# Employee Handbook" {
		t.Errorf("chunk 1 heading_path = %q", chunks[1].Metadata["heading_path"])
	}
}

func TestHeadingChunker_NumberedOutline(t *testing.T) {
	c := NewHeadingChunker(DefaultChunkProfile())
	text := "1. Scope\n\nThis policy applies to all staff.\n\n1.1 Exceptions\n\nContractors are excluded.\n\n1.2 Review\n\nReviewed annually.\n"

	chunks, err := c.Split(context.Background(), text)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d: %+v", len(chunks), chunks)
	}
	if chunks[0].Metadata["heading_path"] != "1. Scope" {
		t.Errorf("chunk 0 heading_path = %q", chunks[0].Metadata["heading_path"])
	}
	if chunks[1].Metadata["heading_path"] != "1. Scope > 1.1 Exceptions" {
		t.Errorf("chunk 1 heading_path = %q", chunks[1].Metadata["heading_path"])
	}
	// 1.2 is a sibling of 1.1 (same level), so it must replace it under 1. Scope, not nest under it.
	if chunks[2].Metadata["heading_path"] != "1. Scope > 1.2 Review" {
		t.Errorf("chunk 2 heading_path = %q, want sibling nesting under '1. Scope' only", chunks[2].Metadata["heading_path"])
	}
}

func TestHeadingChunker_LegalKeywords(t *testing.T) {
	c := NewHeadingChunker(DefaultChunkProfile())
	text := "Part One General Provisions\n\nGeneral intro text.\n\nArticle 1 Definitions\n\nTerms used in this agreement are defined below.\n\nArticle 2 Termination\n\nEither party may terminate with 30 days notice.\n"

	chunks, err := c.Split(context.Background(), text)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d: %+v", len(chunks), chunks)
	}
	// Article (level 4) nests under Part (level 1) even though no
	// intermediate Chapter/Section exists in this document.
	if chunks[1].Metadata["heading_path"] != "Part One General Provisions > Article 1 Definitions" {
		t.Errorf("chunk 1 heading_path = %q", chunks[1].Metadata["heading_path"])
	}
	if chunks[2].Metadata["heading_path"] != "Part One General Provisions > Article 2 Termination" {
		t.Errorf("chunk 2 heading_path = %q", chunks[2].Metadata["heading_path"])
	}
	if !strings.Contains(chunks[2].Text, "30 days notice") {
		t.Errorf("expected chunk 2 to contain the termination body, got %q", chunks[2].Text)
	}
}

func TestHeadingChunker_FrenchLegalKeywords(t *testing.T) {
	c := NewHeadingChunker(DefaultChunkProfile())
	text := "Chapitre 1 Dispositions generales\n\nTexte d'introduction.\n\nArticle 1 Definitions\n\nLes termes utilises sont definis ci-dessous.\n"

	chunks, err := c.Split(context.Background(), text)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d: %+v", len(chunks), chunks)
	}
	if chunks[1].Metadata["heading_path"] != "Chapitre 1 Dispositions generales > Article 1 Definitions" {
		t.Errorf("chunk 1 heading_path = %q", chunks[1].Metadata["heading_path"])
	}
}

func TestHeadingChunker_NoHeadingsFallsBackToParagraphChunker(t *testing.T) {
	c := NewHeadingChunker(DefaultChunkProfile())
	text := "Just a plain document.\n\nWith two paragraphs.\n\nAnd a third one for good measure."

	got, err := c.Split(context.Background(), text)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	want, err := DefaultChunker().Split(context.Background(), text)
	if err != nil {
		t.Fatalf("baseline Split: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("expected fallback to match ParagraphChunker exactly: got %d chunks, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Text != want[i].Text || got[i].Position != want[i].Position {
			t.Fatalf("chunk %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestHeadingChunker_OverLongSectionSplitsButKeepsHeadingPath(t *testing.T) {
	c := NewHeadingChunker(ChunkProfile{Name: ChunkProfileStructured, MaxTokens: 20}) // tiny budget to force a split
	longBody := strings.Repeat("word ", 400)
	text := "Article 1 Long Section\n\n" + longBody

	chunks, err := c.Split(context.Background(), text)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected the over-long section to split into multiple chunks, got %d", len(chunks))
	}
	for i, chunk := range chunks {
		if chunk.Metadata["heading_path"] != "Article 1 Long Section" {
			t.Errorf("chunk %d heading_path = %q, expected every split piece to retain the ancestor path", i, chunk.Metadata["heading_path"])
		}
		if !strings.HasPrefix(chunk.Text, "Article 1 Long Section\n\n") {
			t.Errorf("chunk %d text does not start with the heading path prefix: %q", i, chunk.Text[:min(60, len(chunk.Text))])
		}
	}
}

func TestHeadingChunker_ImplementsDocumentChunker(t *testing.T) {
	var _ DocumentChunker = NewHeadingChunker(DefaultChunkProfile())
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
