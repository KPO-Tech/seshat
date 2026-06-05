package common

import (
	"strings"

	"charm.land/lipgloss/v2"
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

// OverlayOn dims the backdrop and places the overlay vertically centered on it.
// Empty overlay lines are treated as transparent rows.
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
	dim := lipgloss.NewStyle().Faint(true)

	for i, line := range baseLines {
		overlayRow := i - topOffset
		if overlayRow >= 0 && overlayRow < overlayH {
			overlayLine := overlayLines[overlayRow]
			if overlayLine == "" {
				baseLines[i] = dim.Render(line)
				continue
			}
			baseLines[i] = overlayLine
			continue
		}
		baseLines[i] = dim.Render(line)
	}

	return strings.Join(baseLines, "\n")
}
