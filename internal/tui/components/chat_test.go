package components

import (
	"fmt"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestChatAddToolProgressSealsAssistantAndCreatesContinuation(t *testing.T) {
	c := NewChat(common.DefaultStyles(), 80, 20)

	c.AddUserMessage("user prompt")
	c.StartAssistantMessage()
	c.AppendChunk("first answer", false)
	c.AddToolProgress("tool-1", "bash", "running", "running", nil)
	c.AppendChunk("second answer", false)
	c.FinishAssistantMessage(0, 0, "")

	if got := len(c.messages); got != 4 {
		t.Fatalf("expected 4 chat items, got %d", got)
	}

	firstAssistant, ok := c.messages[1].(*assistantItem)
	if !ok {
		t.Fatalf("expected first assistant item at index 1, got %T", c.messages[1])
	}
	if !firstAssistant.showLabel {
		t.Fatalf("expected first assistant item to keep the label")
	}

	tool, ok := c.messages[2].(*toolItem)
	if !ok {
		t.Fatalf("expected tool item at index 2, got %T", c.messages[2])
	}
	if tool.id != "tool-1" {
		t.Fatalf("expected tool id tool-1, got %q", tool.id)
	}

	continuation, ok := c.messages[3].(*assistantItem)
	if !ok {
		t.Fatalf("expected continuation assistant item at index 3, got %T", c.messages[3])
	}
	if continuation.showLabel {
		t.Fatalf("expected continuation assistant item to omit the label")
	}

	if got := c.GetLastAssistantText(); got != "first answer\n\nsecond answer" {
		t.Fatalf("unexpected assistant text: %q", got)
	}
}

func TestChatAddToolProgressDropsEmptyAssistantPlaceholder(t *testing.T) {
	c := NewChat(common.DefaultStyles(), 80, 20)

	c.StartAssistantMessage()
	c.AddToolProgress("tool-1", "bash", "running", "running", nil)

	if got := len(c.messages); got != 1 {
		t.Fatalf("expected 1 chat item after sealing empty assistant, got %d", got)
	}
	if _, ok := c.messages[0].(*toolItem); !ok {
		t.Fatalf("expected only tool item to remain, got %T", c.messages[0])
	}
}

func TestThinkingBlockToggleChangesCollapsedState(t *testing.T) {
	tb := newThinkingBlock()
	for i := 0; i < 12; i++ {
		tb.append("line\n")
	}
	tb.finish()

	if !tb.collapsed {
		t.Fatalf("expected thinking block to start collapsed")
	}

	collapsed := tb.render(common.DefaultStyles(), 50)
	if want := "8 lines hidden"; !strings.Contains(collapsed, want) {
		t.Fatalf("expected collapsed render to mention %q, got %q", want, collapsed)
	}

	tb.toggle()
	if tb.collapsed {
		t.Fatalf("expected toggle to expand thinking block")
	}

	expanded := tb.render(common.DefaultStyles(), 50)
	if strings.Contains(expanded, "lines hidden") {
		t.Fatalf("expected expanded render to show all lines, got %q", expanded)
	}
}

func TestChatToolSelectionAndDetails(t *testing.T) {
	c := NewChat(common.DefaultStyles(), 80, 20)
	c.AddToolProgress("tool-1", "read_file", "completed", "done", map[string]any{
		"tool_input": map[string]any{"file_path": "/tmp/a.txt"},
		"content":    "alpha\nbeta\ngamma",
	})
	c.AddToolProgress("tool-2", "bash", "completed", "done", map[string]any{
		"tool_input": map[string]any{"command": "ls -la"},
		"stdout":     "file-a\nfile-b",
	})

	if !c.HasSelectedTool() {
		t.Fatalf("expected latest tool to be selected")
	}
	if !c.SelectPrevTool() {
		t.Fatalf("expected previous tool selection to succeed")
	}
	if !c.ToggleSelectedToolExpanded() {
		t.Fatalf("expected selected tool expansion to succeed")
	}
	// Ensure details are closed before testing toggle-to-open.
	c.CloseDetails()
	if !c.ToggleDetails() {
		t.Fatalf("expected selected tool details to toggle")
	}
	if !c.DetailsOpen() {
		t.Fatalf("expected details pane to be open")
	}
	if got := c.DetailView(40, 20); !strings.Contains(got, "a.txt") {
		t.Fatalf("expected detail view to mention selected file, got %q", got)
	}
}

func TestToolSummaryUsesCompactFileName(t *testing.T) {
	tool := newToolItem("tool-1", "write_file", "completed", "done", map[string]any{
		"tool_input": map[string]any{"file_path": "/tmp/example/nested/file.txt"},
		"type":       "create",
	})
	if got := tool.summaryText(); got != "file.txt · create" {
		t.Fatalf("unexpected compact summary: %q", got)
	}
}

func TestChatToolLineClickTogglesExpansion(t *testing.T) {
	c := NewChat(common.DefaultStyles(), 80, 20)
	c.AddToolProgress("tool-1", "read_file", "completed", "done", map[string]any{
		"tool_input": map[string]any{"file_path": "/tmp/a.txt"},
		"content":    "alpha\nbeta",
	})
	if len(c.toolRegions) == 0 {
		t.Fatalf("expected tool regions to be populated")
	}
	line := c.toolRegions[0].startLine
	if !c.HandleMouseDown(0, line) {
		t.Fatalf("expected mouse down on tool line to be handled")
	}
	if got := c.HandleMouseUp(0, line); got != "" {
		t.Fatalf("expected click on tool line not to copy text, got %q", got)
	}
	tool := c.selectedToolItem()
	if tool == nil || !tool.expanded {
		t.Fatalf("expected selected tool to toggle expanded on click")
	}
}

func TestChatMouseSelectionExtractsPlainText(t *testing.T) {
	c := NewChat(common.DefaultStyles(), 80, 20)
	c.AddUserMessage("hello world")
	if !c.HandleMouseDown(0, 0) {
		t.Fatalf("expected mouse down to start selection")
	}
	if !c.HandleMouseDrag(5, 0) {
		t.Fatalf("expected mouse drag to update selection")
	}
	got := c.HandleMouseUp(5, 0)
	if strings.TrimSpace(got) == "" {
		t.Fatalf("expected selected text to be copied")
	}
}

func TestChatMouseSelectionHighlightsDuringDrag(t *testing.T) {
	c := NewChat(common.DefaultStyles(), 80, 20)
	c.AddUserMessage("hello world")
	if !c.HandleMouseDown(0, 0) {
		t.Fatalf("expected mouse down to start selection")
	}
	if !c.HandleMouseDrag(5, 0) {
		t.Fatalf("expected mouse drag to update selection")
	}
	view := c.View()
	if strings.TrimSpace(view) == "" {
		t.Fatalf("expected non-empty view during selection")
	}
	if view == c.plainContent {
		t.Fatalf("expected highlighted selection to add styling")
	}
}

func TestChatThinkingLineClickTogglesCollapse(t *testing.T) {
	c := NewChat(common.DefaultStyles(), 80, 20)
	c.StartAssistantMessage()
	for i := 0; i < 12; i++ {
		c.AppendChunk("thinking line\n", true)
	}
	c.FinishAssistantMessage(0, 0, "")

	assistant, ok := c.messages[0].(*assistantItem)
	if !ok || assistant.thinking == nil {
		t.Fatalf("expected assistant thinking block")
	}
	if len(c.thinkingRegions) == 0 {
		t.Fatalf("expected thinking regions to be populated")
	}
	line := c.thinkingRegions[0].startLine
	if !c.HandleMouseDown(0, line) {
		t.Fatalf("expected mouse down on thinking line to be handled")
	}
	if got := c.HandleMouseUp(0, line); got != "" {
		t.Fatalf("expected click on thinking line not to copy text, got %q", got)
	}
	if assistant.thinking.collapsed {
		t.Fatalf("expected thinking block to expand on click")
	}
}

func TestThinkingBlockRenderShowsMouseHint(t *testing.T) {
	tb := newThinkingBlock()
	for i := 0; i < 12; i++ {
		tb.append("line\n")
	}
	tb.finish()
	rendered := tb.render(common.DefaultStyles(), 50)
	if !strings.Contains(rendered, "click to expand") {
		t.Fatalf("expected mouse hint in thinking footer, got %q", rendered)
	}
	if strings.Contains(rendered, "ctrl+t") {
		t.Fatalf("expected ctrl+t hint to be removed from visible footer, got %q", rendered)
	}
}

func TestChatToolDetailsZoneTogglesDetails(t *testing.T) {
	c := NewChat(common.DefaultStyles(), 80, 20)
	c.AddToolProgress("tool-1", "read_file", "completed", "done", map[string]any{
		"tool_input": map[string]any{"file_path": "/tmp/a.txt"},
		"content":    "alpha\nbeta",
	})
	if len(c.toolRegions) == 0 {
		t.Fatalf("expected tool regions to be populated")
	}
	region := c.toolRegions[0]
	if !c.HandleMouseDown(region.detailStart, region.startLine) {
		t.Fatalf("expected mouse down on tool detail zone to be handled")
	}
	if got := c.HandleMouseUp(region.detailStart, region.startLine); got != "" {
		t.Fatalf("expected click on tool detail zone not to copy text, got %q", got)
	}
	if !c.DetailsOpen() {
		t.Fatalf("expected details pane to open from detail click zone")
	}
}

func TestChatToolBodyClickSelectsWithoutToggling(t *testing.T) {
	c := NewChat(common.DefaultStyles(), 80, 20)
	c.AddToolProgress("tool-1", "read_file", "completed", "done", map[string]any{
		"tool_input": map[string]any{"file_path": "/tmp/a.txt"},
		"content":    "alpha\nbeta",
	})
	tool := c.selectedToolItem()
	if tool == nil {
		t.Fatalf("expected selected tool")
	}
	tool.expanded = false
	tool.invalidate()
	c.selectedTool = -1
	c.detailOpen = false
	c.refresh()
	region := c.toolRegions[0]
	if !c.HandleMouseDown(6, region.startLine) {
		t.Fatalf("expected mouse down on tool body to be handled")
	}
	if got := c.HandleMouseUp(6, region.startLine); got != "" {
		t.Fatalf("expected click on tool body not to copy text, got %q", got)
	}
	if c.selectedToolIndex() < 0 {
		t.Fatalf("expected tool body click to select the tool")
	}
	if c.DetailsOpen() {
		t.Fatalf("expected tool body click not to toggle details")
	}
	if tool.expanded {
		t.Fatalf("expected tool body click not to expand preview")
	}
}

func TestUserItemRenderKeepsMessageInlineWithMarker(t *testing.T) {
	c := NewChat(common.DefaultStyles(), 80, 20)
	u := &userItem{content: "hello world"}
	rendered := u.render(c, 80)
	lines := strings.Split(rendered, "\n")
	if len(lines) == 0 || !strings.Contains(lines[0], "hello world") {
		t.Fatalf("expected first rendered line to include user content, got %q", rendered)
	}
}

func TestChatMouseSelectionPersistsAfterRelease(t *testing.T) {
	c := NewChat(common.DefaultStyles(), 80, 20)
	c.AddUserMessage("hello world")
	if !c.HandleMouseDown(0, 0) {
		t.Fatalf("expected mouse down to start selection")
	}
	if !c.HandleMouseDrag(5, 0) {
		t.Fatalf("expected mouse drag to update selection")
	}
	got := c.HandleMouseUp(5, 0)
	if strings.TrimSpace(got) == "" {
		t.Fatalf("expected selected text to be copied")
	}
	if view := c.View(); view == c.renderedContent {
		t.Fatalf("expected highlighted selection to remain after mouse release")
	}
}

func TestApplySelectionStyleReappliesBackgroundAfterReset(t *testing.T) {
	style := common.DefaultStyles().Selection
	prefix, _ := selectionRenderParts(style)
	got := applySelectionStyle("[31mred[0m plain", style)
	if !strings.Contains(got, "[0m"+prefix+" plain") {
		t.Fatalf("expected selection background to be reapplied after reset, got %q", got)
	}
}

func TestChatMouseDoubleClickSelectsWord(t *testing.T) {
	c := NewChat(common.DefaultStyles(), 80, 20)
	c.AddUserMessage("hello brave world")
	if !c.HandleMouseDown(10, 0) {
		t.Fatalf("expected first mouse down")
	}
	_ = c.HandleMouseUp(10, 0)
	if !c.HandleMouseDown(10, 0) {
		t.Fatalf("expected second mouse down")
	}
	if got := c.selectedText(); got != "brave" {
		t.Fatalf("expected double-click to select word, got %q", got)
	}
}

func TestChatMouseTripleClickSelectsLine(t *testing.T) {
	c := NewChat(common.DefaultStyles(), 80, 20)
	c.AddUserMessage("hello brave world")
	for i := 0; i < 2; i++ {
		if !c.HandleMouseDown(10, 0) {
			t.Fatalf("expected mouse down %d", i+1)
		}
		_ = c.HandleMouseUp(10, 0)
	}
	if !c.HandleMouseDown(10, 0) {
		t.Fatalf("expected third mouse down")
	}
	if got := c.selectedText(); got != "hello brave world" {
		t.Fatalf("expected triple-click to select visual line, got %q", got)
	}
}

func TestAutoExpandActionToolOnCompletion(t *testing.T) {
	c := NewChat(common.DefaultStyles(), 80, 20)
	// Running tool should not be auto-expanded.
	c.AddToolProgress("t1", "bash", "running", "", map[string]any{
		"tool_input": map[string]any{"command": "echo hi"},
	})
	idx := c.selectedToolIndex()
	if idx < 0 {
		t.Fatal("no selected tool")
	}
	tool := c.messages[idx].(*toolItem)
	if tool.expanded {
		t.Error("expected tool not expanded while running")
	}
	// Complete the tool — should auto-expand.
	c.AddToolProgress("t1", "bash", "completed", "done", map[string]any{
		"tool_input": map[string]any{"command": "echo hi"},
		"stdout":     "hi\n",
	})
	if !tool.expanded {
		t.Error("expected tool to be auto-expanded on completion")
	}
}

func TestAutoExpandDoesNotExpandWebTools(t *testing.T) {
	c := NewChat(common.DefaultStyles(), 80, 20)
	c.AddToolProgress("w1", "web_search", "completed", "done", map[string]any{
		"tool_input": map[string]any{"query": "golang"},
	})
	idx := c.selectedToolIndex()
	if idx < 0 {
		t.Fatal("no selected tool")
	}
	tool := c.messages[idx].(*toolItem)
	if tool.expanded {
		t.Error("web_search should not auto-expand")
	}
}

func TestRenderCodeBodyLineNumbers(t *testing.T) {
	styles := common.DefaultStyles()
	// No trailing newline: exactly 3 lines.
	src := "package main\n\nfunc main() {}"
	out := renderCodeBody(styles, "main.go", src, 80, 0, 0)
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "1 ") {
		t.Error("expected line numbers in code body output")
	}
	lines := strings.Split(strings.TrimRight(plain, "\n"), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %q", len(lines), lines)
	}
}

