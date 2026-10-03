package pdftext

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"
)

func errorFromPanic(r any) error { return errors.New(fmt.Sprint(r)) }

var relations = map[string]bool{"=": true, "≤": true, "≥": true, "<": true, ">": true, "≈": true, "≠": true, "∼": true, "→": true, "∈": true, "⟹": true}

// linearizeMath writes display formulas in LaTeX-like notation: ^{} and _{} for scripts, \frac{}{} for
// fractions and \begin{cases} for stacked rows after a brace, with the symbols themselves left as the PDF
// has them. It returns one string per formula.
//
// A formula is set in two dimensions and the PDF only says where each glyph is. The x the library gives
// is not reliable for many fonts, but the order the glyphs are drawn in is: a typesetter draws a fraction's
// numerator then its denominator, and a script right after its base. So the formula is read in drawing
// order, and the heights say what each part is: a full-size glyph well above or below the baseline is a
// row of a fraction or of a case list, a smaller one is a script of the glyph drawn before it. Bars,
// roots and matrices are not glyphs and are not recognised: a formula that needs them comes out as its
// parts in reading order, never as a wrong formula.
func linearizeMath(lines []*line) []string {
	var gs []glyph
	for _, l := range lines {
		for _, g := range l.glyphs {
			if !g.isSpace() && !g.isMark() {
				gs = append(gs, g)
			}
		}
	}
	if len(gs) == 0 {
		return nil
	}
	big := 0.0
	for _, g := range gs {
		big = math.Max(big, g.size())
	}
	mainSize := commonSize(gs, big*0.9)

	// One formula per row of relations, found in drawing order: a relation within 1.2 sizes of one already
	// found is in the same formula (a row of cases has its own relation), and the glyphs of a numerator or
	// an exponent belong to the nearest row.
	var rows []float64
	for _, g := range gs {
		if !relations[g.S] || g.size() < mainSize*0.9 {
			continue
		}
		near := false
		for _, y := range rows {
			if math.Abs(y-g.Y) <= mainSize*1.2 {
				near = true
				break
			}
		}
		if !near {
			rows = append(rows, g.Y)
		}
	}
	if len(rows) == 0 {
		var ys []float64
		for _, g := range gs {
			if g.size() >= mainSize*0.9 {
				ys = append(ys, g.Y)
			}
		}
		sort.Float64s(ys)
		return []string{formula(gs, mainSize, ys[len(ys)/2])}
	}
	groups := make([][]glyph, len(rows))
	for _, g := range gs {
		best := 0
		for i, y := range rows {
			if math.Abs(g.Y-y) < math.Abs(g.Y-rows[best]) {
				best = i
			}
		}
		groups[best] = append(groups[best], g)
	}
	out := make([]string, 0, len(rows))
	for i, group := range groups {
		out = append(out, formula(group, mainSize, rows[i]))
	}
	return out
}

// commonSize is the most common glyph size among those at least min, rounded to a tenth.
func commonSize(gs []glyph, min float64) float64 {
	counts := map[float64]int{}
	for _, g := range gs {
		if g.size() >= min {
			counts[math.Round(g.size()*10)/10]++
		}
	}
	best, bestCount := 0.0, 0
	for size, n := range counts {
		if n > bestCount || (n == bestCount && size > best) {
			best, bestCount = size, n
		}
	}
	return best
}

// part is one piece of a formula in drawing order: a run of glyphs on the baseline, or a run set above
// or below it. Scripts are folded into the text of the piece they belong to.
type part struct {
	text string
	side int // 0 on the baseline, +1 above, -1 below
	y    float64
}

