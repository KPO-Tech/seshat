package pdftext

import (
	"math"
	"sort"
)

// A table region is the part of a page a table occupies, found from the lines drawn around and through it. Two
// kinds of table are recognised, because they are what ruled tables look like:
//
//   - a grid: horizontal and vertical rulings that cross each other (a bordered table, or one with vertical
//     rules and header and bottom rules, as in many reports and papers);
//   - a ruled block: a few horizontal rules of the same width with the table's text between them (the
//     "top rule, header rule, bottom rule" tables that typesetting systems produce), whose columns are the
//     white gaps in the text.
//
// A table with no rules at all is not found: with nothing but the text to go on, two columns of prose and a
// borderless table cannot be told apart reliably, and a wrong table is worse than text left as it was.

type tableRegion struct {
	x0, y0, x1, y1 float64 // y1 is the top
	vertical       []vline // vertical rules inside the region, the column separators of a grid
	horizontal     []hline // horizontal rules inside the region, top to bottom
	grid           bool    // the columns come from vertical rules, not from gaps in the text
	headerRule     bool    // one rule under the header row and no other: see headerRuleRegions
}

const (
	joinTolerance  = 2.0  // rulings closer than this touch
	maxRulings     = 6000 // a page with more lines than this is a drawing, not a document with tables
	minRegionRules = 2
)

// findRegions returns the table regions of a page, from its (merged) rulings, left to right and top to bottom.
func findRegions(r rulings) []tableRegion {
	if len(r.h)+len(r.v) > maxRulings || len(r.h) < minRegionRules {
		return nil
	}
	var regions []tableRegion
	usedH := make([]bool, len(r.h))

	for _, comp := range components(r) {
		if distinctX(comp.v) >= 2 && len(comp.h) >= 2 {
			regions = append(regions, gridRegion(comp))
			for _, i := range comp.hIdx {
				usedH[i] = true
			}
		}
	}
	regions = append(regions, blockRegions(r.h, usedH)...)

	sort.Slice(regions, func(i, j int) bool {
		if math.Abs(regions[i].y1-regions[j].y1) > 4 {
			return regions[i].y1 > regions[j].y1
		}
		return regions[i].x0 < regions[j].x0
	})
	return regions
}

// distinctX counts the different x positions of vertical rulings: two pieces of one line are one.
func distinctX(vs []vline) int {
	xs := make([]float64, 0, len(vs))
	for _, v := range vs {
		xs = append(xs, v.x)
	}
	sort.Float64s(xs)
	n := 0
	for i, x := range xs {
		if i == 0 || x-xs[i-1] > 3 {
			n++
		}
	}
	return n
}

// rulingSet is a set of rulings connected through their crossings. hIdx are the indexes of its horizontal
// rulings in the page's list, so a ruling that is part of a grid is not used again.
type rulingSet struct {
	h    []hline
	v    []vline
	hIdx []int
}

// components groups rulings that cross each other (or touch) into connected sets.
func components(r rulings) []rulingSet {
	n := len(r.h) + len(r.v)
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	find := func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	for i, h := range r.h {
		for j, v := range r.v {
			if v.x >= h.x0-joinTolerance && v.x <= h.x1+joinTolerance && h.y >= v.y0-joinTolerance && h.y <= v.y1+joinTolerance {
				parent[find(i)] = find(len(r.h) + j)
			}
		}
	}
	groups := map[int]*rulingSet{}
	var order []int
	group := func(i int) *rulingSet {
		root := find(i)
		g := groups[root]
		if g == nil {
			g = &rulingSet{}
			groups[root] = g
			order = append(order, root)
		}
		return g
	}
	for i := range r.h {
		g := group(i)
		g.h = append(g.h, r.h[i])
		g.hIdx = append(g.hIdx, i)
	}
	for j := range r.v {
		g := group(len(r.h) + j)
		g.v = append(g.v, r.v[j])
	}
	out := make([]rulingSet, 0, len(order))
	for _, root := range order {
		out = append(out, *groups[root])
	}
	return out
}

func gridRegion(c rulingSet) tableRegion {
	reg := tableRegion{x0: math.Inf(1), y0: math.Inf(1), x1: math.Inf(-1), y1: math.Inf(-1), grid: true}
	for _, h := range c.h {
		reg.x0, reg.x1 = math.Min(reg.x0, h.x0), math.Max(reg.x1, h.x1)
		reg.y0, reg.y1 = math.Min(reg.y0, h.y), math.Max(reg.y1, h.y)
	}
	for _, v := range c.v {
		reg.x0, reg.x1 = math.Min(reg.x0, v.x), math.Max(reg.x1, v.x)
		reg.y0, reg.y1 = math.Min(reg.y0, v.y0), math.Max(reg.y1, v.y1)
	}
	reg.vertical = append([]vline(nil), c.v...)
	reg.horizontal = append([]hline(nil), c.h...)
	sort.Slice(reg.vertical, func(i, j int) bool { return reg.vertical[i].x < reg.vertical[j].x })
	sort.Slice(reg.horizontal, func(i, j int) bool { return reg.horizontal[i].y > reg.horizontal[j].y })
	return reg
}

// blockRegions finds sets of horizontal rules that are about as wide as each other and one above the other: the
// rules of a table typeset with a top, a header and a bottom rule. Rules already part of a grid are left out.
func blockRegions(hs []hline, used []bool) []tableRegion {
	type rule struct {
		hline
		idx int
	}
	var free []rule
	for i, h := range hs {
		if !used[i] {
			free = append(free, rule{h, i})
		}
	}
	sort.Slice(free, func(i, j int) bool { return free[i].y > free[j].y }) // top to bottom

	var regions []tableRegion
	taken := make([]bool, len(free))
	for i := range free {
		if taken[i] {
			continue
		}
		group := []rule{free[i]}
		taken[i] = true
		for j := i + 1; j < len(free); j++ {
			if taken[j] || !sameSpan(free[i].hline, free[j].hline) {
				continue
			}
			group = append(group, free[j])
			taken[j] = true
		}
		if len(group) < minRegionRules {
			continue
		}
		reg := tableRegion{x0: math.Inf(1), x1: math.Inf(-1), y1: group[0].y, y0: group[len(group)-1].y}
		for _, g := range group {
			reg.x0, reg.x1 = math.Min(reg.x0, g.x0), math.Max(reg.x1, g.x1)
			reg.horizontal = append(reg.horizontal, g.hline)
		}
		regions = append(regions, reg)
	}
	return regions
}

// sameSpan reports whether two rules span nearly the same width: the rules of one table do.
func sameSpan(a, b hline) bool {
	wa, wb := a.x1-a.x0, b.x1-b.x0
	if wa < 20 || wb < 20 {
		return false
	}
	overlap := math.Min(a.x1, b.x1) - math.Max(a.x0, b.x0)
	return overlap >= 0.85*math.Max(wa, wb) && math.Abs(wa-wb) <= 0.15*math.Max(wa, wb)
}
