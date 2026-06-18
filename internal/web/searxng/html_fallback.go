package searxng

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// parseHTMLSearchResults extracts search results from a SearXNG HTML response.
// Mirrors parseHtmlSearchResults from search.ts.
//
// SearXNG HTML structure:
//
//	<article class="result">
//	  <h3><a href="URL">Title</a></h3>
//	  <p class="content">Snippet</p>
//	</article>
func parseHTMLSearchResults(html, query string) (*SearchResponse, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	results := make([]WebResult, 0)

	// Try article.result first (standard SearXNG), fall back to .result
	sel := doc.Find("article.result")
	if sel.Length() == 0 {
		sel = doc.Find(".result")
	}

	sel.Each(func(_ int, s *goquery.Selection) {
		// Mirrors: querySelector("h3 > a") ?? querySelector("h3 a") ?? querySelector("a[href]")
		link := s.Find("h3 > a").First()
		if link.Length() == 0 {
			link = s.Find("h3 a").First()
		}
		if link.Length() == 0 {
			link = s.Find("a[href]").First()
		}
		if link.Length() == 0 {
			return
		}

		href, exists := link.Attr("href")
		if !exists || strings.TrimSpace(href) == "" {
			return
		}

		// Validate that href is a parseable URL (skip anchor-only links, etc.)
		if !strings.HasPrefix(href, "http://") && !strings.HasPrefix(href, "https://") {
			return
		}

		title := normalizeHTMLText(link.Text())

		snippet := s.Find("p.content").First()
		if snippet.Length() == 0 {
			snippet = s.Find(".content").First()
		}
		content := normalizeHTMLText(snippet.Text())

		results = append(results, WebResult{
			Title:   title,
			URL:     href,
			Content: content,
		})
	})

	return &SearchResponse{
		Query:           query,
		NumberOfResults: len(results),
		Results:         results,
		SourceFormat:    "html",
	}, nil
}

// normalizeHTMLText collapses whitespace, matching normalizeHtmlText from search.ts.
func normalizeHTMLText(s string) string {
	// Replace all whitespace sequences with a single space and trim edges.
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}
