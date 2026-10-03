// Package htmltext turns an HTML document into markdown that reads like the page does: headings, paragraphs,
// nested lists, tables with their merged cells, code blocks, quotations, links and the description of pictures,
// without the scripts, styles, navigation and hidden elements around them.
//
// It is for HTML as a document someone attached or a knowledge base ingests (a saved article, an exported
// report, a documentation page). It is not the way to read HTML as source code, where the tags are the point.
package htmltext

import (
	"bytes"
	"io"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

// Result is a converted document.
type Result struct {
	Markdown string
	Title    string // the <title>, also written as the first heading when the page has none of its own
}

const (
	maxDepth = 256 // deeper than this is not a document; it is a page built to exhaust the stack
	// minMainText is how much text a <main> or <article> must hold to be taken as the page's content rather
	// than a wrapper around a fragment of it.
	minMainText = 200
)

// Convert reads an HTML document in any encoding it declares (or UTF-8) and returns its markdown.
func Convert(data []byte) (Result, error) {
	reader, err := charset.NewReader(bytes.NewReader(data), "")
	if err != nil {
		reader = bytes.NewReader(data)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil || !utf8.Valid(decoded) {
		decoded = bytes.ToValidUTF8(data, []byte("�"))
	}
	doc, err := html.Parse(bytes.NewReader(decoded))
	if err != nil {
		// Too deeply nested for the parser to build a tree (it refuses past 512 open elements): not a document
		// a person wrote, but its text is still its text.
		return Result{Markdown: strings.TrimSpace(plainText(decoded)) + "\n"}, nil
	}

	c := &converter{}
	title := c.title(doc)
	root := contentRoot(doc)
	blocks := c.blocks(root, 0)
	markdown := strings.TrimSpace(strings.Join(blocks, "\n\n"))
	if title != "" && !c.sawH1 {
		markdown = "# " + title + "\n\n" + markdown
	}
	return Result{Markdown: strings.TrimSpace(markdown) + "\n", Title: title}, nil
}

// contentRoot picks where the content is: the page's <main> or <article> when it holds real text, else the body.
func contentRoot(doc *html.Node) *html.Node {
	body := find(doc, func(n *html.Node) bool { return isElement(n, "body") })
	if body == nil {
		return doc
	}
	candidates := []func(*html.Node) bool{
		func(n *html.Node) bool { return isElement(n, "main") },
		func(n *html.Node) bool { return isElement(n, "") && attr(n, "role") == "main" },
		func(n *html.Node) bool { return isElement(n, "article") },
	}
	for _, match := range candidates {
		if node := find(body, match); node != nil && len(strings.TrimSpace(textOf(node))) >= minMainText {
			return node
		}
	}
	return body
}

func find(n *html.Node, match func(*html.Node) bool) *html.Node {
	if match(n) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := find(c, match); found != nil {
			return found
		}
	}
	return nil
}

func isElement(n *html.Node, tag string) bool {
	return n != nil && n.Type == html.ElementNode && (tag == "" || n.Data == tag)
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

// textOf is the plain text under a node, for measuring and for headings and alt text.
func textOf(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
			return
		}
		if n.Type == html.ElementNode && skipped(n) {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return sb.String()
}

// skipped says whether a node and everything under it is left out: scripts and styles, the page's navigation,
// and whatever the page itself hides.
func skipped(n *html.Node) bool {
	switch n.Data {
	case "script", "style", "noscript", "template", "head", "svg", "canvas", "iframe", "object", "embed", "link", "meta", "nav", "button", "select", "input", "textarea", "audio", "video":
		return true
	}
	if hasAttr(n, "hidden") || attr(n, "aria-hidden") == "true" || attr(n, "role") == "navigation" {
		return true
	}
	for _, class := range strings.Fields(attr(n, "class")) {
		if chromeClasses[class] {
			return true
		}
	}
	style := strings.ReplaceAll(strings.ToLower(attr(n, "style")), " ", "")
	return strings.Contains(style, "display:none") || strings.Contains(style, "visibility:hidden")
}

// chromeClasses are class names that mark what surrounds a page's content rather than the content: edit links,
// language menus, navigation boxes, breadcrumbs, text meant only for screen readers. Whole class names only, so
// "toc-heading" in a report is not mistaken for "toc".
var chromeClasses = map[string]bool{
	"mw-editsection": true, "mw-jump-link": true, "navbox": true, "noprint": true, "catlinks": true, "printfooter": true,
	"interlanguage-link": true, "vector-dropdown": true, "vector-menu": true, "vector-toc": true, "mw-portlet": true,
	"breadcrumb": true, "breadcrumbs": true, "skip-link": true, "sr-only": true, "visually-hidden": true,
	"cookie-banner": true, "cookie-notice": true,
}

func normalizeSpace(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, " ", " ")), " ")
}

// plainText is the text of a page without building its tree: what is left when the markup cannot be parsed.
func plainText(data []byte) string {
	var sb strings.Builder
	z := html.NewTokenizer(bytes.NewReader(data))
	skip := 0
	for {
		switch z.Next() {
		case html.ErrorToken:
			return strings.Join(strings.Fields(sb.String()), " ")
		case html.StartTagToken:
			if name, _ := z.TagName(); string(name) == "script" || string(name) == "style" {
				skip++
			}
		case html.EndTagToken:
			if name, _ := z.TagName(); (string(name) == "script" || string(name) == "style") && skip > 0 {
				skip--
			}
		case html.TextToken:
			if skip == 0 {
				sb.Write(z.Text())
				sb.WriteByte(' ')
			}
		}
	}
}
