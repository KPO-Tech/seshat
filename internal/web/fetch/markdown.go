package fetch

import (
	"io"
	"strings"
)

// HTMLToMarkdown converts HTML content into a compact markdown-like representation.
// The V1 pipeline prefers readability-style main-content extraction, then falls back
// to structural DOM selection, and only then uses the raw renderer.
func HTMLToMarkdown(html string, baseURL string) string {
	return extractHTMLContent(html, baseURL)
}

func cleanWhitespace(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	cleaned := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if blank {
				continue
			}
			blank = true
			cleaned = append(cleaned, "")
			continue
		}
		blank = false
		cleaned = append(cleaned, line)
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

// ReadCloser wraps a byte slice as an io.ReadCloser
type ReadCloser struct {
	reader io.Reader
}

func (rc *ReadCloser) Read(p []byte) (n int, err error) {
	return rc.reader.Read(p)
}

func (rc *ReadCloser) Close() error {
	return nil
}

// NewReadCloser creates a new ReadCloser from bytes
func NewReadCloser(data []byte) io.ReadCloser {
	return &ReadCloser{strings.NewReader(string(data))}
}
