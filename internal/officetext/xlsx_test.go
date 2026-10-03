package officetext

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func workbook(t *testing.T, build func(f *excelize.File)) []byte {
	t.Helper()
	f := excelize.NewFile()
	build(f)
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func set(f *excelize.File, sheet, cell string, value any) {
	if err := f.SetCellValue(sheet, cell, value); err != nil {
		panic(err)
	}
}

func TestXLSXSeparateBlocksBecomeSeparateTablesAndATitleBecomesACaption(t *testing.T) {
	data := workbook(t, func(f *excelize.File) {
		s := f.GetSheetName(0)
		set(f, s, "A1", "Sales by region, 2024")
		set(f, s, "A2", "Region")
		set(f, s, "B2", "Sales")
		set(f, s, "A3", "North")
		set(f, s, "B3", 10)
		set(f, s, "A4", "South")
		set(f, s, "B4", 7)
		// two blank rows, then a second table
		set(f, s, "A8", "Staff")
		set(f, s, "B8", "Count")
		set(f, s, "A9", "Paris")
		set(f, s, "B9", 12)
	})
	md, err := ExtractXLSX(data)
	if err != nil {
		t.Fatal(err)
	}
	want := "## Sheet1\n\n**Sales by region, 2024**\n\n| Region | Sales |\n| --- | --- |\n| North | 10 |\n| South | 7 |\n\n| Staff | Count |\n| --- | --- |\n| Paris | 12 |"
	if !strings.Contains(md, want) {
		t.Fatalf("got:\n%s\nwant it to contain:\n%s", md, want)
	}
}

func TestXLSXMergedCellsRepeatTheirValue(t *testing.T) {
	data := workbook(t, func(f *excelize.File) {
		s := f.GetSheetName(0)
		set(f, s, "A1", "Region")
		set(f, s, "B1", "2024")
		_ = f.MergeCell(s, "B1", "C1")
		set(f, s, "A2", "")
		set(f, s, "B2", "Sales")
		set(f, s, "C2", "Cost")
		set(f, s, "A3", "North")
		set(f, s, "B3", 10)
		set(f, s, "C3", 4)
	})
	md, err := ExtractXLSX(data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "| Region | 2024 | 2024 |\n| --- | --- | --- |\n|  | Sales | Cost |\n| North | 10 | 4 |") {
		t.Fatalf("got:\n%s", md)
	}
}

func TestXLSXValuesAreWrittenAsTheWorkbookShowsThem(t *testing.T) {
	data := workbook(t, func(f *excelize.File) {
		s := f.GetSheetName(0)
		date, _ := f.NewStyle(&excelize.Style{NumFmt: 14})
		percent, _ := f.NewStyle(&excelize.Style{NumFmt: 10})
		money, _ := f.NewStyle(&excelize.Style{CustomNumFmt: strPtr(`#,##0.00 "EUR"`)})
		set(f, s, "A1", "When")
		set(f, s, "B1", "Rate")
		set(f, s, "C1", "Price")
		set(f, s, "A2", time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC))
		set(f, s, "B2", 0.256)
		set(f, s, "C2", 1234.5)
		_ = f.SetCellStyle(s, "A2", "A2", date)
		_ = f.SetCellStyle(s, "B2", "B2", percent)
		_ = f.SetCellStyle(s, "C2", "C2", money)
	})
	md, err := ExtractXLSX(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"25.60%", "1,234.50 EUR"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
	if strings.Contains(md, "0.256") || strings.Contains(md, "46090") {
		t.Errorf("a raw value leaked through:\n%s", md)
	}
}

func strPtr(s string) *string { return &s }

func TestXLSXHiddenSheetsAreSkippedAndEmptyOnesLeftOut(t *testing.T) {
	data := workbook(t, func(f *excelize.File) {
		set(f, "Sheet1", "A1", "visible")
		set(f, "Sheet1", "B1", "x")
		_, _ = f.NewSheet("Lookup")
		set(f, "Lookup", "A1", "secret lookup")
		set(f, "Lookup", "B1", "y")
		_ = f.SetSheetVisible("Lookup", false)
		_, _ = f.NewSheet("Empty")
	})
	md, err := ExtractXLSX(data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "## Sheet1") || strings.Contains(md, "secret lookup") || strings.Contains(md, "## Empty") {
		t.Fatalf("got:\n%s", md)
	}
}

func TestXLSXLongSheetsAreCutAndSaySo(t *testing.T) {
	data := workbook(t, func(f *excelize.File) {
		s := f.GetSheetName(0)
		set(f, s, "A1", "id")
		set(f, s, "B1", "value")
		for i := 2; i <= MaxXLSXRows+50; i++ {
			set(f, s, fmt.Sprintf("A%d", i), i)
			set(f, s, fmt.Sprintf("B%d", i), "v")
		}
	})
	md, err := ExtractXLSX(data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "_50 more rows in this sheet are not shown._") {
		t.Fatalf("tail:\n%s", md[len(md)-200:])
	}
	if strings.Contains(md, fmt.Sprintf("| %d |", MaxXLSXRows+10)) {
		t.Errorf("rows past the limit were written")
	}
}

func TestXLSXSingleCellSheetsAreStillRead(t *testing.T) {
	data := workbook(t, func(f *excelize.File) { set(f, "Sheet1", "A1", "Just a note") })
	md, err := ExtractXLSX(data)
	if err != nil || !strings.Contains(md, "Just a note") {
		t.Fatalf("md=%q err=%v", md, err)
	}
}
