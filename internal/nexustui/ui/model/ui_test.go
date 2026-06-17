package model

import (
	"testing"

	"charm.land/bubbles/v2/textarea"
	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/attachments"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/common"
	"github.com/EngineerProjects/nexus-engine/internal/nexustui/ui/styles"
)

func TestRenderEditorViewKeepsTextareaWrapWidth(t *testing.T) {
	const editorWidth = 20

	ta := textarea.New()
	ta.DynamicHeight = true
	ta.MinHeight = TextareaMinHeight
	ta.MaxHeight = TextareaMaxHeight
	ta.SetVirtualCursor(false)
	ta.SetPromptFunc(4, func(info textarea.PromptInfo) string {
		if info.LineNumber == 0 {
			return "  > "
		}
		return "  │ "
	})
	ta.SetValue("abcd efgh ijk")
	ta.SetWidth(editorWidth - 2)

	ui := &UI{
		com:         &common.Common{Styles: &styles.Styles{}},
		textarea:    ta,
		attachments: attachments.New(attachments.NewRenderer(lipgloss.Style{}, lipgloss.Style{}, lipgloss.Style{}, lipgloss.Style{}, lipgloss.Style{}), attachments.Keymap{}),
		focus:       uiFocusEditor,
	}

	rendered := ui.renderEditorView(editorWidth)
	if got := lipgloss.Width(rendered); got != editorWidth {
		t.Fatalf("rendered width = %d, want %d", got, editorWidth)
	}

	wantHeight := ta.Height() + editorHeightMargin
	if got := lipgloss.Height(rendered); got != wantHeight {
		t.Fatalf("rendered height = %d, want %d", got, wantHeight)
	}
}
