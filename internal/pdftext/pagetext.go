package pdftext

import "strings"

// Thresholds, as a share of the font size. A word space is about a quarter of an em, kerning and
// tracking are a few hundredths, so the gap that means "a space was meant here" sits between them.
const (
	wordGap     = 0.15 // a gap wider than this is a word break
	lineShift   = 0.4  // a vertical move larger than this starts a new line (a sub or superscript moves less)
	backwardGap = 1.0  // moving back left by more than this on the same line is a new line, not a gap
)

func trimTrailingSpace(sb *strings.Builder) {
	s := sb.String()
	trimmed := strings.TrimRight(s, " ")
	if len(trimmed) != len(s) {
		sb.Reset()
		sb.WriteString(trimmed)
	}
}
