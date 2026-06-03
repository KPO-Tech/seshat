package web

import internalweb "github.com/EngineerProjects/nexus-engine/internal/web"

type (
	DomainCategory = internalweb.DomainCategory
	SearchRequest  = internalweb.SearchRequest
	SearchResult   = internalweb.SearchResult
	SearchResponse = internalweb.SearchResponse
)

func DomainCatalog() []DomainCategory {
	return internalweb.DomainCatalog()
}
