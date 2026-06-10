package components

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/hooks"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
	"github.com/EngineerProjects/nexus-engine/internal/tui/components/list"
	"github.com/charmbracelet/x/ansi"
)

// toolRenderCache is a two-level render cache for toolItem (adapted from Crush).
//
// Level 1 (raw): the full tool body without selection styling, cached by
// width. Re-computed only when content changes or the tool is still running
// (spinner frame is embedded in the icon).
//
// Level 2 (prefixed): the final output with selection background applied,
// cached by (width, selectionKey) where key 0 = unselected, key 1 = selected.
// Provides O(1) returns on cursor movement when content is stable.
//
// Both levels are only populated for isDone() items. Running tools bypass
// all caches because the spinner icon embeds a mutable frame counter.
type toolRenderCache struct {
	rawRendered string
	rawWidth    int
	rawHeight   int

	prefixedRendered string
	prefixedWidth    int
	prefixedKey      uint64
}

func (c *toolRenderCache) getRaw(width int) (string, int, bool) {
	if c.rawRendered != "" && c.rawWidth == width {
		return c.rawRendered, c.rawHeight, true
	}
	return "", 0, false
}

func (c *toolRenderCache) setRaw(rendered string, width, height int) {
	c.rawRendered = rendered
	c.rawWidth = width
	c.rawHeight = height
}

func (c *toolRenderCache) getPrefixed(width int, key uint64) (string, bool) {
	if c.prefixedRendered != "" && c.prefixedWidth == width && c.prefixedKey == key {
		return c.prefixedRendered, true
	}
	return "", false
}

func (c *toolRenderCache) setPrefixed(rendered string, width int, key uint64) {
	c.prefixedRendered = rendered
	c.prefixedWidth = width
	c.prefixedKey = key
}

func (c *toolRenderCache) clear() { *c = toolRenderCache{} }

type toolItem struct {
	list.Versioned
	c          *Chat
	id         string
	name       string
	status     string
	label      string
	metadata   map[string]any
	expanded   bool
	startedAt  time.Time
	finishedAt time.Time

	// awaitingPermission: isDone() stays false so the render cache never freezes the item.
	awaitingPermission bool

	nestedTools []*toolItem

	// cache is the two-level render cache for chat-list rendering.
	cache toolRenderCache

	// detailCacheW/R caches the expensive right-panel detail view, keyed by
	// width. Separate from cache because the detail panel has no selection
	// state and invalidates independently.
	detailCacheW int
	detailCacheR string
}

