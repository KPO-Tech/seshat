package rag

import (
	"fmt"
	"strings"

	"github.com/KPO-Tech/seshat/internal/docling"
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
	ChunkProfileCustom     ChunkProfileName = "custom"
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

// DoclingChunkOptionsForProfile maps a RAG chunk profile to docling-serve's
// hybrid chunker options while preserving caller-provided structural options.
func DoclingChunkOptionsForProfile(profile ChunkProfile, opts docling.ChunkOptions) docling.ChunkOptions {
	if profile.MaxTokens > 0 {
		opts.MaxTokens = profile.MaxTokens
	}
	return opts
}
