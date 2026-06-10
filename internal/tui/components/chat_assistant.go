package components

import (
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
	"github.com/EngineerProjects/nexus-engine/internal/tui/components/list"
	"github.com/muesli/reflow/wrap"
)

// thinkingViewMode is the three-state view machine for the thinking block.
// toggleThinking() cycles:
//
//	collapsed → tailWindow (skipped when content is short) → fullExpanded → collapsed
type thinkingViewMode uint8

const (
	thinkingCollapsed    thinkingViewMode = iota
	thinkingTailWindow
	thinkingFullExpanded
)

const (
	thinkTailLines              = 4   // lines shown in collapsed state
	maxExpandedThinkingTailLines = 200 // max lines in tail-window state
	interimNarrationLines        = 2
)

// ----------------------------------------------------------------------------
// assistantSection — per-section render cache (adapted from Crush)
//
// Each section (thinking, content, assembled) carries its own (width,
// srcHash, extra) triple. srcHash is an FNV-64 of the section's source text;
// extra captures any other state that changes the rendered output (view mode,
// streaming flag, etc.). valid disambiguates a real hit from the zero value.
// h is the lipgloss.Height of out, stored so callers never need to recompute.
// ----------------------------------------------------------------------------

type assistantSection struct {
	width   int
	srcHash uint64
	extra   uint64
	out     string
	h       int
	valid   bool
}

func (s *assistantSection) hit(width int, srcHash, extra uint64) bool {
	return s.valid && s.width == width && s.srcHash == srcHash && s.extra == extra
}

func (s *assistantSection) store(width int, srcHash, extra uint64, out string) {
	s.width = width
	s.srcHash = srcHash
	s.extra = extra
	s.out = out
	s.h = lipgloss.Height(out)
	s.valid = true
}

func (s *assistantSection) reset() { *s = assistantSection{} }

// fnv64s hashes a single string with FNV-64a.
func fnv64s(str string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(str))
	return h.Sum64()
}

// fnvFields hashes a list of byte slices with length-prefix framing so that
// no concatenation collision can occur between distinct field tuples.
func fnvFields(fields ...[]byte) uint64 {
	h := fnv.New64a()
	var lenBuf [8]byte
	for _, f := range fields {
		binary.LittleEndian.PutUint64(lenBuf[:], uint64(len(f)))
		_, _ = h.Write(lenBuf[:])
		_, _ = h.Write(f)
	}
	return h.Sum64()
}

// u64b encodes a uint64 as 8 bytes (little-endian) for use in fnvFields.
func u64b(v uint64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, v)
	return b
}

// ----------------------------------------------------------------------------
// thinkingBlock — stateful container for streaming reasoning content.
//
// Owns the raw content and its timing; all caching responsibility now lives
// in the parent assistantItem's thinkingSec. render() is intentionally
// stateless with respect to caching — callers must hold the lock themselves.
// ----------------------------------------------------------------------------

type thinkingBlock struct {
	content    string
	streaming  bool
	startedAt  time.Time
	finishedAt time.Time
}

func newThinkingBlock() *thinkingBlock {
	return &thinkingBlock{
		streaming: true,
		startedAt: time.Now(),
	}
}

func (tb *thinkingBlock) append(text string) {
	tb.content += text
}

func (tb *thinkingBlock) finish() {
	tb.streaming = false
	tb.finishedAt = time.Now()
}

