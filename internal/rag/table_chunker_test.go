package rag

import (
	"context"
	"strings"
	"testing"
)

func TestTableChunker_NoTableFallsBackToHeadingChunker(t *testing.T) {
	ctx := context.Background()
	c := NewTableChunker(DefaultChunkProfile())
	text := "## Section One\n\nJust some prose, no tables here."

	got, err := c.Split(ctx, text)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	want, err := NewHeadingChunker(DefaultChunkProfile()).Split(ctx, text)
	if err != nil {
		t.Fatalf("HeadingChunker.Split: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("expected fallback to match HeadingChunker exactly: got %d chunks, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i].Text != want[i].Text {
			t.Errorf("chunk %d: got %q, want %q", i, got[i].Text, want[i].Text)
		}
	}
}

func TestTableChunker_SmallTableStaysOneChunk(t *testing.T) {
	ctx := context.Background()
	c := NewTableChunker(DefaultChunkProfile())
	text := "Intro paragraph.\n\n| Name | Age |\n| --- | --- |\n| Alice | 30 |\n| Bob | 25 |\n\nOutro paragraph."

	chunks, err := c.Split(ctx, text)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}

	var tableChunks []Chunk
	for _, ch := range chunks {
		if ch.Metadata["chunk_type"] == "table" {
			tableChunks = append(tableChunks, ch)
		}
	}
	if len(tableChunks) != 1 {
		t.Fatalf("expected exactly 1 table chunk, got %d: %+v", len(tableChunks), tableChunks)
	}
	tc := tableChunks[0]
	for _, want := range []string{"| Name | Age |", "| --- | --- |", "Alice", "Bob"} {
		if !strings.Contains(tc.Text, want) {
			t.Errorf("expected table chunk to contain %q, got:\n%s", want, tc.Text)
		}
	}

	// Surrounding prose should still be present, just not tagged as table.
	joined := chunksText(chunks)
	if !strings.Contains(joined, "Intro paragraph") || !strings.Contains(joined, "Outro paragraph") {
		t.Errorf("expected surrounding prose preserved, got:\n%s", joined)
	}
}

func TestTableChunker_LargeTableSplitsByRowWithRepeatedHeader(t *testing.T) {
	ctx := context.Background()
	profile, err := NewCustomChunkProfile(30, 0) // small budget to force a split
	if err != nil {
		t.Fatalf("NewCustomChunkProfile: %v", err)
	}
	c := NewTableChunker(profile)

	var sb strings.Builder
	sb.WriteString("| Name | Description |\n| --- | --- |\n")
	for i := 0; i < 20; i++ {
		sb.WriteString("| Row")
		sb.WriteString(strings.Repeat("X", 3))
		sb.WriteString(" | A reasonably long description text to consume tokens quickly |\n")
	}

	chunks, err := c.Split(ctx, sb.String())
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected the table to split into multiple chunks, got %d", len(chunks))
	}
	for i, ch := range chunks {
		if ch.Metadata["chunk_type"] != "table" {
			t.Fatalf("chunk %d: expected chunk_type=table, got %+v", i, ch.Metadata)
		}
		if !strings.Contains(ch.Text, "| Name | Description |") {
			t.Errorf("chunk %d: expected repeated header row, got:\n%s", i, ch.Text)
		}
		if !strings.Contains(ch.Text, "| --- | --- |") {
			t.Errorf("chunk %d: expected repeated separator row, got:\n%s", i, ch.Text)
		}
	}
}

func TestTableChunker_MultipleTablesEachGetOwnChunk(t *testing.T) {
	ctx := context.Background()
	c := NewTableChunker(DefaultChunkProfile())
	text := "| A | B |\n| --- | --- |\n| 1 | 2 |\n\nSome text between tables.\n\n| C | D |\n| --- | --- |\n| 3 | 4 |"

	chunks, err := c.Split(ctx, text)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	var tableChunks []Chunk
	for _, ch := range chunks {
		if ch.Metadata["chunk_type"] == "table" {
			tableChunks = append(tableChunks, ch)
		}
	}
	if len(tableChunks) != 2 {
		t.Fatalf("expected 2 separate table chunks, got %d: %+v", len(tableChunks), tableChunks)
	}
	if !strings.Contains(tableChunks[0].Text, "| A | B |") || strings.Contains(tableChunks[0].Text, "| C | D |") {
		t.Errorf("first table chunk should contain only the first table, got:\n%s", tableChunks[0].Text)
	}
	if !strings.Contains(tableChunks[1].Text, "| C | D |") || strings.Contains(tableChunks[1].Text, "| A | B |") {
		t.Errorf("second table chunk should contain only the second table, got:\n%s", tableChunks[1].Text)
	}
}

func TestTableChunker_SplitDocumentIgnoresBytesUsesText(t *testing.T) {
	ctx := context.Background()
	c := NewTableChunker(DefaultChunkProfile())
	doc := Document{Text: "| A | B |\n| --- | --- |\n| 1 | 2 |", Data: []byte("irrelevant")}

	chunks, err := c.SplitDocument(ctx, doc)
	if err != nil {
		t.Fatalf("SplitDocument: %v", err)
	}
	if len(chunks) != 1 || chunks[0].Metadata["chunk_type"] != "table" {
		t.Fatalf("expected a single table chunk, got %+v", chunks)
	}
}

func chunksText(chunks []Chunk) string {
	var sb strings.Builder
	for _, c := range chunks {
		sb.WriteString(c.Text)
		sb.WriteString("\n")
	}
	return sb.String()
}
