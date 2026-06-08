package components

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
)

type toolItem struct {
	id         string
	name       string
	status     string
	label      string
	metadata   map[string]any
	expanded   bool
	startedAt  time.Time
	finishedAt time.Time

	cacheW int
	cacheR string

	detailCacheW int
	detailCacheR string
}

func newToolItem(id, name, status, label string, metadata map[string]any) *toolItem {
	return &toolItem{
		id:        id,
		name:      name,
		status:    status,
		label:     label,
		metadata:  cloneMap(metadata),
		startedAt: time.Now(),
	}
}

func (t *toolItem) isDone() bool {
	return t.status == "completed" || t.status == "failed" || t.status == "done" || t.status == "error"
}

func (t *toolItem) isFinished() bool { return t.isDone() }
func (t *toolItem) invalidate()      { t.cacheW = 0; t.cacheR = ""; t.detailCacheW = 0; t.detailCacheR = "" }

func (t *toolItem) render(c *Chat, width int) string {
	return t.renderSelected(c, width, false)
}

func (t *toolItem) expanderSymbol() string {
	if !t.supportsPreview() {
		return " "
	}
	if t.expanded {
		return "▾"
	}
	return "▸"
}

func (t *toolItem) detailsSymbol(selected, detailsOpen bool) string {
	if selected && detailsOpen {
		return "⊟"
	}
	return "⊞"
}

func (t *toolItem) renderSelected(c *Chat, width int, selected bool) string {
	if t.isDone() && !selected && !t.expanded && t.cacheW == width && t.cacheR != "" {
		return t.cacheR
	}

	icon := t.renderIcon(c.styles)
	nameStyle := t.renderNameStyle(c.styles)
	summary := truncate(t.summaryText(), max(12, width-34))
	expander := c.styles.MsgTimestamp.Render(t.expanderSymbol())

	// Format: ▸ ✓ ToolName  summary  (Xms)
	// No status label (redundant with icon), no details symbol (open via keyboard/o).
	parts := []string{expander, icon, nameStyle.Render(toolDisplayName(t.name))}
	if summary != "" {
		parts = append(parts, c.styles.MsgTimestamp.Render(summary))
	}
	if dur := t.durationText(); dur != "" {
		parts = append(parts, c.styles.MsgTimestamp.Render("("+dur+")"))
	}

	line := strings.Join(parts, " ")
	if selected {
		line = lipgloss.NewStyle().Foreground(common.ColorText).Background(lipgloss.Color("#1F2937")).Render(line)
	}

	if !t.expanded {
		if t.isDone() && !selected {
			t.cacheW = width
			t.cacheR = line
		}
		return line
	}

	preview := t.inlinePreview(c, width)
	if preview == "" {
		preview = c.styles.MsgTimestamp.Render("No preview available.")
	}
	result := line + "\n" + indentBlock(preview, "    ")
	if t.isDone() && !selected {
		t.cacheW = width
		t.cacheR = result
	}
	return result
}

func (t *toolItem) renderIcon(styles common.Styles) string {
	switch {
	case t.status == "completed" || t.status == "done":
		return styles.MsgTimestamp.Render("✓")
	case t.status == "failed" || t.status == "error":
		return styles.ToolError.Render("✗")
	default:
		return styles.ToolProgress.Render(toolIconFor(t.name))
	}
}

func (t *toolItem) renderNameStyle(styles common.Styles) lipgloss.Style {
	switch {
	case t.status == "completed" || t.status == "done":
		return styles.UserMsg
	case t.status == "failed" || t.status == "error":
		return styles.ToolError
	default:
		return styles.ToolProgress
	}
}

func (t *toolItem) durationText() string {
	if !t.isDone() || t.finishedAt.IsZero() {
		if ms, ok := intFromMap(t.metadata, "execution_duration_ms"); ok && ms > 0 {
			return formatDuration(time.Duration(ms) * time.Millisecond)
		}
		return ""
	}
	return formatDuration(t.finishedAt.Sub(t.startedAt))
}

func (t *toolItem) toolInput() map[string]any {
	return normalizeMap(t.metadata["tool_input"])
}

func (t *toolItem) supportsPreview() bool {
	switch t.name {
	case "read_file", "write_file", "edit_file", "apply_patch", "bash", "web_search", "web_fetch", "spawn_agent", "wait_agent", "close_agent", "send_agent_message":
		return true
	default:
		return strings.TrimSpace(t.resultContent()) != ""
	}
}

func (t *toolItem) statusLabel() string {
	switch t.status {
	case "completed", "done":
		return "done"
	case "failed", "error":
		return "failed"
	case "running", "started":
		return "running"
	default:
		return t.status
	}
}

func (t *toolItem) summaryText() string {
	input := t.toolInput()
	switch t.name {
	case "read_file":
		path := compactPath(stringFromMap(input, "file_path"))
		if path == "" {
			path = t.label
		}
		return path
	case "write_file", "edit_file":
		path := compactPath(stringFromMap(input, "file_path"))
		parts := make([]string, 0, 3)
		if path != "" {
			parts = append(parts, path)
		}
		if kind := stringFromMap(t.metadata, "type"); kind != "" {
			parts = append(parts, kind)
		}
		if stats := t.changeStatsText(); stats != "" {
			parts = append(parts, stats)
		}
		return strings.Join(parts, " · ")
	case "apply_patch":
		if stats := t.changeStatsText(); stats != "" {
			return "patch · " + stats
		}
		if patch := stringFromMap(input, "patch"); patch != "" {
			return firstLine(strings.TrimSpace(patch))
		}
		if content := stringFromMap(t.metadata, "content"); content != "" {
			return firstLine(content)
		}
	case "bash":
		cmd := strings.TrimSpace(stringFromMap(input, "command"))
		if cmd == "" {
			cmd = strings.TrimSpace(stringFromMap(t.metadata, "description"))
		}
		return cmd
	case "web_search":
		query := strings.TrimSpace(stringFromMap(input, "query"))
		if query == "" {
			query = strings.TrimSpace(stringFromMap(t.metadata, "query"))
		}
		if count, ok := intFromMap(t.metadata, "result_count"); ok && count > 0 {
			return fmt.Sprintf("%s · %d results", query, count)
		}
		return query
	case "web_fetch":
		summary := strings.TrimSpace(stringFromMap(input, "url"))
		if summary == "" {
			summary = strings.TrimSpace(stringFromMap(t.metadata, "url"))
		}
		if title := strings.TrimSpace(stringFromMap(t.metadata, "title")); title != "" {
			summary = title
		}
		if code, ok := intFromMap(t.metadata, "code"); ok && code > 0 {
			return fmt.Sprintf("%s · %d", compactPath(summary), code)
		}
		return compactPath(summary)
	case "spawn_agent":
		prompt := strings.TrimSpace(stringFromMap(input, "prompt"))
		nickname := strings.TrimSpace(stringFromMap(input, "nickname"))
		if nickname != "" {
			return nickname + " · " + prompt
		}
		return prompt
	case "wait_agent", "close_agent", "send_agent_message":
		agentID := strings.TrimSpace(stringFromMap(input, "agent_id"))
		if agentID != "" {
			return agentID
		}
	}
	if t.label != "" && t.label != t.status {
		return strings.TrimSpace(t.label)
	}
	if msg := strings.TrimSpace(stringFromMap(t.metadata, "content")); msg != "" {
		return firstLine(msg)
	}
	return ""
}
