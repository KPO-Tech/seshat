// Package model implements the BubbleTea TUI for nexus-engine, adapted from
// Charm's crush project architecture (BubbleTea state machine, workspace
// abstraction, draw cache, permission dialog, session browser).
package model

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui"
)

type uiState uint8

const (
	stateWelcome    uiState = iota
	stateChat
	stateSessions
	statePermission
	stateModelSelect
)

const (
	headerHeight = 1
	footerHeight = 1
	inputMinH    = 3
	inputMaxH    = 7
	inputPadding = 2
)

// Model is the top-level BubbleTea model for nexus-engine's TUI.
type Model struct {
	workspace tui.Workspace
	ctx       context.Context
	cancel    context.CancelFunc

	state  uiState
	keys   KeyMap
	styles Styles

	width  int
	height int

	chat        *chat
	sessions    *sessionList
	permission  *permissionDialog
	modelSelect *modelDialog
	completions *fileCompletions
	attachments *attachments
	input       textarea.Model
	spinner     spinner.Model

	busy          bool
	activeSession string
	lastErr       error
	permInput     string
}

func New(ws tui.Workspace, ctx context.Context) Model {
	ctx, cancel := context.WithCancel(ctx)

	styles := DefaultStyles()
	keys := DefaultKeys()

	ta := textarea.New()
	ta.Placeholder = "Type a message… (enter to send, shift+enter for newline)"
	ta.Focus()
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.SetWidth(80)
	ta.SetHeight(inputMinH)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorYellow)

	return Model{
		workspace:   ws,
		ctx:         ctx,
		cancel:      cancel,
		state:       stateWelcome,
		keys:        keys,
		styles:      styles,
		chat:        newChat(styles, 80, 20),
		sessions:    newSessionList(styles),
		permission:  newPermissionDialog(styles),
		modelSelect: newModelDialog(styles),
		completions: newFileCompletions(styles, ws.WorkingDir()),
		attachments: newAttachments(styles),
		input:       ta,
		spinner:     sp,
	}
}

// ─── BubbleTea v2 interface ───────────────────────────────────────────────────

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.spinner.Tick,
		m.loadSessions(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m = m.relayout()

	case spinner.TickMsg:
		if m.busy {
			newSp, cmd := m.spinner.Update(msg)
			m.spinner = newSp
			cmds = append(cmds, cmd)
		}

	case tui.ChunkMsg:
		if m.state == stateChat || m.state == statePermission {
			m.chat.AppendChunk(msg.Text, msg.IsThinking)
		}

	case tui.ToolProgressMsg:
		label := msg.Label
		if label == "" {
			label = msg.Status
		}
		m.chat.AddToolProgress(msg.ToolName, msg.Status, label)

	case tui.TurnStartMsg:
		m.busy = true
		m.chat.StartAssistantMessage()
		cmds = append(cmds, m.spinner.Tick)

	case tui.TurnDoneMsg:
		m.busy = false
		m.chat.FinishAssistantMessage()
		if msg.Err != nil {
			m.chat.AddError(msg.Err)
		}

	case tui.PromptRequestMsg:
		m.permission.SetPending(&msg)
		m.permInput = ""
		m.state = statePermission

	case tui.SessionListMsg:
		if msg.Err == nil {
			m.sessions.SetSessions(msg.Sessions)
		}

	case tui.SessionCreatedMsg:
		if msg.Err != nil {
			m.lastErr = msg.Err
		} else {
			m.activeSession = msg.ID
			m.state = stateChat
			m.chat.Clear()
			m.chat.AddSystem("New session · " + shortID(msg.ID))
			m.input.Focus()
		}

	case tui.SessionLoadedMsg:
		if msg.Err != nil {
			m.lastErr = msg.Err
		} else {
			m.activeSession = msg.ID
			m.state = stateChat
			m.chat.Clear()
			m.chat.AddSystem("Resumed session · " + shortID(msg.ID))
			m.input.Focus()
		}

	case tui.ModelListMsg:
		if msg.Err == nil {
			m.modelSelect.SetModels(msg.Models)
		}

	case tui.ModelChangedMsg:
		// Header will pick up new model string from workspace.ModelString()

	case tui.ErrMsg:
		m.lastErr = msg.Err

	// v2 uses KeyPressMsg instead of KeyMsg
	case tea.KeyPressMsg:
		cmd := m.handleKey(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}

	if m.state == stateChat {
		newInput, cmd := m.input.Update(msg)
		m.input = newInput
		cmds = append(cmds, cmd)
		m = m.resizeInput()
	}

	return m, tea.Batch(cmds...)
}

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

