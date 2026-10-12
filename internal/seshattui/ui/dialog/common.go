package dialog

import (
	"image/color"
	"strings"
	"sync"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/KPO-Tech/seshat-runtime/internal/seshattui/ui/common"
	"github.com/KPO-Tech/seshat-runtime/internal/seshattui/ui/styles"
	"github.com/charmbracelet/x/ansi"
)

// SecretMask is the character a secret input renders in place of its value.
const SecretMask = '•'

// textInputOverhead is how many columns a text input renders in addition to
// the width it was given with SetWidth: its prompt and its cursor cell. It is
// measured rather than assumed, so that a dependency update cannot silently
// push the dialog borders out again.
var textInputOverhead = sync.OnceValue(func() int {
	const probe = 10
	in := textinput.New()
	in.SetVirtualCursor(false)
	in.SetWidth(probe)
	return lipgloss.Width(in.View()) - probe
})

// textInputWidth returns the width to give a text input so that, rendered
// inside promptStyle, it fills exactly contentWidth columns and no more.
func textInputWidth(promptStyle lipgloss.Style, contentWidth int) int {
	return max(0, contentWidth-promptStyle.GetHorizontalFrameSize()-textInputOverhead())
}

// truncateLines truncates every line of s to width columns, so that no single
// part can widen the dialog it is rendered into.
func truncateLines(s string, width int) string {
	if width <= 0 {
		return s
	}
	if !strings.Contains(s, "\n") {
		return ansi.Truncate(s, width, "")
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, width, "")
	}
	return strings.Join(lines, "\n")
}

// newSecretInput returns a text input whose value is rendered as bullets, so
// that an API key or another secret is never displayed on screen. Use
// setSecretVisible (or EchoMode) to show the value again while it is edited.
func newSecretInput() textinput.Model {
	in := textinput.New()
	in.SetVirtualCursor(false)
	in.EchoMode = textinput.EchoPassword
	in.EchoCharacter = SecretMask
	return in
}

// InputCursor adjusts the cursor position for an input field within a dialog.
func InputCursor(t *styles.Styles, cur *tea.Cursor) *tea.Cursor {
	if cur != nil {
		titleStyle := t.Dialog.Title
		dialogStyle := t.Dialog.View
		inputStyle := t.Dialog.InputPrompt
		// Adjust cursor position to account for dialog layout
		cur.X += inputStyle.GetBorderLeftSize() +
			inputStyle.GetMarginLeft() +
			inputStyle.GetPaddingLeft() +
			dialogStyle.GetBorderLeftSize() +
			dialogStyle.GetPaddingLeft() +
			dialogStyle.GetMarginLeft()
		cur.Y += titleStyle.GetVerticalFrameSize() +
			inputStyle.GetBorderTopSize() +
			inputStyle.GetMarginTop() +
			inputStyle.GetPaddingTop() +
			inputStyle.GetBorderBottomSize() +
			inputStyle.GetMarginBottom() +
			inputStyle.GetPaddingBottom() +
			dialogStyle.GetPaddingTop() +
			dialogStyle.GetMarginTop() +
			dialogStyle.GetBorderTopSize()
	}
	return cur
}

// adjustOnboardingInputCursor removes the dialog view frame offset from an
// input cursor. Onboarding dialogs render without Dialog.View frame, while
// InputCursor includes that frame offset for regular dialogs.
func adjustOnboardingInputCursor(t *styles.Styles, cur *tea.Cursor) *tea.Cursor {
	if cur == nil {
		return nil
	}

	dialogStyle := t.Dialog.View
	cur.X -= dialogStyle.GetBorderLeftSize() +
		dialogStyle.GetPaddingLeft() +
		dialogStyle.GetMarginLeft()
	cur.Y -= dialogStyle.GetBorderTopSize() +
		dialogStyle.GetPaddingTop() +
		dialogStyle.GetMarginTop()
	return cur
}

