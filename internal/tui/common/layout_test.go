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
		"base-0000",
		"base-1111",
		"base-2222",
		"base-3333",
		"base-4444",
	}, "\n")
	overlay := CenterHorizontally("OVR-1\nOVR-2", 10)

	got := OverlayOn(base, overlay, 10, 5)
	lines := strings.Split(got, "\n")

	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}
	if lines[1] != "baOVR-111 " || lines[2] != "baOVR-222 " {
		t.Fatalf("expected overlay to replace only its centered segment, got %q / %q", lines[1], lines[2])
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

	if lines[0] != "TOPe-0  " {
		t.Fatalf("expected first overlay row to replace only its own width, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "line-1") {
		t.Fatalf("expected transparent overlay row to keep base content, got %q", lines[1])
	}
	if lines[2] != "BOTe-2  " {
		t.Fatalf("expected last overlay row to replace only its own width, got %q", lines[2])
	}
}

func TestOverlayOnKeepsBackdropVisibleOutsidePopupWidth(t *testing.T) {
	base := strings.Join([]string{
		"0123456789",
		"abcdefghij",
		"KLMNOPQRST",
	}, "\n")
	overlay := CenterHorizontally("XX", 10)

	got := OverlayOn(base, overlay, 10, 3)
	lines := strings.Split(got, "\n")
	if lines[1] != "abcdXXghij" {
		t.Fatalf("expected popup to cover only the centered width, got %q", lines[1])
	}
}
