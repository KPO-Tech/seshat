package pdftext

import (
	"fmt"
	"strings"
	"testing"
)

// yearTablePDF draws a table as the FOMC projections do: a title over a group of year columns, the years in a band of their own
// whose rules between the columns stop at the header rule, and a body whose figures sit under the years with nothing drawn
// between them. "Longer" and "run" are two lines of one heading, over a column of figures.
func yearTablePDF() []byte {
	var c strings.Builder
	line := func(x0, y0, x1, y1 int) { fmt.Fprintf(&c, "0.5 w %d %d m %d %d l S\n", x0, y0, x1, y1) }
	text := func(x, y int, s string) { fmt.Fprintf(&c, "BT /F1 10 Tf 1 0 0 1 %d %d Tm (%s) Tj ET\n", x, y, s) }
	// rules: top, under the title, the header rule, the bottom
	for _, y := range []int{700, 680, 650, 560} {
		line(100, y, 460, y)
	}
	// outline and the rule between the label column and the years: the whole height; the rules between years: the band of years
	for _, x := range []int{100, 200, 460} {
		line(x, 560, x, 700)
	}
	for _, x := range []int{250, 300, 350} {
		line(x, 650, x, 680)
	}
	text(300, 686, "Median")
	text(105, 666, "Variable")
	for i, s := range []string{"2024", "2025", "2026", "Longer"} {
		text(215+50*i, 666, s)
	}
	text(365, 654, "run")
	for r, row := range [][]string{{"Real GDP", "2.1", "2.0", "2.0", "1.8"}, {"Unemployment", "4.0", "4.1", "4.0", "4.1"}, {"Inflation", "2.4", "2.2", "2.0", "2.0"}, {"Core inflation", "2.6", "2.2", "2.0", "2.0"}, {"Interest rate", "4.6", "3.9", "3.1", "2.6"}, {"Investment", "1.1", "1.9", "2.3", "2.5"}} {
		text(105, 630-14*r, row[0])
		for i, s := range row[1:] {
			text(215+50*i, 630-14*r, s)
		}
	}
	return buildPDF(0, c.String(), helveticaFont(""))
}

func TestFiguresSideBySideUnderYearsAreNotOneCellWrittenThreeTimes(t *testing.T) {
	t.Parallel()
	md, err := PageMarkdown(firstPage(t, yearTablePDF()))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"| Real GDP | 2.1 | 2.0 | 2.0 | 1.8 |", "| Unemployment | 4.0 | 4.1 | 4.0 | 4.1 |", "| Inflation | 2.4 | 2.2 | 2.0 | 2.0 |"} {
		if !strings.Contains(md, want) {
			t.Errorf("row %q is missing:\n%s", want, md)
		}
	}
	if strings.Contains(md, "2.1 2.0") {
		t.Errorf("figures were put in one cell:\n%s", md)
	}
}

func TestTheSecondLineOfAHeadingOverFiguresCarriesOnTheFirst(t *testing.T) {
	t.Parallel()
	md, err := PageMarkdown(firstPage(t, yearTablePDF()))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "Longer run") {
		t.Errorf("the heading is cut in two rows:\n%s", md)
	}
}

func TestARulingThatDoesNotCoverARowStillSplitsTwoCellsOfText(t *testing.T) {
	t.Parallel()
	seps := []separator{{x: 100, rule: &vline{x: 100, y0: 0, y1: 10}}}
	span := func(x0, x1 float64) chunk { return chunk{x0: x0, x1: x1} }
	cases := []struct {
		name   string
		chunks []chunk
		want   bool
	}{
		{"text on both sides, none across", []chunk{span(60, 90), span(110, 140)}, true},
		{"one chunk across the ruling is one merged cell", []chunk{span(60, 140)}, false},
		{"text on one side only", []chunk{span(110, 140), span(150, 180)}, false},
		{"a chunk across and others beside", []chunk{span(60, 90), span(95, 140)}, false},
	}
	for _, tc := range cases {
		active := []bool{false}
		splitMergedByText(textRow{chunks: tc.chunks}, seps, active)
		if active[0] != tc.want {
			t.Errorf("%s: active = %v, want %v", tc.name, active[0], tc.want)
		}
	}
}
