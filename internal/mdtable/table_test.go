package mdtable

import (
	"strings"
	"testing"
)

func render(t Table) string {
	var sb strings.Builder
	t.RenderMarkdown(&sb)
	return sb.String()
}

func TestTableIsAMarkdownTable(t *testing.T) {
	got := render(Table{Rows: [][]string{{"Name", "Score"}, {"Alice", "90"}}, HeaderRows: 1})
	want := "| Name | Score |\n| --- | --- |\n| Alice | 90 |\n\n"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestACellWithPipesAndLineBreaksStaysOnOneRow(t *testing.T) {
	got := render(Table{Rows: [][]string{{"A", "B"}, {"x | y", "line one\n\n  line   two "}}})
	if !strings.Contains(got, `| x \| y | line one<br>line two |`) {
		t.Fatalf("got:\n%s", got)
	}
}

func TestEmptyRowsAndColumnsAreDropped(t *testing.T) {
	got := render(Table{Rows: [][]string{{"A", "", "B"}, {"", "", ""}, {"1", "", "2"}}})
	if got != "| A | B |\n| --- | --- |\n| 1 | 2 |\n\n" {
		t.Fatalf("got:\n%q", got)
	}
}

func TestSeveralHeaderRowsBecomeOne(t *testing.T) {
	got := render(Table{
		HeaderRows: 2,
		Rows: [][]string{
			{"Region", "2024", "2024", "2025", "2025"},
			{"Region", "Sales", "Cost", "Sales", "Cost"},
			{"North", "10", "4", "12", "5"},
		},
	})
	want := "| Region | 2024 / Sales | 2024 / Cost | 2025 / Sales | 2025 / Cost |\n" +
		"| --- | --- | --- | --- | --- |\n" +
		"| North | 10 | 4 | 12 | 5 |\n\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestACaptionIsWrittenAbove(t *testing.T) {
	got := render(Table{Caption: "Table 1. Scores", Rows: [][]string{{"A", "B"}, {"1", "2"}}})
	if !strings.HasPrefix(got, "**Table 1. Scores**\n\n| A | B |") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestASingleColumnTableIsLinesNotATable(t *testing.T) {
	got := render(Table{Rows: [][]string{{"Note: read this"}, {"And this"}}})
	if got != "Note: read this\n\nAnd this\n\n" {
		t.Fatalf("got:\n%q", got)
	}
}

func TestAnEmptyTableWritesNothing(t *testing.T) {
	if got := render(Table{Rows: [][]string{{"", ""}, {" ", ""}}}); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestSpansAreResolvedIntoEveryPositionTheyCover(t *testing.T) {
	// <tr><td rowspan=2>Team</td><td colspan=2>Q1</td></tr>
	// <tr><td>Plan</td><td>Actual</td></tr>
	// <tr><td>Red</td><td>5</td><td>6</td></tr>
	grid := FromSpanRows([][]SpanCell{
		{{Text: "Team", RowSpan: 2}, {Text: "Q1", ColSpan: 2}},
		{{Text: "Plan"}, {Text: "Actual"}},
		{{Text: "Red"}, {Text: "5"}, {Text: "6"}},
	})
	want := [][]string{{"Team", "Q1", "Q1"}, {"Team", "Plan", "Actual"}, {"Red", "5", "6"}}
	if len(grid) != len(want) {
		t.Fatalf("rows = %v", grid)
	}
	for r := range want {
		for c := range want[r] {
			if grid[r][c] != want[r][c] {
				t.Fatalf("grid = %v, want %v", grid, want)
			}
		}
	}
}

func TestRaggedRowsAreAlignedToTheWidestRow(t *testing.T) {
	got := render(Table{Rows: [][]string{{"A", "B", "C"}, {"1"}}})
	if !strings.Contains(got, "| 1 |  |  |") {
		t.Fatalf("got:\n%s", got)
	}
}