// ─── Key handling ─────────────────────────────────────────────────────────────

func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()

	if m.state == statePermission && m.permission.HasPending() {
		switch {
		case k == "y" || k == "Y":
			m.permission.Resolve(true, false)
			m.state = stateChat
		case k == "n" || k == "N" || k == "esc":
			m.permission.Resolve(false, true)
			m.state = stateChat
		case k == "a" || k == "A":
			m.permission.Resolve("always", false)
			m.state = stateChat
		default:
			m.permInput += k
		}
		return nil
	}

	// ── Model selection dialog ───────────────────────────────────────────
	if m.state == stateModelSelect {
		switch k {
		case "esc", "ctrl+m":
			m.state = m.prevChatState()
		case "up", "k":
			m.modelSelect.Up()
		case "down", "j":
			m.modelSelect.Down()
		case "enter":
			if sel := m.modelSelect.Selected(); sel != nil {
				m.workspace.SetModel(sel.Provider, sel.Identifier)
				m.state = m.prevChatState()
			}
		case "backspace":
			m.modelSelect.DeleteFilter()
		default:
			if len(k) == 1 {
				m.modelSelect.TypeFilter(k)
			}
		}
		return nil
	}

	if m.state == stateSessions {
		switch k {
		case "esc", "ctrl+s":
			m.state = m.prevChatState()
		case "up", "k":
			m.sessions.Up()
		case "down", "j":
			m.sessions.Down()
		case "enter":
			id := m.sessions.Selected()
			if id != "" {
				m.state = stateChat
				return m.loadSession(id)
			}
		case "d", "delete":
			id := m.sessions.DeleteSelected()
			if id != "" {
				return m.deleteSession(id)
			}
		case "backspace":
			m.sessions.DeleteFilter()
		default:
			if len(k) == 1 {
				m.sessions.TypeFilter(k)
			}
		}
		return nil
	}

	switch k {
	case "ctrl+c":
		if m.busy {
			m.workspace.Cancel()
			return nil
		}
		m.cancel()
		return tea.Quit
	case "ctrl+q":
		m.cancel()
		return tea.Quit
	case "ctrl+s":
		if m.state == stateChat || m.state == stateWelcome {
			m.state = stateSessions
			return m.loadSessions()
		}
	case "ctrl+n":
		return m.createSession()
	case "ctrl+m":
		if m.state != stateModelSelect {
			m.state = stateModelSelect
			m.modelSelect.ClearFilter()
			return m.listModels()
		}
	}

	if m.state == stateChat || m.state == stateWelcome {
		// ── File completions (@) ────────────────────────────────────────────
		if m.completions.IsOpen() {
			switch k {
			case "esc":
				m.completions.Close()
				return nil
			case "up":
				m.completions.Up()
				return nil
			case "down":
				m.completions.Down()
				return nil
			case "enter", "tab":
				if sel := m.completions.Selected(); sel != "" {
					// Replace "@query" in input with the selected path.
					query := m.completions.Query()
					val := m.input.Value()
					atIdx := strings.LastIndex(val, "@"+query)
					if atIdx >= 0 {
						m.input.SetValue(val[:atIdx] + sel + val[atIdx+len("@"+query):])
					}
					m.completions.Close()
				}
				return nil
			case "backspace":
				m.completions.Backspace()
				return nil
			default:
				if len(k) == 1 && k != "@" {
					m.completions.TypeChar(k)
					return nil
				}
			}
		}

		switch k {
		case "@":
			// Open file completions popup.
			m.completions.Open(m.workspace.WorkingDir())
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return cmd

		case "enter":
			text := strings.TrimSpace(m.input.Value())
			if text == "" || m.busy {
				return nil
			}
			if m.activeSession == "" {
				return tea.Batch(m.createSession(), func() tea.Msg {
					return pendingSubmitMsg{prompt: text}
				})
			}
			// Pass attachments with the message.
			atts := m.attachments.List()
			_ = atts // TODO: pass to workspace.Submit when SDK supports attachments
			m.attachments.Reset()
			m.input.Reset()
			m.chat.AddUserMessage(text)
			m.workspace.Submit(m.ctx, text)
			return nil

		case "shift+enter", "alt+enter":
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return cmd

		case "ctrl+a":
			// Ctrl+A = add attachment (open file dialog stub for now)
			return nil

		case "up":
			if m.input.Value() == "" {
				m.chat.ScrollUp(3)
				return nil
			}
		case "down":
			if m.input.Value() == "" {
				m.chat.ScrollDown(3)
				return nil
			}
		case "pgup":
			m.chat.PageUp()
		case "pgdown":
			m.chat.PageDown()
		case "home":
			m.chat.GotoTop()
		case "end":
			m.chat.GotoBottom()
		}
	}

	return nil
}

