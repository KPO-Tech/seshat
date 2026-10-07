package web

import internalweb "github.com/KPO-Tech/seshat/internal/web"

type (
	// DomainCategory groups related domains for the frontend domain catalog UI.
	DomainCategory = internalweb.DomainCategory
	// SearchRequest is the common input shape for reusable web search services.
	SearchRequest = internalweb.SearchRequest
	// SearchResult is a provider-agnostic search hit used across wrappers and services.
	SearchResult = internalweb.SearchResult
	// SearchResponse is the normalized output shape shared by search callers.
	SearchResponse = internalweb.SearchResponse
)

// DomainCatalog returns the curated pre-approved domain list organized by category.
func DomainCatalog() []DomainCategory {
	return internalweb.DomainCatalog()
}
