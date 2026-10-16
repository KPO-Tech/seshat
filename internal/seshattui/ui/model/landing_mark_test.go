package model

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderLandingMarkBlocks(t *testing.T) {
	out := renderLandingMarkBlocks()
	if out == "" {
		t.Fatal("the embedded mark did not decode")
	}

	lines := strings.Split(out, "\n")
	if len(lines) != landingMarkRows {
		t.Fatalf("got %d rows, want %d", len(lines), landingMarkRows)
	}

	img := landingMarkImage()
	wantCols := landingMarkCols(img)
	painted := 0
	for i, line := range lines {
		if w := ansi.StringWidth(line); w != wantCols {
			t.Fatalf("row %d is %d columns wide, want %d", i, w, wantCols)
		}
		if strings.TrimSpace(ansi.Strip(line)) != "" {
			painted++
		}
	}
	// The flame rises from the top rows and the base fills the bottom ones.
	if painted < landingMarkRows-2 {
		t.Fatalf("only %d of %d rows are painted", painted, landingMarkRows)
	}
}

func TestLandingMarkIsCompact(t *testing.T) {
	img := landingMarkImage()
	if img == nil {
		t.Fatal("the embedded mark did not decode")
	}
	// The old braille logo took more than 25 rows; the mark must stay small
	// enough for a normal window.
	if landingMarkRows > 14 {
		t.Fatalf("mark is %d rows tall, want at most 14", landingMarkRows)
	}
	if cols := landingMarkCols(img); cols < 8 || cols > 30 {
		t.Fatalf("mark is %d columns wide, outside 8..30", cols)
	}
}
