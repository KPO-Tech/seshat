package pdftext

import (
	"math"
	"sort"
	"strings"

	"github.com/ledongthuc/pdf"

	"github.com/KPO-Tech/seshat/internal/mdtable"
)

// A table with no ruling lines cannot be found from the page's drawing, and from its text alone it cannot be told
// from two columns of prose. A layout model can: it sees the page as a reader does, finds where the tables are, and a
// second model reads their structure (the rows, the columns, the header). What such a model does not do well is
// read: its boxes are approximate and the text under them is already in the PDF, exact. So the models say where the
// cells are, and the text comes from the page.

// TableStructure is a table as the models saw it, in points measured from the top left corner of the page. Boxes are
// {x0, top, x1, bottom}; the models' boxes overlap a little and come in any order.
type TableStructure struct {
	X0, Top, X1, Bottom float64
	Score               float64
	Columns             [][4]float64
	Rows                [][4]float64
	Headers             [][4]float64
}

// TableSource gives the tables the models see on a page. It is only asked for a page whose text looks columnar, and
// an error or an empty answer means "no tables": a page is never lost because the models were unavailable.
type TableSource func() ([]TableStructure, error)

// pageBox is the page's visible rectangle in user space, the one the models' page image is taken of.
type pageBox struct {
	x0, y0, x1, y1 float64
	ok             bool
}

func boxOf(page pdf.Page) pageBox {
	for _, name := range []string{"CropBox", "MediaBox"} {
		v := inherited(page.V, name)
		if v.Kind() != pdf.Array || v.Len() != 4 {
			continue
		}
		b := pageBox{v.Index(0).Float64(), v.Index(1).Float64(), v.Index(2).Float64(), v.Index(3).Float64(), true}
		if b.x0 > b.x1 {
			b.x0, b.x1 = b.x1, b.x0
		}
		if b.y0 > b.y1 {
			b.y0, b.y1 = b.y1, b.y0
		}
		if b.x1 > b.x0 && b.y1 > b.y0 {
			return b
		}
	}
	return pageBox{}
}

// inherited reads a page property that may sit on an ancestor in the page tree (the page boxes and the rotation
// usually do, when every page is the same).
func inherited(page pdf.Value, key string) pdf.Value {
	for node, depth := page, 0; !node.IsNull() && depth < 32; node, depth = node.Key("Parent"), depth+1 {
		if v := node.Key(key); !v.IsNull() {
			return v
		}
	}
	return pdf.Value{}
}

// inPage converts a structure's boxes to page user space (y up) and orders them.
type modelTable struct {
	x0, y0, x1, y1 float64 // y0 is the bottom
	columns        [][2]float64
	rows           [][2]float64 // {top, bottom}, top of the page first
	headers        [][2]float64
}

func (b pageBox) convert(s TableStructure) modelTable {
	x := func(v float64) float64 { return b.x0 + v }
	y := func(v float64) float64 { return b.y1 - v }
	m := modelTable{x0: x(s.X0), x1: x(s.X1), y1: y(s.Top), y0: y(s.Bottom)}
	for _, c := range s.Columns {
		m.columns = append(m.columns, [2]float64{x(c[0]), x(c[2])})
	}
	for _, r := range s.Rows {
		m.rows = append(m.rows, [2]float64{y(r[1]), y(r[3])})
	}
	for _, h := range s.Headers {
		m.headers = append(m.headers, [2]float64{y(h[1]), y(h[3])})
	}
	sort.Slice(m.columns, func(i, j int) bool { return m.columns[i][0] < m.columns[j][0] })
	sort.Slice(m.rows, func(i, j int) bool { return m.rows[i][0] > m.rows[j][0] })
	return m
}

// modelTables finds the tables the models see on the part of the page the rulings left alone, and builds them.
// Each returned table comes with the indexes of the glyphs it used.
func modelTables(source TableSource, box pageBox, glyphs []glyph, taken []bool, found []foundTable, pageW float64) ([]foundTable, [][]int) {
	if source == nil || !box.ok {
		return nil, nil
	}
	var free []glyph
	var freeIdx []int
	for i, g := range glyphs {
		if !taken[i] && !g.isJunk() {
			free = append(free, g)
			freeIdx = append(freeIdx, i)
		}
	}
	if !columnar(free) {
		return nil, nil
	}
	structures, err := source()
	if err != nil || len(structures) == 0 {
		return nil, nil
	}

	var tables []foundTable
	var used [][]int
	for _, s := range structures {
		m := box.convert(s)
		if overlapsFound(m, found) || overlapsFound(m, tables) {
			continue
		}
		reg := tableRegion{x0: m.x0, x1: m.x1, y0: m.y0, y1: m.y1}
		var picked []glyph
		var idx []int
		for k, g := range free {
			if inside(reg, g) && !takenBy(freeIdx[k], used) {
				picked = append(picked, g)
				idx = append(idx, freeIdx[k])
			}
		}
		if len(picked) == 0 {
			continue
		}
		table, ok := buildModelTable(m, picked, pageW)
		if !ok {
			continue
		}
		var sb strings.Builder
		table.RenderMarkdown(&sb)
		md := strings.TrimSpace(sb.String())
		if md == "" {
			continue
		}
		tables = append(tables, foundTable{m.x0, m.y0, m.x1, m.y1, md})
		used = append(used, idx)
	}
	return tables, used
}

func takenBy(i int, used [][]int) bool {
	for _, idx := range used {
		for _, j := range idx {
			if j == i {
				return true
			}
		}
	}
	return false
}

