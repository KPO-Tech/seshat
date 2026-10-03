// Package mdtable writes tables as markdown. DOCX, PPTX, XLSX and HTML each describe a table differently, merged
// cells above all; their readers build a Table and RenderMarkdown writes it, so a table looks the same whatever file
// it came from.
package mdtable

import (
	"strings"
)

// Table is a table once its merged cells have been resolved: every row has one string per column, and a cell
// that spans several rows or columns has its text in each of the positions it covers. Markdown has no merged
// cells, and a row that is missing the value its neighbour spans stops making sense once a table is cut into
// chunks, so each row carries all of its own values.
type Table struct {
	Caption    string
	Rows       [][]string
	HeaderRows int // how many leading rows are header rows; 0 when the file does not say
}

// SpanCell is a cell as a file declares it, before merges are resolved.
type SpanCell struct {
	Text    string
	ColSpan int
	RowSpan int
}

// FromSpanRows resolves rows of cells with ColSpan/RowSpan (the HTML model: a spanned cell is written once and
// the positions it covers are left out of the later rows) into a Table.
func FromSpanRows(rows [][]SpanCell) [][]string {
	var grid [][]string
	taken := map[[2]int]bool{}
	for r, row := range rows {
		for len(grid) <= r {
			grid = append(grid, nil)
		}
		col := 0
		for _, cell := range row {
			for taken[[2]int{r, col}] {
				col++
			}
			colSpan, rowSpan := max(cell.ColSpan, 1), max(cell.RowSpan, 1)
			for dr := 0; dr < rowSpan; dr++ {
				for dc := 0; dc < colSpan; dc++ {
					for len(grid) <= r+dr {
						grid = append(grid, nil)
					}
					for len(grid[r+dr]) <= col+dc {
						grid[r+dr] = append(grid[r+dr], "")
					}
					grid[r+dr][col+dc] = cell.Text
					taken[[2]int{r + dr, col + dc}] = true
				}
			}
			col += colSpan
		}
	}
	return grid
}

// RenderMarkdown writes the table as a GitHub-flavored markdown table. Rows and columns with nothing in them
// (spacers) are dropped; several header rows become one, each column's parts joined with " / "; a table with
// one column is a list in disguise (a layout table, a boxed paragraph) and is written as lines.
func (t Table) RenderMarkdown(sb *strings.Builder) {
	rows, headerRows := t.cleaned()
	if len(rows) == 0 {
		return
	}
	columns := len(rows[0])
	if t.Caption != "" {
		sb.WriteString("**")
		sb.WriteString(strings.TrimSpace(t.Caption))
		sb.WriteString("**\n\n")
	}
	if columns == 1 {
		for _, row := range rows {
			sb.WriteString(row[0])
			sb.WriteString("\n\n")
		}
		return
	}

	header := rows[0]
	body := rows[1:]
	if headerRows > 1 {
		header = mergeHeaderRows(rows[:headerRows])
		body = rows[headerRows:]
	}
	writeTableRow(sb, header)
	sb.WriteByte('|')
	for i := 0; i < columns; i++ {
		sb.WriteString(" --- |")
	}
	sb.WriteByte('\n')
	for _, row := range body {
		writeTableRow(sb, row)
	}
	sb.WriteByte('\n')
}

// cleaned returns the rows padded to one width, without empty rows or columns, with cell text made safe for a
// markdown row, and how many of the leading rows are headers once the empty ones are gone.
func (t Table) cleaned() ([][]string, int) {
	width := 0
	for _, row := range t.Rows {
		width = max(width, len(row))
	}
	if width == 0 {
		return nil, 0
	}
	padded := make([][]string, 0, len(t.Rows))
	keptHeaders := 0
	for i, row := range t.Rows {
		fixed := make([]string, width)
		empty := true
		for c := range fixed {
			if c < len(row) {
				fixed[c] = cellText(row[c])
			}
			if fixed[c] != "" {
				empty = false
			}
		}
		if empty {
			continue
		}
		padded = append(padded, fixed)
		if i < t.HeaderRows {
			keptHeaders++
		}
	}
	if len(padded) == 0 {
		return nil, 0
	}
	keep := make([]bool, width)
	for _, row := range padded {
		for c, value := range row {
			if value != "" {
				keep[c] = true
			}
		}
	}
	out := make([][]string, len(padded))
	for r, row := range padded {
		for c, value := range row {
			if keep[c] {
				out[r] = append(out[r], value)
			}
		}
	}
	return out, keptHeaders
}

// cellText makes a cell one line of markdown: whitespace collapsed, line breaks as <br>, pipes escaped.
func cellText(value string) string {
	lines := strings.Split(value, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if line = strings.Join(strings.Fields(line), " "); line != "" {
			kept = append(kept, line)
		}
	}
	return strings.ReplaceAll(strings.Join(kept, "<br>"), "|", `\|`)
}

// mergeHeaderRows turns stacked header rows ("2024" over "Sales") into one ("2024 / Sales"); a part that
// repeats the one before it (a cell that spans the header rows) is written once.
func mergeHeaderRows(rows [][]string) []string {
	merged := make([]string, len(rows[0]))
	for c := range merged {
		var parts []string
		for _, row := range rows {
			value := row[c]
			if value == "" || (len(parts) > 0 && parts[len(parts)-1] == value) {
				continue
			}
			parts = append(parts, value)
		}
		merged[c] = strings.Join(parts, " / ")
	}
	return merged
}

func writeTableRow(sb *strings.Builder, row []string) {
	sb.WriteByte('|')
	for _, value := range row {
		sb.WriteByte(' ')
		sb.WriteString(value)
		sb.WriteString(" |")
	}
	sb.WriteByte('\n')
}
