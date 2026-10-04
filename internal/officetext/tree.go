// Package officetext extracts plain/markdown text natively from DOCX, PPTX
// and XLSX files - no external service required. It exists so the read_file
// tool (and seshat-ai's upload/RAG ingestion paths) don't have to depend on
// docling-serve for formats that are just zipped XML: DOCX and PPTX share an
// OOXML document-tree shape (word/document.xml, ppt/slides/slideN.xml), so
// both walk the same generic tree in tree.go. XLSX is a distinct enough format
// (rows/columns/shared strings/formulas) that it's handled by excelize instead
// (see xlsx.go).
package officetext

import (
	"encoding/xml"
	"io"
)

// node is a namespace-agnostic in-memory XML tree. OOXML mixes several XML
// namespaces (w:, a:, p:, r:...) but element local names are unambiguous for
// what we care about (p, tbl, tr, tc, t, ...), so matching on Local alone
// keeps the walker simple without losing correctness for text extraction.
type node struct {
	Local    string
	Attrs    map[string]string
	rawAttrs []xml.Attr
	Children []*node
	Text     string
}

// attr looks up an attribute by local name (ignoring its namespace prefix,
// e.g. "val" matches both w:val and a:val). When two attributes share a
// local name but differ by namespace prefix (e.g. plain "id" vs "r:id" on
// <p:sldId>), the last one wins - use nsAttr to disambiguate those.
func (n *node) attr(local string) string {
	if n.Attrs == nil {
		return ""
	}
	return n.Attrs[local]
}

// nsAttr looks up an attribute by local name that must carry a non-empty
// (prefixed) namespace, e.g. r:id on <p:sldId r:id="rId2">. Unlike attr,
// this can't collide with a same-named unprefixed attribute.
func (n *node) nsAttr(local string) string {
	for _, a := range n.rawAttrs {
		if a.Name.Local == local && a.Name.Space != "" {
			return a.Value
		}
	}
	return ""
}

// child returns the first direct child with the given local name, or nil.
func (n *node) child(local string) *node {
	for _, c := range n.Children {
		if c.Local == local {
			return c
		}
	}
	return nil
}

// childrenNamed returns the direct children with the given local name, in order.
func (n *node) childrenNamed(local string) []*node {
	var out []*node
	for _, c := range n.Children {
		if c.Local == local {
			out = append(out, c)
		}
	}
	return out
}

// parseXMLTree reads an entire XML document into a generic node tree. OOXML
// parts (document.xml, slideN.xml) are small enough - these are text
// documents, not media - that buffering the whole tree is simpler and safer
// than a hand-rolled streaming/regex walk, and encoding/xml's tokenizer
// handles quoting, CDATA and malformed-adjacent-tag edge cases for free.
func parseXMLTree(r io.Reader) (*node, error) {
	dec := xml.NewDecoder(r)
	root := &node{Local: "#root"}
	stack := []*node{root}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			n := &node{Local: t.Name.Local, Attrs: make(map[string]string, len(t.Attr)), rawAttrs: append([]xml.Attr(nil), t.Attr...)}
			for _, a := range t.Attr {
				n.Attrs[a.Name.Local] = a.Value
			}
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, n)
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			stack[len(stack)-1].Text += string(t)
		}
	}

	return root, nil
}

// findFirst does a depth-first search for the first descendant with the
// given local name (including n itself).
func findFirst(n *node, local string) *node {
	if n.Local == local {
		return n
	}
	for _, c := range n.Children {
		if found := findFirst(c, local); found != nil {
			return found
		}
	}
	return nil
}

// findAll collects every descendant with the given local name, not
// recursing past the first match on any branch (so a table's own rows are
// found once, without also matching rows of a table nested inside a cell -
// nested tables are rare in practice and callers walk them separately when
// they recurse into a cell's children).
func findAll(n *node, local string) []*node {
	var out []*node
	for _, c := range n.Children {
		if c.Local == local {
			out = append(out, c)
			continue
		}
		out = append(out, findAll(c, local)...)
	}
	return out
}
