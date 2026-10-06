package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/KPO-Tech/seshat/internal/documentreader"
)

// HybridDocumentChunker chunks rich documents with a document-aware hybrid
// chunker. The backend can be local, docling-serve, seshat-intelligence, or
// any other document reader implementing documentreader.HybridChunker. It falls back to a
// plain text chunker unless FailOnError is set.
type HybridDocumentChunker struct {
	Client      documentreader.HybridChunker
	Options     documentreader.ChunkOptions
	Profile     ChunkProfile
	Fallback    Chunker
	FailOnError bool
}

// NewHybridDocumentChunker creates a document-aware chunker backed by the given
// document reader.
func NewHybridDocumentChunker(client documentreader.HybridChunker, opts documentreader.ChunkOptions) *HybridDocumentChunker {
	return &HybridDocumentChunker{
		Client:   client,
		Options:  opts,
		Fallback: DefaultChunker(),
	}
}

// NewHybridDocumentChunkerForProfile creates a document-aware chunker using one of
// Seshat's recommended chunking profiles. When the backend is unavailable
// or fails, the fallback depends on the profile: "structured" falls back to
// HeadingChunker (heading/numbering-hierarchy aware), "table" to
// TableChunker (keeps table rows intact), "qa" to QAChunker (one chunk per
// detected Q/A pair) - every other profile keeps the plain fallback
// unchanged.
func NewHybridDocumentChunkerForProfile(client documentreader.HybridChunker, profile ChunkProfile, opts documentreader.ChunkOptions) *HybridDocumentChunker {
	if profile.MaxTokens <= 0 {
		profile = DefaultChunkProfile()
	}
	fallback := DefaultChunker()
	switch profile.Name {
	case ChunkProfileStructured:
		fallback = NewHeadingChunker(profile)
	case ChunkProfileTable:
		fallback = NewTableChunker(profile)
	case ChunkProfileQA:
		fallback = NewQAChunker(profile)
	}
	return &HybridDocumentChunker{
		Client:   client,
		Options:  DocumentReaderChunkOptionsForProfile(profile, opts),
		Profile:  profile,
		Fallback: fallback,
	}
}

func (c *HybridDocumentChunker) Split(ctx context.Context, text string) ([]Chunk, error) {
	return c.fallback().Split(ctx, text)
}

func (c *HybridDocumentChunker) ChunkCacheKey() string {
	if c == nil {
		return "document-hybrid:v1:nil"
	}
	data, err := json.Marshal(struct {
		Options documentreader.ChunkOptions `json:"options"`
		Profile ChunkProfile                `json:"profile,omitempty"`
	}{
		Options: c.Options,
		Profile: c.Profile,
	})
	if err != nil {
		return "document-hybrid:v1"
	}
	return "document-hybrid:v1:" + string(data)
}

func (c *HybridDocumentChunker) SplitDocument(ctx context.Context, doc Document) ([]Chunk, error) {
	if c == nil || c.Client == nil || len(doc.Data) == 0 {
		return c.fallback().Split(ctx, doc.Text)
	}
	if !c.Client.IsAvailable(ctx) {
		return c.handleError(ctx, doc.Text, fmt.Errorf("document reader is unavailable"))
	}
	chunks, err := c.Client.ChunkHybridBytes(ctx, doc.Data, doc.Filename, c.Options)
	if err != nil {
		return c.handleError(ctx, doc.Text, err)
	}
	out := make([]Chunk, 0, len(chunks))
	for i, chunk := range chunks {
		text := strings.TrimSpace(chunk.Text)
		if text == "" {
			text = strings.TrimSpace(chunk.RawText)
		}
		if text == "" {
			continue
		}
		position := chunk.ChunkIndex
		if position < 0 {
			position = i
		}
		out = append(out, Chunk{
			Text:     text,
			Position: position,
			Metadata: c.documentChunkMetadata(chunk),
		})
	}
	if len(out) == 0 {
		return c.handleError(ctx, doc.Text, fmt.Errorf("document reader returned no usable chunks"))
	}
	return out, nil
}

func (c *HybridDocumentChunker) handleError(ctx context.Context, text string, err error) ([]Chunk, error) {
	if c != nil && c.FailOnError {
		return nil, err
	}
	return c.fallback().Split(ctx, text)
}

func (c *HybridDocumentChunker) fallback() Chunker {
	if c != nil && c.Fallback != nil {
		return c.Fallback
	}
	return DefaultChunker()
}

func (c *HybridDocumentChunker) documentChunkMetadata(chunk documentreader.Chunk) map[string]string {
	metadata := map[string]string{
		"chunker":               "document_hybrid",
		"document_reader_index": strconv.Itoa(chunk.ChunkIndex),
	}
	if c != nil && c.Profile.Name != "" {
		metadata["chunk_profile"] = string(c.Profile.Name)
		if c.Profile.MaxTokens > 0 {
			metadata["chunk_max_tokens"] = strconv.Itoa(c.Profile.MaxTokens)
		}
		if c.Profile.OverlapTokens > 0 {
			metadata["chunk_overlap_tokens"] = strconv.Itoa(c.Profile.OverlapTokens)
		}
	}
	if strings.TrimSpace(chunk.Filename) != "" {
		metadata["document_reader_filename"] = chunk.Filename
	}
	if chunk.NumTokens != nil {
		metadata["num_tokens"] = strconv.Itoa(*chunk.NumTokens)
	}
	putJSON := func(key string, value any) {
		b, err := json.Marshal(value)
		if err == nil && string(b) != "null" && string(b) != "[]" && string(b) != "{}" {
			metadata[key] = string(b)
		}
	}
	putJSON("headings", chunk.Headings)
	putJSON("captions", chunk.Captions)
	putJSON("page_numbers", chunk.PageNumbers)
	putJSON("doc_items", chunk.DocItems)
	putJSON("document_reader_metadata", chunk.Metadata)
	if strings.TrimSpace(chunk.RawText) != "" && strings.TrimSpace(chunk.RawText) != strings.TrimSpace(chunk.Text) {
		metadata["raw_text"] = chunk.RawText
	}
	return metadata
}
