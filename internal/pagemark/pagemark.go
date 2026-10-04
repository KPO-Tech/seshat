// Package pagemark is the one place that knows how the pages of a document are marked in the markdown that goes to
// the chunkers.
//
// A PDF is read page by page, and a chunk must be able to say which pages it comes from (for a citation, or to open
// the page). The text that is indexed therefore has, before the text of each page, a line that an HTML comment
// makes invisible to anyone who renders the markdown:
//
//	<!-- page 3 -->
//
// The chunkers read these lines, take them out of the text, and put the pages a chunk covers in its metadata.
package pagemark

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	prefix = "<!-- page "
	suffix = " -->"
)

// Marker is the line that starts page n (counting from 1).
func Marker(page int) string { return fmt.Sprintf("%s%d%s", prefix, page, suffix) }

// Parse says whether a line is a page marker, and which page it starts.
func Parse(line string) (int, bool) {
	line = strings.TrimSpace(line)
	if len(line) < len(prefix)+len(suffix) || !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, suffix) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(line[len(prefix) : len(line)-len(suffix)]))
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

// Strip takes the page markers out of a markdown text, and the blank line that follows each.
func Strip(markdown string) string {
	if !strings.Contains(markdown, prefix) {
		return markdown
	}
	lines := strings.Split(markdown, "\n")
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		if _, ok := Parse(lines[i]); ok {
			if i+1 < len(lines) && strings.TrimSpace(lines[i+1]) == "" {
				i++
			}
			continue
		}
		out = append(out, lines[i])
	}
	return strings.Join(out, "\n")
}