func newToolItem(c *Chat, id, name, status, label string, metadata map[string]any) *toolItem {
	return &toolItem{
		c:         c,
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

func (t *toolItem) Finished() bool { return t.isDone() }
func (t *toolItem) invalidate() {
	t.cache.clear()
	t.detailCacheW = 0
	t.detailCacheR = ""
	t.Bump()
}

// rawRender computes the tool's visual content without any selection styling.
// The result is frozen into the level-1 cache once the tool isDone() so
// that subsequent cursor movements never re-compute the expensive icon,
// summary, and preview rendering.
func (t *toolItem) rawRender(c *Chat, width int) string {
	if t.isDone() {
		if raw, _, ok := t.cache.getRaw(width); ok {
			return raw
		}
	}

	icon := t.renderIcon(c)
	nameStyle := t.renderNameStyle(c.styles)
	summary := truncate(t.summaryText(), max(12, width-34))
	expander := c.styles.MsgTimestamp.Render(t.expanderSymbol())

	parts := []string{expander, icon, nameStyle.Render(toolDisplayName(t.name))}
	if summary != "" {
		parts = append(parts, c.styles.MsgTimestamp.Render(summary))
	}
	if t.awaitingPermission && !t.isDone() {
		parts = append(parts, c.styles.ToolProgress.Render("Awaiting permission…"))
	}
	if dur := t.durationText(); dur != "" {
		parts = append(parts, c.styles.MsgTimestamp.Render("("+dur+")"))
	}
	header := strings.Join(parts, " ")

	var out string
	if !t.expanded {
		out = header
	} else {
		preview := t.inlinePreview(c, width)
		if preview == "" && !t.isDone() {
			preview = c.styles.MsgTimestamp.Render("Running...")
		}
		if hookLine := toolOutputHookIndicator(&c.styles, t.metadata, width-4); hookLine != "" {
			if preview != "" {
				preview = hookLine + "\n\n" + preview
			} else {
				preview = hookLine
			}
		}
		if preview == "" {
			preview = c.styles.MsgTimestamp.Render("No output available.")
		}
		out = header + "\n" + indentBlock(preview, "    ")
	}

	if t.isDone() {
		t.cache.setRaw(out, width, lipgloss.Height(out))
	}
	return out
}

// Render implements list.Item using a two-level cache:
//
//  1. rawRender (level 1): content without selection styling, frozen once done.
//  2. prefixed (level 2): complete output with selection background on the
//     header line, keyed by (width, selectionKey). Moving the cursor back to
//     a previously-selected done item is an O(1) cache hit.
func (t *toolItem) Render(width int) string {
	selected := t.c.selectedToolItem() == t

	// Determine the prefixed-cache key before any computation.
	var key uint64
	if selected {
		key = 1
	}

	// Level 2: O(1) return when the tool is done and the selection state
	// matches a previously-computed frame.
	if t.isDone() {
		if cached, ok := t.cache.getPrefixed(width, key); ok {
			return cached
		}
	}

	raw := t.rawRender(t.c, width)

	var out string
	if !selected {
		out = raw
	} else {
		// Apply selection background to the first line (header) only.
		// The indented preview body is not affected by selection state.
		nl := strings.IndexByte(raw, '\n')
		selStyle := lipgloss.NewStyle().
			Foreground(common.ColorText).
			Background(lipgloss.Color("#1F2937"))
		if nl < 0 {
			out = selStyle.Render(raw)
		} else {
			out = selStyle.Render(raw[:nl]) + raw[nl:]
		}
	}

	if t.isDone() {
		t.cache.setPrefixed(out, width, key)
	}
	return out
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


func (t *toolItem) renderIcon(c *Chat) string {
	switch {
	case t.status == "completed" || t.status == "done":
		return c.styles.MsgTimestamp.Render("✓")
	case t.status == "failed" || t.status == "error":
		return c.styles.ToolError.Render("✗")
	case t.awaitingPermission && !t.isDone():
		return c.styles.ToolProgress.Render("?")
	case !t.isDone():
		frame := strings.TrimSpace(c.SpinnerFrame)
		if frame == "" {
			frame = "⠋"
		}
		return c.styles.ToolProgress.Render(frame)
	default:
		return c.styles.ToolProgress.Render(toolIconFor(t.name))
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
	if !t.isDone() {
		if !t.startedAt.IsZero() {
			return formatDuration(time.Since(t.startedAt))
		}
		if ms, ok := intFromMap(t.metadata, "execution_duration_ms"); ok && ms > 0 {
			return formatDuration(time.Duration(ms) * time.Millisecond)
		}
		return ""
	}
	if t.finishedAt.IsZero() {
		if !t.startedAt.IsZero() {
			return formatDuration(time.Since(t.startedAt))
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
	case "read_file", "write_file", "edit_file", "apply_patch", "bash", "web_search", "web_fetch",
		"agent", "spawn_agent", "wait_agent", "subagent_event", "job_output", "job_kill":
		return true
	default:
		return false
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
		if taskID := stringFromMap(t.metadata, "task_id"); taskID != "" {
			return fmt.Sprintf("Background Task Started (PID %s) · %s", taskID, cmd)
		}
		return cmd
	case "job_output":
		jobID := strings.TrimSpace(stringFromMap(input, "job_id"))
		return fmt.Sprintf("Get Output (PID %s)", jobID)
	case "job_kill":
		jobID := strings.TrimSpace(stringFromMap(input, "job_id"))
		return fmt.Sprintf("Kill Job (PID %s)", jobID)
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

// toolOutputHookIndicator renders hook indicator lines from tool metadata.
func toolOutputHookIndicator(sty *common.Styles, metadata map[string]any, width int) string {
	if metadata == nil {
		return ""
	}
	hookData, ok := metadata["hook"]
	if !ok {
		return ""
	}

	// Re-marshal/unmarshal to get into typed struct.
	data, _ := json.Marshal(hookData)
	var h hooks.HookMetadata
	if err := json.Unmarshal(data, &h); err != nil || len(h.Hooks) == 0 {
		return ""
	}

	// Sanitize names (replace newlines with ¶) and compute max widths
	// for the name, matcher, and detail columns so they align.
	const maxHookNameWidth = 30
	sanitizedNames := make([]string, len(h.Hooks))
	details := make([]string, len(h.Hooks))
	maxNameWidth := 0
	maxMatcherWidth := 0
	maxDetailWidth := 0
	for i, hi := range h.Hooks {
		sanitizedNames[i] = strings.ReplaceAll(hi.Name, "\n", "¶")
		w := lipgloss.Width(sty.HookName.Render(sanitizedNames[i]))
		if w > maxNameWidth {
			maxNameWidth = w
		}
		if hi.Matcher != "" {
			mw := lipgloss.Width(sty.HookMatcher.Render(hi.Matcher))
			if mw > maxMatcherWidth {
				maxMatcherWidth = mw
			}
		}
		details[i] = hookDetail(sty, hi)
		if dw := lipgloss.Width(details[i]); dw > maxDetailWidth {
			maxDetailWidth = dw
		}
	}

	if maxNameWidth > maxHookNameWidth {
		maxNameWidth = maxHookNameWidth
	}

	// Cap the name column so the widest line still fits in width.
	if width > 0 {
		fixed := lipgloss.Width(sty.HookLabel.Render("Hook")) + 1
		if maxMatcherWidth > 0 {
			fixed += 1 + maxMatcherWidth
		}
		fixed += 1 + lipgloss.Width(sty.HookArrow.Render("→")) + 1
		fixed += maxDetailWidth
		if budget := width - fixed; budget < maxNameWidth {
			maxNameWidth = max(1, budget)
		}
	}

	var lines []string
	for i, hi := range h.Hooks {
		name := truncateHookName(sanitizedNames[i], maxNameWidth)
		lines = append(lines, renderHookLine(sty, hi, name, details[i], maxNameWidth, maxMatcherWidth))
	}
	return strings.Join(lines, "\n")
}

func truncateHookName(name string, maxWidth int) string {
	if ansi.StringWidth(name) <= maxWidth {
		return name
	}
	if isLikelyPath(name) {
		n := ansi.StringWidth(name) - maxWidth + 1
		return ansi.TruncateLeft(name, n, "…")
	}
	return ansi.Truncate(name, maxWidth, "…")
}

func isLikelyPath(s string) bool {
	if s == "" || strings.ContainsAny(s, " \t\n¶'\"|&;<>$`*?(){}[]\\") {
		return false
	}
	if filepath.IsAbs(s) {
		return true
	}
	return strings.Contains(s, "/")
}

func hookDetail(sty *common.Styles, hi hooks.HookInfo) string {
	const okMessage = "Allowed"
	const denialMessage = "Denied"
	const rewroteMessage = "Rewrote input"

	switch strings.ToLower(hi.Decision) {
	case "deny":
		if hi.Reason != "" {
			return sty.HookDenied.Render(denialMessage) + " " + sty.HookDeniedReason.Render(hi.Reason)
		}
		return sty.HookDenied.Render(denialMessage)
	case "allow":
		result := sty.HookOK.Render(okMessage)
		if hi.InputRewrite {
			result += " " + sty.HookRewrote.Render(rewroteMessage)
		}
		return result
	default:
		result := sty.HookOK.Render(okMessage)
		if hi.InputRewrite {
			result += " " + sty.HookRewrote.Render(rewroteMessage)
		}
		return result
	}
}

func renderHookLine(sty *common.Styles, hi hooks.HookInfo, rawName, detail string, maxNameWidth, maxMatcherWidth int) string {
	name := sty.HookName.Render(rawName)
	if w := lipgloss.Width(name); w < maxNameWidth {
		name += strings.Repeat(" ", maxNameWidth-w)
	}

	matcher := ""
	if maxMatcherWidth > 0 {
		matcher = sty.HookMatcher.Render(hi.Matcher)
		if w := lipgloss.Width(matcher); w < maxMatcherWidth {
			matcher += strings.Repeat(" ", maxMatcherWidth-w)
		}
		matcher = " " + matcher
	}

	labelStyle := sty.HookLabel
	arrowStyle := sty.HookArrow
	if strings.ToLower(hi.Decision) == "deny" {
		labelStyle = sty.HookDeniedLabel
		arrowStyle = sty.HookDeniedLabel
	}

	return lipgloss.JoinHorizontal(lipgloss.Center,
		labelStyle.Render("Hook"),
		" ",
		name,
		matcher,
		" ",
		arrowStyle.Render("→"),
		" ",
		detail,
	)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
