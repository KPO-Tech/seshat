package pdftext

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ledongthuc/pdf"
)

// testdata/borderless.pdf has a table with no lines at all (page 1, with one label that wraps onto a second line)
// and a page of prose (page 2). The page is A4, 595 x 842 points, and the boxes below are where a layout model
// would put them, in points from the top left corner, as the generator drew the text.
func borderlessPage(t *testing.T, n int, source TableSource) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "borderless.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	md, err := PageMarkdownWith(reader.Page(n), source)
	if err != nil {
		t.Fatal(err)
	}
	return md
}

func modelStructure() TableStructure {
	return TableStructure{
		X0: 55, Top: 140, X1: 510, Bottom: 268, Score: 0.9,
		Columns: [][4]float64{{55, 140, 230, 268}, {243, 140, 300, 268}, {312, 140, 372, 268}, {383, 140, 442, 268}, {453, 140, 510, 268}},
		Rows: [][4]float64{
			{55, 140, 510, 155}, {55, 162, 510, 177}, {55, 182, 510, 197}, {55, 202, 510, 230}, {55, 233, 510, 248}, {55, 253, 510, 268},
		},
		Headers: [][4]float64{{55, 140, 510, 155}},
	}
}

func TestATableWithNoLinesIsReadFromTheModelsStructureAndThePagesText(t *testing.T) {
	calls := 0
	md := borderlessPage(t, 1, func() ([]TableStructure, error) {
		calls++
		return []TableStructure{modelStructure()}, nil
	})
	want := "| Product line | Q1 | Q2 | Q3 | Q4 |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| Hardware | 120 | 135 | 128 | 141 |\n" +
		"| Software licences | 88 | 91 | 97 | 104 |\n" +
		"| Cloud services and managed hosting | 45 | 52 | 61 | 70 |\n" +
		"| Support | 30 | 31 | 33 | 34 |\n" +
		"| Training | 12 | 9 | 14 | 16 |"
	if !strings.Contains(md, want) {
		t.Fatalf("got:\n%s\nwant it to contain:\n%s", md, want)
	}
	if calls != 1 {
		t.Errorf("the models were asked %d times", calls)
	}
	intro, table, after := strings.Index(md, "Revenue by product line"), strings.Index(md, "| Product line"), strings.Index(md, "Totals are audited")
	if !(intro >= 0 && intro < table && table < after) {
		t.Fatalf("the table is out of place (%d, %d, %d):\n%s", intro, table, after, md)
	}
	if strings.Count(md, "Hardware") != 1 {
		t.Fatalf("the table's text is repeated outside it:\n%s", md)
	}
}

func TestAPageOfProseNeverAsksTheModels(t *testing.T) {
	asked := false
	md := borderlessPage(t, 2, func() ([]TableStructure, error) {
		asked = true
		return nil, nil
	})
	if asked {
		t.Error("the models are slow and a page of prose has no columns to look at")
	}
	if strings.Contains(md, "|") || !strings.Contains(md, "The committee met on Tuesday") {
		t.Fatalf("got:\n%s", md)
	}
}

func TestAModelThatFailsOrFindsNothingLeavesThePageAsItWas(t *testing.T) {
	plain := borderlessPage(t, 1, nil)
	for name, source := range map[string]TableSource{
		"error":   func() ([]TableStructure, error) { return nil, errors.New("models not loaded") },
		"nothing": func() ([]TableStructure, error) { return nil, nil },
	} {
		if got := borderlessPage(t, 1, source); got != plain {
			t.Errorf("%s changed the page:\n%s\nwas:\n%s", name, got, plain)
		}
	}
	if strings.Contains(plain, "| ---") {
		t.Fatalf("without models a table with no lines stays text:\n%s", plain)
	}
}

func TestWhenTheModelGivesNoColumnsTheGapsInTheTextAreUsed(t *testing.T) {
	s := modelStructure()
	s.Columns = nil
	md := borderlessPage(t, 1, func() ([]TableStructure, error) { return []TableStructure{s}, nil })
	if !strings.Contains(md, "| Hardware | 120 | 135 | 128 | 141 |") {
		t.Fatalf("got:\n%s", md)
	}
}

func TestRowsTheModelMissedAreStillRows(t *testing.T) {
	s := modelStructure()
	s.Rows = s.Rows[:3] // the model drew the header and two rows only
	md := borderlessPage(t, 1, func() ([]TableStructure, error) { return []TableStructure{s}, nil })
	for _, row := range []string{"| Support | 30 | 31 | 33 | 34 |", "| Training | 12 | 9 | 14 | 16 |"} {
		if !strings.Contains(md, row) {
			t.Errorf("missing %q in:\n%s", row, md)
		}
	}
}

func TestAModelTableOnATableTheRulingsFoundIsIgnored(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "tables.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	// The ruled grid of page 1 spans x 60..450, y 120..220 from the top.
	source := func() ([]TableStructure, error) {
		return []TableStructure{{X0: 60, Top: 120, X1: 450, Bottom: 220, Columns: [][4]float64{{60, 120, 170, 220}, {170, 120, 450, 220}}}}, nil
	}
	with, err := PageMarkdownWith(reader.Page(1), source)
	if err != nil {
		t.Fatal(err)
	}
	without, _ := PageMarkdown(reader.Page(1))
	if with != without {
		t.Fatalf("the ruled table was read twice:\n%s\n---\n%s", with, without)
	}
}

func TestPagesWithoutABoxAreNotAskedAbout(t *testing.T) {
	if box := boxOf(pdf.Page{}); box.ok {
		t.Fatal("a null page has no box")
	}
}

func TestModelBoxesAreConvertedToUserSpace(t *testing.T) {
	b := pageBox{x0: 10, y0: 20, x1: 605, y1: 862, ok: true} // a MediaBox that does not start at the origin
	m := b.convert(TableStructure{X0: 5, Top: 100, X1: 50, Bottom: 200, Rows: [][4]float64{{5, 100, 50, 130}, {5, 140, 50, 170}}})
	if m.x0 != 15 || m.x1 != 60 || m.y1 != 762 || m.y0 != 662 {
		t.Fatalf("table box = %+v", m)
	}
	if len(m.rows) != 2 || m.rows[0][0] != 762 || m.rows[0][1] != 732 {
		t.Fatalf("rows = %+v", m.rows)
	}
}
