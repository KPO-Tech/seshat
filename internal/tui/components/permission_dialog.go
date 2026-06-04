package components

import (
	"fmt"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui"
)

// permissionDialog is the modal overlay shown when the agent needs approval.
type PermissionDialog struct {
	styles  common.Styles
	pending *tui.PromptRequestMsg
	width   int
	height  int
}

func NewPermissionDialog(styles common.Styles) *PermissionDialog {
	return &PermissionDialog{styles: styles}
}

func (p *PermissionDialog) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *PermissionDialog) SetPending(msg *tui.PromptRequestMsg) {
	p.pending = msg
}

func (p *PermissionDialog) HasPending() bool {
	return p.pending != nil
}

// Resolve sends the user's response and clears the pending request.
func (p *PermissionDialog) Resolve(value any, cancelled bool) {
	if p.pending == nil {
		return
	}
	select {
	case p.pending.Response <- tui.PromptResponse{Value: value, Cancelled: cancelled}:
	default:
	}
	p.pending = nil
}

// View renders the permission dialog centred on the screen.
func (p *PermissionDialog) View() string {
	if p.pending == nil {
		return ""
	}

	const boxWidth = 56
	w := min(boxWidth, p.width-4)

	msg := p.pending

	title := p.styles.PermTitle.Render("⚠  Permission Required")
	sep := strings.Repeat("─", w-2)

	var body strings.Builder
	body.WriteString(p.styles.PermBody.Render(wordWrap(msg.Message, w-4)))

	var hint string
	switch msg.Type {
	case "confirm":
		hint = fmt.Sprintf("%s yes  %s no",
			p.styles.PermYes.Render("[y]"),
			p.styles.PermNo.Render("[n/esc]"),
		)
	case "choice":
		var opts []string
		for i, opt := range msg.Options {
			opts = append(opts, fmt.Sprintf("[%d] %s", i+1, opt.Label))
		}
		body.WriteString("\n\n" + strings.Join(opts, "  "))
		hint = p.styles.MsgTimestamp.Render("enter number or esc to cancel")
	default:
		hint = p.styles.MsgTimestamp.Render("type your response and press enter")
	}

	content := strings.Join([]string{
		title,
		p.styles.MsgTimestamp.Render(sep),
		body.String(),
		p.styles.MsgTimestamp.Render(sep),
		hint,
	}, "\n")

	box := p.styles.PermBorder.Width(w).Render(content)

	return centreOverlay(box, p.width, p.height)
}

func centreOverlay(content string, width, height int) string {
	lines := strings.Split(content, "\n")
	boxH := len(lines)
	boxW := lipgloss.Width(content)

	topPad := max(0, (height-boxH)/2)
	leftPad := max(0, (width-boxW)/2)
	pad := strings.Repeat(" ", leftPad)

	var sb strings.Builder
	for i := 0; i < topPad; i++ {
		sb.WriteString("\n")
	}
	for i, line := range lines {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(pad + line)
	}
	return sb.String()
}

func wordWrap(text string, width int) string {
	if width <= 0 {
		return text
	}
	var sb strings.Builder
	words := strings.Fields(text)
	line := ""
	for _, w := range words {
		if line == "" {
			line = w
		} else if len(line)+1+len(w) <= width {
			line += " " + w
		} else {
			sb.WriteString(line + "\n")
			line = w
		}
	}
	if line != "" {
		sb.WriteString(line)
	}
	return sb.String()
}