func TestRenderCodeBodyTruncation(t *testing.T) {
	styles := common.DefaultStyles()
	var sb strings.Builder
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&sb, "line%d", i)
		if i < 19 {
			sb.WriteByte('\n')
		}
	}
	out := renderCodeBody(styles, "file.go", sb.String(), 80, 10, 0)
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "10 lines hidden") || !strings.Contains(plain, "enter for full view") {
		t.Errorf("expected truncation footer with hint, got:\n%s", plain)
	}
}

func TestRenderDiffBodyColors(t *testing.T) {
	styles := common.DefaultStyles()
	diff := "+added line\n-removed line\n@@ -1,1 +1,1 @@\n context\n"
	out := renderDiffBody(styles, diff, 80, 0)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines, got %d", len(lines))
	}
}

func TestBashInlineShowsCommandPrompt(t *testing.T) {
	styles := common.DefaultStyles()
	out := renderBashInline(styles, "go test ./...", "ok  foo\nok  bar\n", 80, inlinePreviewLines)
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "$ go test ./...") {
		t.Errorf("expected command prompt in bash inline preview, got:\n%s", plain)
	}
	if !strings.Contains(plain, "ok  foo") {
		t.Error("expected command output in bash inline preview")
	}
}

func TestBashInlineTruncatesLongOutput(t *testing.T) {
	styles := common.DefaultStyles()
	var sb strings.Builder
	for i := 0; i < 30; i++ {
		fmt.Fprintf(&sb, "output line %d\n", i)
	}
	out := renderBashInline(styles, "cmd", sb.String(), 80, inlinePreviewLines)
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "lines hidden") {
		t.Errorf("expected truncation footer, got:\n%s", plain)
	}
}

