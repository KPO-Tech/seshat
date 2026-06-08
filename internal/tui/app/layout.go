package app

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
)

type chatLayout struct {
	contentW int
	contentX int
	chatX    int
	chatY    int
	chatW    int
	chatH    int
	detailX  int
	detailY  int
	detailW  int
	detailH  int
	inputX   int
	inputY   int
	inputW   int
	inputH   int
	popupX   int
	popupY   int
	popupW   int
	popupH   int
}

func (m Model) currentChatLayout() chatLayout {
	contentW := m.contentWidth()
	contentX := max(0, (m.width-contentW)/2)
	chatY := headerHeight + contentTopGap
	chatW := contentW

	inputW := max(12, contentW-2)
	inputX := max(0, (m.width-inputW)/2)

	var (
		detailX, detailY, detailW, detailH int
		popupW, popupH                     int
		statusH, inputH, chatH             int
	)

	if m.chat.DetailsOpen() && contentW >= 110 {
		paneW := max(36, contentW/3)
		chatW = max(40, contentW-paneW-1)
		detailW = contentW - chatW - 1

		leftStatus := m.statusLineFor(chatW)
		leftInput := m.inputViewFor(chatW)
		statusH = lipgloss.Height(leftStatus)
		inputH = lipgloss.Height(leftInput)
		chatH = m.height - headerHeight - contentTopGap - footerHeight - statusH - inputH
		detailH = max(1, chatH) + statusH + inputH

		detailX = contentX + chatW + 1
		detailY = chatY
		inputX = contentX
		inputW = max(12, chatW-2)
	} else {
		statusView := m.statusLine()
		inputView := m.inputView()
		statusH = lipgloss.Height(statusView)
		inputH = lipgloss.Height(inputView)
		chatH = m.height - headerHeight - contentTopGap - footerHeight - statusH - inputH
	}

	inputY := chatY + max(1, chatH) + statusH

	popupX := inputX
	if m.skillCompletions.IsOpen() {
		popupW = m.skillCompletions.Width(max(24, contentW-4))
		popupH = m.skillCompletions.Height(max(24, contentW-4))
	} else if m.completions.IsOpen() {
		popupW = m.completions.Width(max(20, contentW-4))
		popupH = m.completions.Height(max(20, contentW-4))
	}

	return chatLayout{
		contentW: contentW,
		contentX: contentX,
		chatX:    contentX,
		chatY:    chatY,
		chatW:    chatW,
		chatH:    max(1, chatH),
		detailX:  detailX,
		detailY:  detailY,
		detailW:  detailW,
		detailH:  detailH,
		inputX:   inputX,
		inputY:   inputY,
		inputW:   inputW,
		inputH:   inputH,
		popupX:   popupX,
		popupY:   inputY,
		popupW:   popupW,
		popupH:   popupH,
	}
}

func (m Model) header() string {
	contentW := m.contentWidth()
	logo := m.styles.Logo.Render("NEXUS")
	model := m.styles.HeaderPill.Render(m.workspace.ModelString())
	left := lipgloss.JoinHorizontal(lipgloss.Center, logo, " ", model)
	var modeBadge string
	switch m.chat.ExecutionMode() {
	case "plan":
		modeBadge = lipgloss.NewStyle().
			Foreground(common.ColorPrimary).
			Bold(true).
			Render(" ◈ plan")
	case "pair_programming":
		modeBadge = lipgloss.NewStyle().
			Foreground(common.ColorSecondary).
			Bold(true).
			Render(" ◎ pair")
	default:
		modeBadge = lipgloss.NewStyle().
			Foreground(common.ColorMuted).
			Render(" ● execute")
	}
	left = lipgloss.JoinHorizontal(lipgloss.Center, left, modeBadge)

	var right string
	if m.focus == uiFocusMain && m.state == stateChat {
		right = lipgloss.JoinHorizontal(
			lipgloss.Center,
			m.styles.HeaderPillActive.Render("tools"),
			" ",
			m.styles.HeaderID.Render("n/p navigate · space expand · o details"),
		)
	} else if m.activeSession != "" {
		right = lipgloss.JoinHorizontal(
			lipgloss.Center,
			m.styles.HeaderPillReady.Render("● live"),
			" ",
			m.styles.HeaderPill.Render(common.ShortID(m.activeSession)),
		)
	} else {
		right = m.styles.HeaderPillReady.Render("ready")
	}

	gap := contentW - lipgloss.Width(left) - lipgloss.Width(right) - m.styles.HeaderBar.GetHorizontalFrameSize()
	if gap < 1 {
		gap = 1
	}
	content := m.styles.HeaderBar.Width(contentW).Render(left + strings.Repeat(" ", gap) + right)
	return common.CenterHorizontally(content, m.width)
}

