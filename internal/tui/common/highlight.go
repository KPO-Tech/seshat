package common

import (
	"bytes"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	chromastyles "github.com/alecthomas/chroma/v2/styles"
)

// SyntaxHighlight applies chroma syntax highlighting to source based on fileName.
// Returns the original source unchanged on failure so callers never get blank output.
func SyntaxHighlight(source, fileName string) string {
	if strings.TrimSpace(source) == "" {
		return source
	}

	l := lexers.Match(fileName)
	if l == nil {
		l = lexers.Analyse(source)
	}
	if l == nil {
		return source
	}
	l = chroma.Coalesce(l)

	f := formatters.Get("terminal16m")
	if f == nil {
		f = formatters.Fallback
	}

	style := chromastyles.Get("dracula")
	if style == nil {
		style = chromastyles.Fallback
	}

	it, err := l.Tokenise(nil, source)
	if err != nil {
		return source
	}

	var buf bytes.Buffer
	if err := f.Format(&buf, style, it); err != nil {
		return source
	}
	return strings.TrimRight(buf.String(), "\n")
}