func TestChatDetailViewScrollsLongContent(t *testing.T) {
	c := NewChat(common.DefaultStyles(), 80, 20)
	var output strings.Builder
	for i := 0; i < 80; i++ {
		output.WriteString(fmt.Sprintf("line %02d ", i))
		output.WriteString(strings.Repeat("x", 20))
		output.WriteString("\n")
	}
	c.AddToolProgress("tool-1", "bash", "completed", "done", map[string]any{
		"tool_input": map[string]any{"command": "cat big.log"},
		"stdout":     output.String(),
	})
	if !c.ToggleDetails() {
		t.Fatalf("expected details to open")
	}
	before := c.DetailView(44, 12)
	c.DetailScrollDown(4)
	after := c.DetailView(44, 12)
	if before == after {
		t.Fatalf("expected detail view to change after scrolling, yOffset=%d total=%d height=%d", c.detail.YOffset(), c.detail.TotalLineCount(), c.detail.Height())
	}
}

func TestChatMouseDragAutoScrollsAtBottom(t *testing.T) {
	c := NewChat(common.DefaultStyles(), 40, 4)
	for i := 0; i < 10; i++ {
		c.AddUserMessage("line")
	}
	c.GotoTop()
	startOffset := c.viewport.YOffset()
	if !c.HandleMouseDown(0, c.height-1) {
		t.Fatalf("expected mouse down at bottom visible line")
	}
	if !c.HandleMouseDrag(0, c.height+2) {
		t.Fatalf("expected drag to continue")
	}
	if c.viewport.YOffset() <= startOffset {
		t.Fatalf("expected drag at bottom edge to autoscroll down")
	}
}