// render produces the thinking box for the given width and view mode.
// It is purely functional — no caching; caching is in assistantItem.cachedThinking.
func (tb *thinkingBlock) render(styles common.Styles, width int, viewMode thinkingViewMode) string {
	innerW := width - 6
	if innerW < 10 {
		innerW = 10
	}

	lines := strings.Split(strings.TrimRight(tb.content, "\n"), "\n")

	var shownLines []string
	var hiddenCount int

	switch viewMode {
	case thinkingCollapsed:
		if len(lines) > thinkTailLines {
			hiddenCount = len(lines) - thinkTailLines
			shownLines = lines[len(lines)-thinkTailLines:]
		} else {
			shownLines = lines
		}
	case thinkingTailWindow:
		if len(lines) > maxExpandedThinkingTailLines {
			hiddenCount = len(lines) - maxExpandedThinkingTailLines
			shownLines = lines[len(lines)-maxExpandedThinkingTailLines:]
		} else {
			shownLines = lines
		}
	case thinkingFullExpanded:
		shownLines = lines
	}

	var inner strings.Builder
	if hiddenCount > 0 {
		inner.WriteString(styles.MsgTimestamp.Render(fmt.Sprintf("… %d lines hidden", hiddenCount)))
		inner.WriteString("\n")
	}
	for i, line := range shownLines {
		inner.WriteString(styles.MsgTimestamp.Render(wrap.String(line, innerW)))
		if i < len(shownLines)-1 {
			inner.WriteString("\n")
		}
	}

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(common.ColorBorder).
		Padding(0, 1).
		Width(width - 2)

	box := boxStyle.Render(inner.String())

	var footParts []string
	if tb.streaming {
		footParts = append(footParts, styles.MsgTimestamp.Render("thinking…"))
	} else {
		dur := tb.finishedAt.Sub(tb.startedAt).Round(100 * time.Millisecond)
		footParts = append(footParts,
			styles.MsgTimestamp.Render(fmt.Sprintf("Thought for %.1fs", dur.Seconds())))
		switch viewMode {
		case thinkingCollapsed:
			footParts = append(footParts, styles.Desc.Render("ctrl+t to expand"))
		default:
			footParts = append(footParts, styles.Desc.Render("ctrl+t to collapse"))
		}
	}
	foot := "  " + strings.Join(footParts, "  ")

	return box + "\n" + foot
}

// ----------------------------------------------------------------------------
// assistantItem
// ----------------------------------------------------------------------------

type assistantItem struct {
	list.Versioned
	c            *Chat
	thinking     *thinkingBlock
	content      string
	streaming    bool
	startedAt    time.Time
	finishedAt   time.Time
	showLabel    bool
	showMeta     bool
	inputTokens  int
	outputTokens int
	stopReason   string

	// Three-state view mode for the thinking block. Zero value = collapsed.
	thinkingViewMode thinkingViewMode

	// thinkingBoxHeight is the rendered height of the thinking section (not
	// including the 2-space indent, since indentation doesn't change line count).
	// Set as a side-effect of cachedThinking() so recomputePlainAndRegions()
	// can read it without a second render call.
	thinkingBoxHeight int

	// Per-section render caches. Splitting these out means content streaming
	// does not invalidate the (often expensive) thinking render, and vice versa.
	thinkingSec  assistantSection
	contentSec   assistantSection
	assembledSec assistantSection

	// streamingContent caches the "stable prefix" glamour render of the
	// assistant content body so each streaming flush only re-renders the
	// trailing partial. See streaming_markdown.go for the full algorithm.
	streamingContent streamingMarkdown
}

func newAssistantItem(c *Chat) *assistantItem {
	return &assistantItem{c: c, streaming: true, showLabel: true, startedAt: time.Now()}
}

func newContinuationItem(c *Chat, startedAt time.Time) *assistantItem {
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	return &assistantItem{c: c, streaming: true, showLabel: false, startedAt: startedAt}
}

// appendThinking appends reasoning text. Only the thinking section cache is
// invalidated; the content section cache is untouched — this is the key
// performance invariant that eliminates redundant content re-renders.
func (a *assistantItem) appendThinking(text string) {
	if text == "" {
		return
	}
	if a.thinking == nil {
		a.thinking = newThinkingBlock()
	}
	a.thinking.append(text)
	a.thinkingSec.reset()
	a.Bump()
}

// appendContent appends assistant content text. Finishes any active thinking
// block (updating its footer) and resets only the thinking and content section
// caches; the assembled cache is implicitly invalidated because its key folds
// in both section hashes.
func (a *assistantItem) appendContent(text string) {
	if a.thinking != nil && a.thinking.streaming {
		a.thinking.finish()
		a.thinkingSec.reset() // footer changed: "thinking…" → "Thought for Xs"
	}
	a.content += text
	a.contentSec.reset()
	a.Bump()
}

