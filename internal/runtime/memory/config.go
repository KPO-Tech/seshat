package memory

import "github.com/EngineerProjects/nexus-engine/internal/types"

// Config represents compaction configuration.
type Config struct {
	AutoCompactThreshold      float64               `json:"auto_compact_threshold"`
	CompactTargetPercentage   float64               `json:"compact_target_percentage"`
	MaxSummaryTokens          int                   `json:"max_summary_tokens"`
	AutoCompactBufferTokens   int                   `json:"auto_compact_buffer_tokens"`
	ManualCompactBufferTokens int                   `json:"manual_compact_buffer_tokens"`
	MaxConsecutiveFailures    int                   `json:"max_consecutive_failures"`
	SummaryModel              types.ModelIdentifier `json:"summary_model"`
}

// DefaultConfig returns default compaction configuration.
func DefaultConfig() *Config {
	return &Config{
		AutoCompactThreshold:      0.85,
		CompactTargetPercentage:   0.50,
		MaxSummaryTokens:          defaultMaxSummaryTokens,
		AutoCompactBufferTokens:   defaultAutoCompactBufferTokens,
		ManualCompactBufferTokens: defaultManualCompactBufferTokens,
		MaxConsecutiveFailures:    defaultMaxConsecutiveCompactFailure,
		SummaryModel: types.ModelIdentifier{
			Provider: types.APIProviderAnthropic,
			Model:    "claude-3-5-haiku-20241022",
		},
	}
}
