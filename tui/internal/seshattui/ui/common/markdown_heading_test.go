package common

import (
	"strings"
	"testing"

	"github.com/KPO-Tech/seshat-runtime/tui/internal/seshattui/ui/styles"
)

// A level-1 heading was drawn as a solid orange block whose text could not be
// read. Headings are bold and coloured, never filled.
func TestHeadingsAreNotFilledWithABackground(t *testing.T) {
	themes := map[string]styles.Styles{
		"SeshatDark":           styles.SeshatDark(),
		"CharmtonePantera":     styles.CharmtonePantera(),
		"HyperSeshatObsidiana": styles.HyperSeshatObsidiana(),
	}
	for name, sty := range themes {
		sty := sty
		t.Run(name, func(t *testing.T) {
			out, err := MarkdownRenderer(&sty, 80).Render("# Phase 1\n\n## Section\n\n### Detail\n\ntext\n")
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			if strings.Contains(out, "48;") {
				t.Fatalf("a heading must not set a background colour, got %q", out)
			}
		})
	}
}
