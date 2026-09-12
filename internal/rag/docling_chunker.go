package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/KPO-Tech/seshat/internal/docling"
)

// DoclingChunker chunks rich documents with docling-serve's hybrid chunker.
// It falls back to a plain text chunker unless FailOnError is set.
type DoclingChunker struct {
	Client      *docling.Client
	Options     docling.ChunkOptions
	Profile     ChunkProfile
	Fallback    Chunker
	FailOnError bool
}

// NewDoclingChunker creates a document-aware chunker backed by docling-serve.
func NewDoclingChunker(client *docling.Client, opts docling.ChunkOptions) *DoclingChunker {
	return &DoclingChunker{
		Client:   client,
		Options:  opts,
		Fallback: DefaultChunker(),
	}
}

// NewDoclingChunkerForProfile creates a document-aware chunker using one of
// Seshat's recommended chunking profiles. When docling-serve is unavailable
// or fails, the fallback for the "structured" profile is HeadingChunker
// (heading/numbering-hierarchy aware) instead of plain ParagraphChunker -
// every other profile keeps the plain fallback unchanged.
func NewDoclingChunkerForProfile(client *docling.Client, profile ChunkProfile, opts docling.ChunkOptions) *DoclingChunker {
	if profile.MaxTokens <= 0 {
		profile = DefaultChunkProfile()
	}
	fallback := DefaultChunker()
	if profile.Name == ChunkProfileStructured {
		fallback = NewHeadingChunker(profile)
	}
	return &DoclingChunker{
		Client:   client,
		Options:  DoclingChunkOptionsForProfile(profile, opts),
		Profile:  profile,
		Fallback: fallback,
	}
}

func (c *DoclingChunker) Split(ctx context.Context, text string) ([]Chunk, error) {
	return c.fallback().Split(ctx, text)
}

func (c *DoclingChunker) ChunkCacheKey() string {
	if c == nil {
		return "docling-hybrid:v1:nil"
	}
	data, err := json.Marshal(struct {
		Options docling.ChunkOptions `json:"options"`
		Profile ChunkProfile         `json:"profile,omitempty"`
	}{
		Options: c.Options,
		Profile: c.Profile,
	})
	if err != nil {
		return "docling-hybrid:v1"
	}
	return "docling-hybrid:v1:" + string(data)
}

func (c *DoclingChunker) SplitDocument(ctx context.Context, doc Document) ([]Chunk, error) {
	if c == nil || c.Client == nil || len(doc.Data) == 0 {
		return c.fallback().Split(ctx, doc.Text)
	}
	if !c.Client.IsAvailable(ctx) {
		return c.handleError(ctx, doc.Text, fmt.Errorf("docling-serve is unavailable"))
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
			Metadata: c.doclingChunkMetadata(chunk),
		})
	}
	if len(out) == 0 {
		return c.handleError(ctx, doc.Text, fmt.Errorf("docling returned no usable chunks"))
	}
	return out, nil
}

func (c *DoclingChunker) handleError(ctx context.Context, text string, err error) ([]Chunk, error) {
	if c != nil && c.FailOnError {
		return nil, err
	}
	return c.fallback().Split(ctx, text)
}

func (c *DoclingChunker) fallback() Chunker {
	if c != nil && c.Fallback != nil {
		return c.Fallback
	}
	return DefaultChunker()
}

func (c *DoclingChunker) doclingChunkMetadata(chunk docling.Chunk) map[string]string {
	metadata := map[string]string{
		"chunker":       "docling_hybrid",
		"docling_index": strconv.Itoa(chunk.ChunkIndex),
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
		metadata["docling_filename"] = chunk.Filename
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
	putJSON("docling_metadata", chunk.Metadata)
	if strings.TrimSpace(chunk.RawText) != "" && strings.TrimSpace(chunk.RawText) != strings.TrimSpace(chunk.Text) {
		metadata["raw_text"] = chunk.RawText
	}
	return metadata
}