// pendingSubmitMsg is used to queue a prompt while session creation is pending.
type pendingSubmitMsg struct{ prompt string }

// ─── Views ────────────────────────────────────────────────────────────────────

func (m Model) viewWelcome() string {
	logo := m.styles.Logo.Render("◉ NEXUS")
	tagline := m.styles.HeaderModel.Render("One runtime. Any LLM. Any language.")

	hint := strings.Join([]string{
		m.styles.Key.Render("ctrl+n") + " " + m.styles.Desc.Render("new session"),
		m.styles.Key.Render("ctrl+s") + " " + m.styles.Desc.Render("sessions"),
		m.styles.Key.Render("ctrl+q") + " " + m.styles.Desc.Render("quit"),
	}, "  ")

	body := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height - 2).
		Align(lipgloss.Center, lipgloss.Center).
		Render(logo + "\n\n" + tagline + "\n\n" + hint)

	return m.header() + "\n" + body
}

func (m Model) viewChat() string {
	inputView := m.inputView()
	chatH := m.height - headerHeight - footerHeight - lipgloss.Height(inputView)
	m.chat.SetSize(m.width, max(1, chatH))
	chatView := m.chat.View()

	base := strings.Join([]string{
		m.header(),
		chatView,
		inputView,
		m.footer(),
	}, "\n")

	if m.state == statePermission && m.permission.HasPending() {
		overlay := m.permission.View()
		return overlayOn(base, overlay, m.width, m.height)
	}
	return base
}

func (m Model) viewSessions() string {
	m.sessions.SetSize(m.width, m.height)
	overlay := m.sessions.centred()
	var backdrop string
	if m.activeSession != "" {
		backdrop = m.viewChat()
	} else {
		backdrop = m.viewWelcome()
	}
	return overlayOn(backdrop, overlay, m.width, m.height)
}

func (m Model) viewModelSelect() string {
	m.modelSelect.SetSize(m.width, m.height)
	overlay := m.modelSelect.centred()
	var backdrop string
	if m.activeSession != "" {
		backdrop = m.viewChat()
	} else {
		backdrop = m.viewWelcome()
	}
	return overlayOn(backdrop, overlay, m.width, m.height)
}

func (m Model) header() string {
	logo := m.styles.Logo.Render("◉ NEXUS")
	sep := m.styles.HeaderSep.Render("  │  ")
	model := m.styles.HeaderModel.Render(m.workspace.ModelString())

	var status string
	if m.busy {
		status = m.spinner.View() + " " + m.styles.HeaderBusy.Render("working")
	} else if m.activeSession != "" {
		status = m.styles.HeaderReady.Render("●") + " " + m.styles.HeaderID.Render(shortID(m.activeSession))
	} else {
		status = m.styles.HeaderReady.Render("ready")
	}

	left := logo + sep + model
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(status) - 2
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + status
}

