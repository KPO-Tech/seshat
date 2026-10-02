package pdftext

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
	"golang.org/x/text/unicode/norm"
)

// Thresholds, as a share of the font size. A word space is about a quarter of an em, kerning and
// tracking are a few hundredths, so the gap that means "a space was meant here" sits between them.
const (
	wordGap     = 0.15 // a gap wider than this is a word break
	lineShift   = 0.4  // a vertical move larger than this starts a new line (a sub or superscript moves less)
	backwardGap = 1.0  // moving back left by more than this on the same line is a new line, not a gap
)

// PageText returns a page's text with its word spaces and line breaks.
//
// The library's own GetPlainText concatenates the strings a PDF draws and ignores the gaps between them.
// Many PDFs (LaTeX output among them) draw every word separately and encode the space only as a gap, so
// that function returns "danslecasd'uneclassification" for a whole paragraph. Here the glyphs are read
// with their positions, in the order the PDF draws them (which keeps the reading order of columns), and
// a space or a line break is put back wherever the geometry says there was one.
func PageText(page pdf.Page) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			text, err = "", errors.New(fmt.Sprint(r))
		}
	}()
	if page.V.IsNull() {
		return "", nil
	}
	return layoutText(page.Content().Text), nil
}

func layoutText(glyphs []pdf.Text) string {
	var sb strings.Builder
	var prev *pdf.Text
	endX := 0.0 // where the last letter, with any accent drawn after it, ends
	pendingSpace := false

	for i := range glyphs {
		g := &glyphs[i]
		switch g.S {
		case "\n", "\r", "", "�":
			// The library appends a newline after each TJ and decodes it with the page's font, which
			// often gives U+FFFD. Neither is text.
			continue
		case " ", "\t", " ":
			pendingSpace = true
			continue
		}

		if r, _ := utf8.DecodeRuneInString(g.S); unicode.IsMark(r) {
			// An accent drawn as its own glyph belongs to the letter before it, whatever gap the font puts
			// between them. NFC below joins the two into one character.
			sb.WriteString(g.S)
			endX = math.Max(endX, g.X+g.W)
			continue
		}

		if prev != nil {
			size := math.Abs(g.FontSize)
			if size == 0 {
				size = math.Abs(prev.FontSize)
			}
			if size == 0 {
				size = 10
			}
			dy := math.Abs(g.Y - prev.Y)
			gap := g.X - endX
			switch {
			case dy > size*lineShift:
				trimTrailingSpace(&sb)
				sb.WriteByte('\n')
				pendingSpace = false
			case gap < -size*backwardGap:
				trimTrailingSpace(&sb)
				sb.WriteByte('\n')
				pendingSpace = false
			case gap > size*wordGap:
				pendingSpace = true
			}
		}
		if pendingSpace && sb.Len() > 0 && !strings.HasSuffix(sb.String(), "\n") {
			sb.WriteByte(' ')
		}
		pendingSpace = false
		sb.WriteString(g.S)
		prev = g
		endX = g.X + g.W
	}
	return norm.NFC.String(sb.String())
}

func trimTrailingSpace(sb *strings.Builder) {
	s := sb.String()
	trimmed := strings.TrimRight(s, " ")
	if len(trimmed) != len(s) {
		sb.Reset()
		sb.WriteString(trimmed)
	}
}
