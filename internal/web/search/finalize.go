package search

import "github.com/EngineerProjects/nexus-engine/internal/web/search/providers"

// FinalizeOutput applies the shared normalization, filtering, dedupe, and ranking
// used by the core search service so backend callers can stay behaviorally aligned.
func FinalizeOutput(input Input, providerOutput providers.ProviderOutput) Output {
	output := OutputFromProvider(input, providerOutput)
	output.Results = rankResults(input.Query, filterAndDedupeResults(output.Results, input))
	return output
}