func (m Model) footer() string {
	items := []string{
		m.styles.Key.Render("ctrl+n") + " " + m.styles.Desc.Render("new"),
		m.styles.Key.Render("ctrl+s") + " " + m.styles.Desc.Render("sessions"),
		m.styles.Key.Render("ctrl+m") + " " + m.styles.Desc.Render("model"),
		m.styles.Key.Render("@") + " " + m.styles.Desc.Render("file"),
		m.styles.Key.Render("ctrl+c") + " " + m.styles.Desc.Render("cancel/quit"),
	}
	return m.styles.Footer.Render(strings.Join(items, "  "))
}

func (m Model) inputView() string {
	inner := m.input.View()

	// Attachments strip above the textarea.
	if attView := m.attachments.View(m.width - 4); attView != "" {
		inner = attView + "\n" + inner
	}

	box := m.styles.InputBorder.Width(m.width - 2).Render(inner)

	// File completions popup rendered directly above the input box.
	if m.completions.IsOpen() {
		popup := m.completions.View(m.width - 4)
		return popup + "\n" + box
	}
	return box
}

// ─── Layout ───────────────────────────────────────────────────────────────────

func (m Model) relayout() Model {
	inputW := m.width - 4
	if inputW < 10 {
		inputW = 10
	}
	m.input.SetWidth(inputW)
	m.sessions.SetSize(m.width, m.height)
	m.permission.SetSize(m.width, m.height)
	m.modelSelect.SetSize(m.width, m.height)
	m.chat.SetSize(m.width, max(1, m.height-headerHeight-footerHeight-inputMinH-inputPadding))
	return m
}

func (m Model) resizeInput() Model {
	lines := strings.Count(m.input.Value(), "\n") + 1
	h := clamp(lines, inputMinH, inputMaxH)
	m.input.SetHeight(h)
	return m
}

func (m Model) prevChatState() uiState {
	if m.activeSession != "" {
		return stateChat
	}
	return stateWelcome
}

// ─── Workspace commands ───────────────────────────────────────────────────────

func (m Model) loadSessions() tea.Cmd {
	return func() tea.Msg { m.workspace.ListSessions(m.ctx); return nil }
}

func (m Model) listModels() tea.Cmd {
	return func() tea.Msg { m.workspace.ListModels(m.ctx); return nil }
}

func (m Model) createSession() tea.Cmd {
	return func() tea.Msg { m.workspace.CreateSession(m.ctx); return nil }
}

func (m Model) loadSession(id string) tea.Cmd {
	return func() tea.Msg { m.workspace.LoadSession(m.ctx, id); return nil }
}

func (m Model) deleteSession(id string) tea.Cmd {
	return func() tea.Msg {
		_ = m.workspace.DeleteSession(m.ctx, id)
		m.workspace.ListSessions(m.ctx)
		return nil
	}
}

// ─── Overlay compositor ───────────────────────────────────────────────────────

func overlayOn(base, overlay string, width, height int) string {
	if overlay == "" {
		return base
	}
	baseLines := strings.Split(base, "\n")
	overlayLines := strings.Split(overlay, "\n")
	overlayH := len(overlayLines)
	for len(baseLines) < height {
		baseLines = append(baseLines, strings.Repeat(" ", width))
	}
	topOffset := max(0, (height-overlayH)/2)
	dim := lipgloss.NewStyle().Faint(true)
	for i, line := range baseLines {
		overlayRow := i - topOffset
		if overlayRow >= 0 && overlayRow < overlayH {
			baseLines[i] = overlayLines[overlayRow]
		} else {
			baseLines[i] = dim.Render(line)
		}
	}
	return strings.Join(baseLines, "\n")
}

// ─── Utilities ────────────────────────────────────────────────────────────────

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Run starts the BubbleTea program and blocks until it exits.
func Run(ws tui.Workspace, ctx context.Context) error {
	m := New(ws, ctx)
	p := tea.NewProgram(m, tea.WithContext(ctx))
	ws.Subscribe(p)
	_, err := p.Run()
	return err
}

var _ = fmt.Sprintf
