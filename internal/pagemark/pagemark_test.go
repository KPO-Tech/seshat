package pagemark

import "testing"

func TestMarkerRoundTrips(t *testing.T) {
	for _, n := range []int{1, 7, 120} {
		got, ok := Parse(Marker(n))
		if !ok || got != n {
			t.Errorf("Parse(Marker(%d)) = %d, %v", n, got, ok)
		}
	}
	for _, line := range []string{"", "page 3", "<!-- page -->", "<!-- page 0 -->", "<!-- page x -->", "text <!-- page 3 -->", "<!-- note -->"} {
		if n, ok := Parse(line); ok {
			t.Errorf("Parse(%q) = %d, true", line, n)
		}
	}
	if n, ok := Parse("  " + Marker(4) + "  "); !ok || n != 4 {
		t.Errorf("a marker with spaces around it is still a marker: %d %v", n, ok)
	}
}

func TestStripTakesOutTheMarkersAndTheirBlankLine(t *testing.T) {
	md := Marker(1) + "\n\nFirst page.\n\n" + Marker(2) + "\n\nSecond page.\n"
	if got, want := Strip(md), "First page.\n\nSecond page.\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := Strip("no marker here"); got != "no marker here" {
		t.Fatalf("got %q", got)
	}
}