// finish marks the message as complete and resets all section caches so the
// next Render() picks up the terminal state (compact narration, meta line, etc.).
func (a *assistantItem) finish(inputTokens, outputTokens int, stopReason string, showMeta bool) {
	a.streaming = false
	a.finishedAt = time.Now()
	a.showMeta = showMeta
	a.inputTokens = inputTokens
	a.outputTokens = outputTokens
	a.stopReason = stopReason
	if a.thinking != nil && a.thinking.streaming {
		a.thinking.finish()
	}
	// Both sections change on finish: thinking footer updates, content may
	// switch from full markdown to compact narration.
	a.thinkingSec.reset()
	a.contentSec.reset()
	a.assembledSec.reset()
	a.Bump()
}

// Finished implements list.Item. A non-streaming item's output is terminal
// and may be frozen by the list cache.
func (a *assistantItem) Finished() bool { return !a.streaming }

// invalidate drops all section caches (e.g. on width or style change) and
// resets the streaming-markdown stable-prefix cache because the cached glamour
// render embeds ANSI sequences for the old width/style.
func (a *assistantItem) invalidate() {
	a.thinkingSec.reset()
	a.contentSec.reset()
	a.assembledSec.reset()
	a.streamingContent.Reset()
	a.Bump()
}

// toggleThinking advances the three-state view cycle and invalidates the
// thinking section cache. The assembled cache will miss naturally because its
// key includes the thinking section's hash.
func (a *assistantItem) toggleThinking() {
	if a.thinking == nil || strings.TrimSpace(a.thinking.content) == "" {
		return
	}
	switch a.thinkingViewMode {
	case thinkingCollapsed:
		if a.tailWindowWouldTruncate() {
			a.thinkingViewMode = thinkingTailWindow
		} else {
			a.thinkingViewMode = thinkingFullExpanded
		}
	case thinkingTailWindow:
		a.thinkingViewMode = thinkingFullExpanded
	case thinkingFullExpanded:
		a.thinkingViewMode = thinkingCollapsed
	}
	a.thinkingSec.reset()
	a.Bump()
}

// tailWindowWouldTruncate reports whether the thinking content is long enough
// that the tail-window step is worth inserting into the toggle cycle. Uses
// source-text logical-line count as the heuristic rather than the rendered
// height to avoid triggering a glamour render just to make this decision.
func (a *assistantItem) tailWindowWouldTruncate() bool {
	if a.thinking == nil {
		return false
	}
	return 1+strings.Count(a.thinking.content, "\n") > maxExpandedThinkingTailLines
}

// ----------------------------------------------------------------------------
// Section cache key functions
// ----------------------------------------------------------------------------

// thinkingKey returns the (srcHash, extra) cache key components for the
// thinking section. extra folds in the view mode and the footer state.
func (a *assistantItem) thinkingKey() (srcHash uint64, extra uint64) {
	srcHash = fnv64s(a.thinking.content)

	var streamingByte byte
	if a.thinking.streaming {
		streamingByte = 1
	}
	// Duration string changes the footer once thinking finishes. Round to
	// 100 ms so timer jitter does not thrash the cache.
	var durBytes []byte
	if !a.thinking.streaming && !a.thinking.finishedAt.IsZero() {
		dur := a.thinking.finishedAt.Sub(a.thinking.startedAt).Round(100 * time.Millisecond)
		durBytes = []byte(dur.String())
	}
	extra = fnvFields(
		[]byte{byte(a.thinkingViewMode), streamingByte},
		durBytes,
	)
	return
}

// contentKey returns the (srcHash, extra) cache key components for the
// content section. extra captures whether compact narration applies.
func (a *assistantItem) contentKey() (srcHash uint64, extra uint64) {
	srcHash = fnv64s(a.content)

	var streamingByte, compactByte byte
	if a.streaming {
		streamingByte = 1
	}
	// Compact narration is used when the message is done, has no meta, and
	// verbose interim is off. All three conditions must be stable before we
	// cache, which they are for non-streaming items.
	if !a.streaming && !a.showMeta && (a.c == nil || !a.c.verboseInterim) {
		compactByte = 1
	}
	extra = fnvFields([]byte{streamingByte, compactByte})
	return
}