// formula reads one formula: its glyphs in drawing order, with mainY the baseline of its relation.
func formula(gs []glyph, mainSize, mainY float64) string {
	var parts []part
	var base *glyph // the last full-size glyph, which a following small glyph is a script of
	var sup, sub []glyph

	flushScripts := func() {
		if base != nil && len(parts) > 0 {
			var sb strings.Builder
			if len(sup) > 0 {
				sb.WriteString("^{" + scriptSet(sup) + "}")
			}
			if len(sub) > 0 {
				sb.WriteString("_{" + scriptSet(sub) + "}")
			}
			parts[len(parts)-1].text += sb.String()
		}
		sup, sub = nil, nil
	}

	for i := range gs {
		g := gs[i]
		if g.size() < mainSize*scriptRatio && base != nil {
			if g.Y > base.Y+mainSize*0.1 {
				sup = append(sup, g)
			} else {
				sub = append(sub, g)
			}
			continue
		}
		flushScripts()
		side := 0
		if d := g.Y - mainY; math.Abs(d) > mainSize*fractionShift {
			if d > 0 {
				side = 1
			} else {
				side = -1
			}
		}
		if n := len(parts); n > 0 && parts[n-1].side == side && math.Abs(parts[n-1].y-g.Y) < mainSize*0.3 {
			// The same row: glued to the previous piece, with a space where the drawing leaves a gap.
			if i > 0 && gs[i].X-(gs[i-1].X+gs[i-1].W) > mainSize*wordGap {
				parts[n-1].text += " "
			}
			parts[n-1].text += g.S
		} else {
			parts = append(parts, part{text: g.S, side: side, y: g.Y})
		}
		base = &gs[i]
	}
	flushScripts()

	return strings.TrimSpace(tidyScripts(norm.NFC.String(joinParts(parts))))
}

// tidyScripts removes the space the drawing leaves between a script and the punctuation after it.
func tidyScripts(s string) string {
	for _, p := range []string{"} )", "} ,", "} ]", "} ;"} {
		s = strings.ReplaceAll(s, p, "}"+p[2:])
	}
	return s
}

// joinParts turns the pieces into text, recognising a numerator over a denominator and rows after a brace.
func joinParts(parts []part) string {
	var sb strings.Builder
	add := func(s string) {
		if sb.Len() > 0 && !strings.HasSuffix(sb.String(), " ") {
			sb.WriteByte(' ')
		}
		sb.WriteString(s)
	}
	for i := 0; i < len(parts); {
		p := parts[i]
		if p.side == 0 {
			sb.WriteString(p.text)
			i++
			continue
		}
		// A stack: consecutive pieces off the baseline with nothing on it between them.
		j := i
		for j < len(parts) && parts[j].side != 0 {
			j++
		}
		stack := parts[i:j]
		switch {
		case len(stack) >= 2 && strings.HasSuffix(strings.TrimRight(sb.String(), " "), "{"):
			sorted := append([]part(nil), stack...)
			sort.SliceStable(sorted, func(a, b int) bool { return sorted[a].y > sorted[b].y })
			rows := make([]string, len(sorted))
			for k, r := range sorted {
				rows[k] = r.text
			}
			trimmed := strings.TrimSuffix(strings.TrimRight(sb.String(), " "), "{")
			sb.Reset()
			sb.WriteString(trimmed)
			add(`\begin{cases} ` + strings.Join(rows, ` \\ `) + ` \end{cases}`)
		case len(stack) == 2 && stack[0].side != stack[1].side:
			num, den := stack[0], stack[1]
			if num.side < 0 {
				num, den = den, num
			}
			add(`\frac{` + num.text + `}{` + den.text + `}`)
		default:
			for _, s := range stack {
				add(s.text)
			}
		}
		i = j
	}
	return sb.String()
}

// scriptSet writes the glyphs of a script in drawing order; a glyph smaller than the rest of the script,
// drawn after another, is a script of that one.
func scriptSet(gs []glyph) string {
	size := commonSize(gs, 0)
	var sb strings.Builder
	var base *glyph
	var sup, sub []glyph
	flush := func() {
		if len(sup) > 0 {
			sb.WriteString("^{" + scriptSet(sup) + "}")
		}
		if len(sub) > 0 {
			sb.WriteString("_{" + scriptSet(sub) + "}")
		}
		sup, sub = nil, nil
	}
	for i := range gs {
		g := gs[i]
		if base != nil && g.size() < size*scriptRatio {
			if g.Y > base.Y {
				sup = append(sup, g)
			} else {
				sub = append(sub, g)
			}
			continue
		}
		flush()
		if i > 0 && g.X-(gs[i-1].X+gs[i-1].W) > size*wordGap {
			sb.WriteByte(' ')
		}
		sb.WriteString(g.S)
		base = &gs[i]
	}
	flush()
	return sb.String()
}
