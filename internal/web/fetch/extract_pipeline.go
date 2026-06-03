package fetch

import (
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	readability "github.com/go-shiori/go-readability"
)

// extractHTMLContent runs the shared HTML extraction pipeline.
// The order is deliberate:
// 1. readability for article-like pages and docs with a clear main content block
// 2. goquery subtree selection for pages that are still parseable but not article-shaped
// 3. raw fragment rendering as the final fallback
func extractHTMLContent(raw string, baseURL string) string {
	if content := extractReadableHTML(raw, baseURL); content != "" {
		return content
	}
	if content := extractStructuredFallback(raw); content != "" {
		return content
	}
	return renderHTMLFragment(raw)
}

func extractReadableHTML(raw string, baseURL string) string {
	parsedURL, _ := url.Parse(strings.TrimSpace(baseURL))
	article, err := readability.FromReader(strings.NewReader(raw), parsedURL)
	if err != nil {
		return ""
	}

	body := cleanWhitespace(renderHTMLFragment(article.Content))
	if body == "" {
		body = cleanWhitespace(article.TextContent)
	}
	if len(body) < 160 && cleanWhitespace(article.Excerpt) == "" {
		return ""
	}

	parts := make([]string, 0, 3)
	title := cleanWhitespace(article.Title)
	excerpt := cleanWhitespace(article.Excerpt)
	if title != "" && !containsFold(body, title) {
		parts = append(parts, "# "+title)
	}
	if excerpt != "" && excerpt != title && !containsFold(body, excerpt) {
		parts = append(parts, excerpt)
	}
	if body != "" {
		parts = append(parts, body)
	}
	return cleanWhitespace(strings.Join(parts, "\n\n"))
}

func extractStructuredFallback(raw string) string {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(raw))
	if err != nil {
		return ""
	}

	// Strip layout chrome early so the fallback selection does not overvalue menus,
	// cookie banners, and other repetitive scaffolding.
	doc.Find("script,style,noscript,svg,canvas,iframe,nav,footer,aside,form,dialog").Each(func(_ int, sel *goquery.Selection) {
		sel.Remove()
	})

	root := selectBestContentRoot(doc)
	if root == nil || root.Length() == 0 {
		root = doc.Find("body").First()
	}
	if root == nil || root.Length() == 0 {
		return ""
	}

	htmlFragment, err := root.Html()
	if err != nil {
		return cleanWhitespace(root.Text())
	}
	if rendered := cleanWhitespace(renderHTMLFragment(htmlFragment)); rendered != "" {
		return rendered
	}
	return cleanWhitespace(root.Text())
}

func selectBestContentRoot(doc *goquery.Document) *goquery.Selection {
	if doc == nil {
		return nil
	}

	selectors := []string{
		"article",
		"main",
		`[role="main"]`,
		"#content",
		".content",
		".article",
		".post",
		".markdown-body",
		".documentation",
		".docs",
		".docMainContainer",
	}

	var best *goquery.Selection
	bestScore := 0
	for _, selector := range selectors {
		doc.Find(selector).Each(func(_ int, sel *goquery.Selection) {
			score := len(cleanWhitespace(sel.Text()))
			if score > bestScore {
				best = sel
				bestScore = score
			}
		})
	}

	if best != nil && bestScore >= 120 {
		return best
	}
	return doc.Find("body").First()
}

func containsFold(haystack string, needle string) bool {
	haystack = strings.TrimSpace(strings.ToLower(haystack))
	needle = strings.TrimSpace(strings.ToLower(needle))
	return needle != "" && strings.Contains(haystack, needle)
}