func (m Model) statusLineFor(w int) string {
	var line string
	switch {
	case m.busy && m.cancelling:
		line = m.styles.Footer.Width(w).Render(m.styles.HeaderPillBusy.Render(m.spinner.View() + " interrupting…"))
	case m.busy:
		line = m.styles.Footer.Width(w).Render(m.styles.HeaderPillBusy.Render(m.spinner.View() + " working"))
	case m.lastTurnErr != "":
		line = m.styles.Footer.Width(w).Render(m.styles.ToolError.Render("failed") + "  " + m.styles.Desc.Render(truncateStatus(m.lastTurnErr, max(12, w/2))))
	default:
		line = m.styles.Footer.Width(w).Render(m.styles.Desc.Render("ready"))
	}
	return line
}

func (m Model) statusLine() string {
	return common.CenterHorizontally(m.statusLineFor(m.contentWidth()), m.width)
}

func (m Model) tokenSummary() string {
	total := m.sessionInputTokens + m.sessionOutputTokens
	if total <= 0 {
		return ""
	}
	parts := []string{formatTokenCount(total) + " total"}
	if m.sessionInputTokens > 0 {
		parts = append(parts, "in "+formatTokenCount(m.sessionInputTokens))
	}
	if m.sessionOutputTokens > 0 {
		parts = append(parts, "out "+formatTokenCount(m.sessionOutputTokens))
	}
	return m.styles.Desc.Render(strings.Join(parts, " · "))
}

func truncateStatus(s string, maxLen int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= maxLen {
		return string(r)
	}
	if maxLen <= 1 {
		return string(r[:1])
	}
	return string(r[:maxLen-1]) + "…"
}

func formatTokenCount(n int) string {
	switch {
	case n >= 1_000_000:
		return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1_000_000), ".0"), ".") + "M"
	case n >= 1_000:
		return strings.TrimSuffix(strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1_000), ".0"), ".") + "k"
	default:
		return strconv.Itoa(n)
	}
}

func (m Model) contentWidth() int {
	if m.width <= 4 {
		return m.width
	}
	return m.width - 4
}

func (m Model) footer() string {
	contentW := m.contentWidth()
	if m.copyNotice != "" {
		return common.CenterHorizontally(m.styles.ToolDone.Width(contentW).Render("✓ "+m.copyNotice), m.width)
	}

	var leftItems []string
	if m.focus == uiFocusMain && m.state == stateChat {
		leftItems = []string{
			m.styles.Key.Render("↑↓") + " " + m.styles.Desc.Render("scroll"),
			m.styles.Key.Render("n/p") + " " + m.styles.Desc.Render("tools"),
			m.styles.Key.Render("space") + " " + m.styles.Desc.Render("preview"),
			m.styles.Key.Render("ctrl+o") + " " + m.styles.Desc.Render("details"),
			m.styles.Key.Render("tab") + " " + m.styles.Desc.Render("focus"),
			m.styles.Key.Render("ctrl+p") + " " + m.styles.Desc.Render("settings"),
		}
	} else {
		leftItems = []string{
			m.styles.Key.Render("tab") + " " + m.styles.Desc.Render("tools"),
			m.styles.Key.Render("ctrl+p") + " " + m.styles.Desc.Render("settings"),
			m.styles.Key.Render("ctrl+n") + " " + m.styles.Desc.Render("new"),
			m.styles.Key.Render("ctrl+s") + " " + m.styles.Desc.Render("sessions"),
			m.styles.Key.Render("ctrl+c") + " " + m.styles.Desc.Render("cancel/quit"),
		}
	}
	left := strings.Join(leftItems, "  ")
	right := m.tokenSummary()
	var line string
	if right == "" {
		line = m.styles.Footer.Width(contentW).Render(left)
	} else {
		gap := contentW - lipgloss.Width(left) - lipgloss.Width(right)
		if gap < 2 {
			gap = 2
		}
		line = m.styles.Footer.Width(contentW).Render(left + strings.Repeat(" ", gap) + right)
	}
	return common.CenterHorizontally(line, m.width)
}

