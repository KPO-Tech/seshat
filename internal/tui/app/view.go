package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
)

// View returns a tea.View (v2 API — not a string).
func (m Model) View() tea.View {
	if m.width == 0 {
		return tea.NewView("")
	}

	var content string
	switch m.state {
	case stateWelcome:
		content = m.viewWelcome()
	case stateSessions:
		content = m.viewSessions()
	case stateModelSelect:
		content = m.viewModelSelect()
	case stateCommands:
		content = m.viewCommands()
	case stateProviderConfig:
		content = m.viewProviderConfig()
	case stateSearchConfig:
		content = m.viewSearchConfig()
	case stateChat, statePermission:
		content = m.viewChat()
	default:
		content = m.viewChat()
	}

	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// ─── Views ────────────────────────────────────────────────────────────────────

func (m Model) viewWelcome() string {
	// Braille logo rendered in orange primary colour.
	logoArt := lipgloss.NewStyle().Foreground(common.ColorPrimary).Render(common.NexusLogo)

	wordmark := m.styles.Logo.Render("NEXUS")
	tagline := m.styles.HeaderModel.Render("One runtime. Any LLM. Any language.")

	hint := strings.Join([]string{
		m.styles.Key.Render("ctrl+n") + " " + m.styles.Desc.Render("new session"),
		m.styles.Key.Render("ctrl+s") + " " + m.styles.Desc.Render("sessions"),
		m.styles.Key.Render("ctrl+q") + " " + m.styles.Desc.Render("quit"),
	}, "  ")

	extra := ""
	if m.workspace.ModelString() == "" {
		extra = "\n\n" + m.styles.MsgTimestamp.Render("No provider configured — press ") +
			m.styles.Key.Render("ctrl+p") +
			m.styles.MsgTimestamp.Render(" to set one up")
	}

	contentW := m.contentWidth()
	body := lipgloss.NewStyle().
		Width(contentW).
		Height(m.height-2).
		Align(lipgloss.Center, lipgloss.Center).
		Render(logoArt + "\n" + wordmark + "\n\n" + tagline + "\n\n" + hint + extra)

	return m.header() + "\n\n" + common.CenterHorizontally(body, m.width)
}

func (m Model) viewChat() string {
	contentW := m.contentWidth()

	// ── Sidebar-open: join left column (chat+status+input) with sidebar ───
	if m.chat.DetailsOpen() && contentW >= 110 {
		paneW := max(36, contentW/3)
		chatW := max(40, contentW-paneW-1)
		detailW := contentW - chatW - 1

		leftStatus := m.statusLineFor(chatW)
		leftInput := m.inputViewFor(chatW)
		statusH := lipgloss.Height(leftStatus)
		inputH := lipgloss.Height(leftInput)
		chatH := m.height - headerHeight - contentTopGap - footerHeight - statusH - inputH

		m.chat.SetSize(chatW, max(1, chatH))
		leftContent := strings.Join([]string{m.chat.View(), leftStatus, leftInput}, "\n")
		sideH := lipgloss.Height(leftContent)

		detailView := m.chat.DetailView(detailW, sideH)
		body := lipgloss.JoinHorizontal(lipgloss.Top, leftContent, " ", detailView)
		body = common.CenterHorizontally(lipgloss.NewStyle().Width(contentW).Render(body), m.width)

		base := strings.Join([]string{m.header(), "", body, m.footer()}, "\n")
		if m.state == statePermission && m.permission.HasPending() {
			return common.OverlayOn(base, m.permission.View(), m.width, m.height)
		}
		return base
	}

	// ── Normal layout ─────────────────────────────────────────────────────
	inputView := m.inputView()
	statusView := m.statusLine()
	chatH := m.height - headerHeight - contentTopGap - footerHeight - lipgloss.Height(statusView) - lipgloss.Height(inputView)
	m.chat.SetSize(contentW, max(1, chatH))
	body := common.CenterHorizontally(lipgloss.NewStyle().Width(contentW).Render(m.chat.View()), m.width)

	base := strings.Join([]string{m.header(), "", body, statusView, inputView, m.footer()}, "\n")
	if m.state == statePermission && m.permission.HasPending() {
		return common.OverlayOn(base, m.permission.View(), m.width, m.height)
	}
	return base
}

func (m Model) viewSessions() string {
	m.sessions.SetSize(m.width, m.height)
	overlay := m.sessions.Centered()
	var backdrop string
	if m.activeSession != "" {
		backdrop = m.viewChat()
	} else {
		backdrop = m.viewWelcome()
	}
	return common.OverlayOn(backdrop, overlay, m.width, m.height)
}

func (m Model) viewModelSelect() string {
	m.modelSelect.SetSize(m.width, m.height)
	overlay := m.modelSelect.Centered()
	var backdrop string
	if m.activeSession != "" {
		backdrop = m.viewChat()
	} else {
		backdrop = m.viewWelcome()
	}
	return common.OverlayOn(backdrop, overlay, m.width, m.height)
}

func (m Model) viewCommands() string {
	m.commands.SetSize(m.width, m.height)
	overlay := m.commands.Centered()
	var backdrop string
	if m.activeSession != "" {
		backdrop = m.viewChat()
	} else {
		backdrop = m.viewWelcome()
	}
	return common.OverlayOn(backdrop, overlay, m.width, m.height)
}

func (m Model) viewProviderConfig() string {
	m.configPanel.SetSize(m.width, m.height)
	overlay := m.configPanel.Centered()
	var backdrop string
	if m.activeSession != "" {
		backdrop = m.viewChat()
	} else {
		backdrop = m.viewWelcome()
	}
	return common.OverlayOn(backdrop, overlay, m.width, m.height)
}

func (m Model) viewSearchConfig() string {
	m.searchPanel.SetSize(m.width, m.height)
	overlay := m.searchPanel.Centered()
	var backdrop string
	if m.activeSession != "" {
		backdrop = m.viewChat()
	} else {
		backdrop = m.viewWelcome()
	}
	return common.OverlayOn(backdrop, overlay, m.width, m.height)
}
