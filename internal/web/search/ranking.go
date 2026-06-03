package search

import (
	"net/url"
	"sort"
	"strings"

	webcore "github.com/EngineerProjects/nexus-engine/internal/web"
)

func rankResults(query string, hits []webcore.SearchResult) []webcore.SearchResult {
	queryTerms := strings.Fields(strings.ToLower(query))
	scored := make([]scoredResult, 0, len(hits))
	for _, hit := range hits {
		score := 0
		title := strings.ToLower(hit.Title)
		description := strings.ToLower(hit.Description)
		host := ""
		if parsed, err := url.Parse(hit.URL); err == nil {
			host = strings.ToLower(parsed.Hostname())
		}
		for _, term := range queryTerms {
			if strings.Contains(title, term) {
				score += 5
			}
			if strings.Contains(description, term) {
				score += 2
			}
			if strings.Contains(host, term) {
				score++
			}
		}
		if hit.URL != "" {
			score++
		}
		scored = append(scored, scoredResult{result: hit, score: score})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].result.URL < scored[j].result.URL
		}
		return scored[i].score > scored[j].score
	})

	ranked := make([]webcore.SearchResult, 0, len(scored))
	for _, item := range scored {
		ranked = append(ranked, item.result)
	}
	return ranked
}

type scoredResult struct {
	result webcore.SearchResult
	score  int
}
