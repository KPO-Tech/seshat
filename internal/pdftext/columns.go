package pdftext

import (
	"math"
	"sort"
)

// A page set in two columns has two left margins and two right edges. A line of the left column stops far short of the right
// edge of the page, which the paragraph rules read as "a list of short lines, keep its breaks": every line of a paper stayed a
// line of its own, and a word hyphenated at the end of a line was never joined.

// columns holds the left margin and the right edge of the body text, per column when the page has two.
type columns struct {
	split       float64 // the x between the two columns; zero on a page of one
	left, right [2]float64
}

// A column break needs a strip at least this wide, in body sizes, that no line of text crosses.
const columnGap = 0.8

// pageColumns finds the columns of the page from the long lines set in the body size (headings, captions and formulas do not
// tell where the text is). Two columns are two groups of lines with a strip between them that almost no line crosses.
func pageColumns(lines []*line, body float64) columns {
	margin, right := bodyMargins(lines, body)
	one := columns{left: [2]float64{margin, margin}, right: [2]float64{right, right}}

	var long []*line
	for _, l := range lines {
		if math.Abs(l.size-body) <= body*0.1 && len(l.glyphs) >= 30 && !isMonoLine(l) {
			long = append(long, l)
		}
	}
	if len(long) < 16 {
		return one
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for _, l := range long {
		lo, hi = math.Min(lo, l.x0), math.Max(hi, l.x1)
	}
	span := hi - lo
	if span <= 0 {
		return one
	}
	// The strip is looked for in the middle of the text, and may be crossed by a few lines (a full-width title or note).
	allowed := len(long) / 25
	from, to := lo+span*0.3, lo+span*0.7
	bestStart, bestLen, runStart := 0.0, 0.0, math.NaN()
	for x := from; x <= to; x++ {
		crossing := 0
		for _, l := range long {
			if l.x0 < x && l.x1 > x {
				crossing++
			}
		}
		if crossing <= allowed {
			if math.IsNaN(runStart) {
				runStart = x
			}
			if x-runStart > bestLen {
				bestStart, bestLen = runStart, x-runStart
			}
		} else {
			runStart = math.NaN()
		}
	}
	if bestLen < body*columnGap {
		return one
	}
	split := bestStart + bestLen/2
	var side [2][]*line
	for _, l := range long {
		if l.x0 < split && l.x1 > split {
			continue // crosses the strip: belongs to neither
		}
		if l.x0 >= split {
			side[1] = append(side[1], l)
		} else {
			side[0] = append(side[0], l)
		}
	}
	if len(side[0]) < 8 || len(side[1]) < 8 {
		return one
	}
	c := columns{split: split}
	for i, group := range side {
		c.left[i] = math.Inf(1)
		rights := make([]float64, 0, len(group))
		for _, l := range group {
			c.left[i] = math.Min(c.left[i], l.x0)
			rights = append(rights, l.x1)
		}
		sort.Float64s(rights)
		c.right[i] = rights[len(rights)*9/10]
	}
	return c
}

func (c columns) side(l *line) int {
	if c.split != 0 && l.x0 >= c.split {
		return 1
	}
	return 0
}

// margin is the left margin of the column the line is in.
func (c columns) margin(l *line) float64 { return c.left[c.side(l)] }

// rightEdge is the right edge of the text of the column the line is in.
func (c columns) rightEdge(l *line) float64 { return c.right[c.side(l)] }
