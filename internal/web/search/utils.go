package search

import (
	"net/url"
	"sort"
	"strings"

	webcore "github.com/EngineerProjects/nexus-engine/internal/web"
	"github.com/EngineerProjects/nexus-engine/internal/web/search/providers"
)

// normalizePayloadToHits converts a generic payload to search hits.
func normalizePayloadToHits(payload any) []providers.SearchHit {
	hits := extractHits(payload)
	return dedupeHits(hits)
}

// extractHits recursively extracts search hits from a payload.
func extractHits(value any) []providers.SearchHit {
	switch v := value.(type) {
	case map[string]any:
		if hits, ok := extractFromKnownContainers(v); ok {
			return hits
		}
		if hit, ok := normalizeHit(v); ok {
			return []providers.SearchHit{hit}
		}
	case []any:
		results := make([]providers.SearchHit, 0)
		for _, item := range v {
			results = append(results, extractHits(item)...)
		}
		return results
	}
	return nil
}

// extractFromKnownContainers tries to find hits in known container keys.
func extractFromKnownContainers(m map[string]any) ([]providers.SearchHit, bool) {
	for _, key := range []string{"results", "items", "hits", "data", "web"} {
		if value, ok := m[key]; ok {
			hits := extractHits(value)
			if len(hits) > 0 {
				return hits, true
			}
		}
	}
	return nil, false
}

// normalizeHit converts a map to a SearchHit if it has required fields.
func normalizeHit(m map[string]any) (providers.SearchHit, bool) {
	title := firstString(m, "title", "headline", "name")
	link := firstString(m, "url", "link", "href", "uri")
	if title == "" && link == "" {
		return providers.SearchHit{}, false
	}
	if title == "" {
		title = link
	}
	if link == "" {
		return providers.SearchHit{}, false
	}
	return providers.SearchHit{
		Title:       title,
		URL:         link,
		Description: firstString(m, "description", "snippet", "summary", "content", "text"),
		Source:      firstString(m, "source", "domain", "engine", "displayLink"),
	}, true
}

// firstString returns the first non-empty string value for the given keys.
func firstString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// dedupeHits removes duplicate URLs from the hits.
func dedupeHits(hits []providers.SearchHit) []providers.SearchHit {
	seen := make(map[string]bool)
	result := make([]providers.SearchHit, 0, len(hits))
	for _, hit := range hits {
		if seen[hit.URL] {
			continue
		}
		seen[hit.URL] = true
		result = append(result, hit)
	}
	return result
}

func searchCacheKey(input Input) string {
	allowed := append([]string(nil), input.AllowedDomains...)
	blocked := append([]string(nil), input.BlockedDomains...)
	sort.Strings(allowed)
	sort.Strings(blocked)

	var builder strings.Builder
	builder.WriteString(strings.TrimSpace(strings.ToLower(input.Query)))
	builder.WriteString("|allow:")
	builder.WriteString(strings.Join(allowed, ","))
	builder.WriteString("|block:")
	builder.WriteString(strings.Join(blocked, ","))
	return builder.String()
}

// applyDomainFilters filters hits based on allowed/blocked domains.
func applyDomainFilters(hits []providers.SearchHit, input providers.SearchInput) []providers.SearchHit {
	filtered := hits
	if len(input.BlockedDomains) > 0 {
		filtered = filterHits(filtered, func(host string) bool {
			for _, domain := range input.BlockedDomains {
				if webcore.HostMatchesDomain(host, domain) {
					return false
				}
			}
			return true
		})
	}
	if len(input.AllowedDomains) > 0 {
		filtered = filterHits(filtered, func(host string) bool {
			for _, domain := range input.AllowedDomains {
				if webcore.HostMatchesDomain(host, domain) {
					return true
				}
			}
			return false
		})
	}
	return filtered
}

// filterHits filters hits based on a predicate function.
func filterHits(hits []providers.SearchHit, keep func(host string) bool) []providers.SearchHit {
	result := make([]providers.SearchHit, 0, len(hits))
	for _, hit := range hits {
		host := safeHostname(hit.URL)
		if host == "" {
			continue
		}
		if keep(host) {
			result = append(result, hit)
		}
	}
	return result
}

func filterAndDedupeResults(results []webcore.SearchResult, input Input) []webcore.SearchResult {
	seen := make(map[string]bool)
	filtered := make([]webcore.SearchResult, 0, len(results))
	for _, hit := range results {
		host := safeHostname(hit.URL)
		if host == "" || !keepHost(host, input) {
			continue
		}
		if seen[hit.URL] {
			continue
		}
		seen[hit.URL] = true
		filtered = append(filtered, hit)
	}
	return filtered
}

func keepHost(host string, input Input) bool {
	if len(input.BlockedDomains) > 0 {
		for _, domain := range input.BlockedDomains {
			if webcore.HostMatchesDomain(host, domain) {
				return false
			}
		}
	}
	if len(input.AllowedDomains) == 0 {
		return true
	}
	for _, domain := range input.AllowedDomains {
		if webcore.HostMatchesDomain(host, domain) {
			return true
		}
	}
	return false
}

// safeHostname extracts the hostname from a URL.
func safeHostname(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}
