package rag

import (
	"context"
	"strings"
	"testing"
)

func TestQAChunker_NoQAFallsBackToHeadingChunker(t *testing.T) {
	ctx := context.Background()
	c := NewQAChunker(DefaultChunkProfile())
	text := "## Section One\n\nJust some prose, no Q/A pairs here."

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

func TestQAChunker_LabeledPairsOnePerChunk(t *testing.T) {
	ctx := context.Background()
	c := NewQAChunker(DefaultChunkProfile())
	text := "Q: What is your return policy?\nA: You can return items within 30 days.\n\nQ: Do you ship internationally?\nA: Yes, to most countries."

	chunks, err := c.Split(ctx, text)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected exactly 2 chunks (one per pair), got %d: %+v", len(chunks), chunks)
	}
	if chunks[0].Metadata["qa_question"] != "What is your return policy?" {
		t.Errorf("chunk 0: unexpected qa_question metadata: %q", chunks[0].Metadata["qa_question"])
	}
	if !strings.Contains(chunks[0].Text, "return items within 30 days") {
		t.Errorf("chunk 0: expected the answer text, got:\n%s", chunks[0].Text)
	}
	if chunks[1].Metadata["qa_question"] != "Do you ship internationally?" {
		t.Errorf("chunk 1: unexpected qa_question metadata: %q", chunks[1].Metadata["qa_question"])
	}
}

func TestQAChunker_LabeledPairsWithVariants(t *testing.T) {
	ctx := context.Background()
	c := NewQAChunker(DefaultChunkProfile())
	text := "**Q1:** How do I reset my password?\n**A1:** Click 'forgot password' on the login page.\n\nQuestion: How do I contact support?\nAnswer: Email us at support@example.com."

	chunks, err := c.Split(ctx, text)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected exactly 2 chunks, got %d: %+v", len(chunks), chunks)
	}
	if !strings.Contains(chunks[0].Metadata["qa_question"], "reset my password") {
		t.Errorf("chunk 0: unexpected qa_question: %q", chunks[0].Metadata["qa_question"])
	}
	if !strings.Contains(chunks[1].Metadata["qa_question"], "contact support") {
		t.Errorf("chunk 1: unexpected qa_question: %q", chunks[1].Metadata["qa_question"])
	}
}

func TestQAChunker_LabeledPairsPreamblePreserved(t *testing.T) {
	ctx := context.Background()
	c := NewQAChunker(DefaultChunkProfile())
	text := "Welcome to our FAQ page.\n\nQ: What is X?\nA: X is a thing."

	chunks, err := c.Split(ctx, text)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected preamble chunk + 1 pair chunk, got %d: %+v", len(chunks), chunks)
	}
	if !strings.Contains(chunks[0].Text, "Welcome to our FAQ page") {
		t.Errorf("expected preamble preserved as its own chunk, got:\n%s", chunks[0].Text)
	}
	if chunks[0].Metadata["qa_question"] != "" {
		t.Errorf("preamble chunk should not carry qa_question metadata, got %q", chunks[0].Metadata["qa_question"])
	}
}

func TestQAChunker_HeadingQuestionsOnePerChunk(t *testing.T) {
	ctx := context.Background()
	c := NewQAChunker(DefaultChunkProfile())
	text := "## What is your return policy?\n\nYou can return items within 30 days.\n\n## Do you ship internationally?\n\nYes, to most countries."

	chunks, err := c.Split(ctx, text)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected exactly 2 chunks (one per question heading), got %d: %+v", len(chunks), chunks)
	}
	if chunks[0].Metadata["qa_question"] != "## What is your return policy?" {
		t.Errorf("chunk 0: unexpected qa_question: %q", chunks[0].Metadata["qa_question"])
	}
	if chunks[1].Metadata["qa_question"] != "## Do you ship internationally?" {
		t.Errorf("chunk 1: unexpected qa_question: %q", chunks[1].Metadata["qa_question"])
	}
}

func TestQAChunker_HeadingModeNonQuestionSectionNotDropped(t *testing.T) {
	ctx := context.Background()
	c := NewQAChunker(DefaultChunkProfile())
	text := "## Overview\n\nThis is background information, not a question.\n\n## What is your return policy?\n\nYou can return items within 30 days."

	chunks, err := c.Split(ctx, text)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks (1 ordinary section + 1 QA pair), got %d: %+v", len(chunks), chunks)
	}
	// The "Overview" section should NOT be tagged as a QA pair, but its
	// content must still be present - nothing silently dropped.
	if chunks[0].Metadata["qa_question"] != "" {
		t.Errorf("expected the Overview section to have no qa_question tag, got %q", chunks[0].Metadata["qa_question"])
	}
	if !strings.Contains(chunks[0].Text, "background information") {
		t.Errorf("expected the Overview section's content preserved, got:\n%s", chunks[0].Text)
	}
	if chunks[1].Metadata["qa_question"] == "" {
		t.Error("expected the return-policy section to be tagged as a QA pair")
	}
}

func TestQAChunker_HeadingModeNestedCategoryKeepsAncestorPath(t *testing.T) {
	ctx := context.Background()
	c := NewQAChunker(DefaultChunkProfile())
	text := "## Shipping\n\n### Do you ship internationally?\n\nYes, to most countries."

	chunks, err := c.Split(ctx, text)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected exactly 1 chunk, got %d: %+v", len(chunks), chunks)
	}
	if !strings.Contains(chunks[0].Metadata["heading_path"], "Shipping") {
		t.Errorf("expected ancestor path to include the category heading, got %q", chunks[0].Metadata["heading_path"])
	}
	if chunks[0].Metadata["qa_question"] == "" {
		t.Error("expected the nested question section to still be tagged as a QA pair")
	}
}

func TestQAChunker_SplitDocumentIgnoresBytesUsesText(t *testing.T) {
	ctx := context.Background()
	c := NewQAChunker(DefaultChunkProfile())
	doc := Document{Text: "Q: What is X?\nA: X is a thing.", Data: []byte("irrelevant")}

	chunks, err := c.SplitDocument(ctx, doc)
	if err != nil {
		t.Fatalf("SplitDocument: %v", err)
	}
	if len(chunks) != 1 || chunks[0].Metadata["qa_question"] != "What is X?" {
		t.Fatalf("expected a single QA chunk, got %+v", chunks)
	}
}
