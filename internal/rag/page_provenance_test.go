package rag

import (
	"fmt"
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat/internal/pagemark"
)

func pagedDoc(pages ...string) string {
	var sb strings.Builder
	for i, p := range pages {
		sb.WriteString(pagemark.Marker(i + 1))
		sb.WriteString("\n\n")
		sb.WriteString(p)
		sb.WriteString("\n\n")
	}
	return sb.String()
}

func TestChunksSayWhichPagesTheyCome(t *testing.T) {
	c := NewHeadingChunker(small)
	text := pagedDoc(
		"# Report\n\n"+words("one", 40),
		words("two", 40)+"\n\n## Results\n\n"+words("three", 22),
		"## Appendix\n\n"+words("four", 22),
	)
	chunks := split(t, c, text)
	if len(chunks) < 3 {
		t.Fatalf("got %d chunks: %+v", len(chunks), chunks)
	}
	for i, ch := range chunks {
		if strings.Contains(ch.Text, "<!--") {
			t.Errorf("chunk %d has a page marker in its text:\n%s", i, ch.Text)
		}
		if ch.Metadata["page_numbers"] == "" {
			t.Errorf("chunk %d has no page_numbers", i)
		}
	}
	// "one..." is on page 1 only, "four..." on page 3 only.
	for _, ch := range chunks {
		if strings.Contains(ch.Text, "one39") && !strings.HasPrefix(ch.Metadata["page_numbers"], "[1") {
			t.Errorf("the end of page 1 is on %s", ch.Metadata["page_numbers"])
		}
		if strings.Contains(ch.Text, "four0") && ch.Metadata["page_numbers"] != "[3]" {
			t.Errorf("the appendix is on page 3, got %s", ch.Metadata["page_numbers"])
		}
	}
}

func TestAChunkThatSpansPagesListsThemAll(t *testing.T) {
	c := NewHeadingChunker(DefaultChunkProfile())
	text := pagedDoc("# Notes\n\nFirst page text.", "Second page text.", "Third page text.")
	chunks := split(t, c, text)
	if len(chunks) != 1 || chunks[0].Metadata["page_numbers"] != "[1,2,3]" {
		t.Fatalf("got %+v", chunks)
	}
	if strings.Contains(chunks[0].Text, "<!--") {
		t.Errorf("a marker is in the text: %q", chunks[0].Text)
	}
	for _, want := range []string{"First page text.", "Second page text.", "Third page text."} {
		if !strings.Contains(chunks[0].Text, want) {
			t.Errorf("lost %q", want)
		}
	}
}

func TestTheRowsOfATableCutAcrossChunksKeepTheirPage(t *testing.T) {
	c := NewHeadingChunker(ChunkProfile{Name: ChunkProfileStructured, MaxTokens: 80})
	rows := []string{"| Id | Name |", "| --- | --- |"}
	for i := 0; i < 40; i++ {
		rows = append(rows, fmt.Sprintf("| %d | item%d |", i, i))
	}
	chunks := split(t, c, pagedDoc("intro", "## Data\n\n"+strings.Join(rows, "\n")))
	var tables int
	for _, ch := range chunks {
		if ch.Metadata["chunk_type"] == "table" {
			tables++
			if ch.Metadata["page_numbers"] != "[2]" {
				t.Errorf("a table of page 2 has pages %q", ch.Metadata["page_numbers"])
			}
		}
	}
	if tables < 2 {
		t.Fatalf("the table was not cut: %d chunks", len(chunks))
	}
}

func TestATextWithNoPageMarkersHasNoPageNumbers(t *testing.T) {
	chunks := split(t, NewHeadingChunker(DefaultChunkProfile()), "# A\n\nSome text.\n")
	if len(chunks) != 1 || chunks[0].Metadata["page_numbers"] != "" {
		t.Fatalf("got %+v", chunks)
	}
}

// A page marker never makes a heading, a block or a word of its own.
func TestPageMarkersAreInvisibleToTheStructure(t *testing.T) {
	with := split(t, NewHeadingChunker(DefaultChunkProfile()), pagedDoc("# Title\n\nBody text here.", "More body text."))
	without := split(t, NewHeadingChunker(DefaultChunkProfile()), "# Title\n\nBody text here.\n\nMore body text.\n")
	if len(with) != len(without) || with[0].Text != without[0].Text {
		t.Fatalf("with markers:\n%q\nwithout:\n%q", with[0].Text, without[0].Text)
	}
}
