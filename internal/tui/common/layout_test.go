package common

import (
	"strings"
	"testing"
)

func TestCenterHorizontallyPadsRenderedBox(t *testing.T) {
	got := CenterHorizontally("abc\ndef", 9)
	lines := strings.Split(got, "\n")

	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "   abc" || lines[1] != "   def" {
		t.Fatalf("expected centered lines with left padding, got %q / %q", lines[0], lines[1])
	}
}

func TestOverlayOnCentersOverlay(t *testing.T) {
	base := strings.Join([]string{
		"base-0",
		"base-1",
		"base-2",
		"base-3",
		"base-4",
	}, "\n")
	overlay := "OVR-1\nOVR-2"

	got := OverlayOn(base, overlay, 10, 5)
	lines := strings.Split(got, "\n")

	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}
	if lines[1] != "OVR-1" || lines[2] != "OVR-2" {
		t.Fatalf("expected overlay to be vertically centered, got %q / %q", lines[1], lines[2])
	}
}

func TestOverlayOnKeepsTransparentRowsFromOverlay(t *testing.T) {
	base := strings.Join([]string{
		"line-0",
		"line-1",
		"line-2",
	}, "\n")
	overlay := "TOP\n\nBOT"

	got := OverlayOn(base, overlay, 8, 3)
	lines := strings.Split(got, "\n")

	if lines[0] != "TOP" {
		t.Fatalf("expected first overlay row, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "line-1") {
		t.Fatalf("expected transparent overlay row to keep base content, got %q", lines[1])
	}
	if lines[2] != "BOT" {
		t.Fatalf("expected last overlay row, got %q", lines[2])
	}
}
