package common

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// CenterHorizontally pads a rendered box so it is centered within the viewport.
func CenterHorizontally(box string, width int) string {
	if box == "" {
		return ""
	}

	lines := strings.Split(box, "\n")
	boxW := lipgloss.Width(lines[0])
	leftPad := max(0, (width-boxW)/2)
	pad := strings.Repeat(" ", leftPad)

	var sb strings.Builder
	for i, line := range lines {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(pad)
		sb.WriteString(line)
	}
	return sb.String()
}

// OverlayOn places the overlay vertically centered on top of the backdrop.
// Only the popup's visible width replaces the backdrop; text outside the popup
// remains visible so overlays feel like true floating panels rather than modals.
func OverlayOn(base, overlay string, width, height int) string {
	if overlay == "" {
		return base
	}

	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")
	overlayH := len(overlayLines)

	for len(baseLines) < height {
		baseLines = append(baseLines, strings.Repeat(" ", width))
	}

	topOffset := max(0, (height-overlayH)/2)

	for i := range baseLines {
		baseLines[i] = padVisualWidth(baseLines[i], width)
		overlayRow := i - topOffset
		if overlayRow < 0 || overlayRow >= overlayH {
			continue
		}
		overlayLine := overlayLines[overlayRow]
		trimmed := strings.TrimLeft(overlayLine, " ")
		if ansi.StringWidth(trimmed) == 0 {
			continue
		}
		leftPad := len(overlayLine) - len(trimmed)
		leftWidth := leftPad
		overlayWidth := ansi.StringWidth(trimmed)
		before := ansi.Cut(baseLines[i], 0, leftWidth)
		after := ansi.Cut(baseLines[i], leftWidth+overlayWidth, width)
		baseLines[i] = before + trimmed + after
	}

	return strings.Join(baseLines, "\n")
}

func padVisualWidth(s string, width int) string {
	w := ansi.StringWidth(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}
