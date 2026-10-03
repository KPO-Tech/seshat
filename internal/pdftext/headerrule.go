package pdftext

import "math"

// A header-rule table has one rule, under its header, and nothing else: no rule above, none below, no vertical
// rules. Invoices and statements are full of them (a row of column titles underlined, then the lines). A single
// rule is not a table by itself, so one is looked for only where the text says so: a row of at least three chunks
// just above the rule, and lines below it that run on without a gap, which the usual guards then judge.

const (
	minHeaderChunks   = 3
	minHeaderRuleSpan = 60.0 // a shorter rule is an underline, not the rule of a table
	maxBodyGapEm      = 1.9  // a gap between lines bigger than this, in text sizes, ends the table
)

// headerRuleRegions returns a region for each rule that has a header row above it and rows under it, leaving out
// the rules that are already inside a table built from the rulings.
func headerRuleRegions(rul rulings, glyphs []glyph, built []foundTable) []tableRegion {
	var regions []tableRegion
	for _, h := range rul.h {
		if h.x1-h.x0 < minHeaderRuleSpan || insideTables(h, built) {
			continue
		}
		var sel []glyph
		for _, g := range glyphs {
			if cx := g.gx + g.gw/2; !g.isJunk() && cx >= h.x0-edgeSlack && cx <= h.x1+edgeSlack {
				sel = append(sel, g)
			}
		}
		if reg, ok := headerRuleRegion(h, textRows(sel)); ok {
			regions = append(regions, reg)
		}
	}
	return regions
}

func insideTables(h hline, tables []foundTable) bool {
	for _, t := range tables {
		if h.y >= t.y0-2 && h.y <= t.y1+2 && math.Min(h.x1, t.x1)-math.Max(h.x0, t.x0) > 0.5*(h.x1-h.x0) {
			return true
		}
	}
	return false
}

func headerRuleRegion(h hline, rows []textRow) (tableRegion, bool) {
	header := -1
	for i, r := range rows {
		above := r.y - h.y // the baseline is just over the rule
		if above >= -0.5 && above <= 0.8*r.size+2 {
			header = i
		}
	}
	if header < 0 || len(rows[header].chunks) < minHeaderChunks {
		return tableRegion{}, false
	}
	head := rows[header]
	width := h.x1 - h.x0

	var body []textRow
	for _, r := range rows[header+1:] {
		prev := head.y
		if len(body) > 0 {
			prev = body[len(body)-1].y
		}
		limit := maxBodyGapEm * math.Max(head.size, r.size)
		if len(body) == 0 {
			limit += head.y - h.y
		}
		if prev-r.y > limit || proseRow(r, width) {
			break
		}
		body = append(body, r)
	}
	if len(body) < minBlockRows-1 {
		return tableRegion{}, false
	}
	last := body[len(body)-1]
	return tableRegion{
		x0: h.x0, x1: h.x1, y1: head.y + head.size, y0: last.y - 1,
		horizontal: []hline{h}, headerRule: true,
	}, true
}

// proseRow says whether a row is one long chunk across most of the width: running text, which no table has.
func proseRow(r textRow, width float64) bool {
	if len(r.chunks) != 1 {
		return false
	}
	c := r.chunks[0]
	return len([]rune(c.text)) >= 35 && c.x1-c.x0 >= 0.85*width
}
