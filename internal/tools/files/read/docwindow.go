package read

import (
	"fmt"
	"strings"
)

// DocumentMaxCharsPerRead bounds the text of one read of a converted document (DOCX, PPTX, XLSX, ...), as
// PDFMaxCharsPerRead does for a PDF: a long document is read in pieces, and the result says where to continue.
const DocumentMaxCharsPerRead = 120_000

// documentWindow is the part of a document's markdown one read returns.
type documentWindow struct {
	Text  string
	First int // first line returned, counting from 1
	Last  int // last line returned
	Total int // lines in the document
	// Cut is set when one line alone was longer than the size limit and was cut.
	Cut bool
}

// Partial says whether the read stopped short of the whole document.
func (w documentWindow) Partial() bool { return w.First > 1 || w.Last < w.Total || w.Cut }

// windowDocument takes lines from markdown, starting at offset (counting from 1; a negative offset counts from the
// end, as for a text file) and stopping after limit lines (zero means no line limit) or at maxChars characters,
// whichever comes first. It stops at the end of a line, and always returns at least one line.
func windowDocument(markdown string, offset, limit, maxChars int) documentWindow {
	lines := strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n")
	total := len(lines)
	switch {
	case offset < 0:
		offset = max(total+offset+1, 1)
	case offset < 1:
		offset = 1
	}
	if offset > total {
		return documentWindow{First: total + 1, Last: total, Total: total}
	}
	if maxChars <= 0 {
		maxChars = DocumentMaxCharsPerRead
	}

	w := documentWindow{First: offset, Total: total}
	var sb strings.Builder
	for i := offset - 1; i < total; i++ {
		if limit > 0 && i-(offset-1) >= limit {
			break
		}
		line := lines[i]
		if sb.Len() > 0 && sb.Len()+len(line)+1 > maxChars {
			break
		}
		if sb.Len() == 0 && len(line) > maxChars {
			line = strings.ToValidUTF8(line[:maxChars], "")
			w.Cut = true
		}
		if i > offset-1 {
			sb.WriteByte('\n')
		}
		sb.WriteString(line)
		w.Last = i + 1
	}
	w.Text = sb.String()
	return w
}

// continuation tells the agent how to read on, or "" when the read reached the end.
func (w documentWindow) continuation() string {
	if !w.Partial() {
		return ""
	}
	note := fmt.Sprintf("Showing lines %d-%d of %d.", w.First, w.Last, w.Total)
	if w.Cut {
		note += " One line was longer than the size limit and was cut."
	}
	if w.Last < w.Total {
		note += fmt.Sprintf(" Read the file again with offset=%d to continue.", w.Last+1)
	}
	return note
}
