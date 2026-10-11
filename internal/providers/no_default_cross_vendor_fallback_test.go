package providers

import (
	"testing"

	"github.com/KPO-Tech/seshat/internal/types"
)

// TestAnthropicDefaultConfigHasNoCrossVendorFallback guards M5f in TODO.md
// (privacy): a session that selected Anthropic must not silently continue
// on a different vendor by default just because, say, an OpenAI key happens
// to be present in the environment. Cross-vendor fallback must be an
// explicit opt-in (ProviderConfig.Routing.FallbackProviders), never baked
// into the default config.
func TestAnthropicDefaultConfigHasNoCrossVendorFallback(t *testing.T) {
	cfg := DefaultConfigs()[types.APIProviderAnthropic]
	if cfg == nil || cfg.Routing == nil {
		t.Fatal("expected a default Anthropic config with routing")
	}
	if len(cfg.Routing.FallbackProviders) != 0 {
		t.Errorf("expected no default cross-vendor fallback providers, got %v", cfg.Routing.FallbackProviders)
	}
	// Same-vendor model fallback (e.g. sonnet -> haiku) is fine to keep by
	// default - it's still Anthropic, not a different vendor.
	if len(cfg.Routing.FallbackModels) == 0 {
		t.Error("expected same-vendor FallbackModels to remain configured")
	}
}
