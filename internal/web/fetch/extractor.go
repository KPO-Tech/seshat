package fetch

import (
	"html"
	"strings"

	xhtml "golang.org/x/net/html"
)

var ignoredTags = map[string]bool{
	"script":   true,
	"style":    true,
	"noscript": true,
	"svg":      true,
	"canvas":   true,
	"iframe":   true,
}

var boilerplateTags = map[string]bool{
	"nav":    true,
	"footer": true,
	"aside":  true,
	"form":   true,
}

type markdownRenderer struct {
	builder strings.Builder
}

// renderHTMLFragment converts an HTML fragment into compact markdown-like text.
// It is intentionally conservative and sits behind higher-level extractors that
// decide which DOM subtree is the most valuable for LLM consumption.
func renderHTMLFragment(raw string) string {
	doc, err := xhtml.Parse(strings.NewReader(raw))
	if err != nil {
		return cleanWhitespace(raw)
	}

	root := findPreferredContentNode(doc)
	if root == nil {
		root = doc
	}

	renderer := &markdownRenderer{}
	renderer.renderNode(root, 0)
	return cleanWhitespace(renderer.builder.String())
}

func findPreferredContentNode(node *xhtml.Node) *xhtml.Node {
	var best *xhtml.Node
	var walk func(*xhtml.Node)
	walk = func(current *xhtml.Node) {
		if current == nil || best != nil {
			return
		}
		if current.Type == xhtml.ElementNode {
			tag := strings.ToLower(current.Data)
			if tag == "article" || tag == "main" {
				best = current
				return
			}
			if attr(current, "role") == "main" || likelyContentNode(current) {
				best = current
				return
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
			if best != nil {
				return
			}
		}
	}
	walk(node)
	return best
}

func likelyContentNode(node *xhtml.Node) bool {
	className := strings.ToLower(attr(node, "class") + " " + attr(node, "id"))
	for _, marker := range []string{"content", "article", "post", "markdown", "docs", "documentation"} {
		if strings.Contains(className, marker) {
			return true
		}
	}
	return false
}

func attr(node *xhtml.Node, key string) string {
	for _, item := range node.Attr {
		if strings.EqualFold(item.Key, key) {
			return strings.TrimSpace(item.Val)
		}
	}
	return ""
}

func (r *markdownRenderer) renderNode(node *xhtml.Node, depth int) {
	if node == nil {
		return
	}
	switch node.Type {
	case xhtml.TextNode:
		r.writeText(node.Data)
		return
	case xhtml.ElementNode:
		tag := strings.ToLower(node.Data)
		if ignoredTags[tag] || boilerplateTags[tag] {
			return
		}
		switch tag {
		case "h1", "h2", "h3", "h4", "h5", "h6":
			level := int(tag[1] - '0')
			r.writeBlock(strings.Repeat("#", level) + " " + textContent(node))
			return
		case "pre":
			r.writeBlock("```\n" + strings.TrimSpace(textContent(node)) + "\n```")
			return
		case "blockquote":
			text := strings.TrimSpace(textContent(node))
			if text != "" {
				lines := strings.Split(text, "\n")
				for i, line := range lines {
					lines[i] = "> " + strings.TrimSpace(line)
				}
				r.writeBlock(strings.Join(lines, "\n"))
			}
			return
		case "li":
			r.writeBlock("- " + strings.TrimSpace(textContent(node)))
			return
		case "br":
			r.builder.WriteByte('\n')
			return
		case "hr":
			r.writeBlock("---")
			return
		case "img":
			alt := attr(node, "alt")
			src := attr(node, "src")
			if src != "" {
				r.writeBlock("![" + alt + "](" + src + ")")
			}
			return
		case "a":
			label := strings.TrimSpace(textContent(node))
			href := attr(node, "href")
			if label != "" && href != "" {
				r.writeInline("[" + label + "](" + href + ")")
				return
			}
		case "code":
			r.writeInline("`" + strings.TrimSpace(textContent(node)) + "`")
			return
		case "strong", "b":
			text := strings.TrimSpace(textContent(node))
			if text != "" {
				r.writeInline("**" + text + "**")
			}
			return
		case "em", "i":
			text := strings.TrimSpace(textContent(node))
			if text != "" {
				r.writeInline("*" + text + "*")
			}
			return
		case "p", "div", "section", "article", "main":
			text := strings.TrimSpace(textContent(node))
			if text != "" {
				r.writeBlock(text)
			}
			return
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		r.renderNode(child, depth+1)
	}
}

func (r *markdownRenderer) writeText(value string) {
	text := normalizeText(value)
	if text == "" {
		return
	}
	r.builder.WriteString(text)
	r.builder.WriteByte(' ')
}

func (r *markdownRenderer) writeInline(value string) {
	value = normalizeText(value)
	if value == "" {
		return
	}
	r.builder.WriteString(value)
	r.builder.WriteByte(' ')
}

func (r *markdownRenderer) writeBlock(value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if r.builder.Len() > 0 {
		r.builder.WriteString("\n\n")
	}
	r.builder.WriteString(value)
}

func textContent(node *xhtml.Node) string {
	var builder strings.Builder
	var walk func(*xhtml.Node)
	walk = func(current *xhtml.Node) {
		if current == nil {
			return
		}
		if current.Type == xhtml.TextNode {
			builder.WriteString(current.Data)
			builder.WriteByte(' ')
		}
		if current.Type == xhtml.ElementNode {
			tag := strings.ToLower(current.Data)
			if ignoredTags[tag] || boilerplateTags[tag] {
				return
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return normalizeText(builder.String())
}

func normalizeText(value string) string {
	value = html.UnescapeString(value)
	return strings.Join(strings.Fields(value), " ")
}
