package officetext

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/KPO-Tech/seshat/internal/mdtable"
	"github.com/xuri/excelize/v2"
)

// MaxXLSXRows bounds how many rows of one sheet are written. A sheet can hold a million rows of data, which is
// not something to put in front of a reader as markdown; the rows past the limit are counted, not dropped
// silently, so the reader knows there is more and can ask for it with a tool that queries the data.
const MaxXLSXRows = 2000

// ExtractXLSX converts every visible sheet of a workbook to markdown, in workbook order. Values are written as
// the workbook shows them (a date as a date, a percentage as a percentage, a formula as its last calculated
// value, which is what a user with the workbook open sees), merged cells repeat their value over the cells they
// cover, a sheet with several blocks of data separated by blank rows becomes several tables, and a title line
// above a table becomes its caption. Hidden sheets are skipped: they are usually lookup data, not the content.
func ExtractXLSX(data []byte) (string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("not a valid XLSX: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return "", fmt.Errorf("not a valid XLSX: no sheets found")
	}

	var sb strings.Builder
	for _, sheet := range sheets {
		if visible, err := f.GetSheetVisible(sheet); err == nil && !visible {
			continue
		}
		rows, more := readSheetRows(f, sheet)
		fillMergedCells(f, sheet, rows)
		blocks := splitBlocks(rows)
		if len(blocks) == 0 {
			continue
		}
		sb.WriteString("## " + sheet + "\n\n")
		for _, block := range blocks {
			block.RenderMarkdown(&sb)
		}
		if more > 0 {
			fmt.Fprintf(&sb, "_%d more rows in this sheet are not shown._\n\n", more)
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

// readSheetRows reads up to MaxXLSXRows rows, keeping each row's position (a blank row stays blank, which is
// what separates two tables) and says how many rows came after the limit.
func readSheetRows(f *excelize.File, sheet string) (rows [][]string, more int) {
	it, err := f.Rows(sheet)
	if err != nil {
		return nil, 0
	}
	defer it.Close()
	for it.Next() {
		cols, err := it.Columns()
		if err != nil {
			cols = nil
		}
		if len(rows) >= MaxXLSXRows {
			if hasText(cols) {
				more++
			}
			continue
		}
		rows = append(rows, cols)
	}
	return rows, more
}

// fillMergedCells copies the value of each merged range into every cell it covers.
func fillMergedCells(f *excelize.File, sheet string, rows [][]string) {
	merged, err := f.GetMergeCells(sheet)
	if err != nil {
		return
	}
	for _, m := range merged {
		c1, r1, err1 := excelize.CellNameToCoordinates(m.GetStartAxis())
		c2, r2, err2 := excelize.CellNameToCoordinates(m.GetEndAxis())
		if err1 != nil || err2 != nil {
			continue
		}
		value := m.GetCellValue()
		for r := r1; r <= r2 && r <= len(rows); r++ {
			for len(rows[r-1]) < c2 {
				rows[r-1] = append(rows[r-1], "")
			}
			for c := c1; c <= c2; c++ {
				rows[r-1][c-1] = value
			}
		}
	}
}

func hasText(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return true
		}
	}
	return false
}

// splitBlocks cuts the rows of a sheet at its blank rows: a sheet that holds a table, a gap and another table
// holds two tables. A single-cell first row above a table with at least two columns is the table's title.
func splitBlocks(rows [][]string) []mdtable.Table {
	var tables []mdtable.Table
	var current [][]string
	flush := func() {
		if len(current) == 0 {
			return
		}
		tables = append(tables, tableFromBlock(current))
		current = nil
	}
	for _, row := range rows {
		if !hasText(row) {
			flush()
			continue
		}
		current = append(current, row)
	}
	flush()
	return tables
}

func tableFromBlock(block [][]string) mdtable.Table {
	t := mdtable.Table{Rows: block, HeaderRows: 1}
	if len(block) >= 3 && countText(block[0]) == 1 && countText(block[1]) > 1 {
		t.Caption = strings.TrimSpace(firstText(block[0]))
		t.Rows = block[1:]
	}
	return t
}

func countText(row []string) int {
	n := 0
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			n++
		}
	}
	return n
}

func firstText(row []string) string {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return cell
		}
	}
	return ""
}