// assembledKey returns the (srcHash, extra) for the top-level assembled cache.
// srcHash folds the hashes of all live section caches; extra covers the
// structural display flags and width.
func (a *assistantItem) assembledKey(width int, showDivider bool) (srcHash uint64, extra uint64) {
	var thinkSrc, thinkExtra uint64
	if a.thinking != nil {
		thinkSrc, thinkExtra = a.thinkingKey()
	}
	cSrc, cExtra := a.contentKey()

	// srcHash: combination of all section sources + extras.
	srcHash = fnvFields(
		u64b(thinkSrc), u64b(thinkExtra),
		u64b(cSrc), u64b(cExtra),
	)

	// extra: structural flags + width (packed via fnvFields for correctness).
	var flags byte
	if a.showLabel   { flags |= 1 << 0 }
	if a.showMeta    { flags |= 1 << 1 }
	if showDivider   { flags |= 1 << 2 }
	if a.c != nil && a.c.verboseInterim { flags |= 1 << 3 }
	extra = fnvFields([]byte{flags}, u64b(uint64(width)))
	return
}

// ----------------------------------------------------------------------------
// Cached section renderers
// ----------------------------------------------------------------------------

// cachedThinking returns the rendered + indented thinking section, computing
// and caching it on miss. Sets thinkingBoxHeight as a side-effect so
// recomputePlainAndRegions() can read the click-region height without a
// second render call.
func (a *assistantItem) cachedThinking(width int) string {
	srcHash, extra := a.thinkingKey()
	if a.thinkingSec.hit(width, srcHash, extra) {
		a.thinkingBoxHeight = a.thinkingSec.h
		return a.thinkingSec.out
	}

	raw := a.thinking.render(a.c.styles, width, a.thinkingViewMode)

	// Apply 2-space indent. Height is unchanged by indentation.
	out := indentBlock(raw, "  ")

	a.thinkingBoxHeight = lipgloss.Height(raw)
	a.thinkingSec.store(width, srcHash, extra, out)
	return out
}

// cachedContent returns the rendered + indented content section.
// During streaming the section cache is bypassed (content changes every token)
// but streamingContent.Render provides its own stable-prefix sub-cache.
func (a *assistantItem) cachedContent(width int) string {
	srcHash, extra := a.contentKey()
	if a.contentSec.hit(width, srcHash, extra) {
		return a.contentSec.out
	}

	var rendered string
	if !a.streaming && !a.showMeta && (a.c == nil || !a.c.verboseInterim) {
		rendered = renderCompactAssistantNarration(a.c.styles, a.content, width)
	} else {
		renderer := common.MarkdownRenderer(max(10, width))
		if renderer == nil {
			rendered = a.content
		} else {
			rendered = a.streamingContent.Render(a.content, max(10, width), renderer)
		}
	}

	out := indentBlock(rendered, "  ")

	// Only freeze into the section cache for terminal (non-streaming) items.
	// Streaming items re-compute every token; the stable-prefix sub-cache
	// inside streamingContent handles the hot path.
	if !a.streaming {
		a.contentSec.store(width, srcHash, extra, out)
	}
	return out
}

// ----------------------------------------------------------------------------
// Render
// ----------------------------------------------------------------------------

