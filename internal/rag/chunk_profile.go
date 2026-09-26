package rag

import (
	"fmt"
	"strings"

	"github.com/KPO-Tech/seshat/internal/documentreader"
)

// ChunkProfileName identifies a recommended chunking profile for RAG
// ingestion. The profile controls indexing granularity; answer-time context
// expansion should still be used when a large LLM needs neighboring chunks.
type ChunkProfileName string

const (
	ChunkProfileSmall      ChunkProfileName = "small"
	ChunkProfileMedium     ChunkProfileName = "medium"
	ChunkProfileLarge      ChunkProfileName = "large"
	ChunkProfileStructured ChunkProfileName = "structured"
	// ChunkProfileTable routes to TableChunker: table rows/structure stay
	// intact as their own chunk(s) (header row repeated on every split
	// piece) instead of a generic splitter cutting through the middle of a
	// table. See TableChunker's own doc comment.
	ChunkProfileTable ChunkProfileName = "table"
	// ChunkProfileQA routes to QAChunker: one chunk per detected
	// question/answer pair instead of arbitrary token-count splitting. See
	// QAChunker's own doc comment.
	ChunkProfileQA     ChunkProfileName = "qa"
	ChunkProfileCustom ChunkProfileName = "custom"
)

// ChunkProfile describes the chunking policy to use for document ingestion.
// Token limits are intentionally conservative: large-context LLMs can read
// more, but retrieval quality usually degrades when indexed chunks become too
// broad or noisy.
type ChunkProfile struct {
	Name          ChunkProfileName `json:"name"`
	MaxTokens     int              `json:"max_tokens"`
	OverlapTokens int              `json:"overlap_tokens,omitempty"`
}

var recommendedChunkProfiles = map[ChunkProfileName]ChunkProfile{
	ChunkProfileSmall: {
		Name:          ChunkProfileSmall,
		MaxTokens:     384,
		OverlapTokens: 64,
	},
	ChunkProfileMedium: {
		Name:          ChunkProfileMedium,
		MaxTokens:     768,
		OverlapTokens: 120,
	},
	ChunkProfileLarge: {
		Name:          ChunkProfileLarge,
		MaxTokens:     1536,
		OverlapTokens: 200,
	},
	ChunkProfileStructured: {
		Name:          ChunkProfileStructured,
		MaxTokens:     1024,
		OverlapTokens: 120,
	},
	ChunkProfileTable: {
		Name:      ChunkProfileTable,
		MaxTokens: 1536,
		// No overlap: repeating the header row on every split piece (see
		// TableChunker) already gives each chunk the context an overlap
		// would otherwise exist to provide - repeating data rows on top of
		// that would just be noise for a structured table.
		OverlapTokens: 0,
	},
	ChunkProfileQA: {
		Name:      ChunkProfileQA,
		MaxTokens: 512,
		// No overlap: each chunk is already exactly one self-contained
		// Q/A pair - there is no "next chunk" content an overlap would
		// usefully carry forward.
		OverlapTokens: 0,
	},
}

// DefaultChunkProfile returns the default enterprise RAG profile.
func DefaultChunkProfile() ChunkProfile {
	return recommendedChunkProfiles[ChunkProfileMedium]
}

// RecommendedChunkProfile returns one of Seshat's built-in chunk profiles.
func RecommendedChunkProfile(name ChunkProfileName) (ChunkProfile, bool) {
	normalized := ChunkProfileName(strings.ToLower(strings.TrimSpace(string(name))))
	if normalized == "" {
		normalized = ChunkProfileMedium
	}
	profile, ok := recommendedChunkProfiles[normalized]
	return profile, ok
}

// NewCustomChunkProfile validates and returns a custom chunk profile.
func NewCustomChunkProfile(maxTokens, overlapTokens int) (ChunkProfile, error) {
	if maxTokens <= 0 {
		return ChunkProfile{}, fmt.Errorf("max_tokens must be positive")
	}
	if overlapTokens < 0 {
		return ChunkProfile{}, fmt.Errorf("overlap_tokens must be zero or positive")
	}
	if overlapTokens >= maxTokens {
		return ChunkProfile{}, fmt.Errorf("overlap_tokens must be lower than max_tokens")
	}
	return ChunkProfile{
		Name:          ChunkProfileCustom,
		MaxTokens:     maxTokens,
		OverlapTokens: overlapTokens,
	}, nil
}

// DocumentReaderChunkOptionsForProfile maps a RAG chunk profile to hybrid
// document-reader chunk options while preserving caller-provided structural
// options.
func DocumentReaderChunkOptionsForProfile(profile ChunkProfile, opts documentreader.ChunkOptions) documentreader.ChunkOptions {
	if profile.MaxTokens > 0 {
		opts.MaxTokens = profile.MaxTokens
	}
	return opts
}