// RenderContext is a dialog rendering context that can be used to render
// common dialog layouts.
type RenderContext struct {
	// Styles is the styles to use for rendering.
	Styles *styles.Styles
	// TitleStyle is the style of the dialog title by default it uses Styles.Dialog.Title
	TitleStyle lipgloss.Style
	// ViewStyle is the style of the dialog title by default it uses Styles.Dialog.View
	ViewStyle lipgloss.Style
	// TitleGradientFromColor is the color the title gradient starts by default
	// its Styles.Dialog.TitleGradFromColor
	TitleGradientFromColor color.Color
	// TitleGradientToColor is the color the title gradient ends by default its
	// Styles.Dialog.TitleGradToColor
	TitleGradientToColor color.Color
	// Width is the total width of the dialog including any margins, borders,
	// and paddings.
	Width int
	// Gap is the gap between content parts. Zero means no gap.
	Gap int
	// Title is the title of the dialog. This will be styled using the default
	// dialog title style and prepended to the content parts slice.
	Title string
	// TitleInfo is additional information to display next to the title. This
	// part is displayed as is, any styling must be applied before setting this
	// field.
	TitleInfo string
	// Parts are the rendered parts of the dialog.
	Parts []string
	// Help is the help view content. This will be appended to the content parts
	// slice using the default dialog help style.
	Help string
	// IsOnboarding indicates whether to render the dialog as part of the
	// onboarding flow. This means that the content will be rendered at the
	// bottom left of the screen.
	IsOnboarding bool
}

// NewRenderContext creates a new RenderContext with the provided styles and width.
func NewRenderContext(t *styles.Styles, width int) *RenderContext {
	return &RenderContext{
		Styles:                 t,
		TitleStyle:             t.Dialog.Title,
		ViewStyle:              t.Dialog.View,
		TitleGradientFromColor: t.Dialog.TitleGradFromColor,
		TitleGradientToColor:   t.Dialog.TitleGradToColor,
		Width:                  width,
		Parts:                  []string{},
	}
}

// AddPart adds a rendered part to the dialog.
func (rc *RenderContext) AddPart(part string) {
	if len(part) > 0 {
		rc.Parts = append(rc.Parts, part)
	}
}

// Render renders the dialog using the provided context.
func (rc *RenderContext) Render() string {
	titleStyle := rc.TitleStyle
	dialogStyle := rc.ViewStyle.Width(rc.Width)

	var parts []string

	if len(rc.Title) > 0 {
		var titleInfoWidth int
		if len(rc.TitleInfo) > 0 {
			titleInfoWidth = lipgloss.Width(rc.TitleInfo)
		}
		title := common.DialogTitle(rc.Styles, rc.Title,
			max(0, rc.Width-dialogStyle.GetHorizontalFrameSize()-
				titleStyle.GetHorizontalFrameSize()-
				titleInfoWidth), rc.TitleGradientFromColor, rc.TitleGradientToColor)
		if len(rc.TitleInfo) > 0 {
			title += rc.TitleInfo
		}
		parts = append(parts, titleStyle.Render(title))
		if rc.Gap > 0 {
			parts = append(parts, make([]string, rc.Gap)...)
		}
	}

	if rc.Gap <= 0 {
		parts = append(parts, rc.Parts...)
	} else {
		for i, p := range rc.Parts {
			if len(p) > 0 {
				parts = append(parts, p)
			}
			if i < len(rc.Parts)-1 {
				parts = append(parts, make([]string, rc.Gap)...)
			}
		}
	}

	// No part may be wider than the content area, or it pushes the border out
	// and leaves the separators short of it.
	contentWidth := max(0, rc.Width-dialogStyle.GetHorizontalFrameSize())
	for i, p := range parts {
		parts[i] = truncateLines(p, contentWidth)
	}

	if len(rc.Help) > 0 {
		if rc.Gap > 0 {
			parts = append(parts, make([]string, rc.Gap)...)
		}
		helpWidth := rc.Width - dialogStyle.GetHorizontalFrameSize()
		helpStyle := rc.Styles.Dialog.HelpView
		helpStyle = helpStyle.Width(helpWidth)
		helpView := ansi.Truncate(helpStyle.Render(rc.Help), helpWidth-1, "")
		parts = append(parts, helpView)
	}

	content := strings.Join(parts, "\n")
	if rc.IsOnboarding {
		return content
	}
	return dialogStyle.Render(content)
}
