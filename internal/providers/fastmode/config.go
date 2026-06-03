package fastmode

import (
	"os"
	"strings"

	"github.com/EngineerProjects/nexus-engine/internal/types"
)

// FastmodeConfig controls which model is used for low-latency background tasks
// (e.g. classifier calls, quick tool results) versus the primary model used for
// the main conversation loop.
type FastmodeConfig struct {
	// Enabled toggles fastmode model substitution globally.
	Enabled bool

	// FastModel is the model identifier to use for quick operations.
	// Defaults to claude-haiku-4-5-20251001 on the Anthropic provider.
	FastModel types.ModelIdentifier

	// MaxTokens caps output for fastmode calls to keep them cheap.
	MaxTokens int
}

// DefaultFastmodeConfig returns a sensible default configuration.
// The fast model can be overridden via NEXUS_FAST_MODEL env var (format: "provider:model").
func DefaultFastmodeConfig() *FastmodeConfig {
	cfg := &FastmodeConfig{
		Enabled: true,
		FastModel: types.ModelIdentifier{
			Provider: types.APIProviderAnthropic,
			Model:    "claude-haiku-4-5-20251001",
		},
		MaxTokens: 1024,
	}

	if raw := os.Getenv("NEXUS_FAST_MODEL"); raw != "" {
		if idx := strings.Index(raw, ":"); idx > 0 {
			cfg.FastModel = types.ModelIdentifier{
				Provider: types.APIProvider(raw[:idx]),
				Model:    raw[idx+1:],
			}
		} else {
			cfg.FastModel.Model = raw
		}
	}

	return cfg
}
