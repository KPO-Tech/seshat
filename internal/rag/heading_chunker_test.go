package rag

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat/internal/utils"
)

// words returns n distinct words, so a test can tell what ended up where.
func words(prefix string, n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf("%s%d", prefix, i)
	}
	return strings.Join(parts, " ")
}

func split(t *testing.T, c Chunker, text string) []Chunk {
	t.Helper()
	chunks, err := c.Split(context.Background(), text)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	return chunks
}

// profile makes sections of about 30 tokens stand alone and bigger ones split: max 100 tokens, min 25.
var small = ChunkProfile{Name: ChunkProfileStructured, MaxTokens: 100}

func TestHeadingChunker_SectionsBigEnoughToStandAloneStaySeparateWithTheirPath(t *testing.T) {
	c := NewHeadingChunker(small)
	text := "# Employee Handbook\n\n" + words("intro", 22) + "\n\n## Leave Policy\n\n" + words("leave", 22) + "\n\n### Sick Leave\n\n" + words("sick", 22) + "\n"

	chunks := split(t, c, text)
	if len(chunks) != 3 {
		t.Fatalf("want 3 chunks, got %d: %+v", len(chunks), chunks)
	}
	for i, want := range []string{"Employee Handbook", "Employee Handbook > Leave Policy", "Employee Handbook > Leave Policy > Sick Leave"} {
		if got := chunks[i].Metadata["heading_path"]; got != want {
			t.Errorf("chunk %d heading_path = %q, want %q (the path is clean, with no # marks)", i, got, want)
		}
		if !strings.HasPrefix(chunks[i].Text, want+"\n\n") {
			t.Errorf("chunk %d does not start with its path: %q", i, chunks[i].Text[:min(50, len(chunks[i].Text))])
		}
	}
	if !strings.Contains(chunks[2].Text, "sick21") {
		t.Errorf("the body of the last section is missing: %q", chunks[2].Text)
	}
}

