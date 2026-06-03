// Package search contains the reusable web search core shared by tool wrappers.
package search

import (
	"fmt"

	webcore "github.com/EngineerProjects/nexus-engine/internal/web"
	"github.com/EngineerProjects/nexus-engine/internal/web/search/providers"
)

// Input represents a normalized search request for the shared search core.
type Input struct {
	Query          string
	AllowedDomains []string
	BlockedDomains []string
}

// Output represents a normalized search response produced by the shared search core.
type Output = webcore.SearchResponse

// Validate enforces the shared constraints used by every search caller.
func (i *Input) Validate() error {
	if len(i.Query) < 2 {
		return fmt.Errorf("query must be at least 2 characters")
	}
	if len(i.AllowedDomains) > 0 && len(i.BlockedDomains) > 0 {
		return fmt.Errorf("cannot specify both allowed_domains and blocked_domains")
	}
	return nil
}

// Request converts the local search input into the shared request type used across the web stack.
func (i Input) Request() webcore.SearchRequest {
	return webcore.SearchRequest{
		Query:          i.Query,
		AllowedDomains: append([]string(nil), i.AllowedDomains...),
		BlockedDomains: append([]string(nil), i.BlockedDomains...),
	}
}

// OutputFromProvider adapts provider-native hits into the shared search response contract.
func OutputFromProvider(input Input, output providers.ProviderOutput) Output {
	results := make([]webcore.SearchResult, 0, len(output.Hits))
	for _, hit := range output.Hits {
		results = append(results, webcore.SearchResult{
			Title:       hit.Title,
			URL:         hit.URL,
			Description: hit.Description,
			Source:      hit.Source,
		})
	}
	return Output{
		Query:           input.Query,
		Results:         results,
		Provider:        output.ProviderName,
		DurationSeconds: output.DurationSeconds,
	}
}
