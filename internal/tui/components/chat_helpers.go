package components

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
)

func compactTokenCount(n int) string {
	switch {
	case n >= 1_000_000:
		return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1_000_000), ".0"), ".") + "M"
	case n >= 1_000:
		return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1_000), ".0"), ".") + "k"
	default:
		return fmt.Sprintf("%d", n)
	}
}

func headerLine(style lipgloss.Style, width int) string {
	return style.Render(strings.Repeat("─", max(0, width)))
}

func truncate(s string, maxLen int) string {
	if maxLen <= 0 || len([]rune(s)) <= maxLen {
		return s
	}
	r := []rune(s)
	return string(r[:maxLen-1]) + "…"
}

// renderToolGroup renders N consecutive completed same-name tool calls as one row.
// Format: "  ✓ ToolName (N×)  path1, path2, …  (total Xms)"
func renderToolGroup(c *Chat, tools []*toolItem, width int, selected bool) string {
	icon := c.styles.MsgTimestamp.Render("✓")
	nameStyle := c.styles.UserMsg
	name := toolDisplayName(tools[0].name)
	count := c.styles.MsgTimestamp.Render(fmt.Sprintf("(%d×)", len(tools)))

	// Collect per-call summaries (file paths, etc.).
	var summaries []string
	for _, t := range tools {
		if s := t.summaryText(); s != "" {
			summaries = append(summaries, s)
		}
	}
	var summaryStr string
	if len(summaries) > 0 {
		const maxShow = 3
		shown := summaries
		rest := 0
		if len(summaries) > maxShow {
			shown = summaries[:maxShow]
			rest = len(summaries) - maxShow
		}
		summaryStr = strings.Join(shown, ", ")
		if rest > 0 {
			summaryStr += fmt.Sprintf(", +%d", rest)
		}
	}

	// Total wall-clock duration.
	var totalDur time.Duration
	for _, t := range tools {
		if !t.finishedAt.IsZero() && !t.startedAt.IsZero() {
			totalDur += t.finishedAt.Sub(t.startedAt)
		}
	}
	durStr := ""
	if totalDur > 0 {
		durStr = "(" + formatDuration(totalDur) + ")"
	}

	// The leading space keeps column alignment with single-tool rows (expander position).
	parts := []string{" ", icon, nameStyle.Render(name), count}
	if summaryStr != "" {
		maxSum := max(12, width-len(name)-30)
		parts = append(parts, c.styles.MsgTimestamp.Render(truncate(summaryStr, maxSum)))
	}
	if durStr != "" {
		parts = append(parts, c.styles.MsgTimestamp.Render(durStr))
	}

	line := strings.Join(parts, " ")
	if selected {
		line = lipgloss.NewStyle().Foreground(common.ColorText).Background(lipgloss.Color("#1F2937")).Render(line)
	}
	return line
}