func TestHeadingChunker_TinySectionsAreJoinedWithTheirHeadingsInTheText(t *testing.T) {
	c := NewHeadingChunker(DefaultChunkProfile())
	text := "# Employee Handbook\n\nIntro paragraph.\n\n## Leave Policy\n\nEmployees get 25 days per year.\n\n### Sick Leave\n\nSick leave is unlimited with a doctor's note.\n"

	chunks := split(t, c, text)
	if len(chunks) != 1 {
		t.Fatalf("three tiny sections must make one chunk, got %d: %+v", len(chunks), chunks)
	}
	got := chunks[0].Text
	for _, want := range []string{"Employee Handbook\n\nIntro paragraph.", "## Leave Policy", "Employees get 25 days", "### Sick Leave", "doctor's note"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Count(got, "Employee Handbook") != 1 {
		t.Errorf("the top heading is in the path already and must not be repeated:\n%s", got)
	}
	if chunks[0].Metadata["heading_path"] != "Employee Handbook" {
		t.Errorf("heading_path = %q, want the heading the sections share", chunks[0].Metadata["heading_path"])
	}
}

func TestHeadingChunker_PreambleBeforeFirstHeadingHasNoPath(t *testing.T) {
	c := NewHeadingChunker(small)
	text := words("preamble", 22) + "\n\n# Employee Handbook\n\n" + words("body", 22) + "\n"

	chunks := split(t, c, text)
	if len(chunks) != 2 {
		t.Fatalf("want 2 chunks (preamble, Employee Handbook), got %d: %+v", len(chunks), chunks)
	}
	if chunks[0].Metadata["heading_path"] != "" || !strings.Contains(chunks[0].Text, "preamble0") {
		t.Errorf("the preamble has no path: %+v", chunks[0])
	}
	if chunks[1].Metadata["heading_path"] != "Employee Handbook" {
		t.Errorf("chunk 1 heading_path = %q", chunks[1].Metadata["heading_path"])
	}
}

func TestHeadingChunker_NumberedOutline(t *testing.T) {
	c := NewHeadingChunker(small)
	text := "1. Scope\n\n" + words("scope", 22) + "\n\n1.1 Exceptions\n\n" + words("exc", 22) + "\n\n1.2 Review\n\n" + words("rev", 22) + "\n"

	chunks := split(t, c, text)
	if len(chunks) != 3 {
		t.Fatalf("want 3 chunks, got %d: %+v", len(chunks), chunks)
	}
	for i, want := range []string{"1. Scope", "1. Scope > 1.1 Exceptions", "1. Scope > 1.2 Review"} {
		if chunks[i].Metadata["heading_path"] != want {
			t.Errorf("chunk %d heading_path = %q, want %q (1.2 is a sibling of 1.1)", i, chunks[i].Metadata["heading_path"], want)
		}
	}
}

func TestHeadingChunker_StructuralWordsInSeveralLanguages(t *testing.T) {
	c := NewHeadingChunker(small)
	cases := map[string]struct{ top, sub string }{
		"english":    {"Part One General Provisions", "Article 1 Definitions"},
		"french":     {"Chapitre 1 Dispositions generales", "Article 1 Definitions"},
		"portuguese": {"CAPÍTULO PRIMEIRO", "Artigo Primeiro"},
		"german":     {"Kapitel 1 Allgemeines", "Artikel 1 Begriffe"},
	}
	for name, h := range cases {
		text := h.top + "\n\n" + words("intro", 22) + "\n\n" + h.sub + "\n\n" + words("body", 22) + "\n"
		chunks := split(t, c, text)
		if len(chunks) != 2 {
			t.Fatalf("%s: want 2 chunks, got %d: %+v", name, len(chunks), chunks)
		}
		if want := h.top + " > " + h.sub; chunks[1].Metadata["heading_path"] != want {
			t.Errorf("%s: heading_path = %q, want %q", name, chunks[1].Metadata["heading_path"], want)
		}
	}
}

func TestHeadingChunker_ASentenceThatStartsWithAKeywordIsNotAHeading(t *testing.T) {
	c := NewHeadingChunker(DefaultChunkProfile())
	text := "Section 8 of the agreement states that the parties shall meet every year to review the terms and conditions.\n\nIt matters.\n"
	chunks := split(t, c, text)
	if len(chunks) != 1 || chunks[0].Metadata["heading_path"] != "" {
		t.Fatalf("got %+v", chunks)
	}
}

// A numbered list is a list: none of its items is a heading, and none is lost.
func TestHeadingChunker_ANumberedListLosesNoItem(t *testing.T) {
	c := NewHeadingChunker(DefaultChunkProfile())
	text := "# Setup\n\nFollow these steps:\n\n1. Install the tool\n2. Configure the file\n3. Run it\n\nThen check the logs.\n\n# Usage\n\nRun it daily.\n"
	got := strings.Join(texts(split(t, c, text)), "\n")
	for _, want := range []string{"1. Install the tool", "2. Configure the file", "3. Run it", "Then check the logs.", "Run it daily."} {
		if !strings.Contains(got, want) {
			t.Errorf("lost %q:\n%s", want, got)
		}
	}
}

// Numbered lines with blank lines between them are still a list, not a series of headings.
func TestHeadingChunker_ASpacedNumberedListIsStillAList(t *testing.T) {
	c := NewHeadingChunker(DefaultChunkProfile())
	text := "Steps\n\n1. Install the tool\n\n2. Configure the file\n\n3. Run it\n"
	chunks := split(t, c, text)
	if len(chunks) != 1 || chunks[0].Metadata["heading_path"] != "" || !strings.Contains(chunks[0].Text, "2. Configure the file") {
		t.Fatalf("got %+v", chunks)
	}
}

func TestHeadingChunker_CodeIsNeverReadAsHeadingsAndKeepsItsFences(t *testing.T) {
	c := NewHeadingChunker(DefaultChunkProfile())
	text := "# Script\n\nHere is the code:\n\n```python\n# load the data\ndata = load()\n# train\nmodel.fit(data)\n```\n\nThat is all.\n"
	chunks := split(t, c, text)
	if len(chunks) != 1 {
		t.Fatalf("code comments are not headings: got %d chunks %+v", len(chunks), chunks)
	}
	if !strings.Contains(chunks[0].Text, "```python\n# load the data\ndata = load()\n# train\nmodel.fit(data)\n```") {
		t.Errorf("the code block was altered:\n%s", chunks[0].Text)
	}
}

func TestHeadingChunker_LongCodeIsCutBetweenLinesAndEveryPieceIsFenced(t *testing.T) {
	c := NewHeadingChunker(ChunkProfile{Name: ChunkProfileStructured, MaxTokens: 60})
	var code []string
	for i := 0; i < 60; i++ {
		code = append(code, fmt.Sprintf("value_%d = compute(%d)", i, i))
	}
	text := "# Code\n\n```go\n" + strings.Join(code, "\n") + "\n```\n"
	chunks := split(t, c, text)
	if len(chunks) < 2 {
		t.Fatalf("a long listing must be cut, got %d chunk", len(chunks))
	}
	seen := 0
	for i, ch := range chunks {
		if strings.Count(ch.Text, "```")%2 != 0 {
			t.Errorf("chunk %d has an unbalanced fence:\n%s", i, ch.Text)
		}
		if !strings.Contains(ch.Text, "```go") {
			t.Errorf("chunk %d lost the language of the fence", i)
		}
		seen += strings.Count(ch.Text, "= compute(")
	}
	if seen != 60 {
		t.Errorf("code lines found %d times, want 60 (none lost, none repeated)", seen)
	}
}

func TestHeadingChunker_ATableStaysWholeWithTheTextAroundItWhenItFits(t *testing.T) {
	c := NewHeadingChunker(DefaultChunkProfile())
	text := "# Results\n\nThe table below gives the figures.\n\n| Name | Score |\n| --- | --- |\n| Alice | 90 |\n| Bob | 85 |\n\nBob was close.\n"
	chunks := split(t, c, text)
	if len(chunks) != 1 || !strings.Contains(chunks[0].Text, "| Name | Score |\n| --- | --- |\n| Alice | 90 |\n| Bob | 85 |") {
		t.Fatalf("got %+v", chunks)
	}
}

func TestHeadingChunker_ABigTableIsCutBetweenRowsWithItsHeaderRepeated(t *testing.T) {
	c := NewHeadingChunker(ChunkProfile{Name: ChunkProfileStructured, MaxTokens: 80})
	rows := []string{"| Id | Name |", "| --- | --- |"}
	for i := 0; i < 50; i++ {
		rows = append(rows, fmt.Sprintf("| %d | item%d |", i, i))
	}
	chunks := split(t, c, "## Sheet1\n\n"+strings.Join(rows, "\n")+"\n")
	if len(chunks) < 2 {
		t.Fatalf("got %d chunk", len(chunks))
	}
	data := 0
	for i, ch := range chunks {
		if !strings.Contains(ch.Text, "| Id | Name |\n| --- | --- |") {
			t.Errorf("chunk %d lost the table header:\n%s", i, ch.Text)
		}
		if ch.Metadata["chunk_type"] != "table" || ch.Metadata["heading_path"] != "Sheet1" {
			t.Errorf("chunk %d metadata = %v", i, ch.Metadata)
		}
		data += strings.Count(ch.Text, "| item")
	}
	if data != 50 {
		t.Errorf("data rows found %d times, want 50", data)
	}
}

func TestHeadingChunker_ALongParagraphIsCutBetweenSentencesWithAnOverlap(t *testing.T) {
	c := NewHeadingChunker(ChunkProfile{Name: ChunkProfileStructured, MaxTokens: 60, OverlapTokens: 20})
	var sentences []string
	for i := 0; i < 30; i++ {
		sentences = append(sentences, fmt.Sprintf("Sentence number %d says something about topic %d.", i, i))
	}
	chunks := split(t, c, "# Topic\n\n"+strings.Join(sentences, " ")+"\n")
	if len(chunks) < 3 {
		t.Fatalf("got %d chunks", len(chunks))
	}
	for i, ch := range chunks {
		if !strings.HasSuffix(strings.TrimSpace(ch.Text), ".") {
			t.Errorf("chunk %d ends in the middle of a sentence: %q", i, ch.Text[max(0, len(ch.Text)-40):])
		}
		if tk := utils.CountTokensInText(ch.Text); tk > 60 {
			t.Errorf("chunk %d has %d tokens, over the limit of 60", i, tk)
		}
		if i > 0 {
			// The first sentence of a chunk is the last of the one before.
			firstSentence := sentenceAfterPath(ch.Text)
			if !strings.Contains(chunks[i-1].Text, firstSentence) {
				t.Errorf("chunk %d does not start with the end of chunk %d: %q", i, i-1, firstSentence)
			}
		}
	}
	all := strings.Join(texts(chunks), " ")
	for i := range sentences {
		if !strings.Contains(all, fmt.Sprintf("Sentence number %d says", i)) {
			t.Errorf("sentence %d is in no chunk", i)
		}
	}
}

func sentenceAfterPath(chunkText string) string {
	body := chunkText[strings.Index(chunkText, "\n\n")+2:]
	return strings.TrimSpace(body[:strings.Index(body, ".")+1])
}

func texts(chunks []Chunk) []string {
	out := make([]string, len(chunks))
	for i, c := range chunks {
		out[i] = c.Text
	}
	return out
}

func TestHeadingChunker_NoHeadingsPacksTheParagraphs(t *testing.T) {
	c := NewHeadingChunker(DefaultChunkProfile())
	chunks := split(t, c, "Just a plain document.\n\nWith two paragraphs.\n\nAnd a third one for good measure.")
	if len(chunks) != 1 || !strings.Contains(chunks[0].Text, "third one") {
		t.Fatalf("three short paragraphs are one chunk, got %+v", chunks)
	}
}

func TestHeadingChunker_NoChunkIsTooBigAndNoTextIsLost(t *testing.T) {
	c := NewHeadingChunker(ChunkProfile{Name: ChunkProfileStructured, MaxTokens: 120, OverlapTokens: 0})
	var sb strings.Builder
	for s := 0; s < 12; s++ {
		fmt.Fprintf(&sb, "## Part %d\n\n", s)
		for p := 0; p < 4; p++ {
			fmt.Fprintf(&sb, "%s.\n\n", words(fmt.Sprintf("s%dp%dw", s, p), 25+7*p))
		}
		fmt.Fprintf(&sb, "- item a%d\n- item b%d\n\n", s, s)
	}
	chunks := split(t, c, sb.String())
	all := strings.Join(texts(chunks), "\n")
	for s := 0; s < 12; s++ {
		for p := 0; p < 4; p++ {
			if !strings.Contains(all, fmt.Sprintf("s%dp%dw0 ", s, p)) || !strings.Contains(all, fmt.Sprintf("s%dp%dw%d", s, p, 24+7*p)) {
				t.Fatalf("paragraph s%dp%d is missing or cut", s, p)
			}
		}
		if !strings.Contains(all, fmt.Sprintf("- item a%d\n- item b%d", s, s)) {
			t.Errorf("the list of part %d is missing or cut", s)
		}
	}
	for i, ch := range chunks {
		if tk := utils.CountTokensInText(ch.Text); tk > 120 {
			t.Errorf("chunk %d has %d tokens, over 120", i, tk)
		}
		if ch.Position != i {
			t.Errorf("chunk %d has position %d", i, ch.Position)
		}
	}
}

func TestHeadingChunker_ATrailingTinySectionIsJoinedToTheChunkBeforeIt(t *testing.T) {
	c := NewHeadingChunker(small)
	text := "# A\n\n" + words("a", 40) + "\n\n# B\n\nEnd.\n"
	chunks := split(t, c, text)
	if len(chunks) != 1 || !strings.Contains(chunks[0].Text, "End.") {
		t.Fatalf("got %d chunks %+v", len(chunks), chunks)
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