func TestAssistantItemRenderCompactsInterimNarration(t *testing.T) {
	c := NewChat(common.DefaultStyles(), 50, 20)
	a := &assistantItem{
		content:    "I will now inspect the workspace and check several files before running the next tool in order to verify the repository layout and current state.",
		showLabel:  true,
		streaming:  false,
		finishedAt: time.Now(),
		showMeta:   false,
	}
	rendered := ansi.Strip(a.render(c, 50))
	if strings.Contains(rendered, "\n\nI will now inspect") {
		t.Fatalf("expected interim narration to render compactly, got %q", rendered)
	}
	if !strings.Contains(rendered, "…") {
		t.Fatalf("expected interim narration to be truncated with ellipsis, got %q", rendered)
	}
}

func TestChatSelectedTextStripsAssistantMarkerLine(t *testing.T) {
	c := NewChat(common.DefaultStyles(), 80, 20)
	c.StartAssistantMessage()
	c.AppendChunk("hello from assistant", false)
	c.FinishAssistantMessage(0, 0, "")
	c.HandleMouseDown(0, 0)
	c.HandleMouseDrag(25, 1)
	_ = c.HandleMouseUp(25, 1)
	if got := c.selectedText(); strings.Contains(got, "●") {
		t.Fatalf("expected copied assistant text to drop visual marker, got %q", got)
	}
}