// Render implements list.Item. The rendering is layered:
//
//  1. Assembled cache (non-streaming only): caches the fully composed output
//     keyed by a fingerprint of all section hashes + structural flags. A hit
//     returns in O(1) without touching any section.
//
//  2. Per-section caches (thinkingSec, contentSec): each section is re-rendered
//     only when its own source text or extras have changed. Content streaming
//     does not invalidate the thinking section cache.
//
//  3. streamingMarkdown stable-prefix sub-cache: within cachedContent(),
//     each streaming flush re-renders only the trailing partial of the
//     markdown document.
func (a *assistantItem) Render(width int) string {
	c := a.c

	// showDivider must be computed before the assembled cache check because it
	// is part of the assembled key. The scan is O(n) in messages but cheap,
	// and happens at most once per frame for non-streaming items.
	showDivider := false
	if a.showLabel && c != nil {
		for i, m := range c.messages {
			if m == a {
				if i > 0 {
					if _, ok := c.messages[i-1].(*userItem); ok {
						showDivider = true
					}
				}
				break
			}
		}
	}

	// Assembled cache: skip during streaming (content changes every token).
	// Key is computed once here and reused for both hit-check and store.
	var asmSrc, asmExtra uint64
	useAssembledCache := !a.streaming
	if useAssembledCache {
		asmSrc, asmExtra = a.assembledKey(width, showDivider)
		if a.assembledSec.hit(width, asmSrc, asmExtra) {
			return a.assembledSec.out
		}
	}

	var sb strings.Builder

	if showDivider {
		sb.WriteString(c.styles.HeaderSep.Render(strings.Repeat("─", width)))
		sb.WriteString("\n\n")
	}

	if a.showLabel {
		sb.WriteString(a.renderHeader(c, width))
		sb.WriteString("\n")
	}

	if a.thinking != nil && strings.TrimSpace(a.thinking.content) != "" {
		sb.WriteString(a.cachedThinking(width - 2))
		if a.content != "" {
			sb.WriteString("\n\n")
		} else {
			sb.WriteString("\n")
		}
	}

	if a.content != "" {
		sb.WriteString(a.cachedContent(width - 2))
	} else if a.streaming {
		sb.WriteString("  " + c.styles.MsgTimestamp.Render("…"))
	}

	if meta := a.metaLine(c.styles, width-2); meta != "" {
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString("  " + meta)
	}

	out := sb.String()

	if useAssembledCache {
		a.assembledSec.store(width, asmSrc, asmExtra, out)
	}

	return out
}

// renderHeader builds the "✦ Nexus  HH:MM:SS" header line.
func (a *assistantItem) renderHeader(c *Chat, width int) string {
	leftStyled := c.styles.AssistantMarker.Render("✦") + " " + c.styles.AssistantLabel.Render("Nexus")
	if a.startedAt.IsZero() {
		return leftStyled
	}
	rightStyled := c.styles.MsgTimestamp.Render(a.startedAt.Format("15:04:05"))
	padding := width - lipgloss.Width(leftStyled) - lipgloss.Width(rightStyled)
	if padding > 0 {
		return leftStyled + strings.Repeat(" ", padding) + rightStyled
	}
	return leftStyled + " " + rightStyled
}

func renderCompactAssistantNarration(styles common.Styles, content string, width int) string {
	innerW := max(20, width-2)
	normalized := strings.Join(strings.Fields(strings.TrimSpace(content)), " ")
	if normalized == "" {
		return ""
	}
	wrapped := strings.TrimSpace(wrap.String(normalized, innerW))
	lines := strings.Split(wrapped, "\n")
	if len(lines) > interimNarrationLines {
		lines = lines[:interimNarrationLines]
		last := []rune(strings.TrimRight(lines[len(lines)-1], " "))
		if len(last) >= innerW {
			last = last[:innerW-1]
		}
		lines[len(lines)-1] = strings.TrimRight(string(last), " ") + "…"
	}
	for i, line := range lines {
		lines[i] = styles.InterimAssistant.Render(line)
	}
	return strings.Join(lines, "\n")
}

func (a *assistantItem) metaLine(styles common.Styles, width int) string {
	if a.streaming || a.finishedAt.IsZero() || !a.showMeta {
		return ""
	}
	left := styles.ToolDone.Render("done")
	if !a.startedAt.IsZero() {
		left += styles.TurnMeta.Render(" · " + formatDuration(a.finishedAt.Sub(a.startedAt)))
	}
	turnTokens := a.inputTokens + a.outputTokens
	if turnTokens <= 0 {
		return left
	}
	right := styles.TurnMeta.Render(compactTokenCount(turnTokens) + " tok")
	sepLen := width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if sepLen < 3 {
		sepLen = 3
	}
	sep := styles.TurnMeta.Render(strings.Repeat("·", sepLen))
	return left + " " + sep + " " + right
}
