package pdftext

import (
	"fmt"
	"strings"
	"testing"
)

// columnsPDF is a page of two columns of lines of one font (10 points, with a width table), the left one from x 72 and the right
// one from x 320, so that a line of one column stops far from the right edge of the other.
func columnsPDF(left, right []string) []byte {
	var content strings.Builder
	for column, lines := range [][]string{left, right} {
		for i, text := range lines {
			fmt.Fprintf(&content, "BT /F1 10 Tf 1 0 0 1 %d %d Tm (%s) Tj ET\n", 72+column*248, 700-14*i, text)
		}
	}
	return buildPDF(0, content.String(), helveticaFont(""))
}

var (
	leftColumn = []string{
		"Language models are trained on approxima-",
		"tion of many documents and they are used for",
		"a task-specific purpose after the training of",
		"the whole network on a large corpus of text",
		"that was collected from many different sources",
		"and cleaned by hand before any use, so that the",
		"results can be compared with those of other",
		"systems on the same data over several runs of",
		"the experiment with different random seeds and",
		"a fixed budget of computation for each of them",
	}
	rightColumn = []string{
		"The second column goes on with the same story",
		"and fits a task-specific head on the output of",
		"the encoder, which is kept as it is during the",
		"first epochs. The loss is a cross entropy over",
		"the labels of the task, as in the earlier work",
		"on the subject, and the schedule of the rate of",
		"learning is the usual one for a model of this",
		"size. Every step of the pro-",
		"cedure is repeated for each of the five seeds",
		"and the best result of each run is the one kept",
	}
)

func TestTwoColumnsAreEachAParagraphAndWordsBrokenAtALineEndAreJoined(t *testing.T) {
	t.Parallel()
	md, err := PageMarkdown(firstPage(t, columnsPDF(leftColumn, rightColumn)))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(md, "\n") && strings.Contains(strings.TrimSpace(strings.ReplaceAll(md, "\n\n", "\x00")), "\n") {
		t.Errorf("a line break was kept inside a paragraph:\n%s", md)
	}
	for _, want := range []string{"approximation of many documents", "procedure is repeated"} {
		if !strings.Contains(md, want) {
			t.Errorf("%q is missing:\n%s", want, md)
		}
	}
	if strings.Contains(md, "approxima-") || strings.Contains(md, "pro-") {
		t.Errorf("a syllable break was kept:\n%s", md)
	}
	if got := strings.Count(md, "task-specific"); got != 2 {
		t.Errorf("a word the page writes with a hyphen keeps it, found %d:\n%s", got, md)
	}
}

func TestACompoundBrokenAtAHyphenKeepsItWhenThePageWritesItThatWay(t *testing.T) {
	t.Parallel()
	left := append([]string(nil), leftColumn...)
	left[2] = "a purpose that is the one of the whole task-"
	left[3] = "specific purpose after the training of the network"
	md, err := PageMarkdown(firstPage(t, columnsPDF(left, rightColumn)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(md, "task-specific purpose") || strings.Contains(md, "taskspecific") {
		t.Errorf("the compound lost its hyphen:\n%s", md)
	}
}

func TestAPageOfOneColumnIsNotSplit(t *testing.T) {
	t.Parallel()
	c := pageColumns(nil, 10)
	if c.split != 0 {
		t.Errorf("no lines, no columns: %+v", c)
	}
}
