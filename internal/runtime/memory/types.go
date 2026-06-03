package memory

import "github.com/EngineerProjects/nexus-engine/internal/types"

// TrackingState carries circuit-breaker state across loop iterations.
type TrackingState struct {
	ConsecutiveFailures int `json:"consecutive_failures"`
}

// CompactionResult is the canonical compaction outcome.
type CompactionResult struct {
	Messages            []types.Message           `json:"messages"`
	DidCompact          bool                      `json:"did_compact"`
	UsedMicroCompact    bool                      `json:"used_micro_compact"`
	UsedSummaryCompact  bool                      `json:"used_summary_compact"`
	PreCompactTokens    int                       `json:"pre_compact_tokens"`
	PostCompactTokens   int                       `json:"post_compact_tokens"`
	TargetTokens        int                       `json:"target_tokens"`
	Summary             string                    `json:"summary,omitempty"`
	ConsecutiveFailures int                       `json:"consecutive_failures"`
	RuntimeMetadata     *types.CompactionMetadata `json:"runtime_metadata,omitempty"`
}