// overlapsFound says whether most of a model table lies on a table already found from the rulings.
func overlapsFound(m modelTable, found []foundTable) bool {
	area := (m.x1 - m.x0) * (m.y1 - m.y0)
	if area <= 0 {
		return true
	}
	for _, t := range found {
		w := math.Min(m.x1, t.x1) - math.Max(m.x0, t.x0)
		h := math.Min(m.y1, t.y1) - math.Max(m.y0, t.y0)
		if w > 0 && h > 0 && w*h > 0.5*area {
			return true
		}
	}
	return false
}

// columnar says whether some run of consecutive text rows has several chunks on each row, lined up in columns: the
// cheap look that decides whether the page is worth the models' time. A page of prose has one chunk per line.
func columnar(glyphs []glyph) bool {
	rows := textRows(glyphs)
	run := 0
	var runRows []textRow
	flush := func() bool {
		defer func() { run, runRows = 0, nil }()
		if run < 3 {
			return false
		}
		x0, x1 := math.Inf(1), math.Inf(-1)
		for _, r := range runRows {
			for _, c := range r.chunks {
				x0, x1 = math.Min(x0, c.x0), math.Max(x1, c.x1)
			}
		}
		return len(gapSeparators(runRows, tableRegion{x0: x0, x1: x1})) > 0
	}
	for _, r := range rows {
		if len(r.chunks) >= 2 {
			run++
			runRows = append(runRows, r)
			continue
		}
		if flush() {
			return true
		}
	}
	return flush()
}

// buildModelTable reads one table from the models' structure and the text on the page. The columns are the gaps
// between the model's column boxes, the rows are the model's rows (a line of text belongs to the row it lies in),
// and the header is the rows under the model's header box. Where the model is missing something, the text fills it
// in: no usable columns means the gaps in the text, and a line outside every row is a row of its own.
func buildModelTable(m modelTable, glyphs []glyph, pageW float64) (mdtable.Table, bool) {
	rows := textRows(glyphs)
	if len(rows) < 2 || mostlyCode(glyphs) {
		return mdtable.Table{}, false
	}
	reg := tableRegion{x0: m.x0, x1: m.x1, y0: m.y0, y1: m.y1}

	var seps []separator
	for k := 0; k+1 < len(m.columns); k++ {
		left, right := m.columns[k], m.columns[k+1]
		// The boxes overlap a little or leave a gap: the line between the columns goes in the middle of it.
		seps = append(seps, separator{x: (left[1] + right[0]) / 2})
	}
	// The gaps in the text are what separates columns on the page; the model's boxes say it too, less exactly. Both
	// count: a column the model merged with its neighbour is found again by the gap, and a gap inside a cell is not
	// taken for a column when the model has no column there.
	seps = unionSeparators(gapSeparators(rows, reg), dedupeSeparators(seps))
	if len(seps) < 1 {
		return mdtable.Table{}, false
	}
	cols := len(seps) + 1

	inHeader := func(y float64) bool {
		for _, h := range m.headers {
			if y <= h[0]+edgeSlack && y >= h[1]-edgeSlack {
				return true
			}
		}
		return false
	}

	// Each text row goes to the model row that contains it; a row the model did not draw stands alone.
	unit := make([]int, len(rows))
	for i, r := range rows {
		unit[i] = -1
		y := r.y + r.size*0.3
		for k, mr := range m.rows {
			if y <= mr[0]+edgeSlack && y >= mr[1]-edgeSlack {
				unit[i] = k
				break
			}
		}
	}
	// A line of text lies in a row of the model; when several lines lie in one, only a line that carries on a cell
	// above it (the second line of a wrapped label) is joined to it. A line that is a full row of its own (a model
	// row can cover a group of rows) stays one.
	cells := make([][]cell, len(rows))
	for i, r := range rows {
		cells[i] = rowCells(r, seps, cols, inHeader(r.y+r.size*0.3))
	}
	numeric := numericColumns(cells, cols)
	type unitCells struct {
		header bool
		cells  []cell
	}
	var units []unitCells
	last := map[int]int{} // model row -> the table row its latest line went into
	for i, r := range rows {
		y := r.y + r.size*0.3
		if at, ok := last[unit[i]]; ok && unit[i] >= 0 && continues(units[at].cells, cells[i], numeric) {
			units[at].cells = appendRow(units[at].cells, cells[i])
			continue
		}
		last[unit[i]] = len(units)
		units = append(units, unitCells{header: inHeader(y), cells: cells[i]})
	}
	grouped := make([][]cell, len(units))
	header := 0
	for i, u := range units {
		grouped[i] = u.cells
		if u.header {
			header++
		}
	}
	if len(grouped) < 2 || !tabular(grouped) || codeTokens(grouped) || paragraphCells(grouped, false) {
		return mdtable.Table{}, false
	}
	t, ok := tableFromCells(grouped, cols)
	if !ok {
		return mdtable.Table{}, false
	}
	t.HeaderRows = max(min(header, len(grouped)-1), 1)
	return withTitleRow(t, cols), true
}

// unionSeparators puts the gaps of the text and the model's separators together: where both name a column boundary
// within a few points of each other, the text's is kept.
func unionSeparators(gaps, model []separator) []separator {
	out := append([]separator(nil), gaps...)
	for _, m := range model {
		near := false
		for _, g := range gaps {
			if math.Abs(g.x-m.x) < 12 {
				near = true
			}
		}
		if !near {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].x < out[j].x })
	return out
}

// dedupeSeparators drops separators closer than a few points: two column boxes that touch give one line.
func dedupeSeparators(seps []separator) []separator {
	sort.Slice(seps, func(i, j int) bool { return seps[i].x < seps[j].x })
	var out []separator
	for _, s := range seps {
		if len(out) > 0 && s.x-out[len(out)-1].x < 3 {
			continue
		}
		out = append(out, s)
	}
	return out
}