// inputViewFor renders the input box at a given outer width (without global centering).
func (m Model) inputViewFor(w int) string {
	inner := m.input.View()
	if attView := m.attachments.View(max(20, w-4)); attView != "" {
		inner = attView + "\n" + inner
	}
	box := m.styles.InputBorder.Width(max(12, w-2)).Render(inner)
	boxW := lipgloss.Width(box)

	// Only show popups in chat/welcome states.
	if m.state == stateChat || m.state == stateWelcome {
		if m.skillCompletions.IsOpen() {
			popup := m.skillCompletions.View(max(24, w-4))
			return lipgloss.NewStyle().Width(boxW).Render(popup) + "\n" + box
		}
		if m.completions.IsOpen() {
			popup := m.completions.View(max(20, w-4))
			return lipgloss.NewStyle().Width(boxW).Render(popup) + "\n" + box
		}
	}
	return box
}

func (m Model) inputView() string {
	return common.CenterHorizontally(m.inputViewFor(m.contentWidth()), m.width)
}

func (m Model) relayout() Model {
	contentW := m.contentWidth()
	chatW := contentW
	inputW := contentW - 4
	if m.chat.DetailsOpen() && contentW >= 110 {
		paneW := max(36, contentW/3)
		chatW = max(40, contentW-paneW-1)
		inputW = chatW - 4
	}
	if inputW < 10 {
		inputW = 10
	}
	m.input.SetWidth(inputW)
	m.sessions.SetSize(m.width, m.height)
	m.permission.SetSize(m.width, m.height)
	m.modelSelect.SetSize(m.width, m.height)
	m.commands.SetSize(m.width, m.height)
	m.configPanel.SetSize(m.width, m.height)
	m.chat.SetSize(chatW, max(1, m.height-headerHeight-contentTopGap-footerHeight-statusHeight-inputMinH-inputPadding))
	return m
}

func (m Model) resizeInput() Model {
	// Count visual rows, not just explicit newlines.
	// Text that wraps due to line width doesn't insert \n into the value.
	contentW := m.contentWidth()
	inputW := contentW - 4 // matches SetWidth in relayout
	if m.chat.DetailsOpen() && contentW >= 110 {
		paneW := max(36, contentW/3)
		chatW := max(40, contentW-paneW-1)
		inputW = chatW - 4
	}
	promptW := 4 // matches SetPromptFunc(4, ...)
	textW := max(1, inputW-promptW)

	visualLines := 0
	for _, line := range strings.Split(m.input.Value(), "\n") {
		runes := []rune(line)
		if len(runes) == 0 {
			visualLines++
		} else {
			visualLines += (len(runes) + textW - 1) / textW
		}
	}
	if visualLines < 1 {
		visualLines = 1
	}
	h := common.Clamp(visualLines, inputMinH, inputMaxH)
	m.input.SetHeight(h)
	return m
}

func (m Model) prevChatState() uiState {
	if m.activeSession != "" {
		return stateChat
	}
	return stateWelcome
}
