// Package auto - Feature flags for runtime configuration.
//
// This module provides feature flag support for the auto mode classifier,
// allowing runtime configuration via environment variables. This aligns with
// OpenClaude's GrowthBook-based feature flag system, but uses env vars for
// simpler deployment in Nexus.
//
// Environment Variables:
//   - CLAUDE_CODE_TWO_STAGE_CLASSIFIER: Enable two-stage classifier (true/fast/thinking)
//   - CLAUDE_CODE_JSONL_TRANSCRIPT: Use JSONL format for transcript
//   - CLAUDE_CODE_CACHE_CONTROL: Enable cache control headers
//   - CLAUDE_CODE_THINKING: Enable thinking mode
//   - CLAUDE_CODE_MAX_TRANSCRIPT_CHARS: Maximum transcript characters
package auto

import (
	"os"
	"strings"
)

// FeatureFlags holds all configurable feature flags for auto mode.
// These values are determined at startup from environment variables.
type FeatureFlags struct {
	TwoStageClassifier     bool         // Enable two-stage classifier
	TwoStageClassifierMode TwoStageMode // Which stages to run (both/fast/thinking)
	TranscriptJSONL        bool         // Use JSONL format for transcript
	CacheControl           bool         // Enable cache control headers
	ThinkingEnabled        bool         // Enable thinking/reasoning
	MaxTranscriptChars     int          // Maximum transcript character length
}

func GetFeatureFlags() FeatureFlags {
	flags := FeatureFlags{
		TwoStageClassifier:     false,
		TwoStageClassifierMode: TwoStageModeBoth,
		TranscriptJSONL:        false,
		CacheControl:           true,
		ThinkingEnabled:        true,
		MaxTranscriptChars:     MaxTranscriptChars,
	}

	// CLAUDE_CODE_TWO_STAGE_CLASSIFIER env var
	twoStageEnv := os.Getenv("CLAUDE_CODE_TWO_STAGE_CLASSIFIER")
	if twoStageEnv != "" {
		switch strings.ToLower(twoStageEnv) {
		case "true", "1":
			flags.TwoStageClassifier = true
			flags.TwoStageClassifierMode = TwoStageModeBoth
		case "fast":
			flags.TwoStageClassifier = true
			flags.TwoStageClassifierMode = TwoStageModeFast
		case "thinking":
			flags.TwoStageClassifier = true
			flags.TwoStageClassifierMode = TwoStageModeThinking
		case "false", "0":
			flags.TwoStageClassifier = false
		}
	}

	// CLAUDE_CODE_JSONL_TRANSCRIPT env var
	jsonlEnv := os.Getenv("CLAUDE_CODE_JSONL_TRANSCRIPT")
	if jsonlEnv != "" {
		flags.TranscriptJSONL = strings.ToLower(jsonlEnv) == "true" || jsonlEnv == "1"
	}

	// CLAUDE_CODE_CACHE_CONTROL env var (optional override)
	cacheEnv := os.Getenv("CLAUDE_CODE_CACHE_CONTROL")
	if cacheEnv != "" {
		flags.CacheControl = strings.ToLower(cacheEnv) == "true" || cacheEnv == "1"
	}

	// CLAUDE_CODE_THINKING env var
	thinkingEnv := os.Getenv("CLAUDE_CODE_THINKING")
	if thinkingEnv != "" {
		flags.ThinkingEnabled = strings.ToLower(thinkingEnv) == "true" || thinkingEnv == "1"
	}

	// CLAUDE_CODE_MAX_TRANSCRIPT_CHARS env var
	maxCharsEnv := os.Getenv("CLAUDE_CODE_MAX_TRANSCRIPT_CHARS")
	if maxCharsEnv != "" {
		if parsed := parseEnvInt(maxCharsEnv); parsed > 0 {
			flags.MaxTranscriptChars = parsed
		}
	}

	return flags
}

func parseEnvInt(s string) int {
	var result int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result*10 + int(c-'0')
		} else {
			break
		}
	}
	return result
}

func IsTwoStageClassifierEnabled() bool {
	return GetFeatureFlags().TwoStageClassifier
}

func IsJSONLTranscriptEnabled() bool {
	return GetFeatureFlags().TranscriptJSONL
}
