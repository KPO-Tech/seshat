// Package model implements the BubbleTea TUI for nexus-engine, adapted from
// Charm's crush project architecture (BubbleTea state machine, workspace
// abstraction, draw cache, permission dialog, session browser).
package app

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
	"github.com/EngineerProjects/nexus-engine/internal/tui/components"
	clipboard "github.com/atotto/clipboard"
)

type uiState uint8

const (
	stateWelcome uiState = iota
	stateChat
	stateSessions
	statePermission
	stateModelSelect
	stateCommands
	stateProviderConfig
)

// uiFocus mirrors crush's focus model: editor has the cursor / main lets
// the user scroll chat with arrow keys.
type uiFocus uint8

const (
	uiFocusEditor uiFocus = iota // textarea is active (default)
	uiFocusMain                  // chat list is scrollable with arrow keys
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
	keys   common.KeyMap
	styles common.Styles

	width  int
	height int

	chat        *components.Chat
	sessions    *components.SessionList
	permission  *components.PermissionDialog
	modelSelect *components.ModelPicker
	commands    *components.CommandPalette
	configPanel *components.ConfigPanel
	completions *components.FileCompletions
	attachments *components.Attachments
	input       textarea.Model
	spinner     spinner.Model

	focus         uiFocus
	busy          bool
	activeSession string
	lastErr       error
	permInput     string
	copyNotice    string  // transient "Copied!" message shown in footer
	selectMode    bool    // when true: mouse capture disabled so terminal handles selection
	returnState   uiState // state to restore when pressing ← from a sub-dialog
}

func New(ws tui.Workspace, ctx context.Context) Model {
	ctx, cancel := context.WithCancel(ctx)

	styles := common.DefaultStyles()
	keys := common.DefaultKeys()

	ta := textarea.New()
	ta.Placeholder = "Type a message… (enter to send, shift+enter for newline)"
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.SetWidth(80)
	ta.SetHeight(inputMinH)
	// Don't call Focus() here — do it in Init() so the Cmd runs properly.

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(common.ColorYellow)

	return Model{
		workspace:   ws,
		ctx:         ctx,
		cancel:      cancel,
		state:       stateWelcome,
		focus:       uiFocusEditor,
		keys:        keys,
		styles:      styles,
		chat:        components.NewChat(styles, 80, 20),
		sessions:    components.NewSessionList(styles),
		permission:  components.NewPermissionDialog(styles),
		modelSelect: components.NewModelPicker(styles),
		commands:    components.NewCommandPalette(styles),
		configPanel: components.NewConfigPanel(styles),
		completions: components.NewFileCompletions(styles, ws.WorkingDir()),
		attachments: components.NewAttachments(styles),
		input:       ta,
		spinner:     sp,
	}
}

// ─── BubbleTea v2 interface ───────────────────────────────────────────────────

func (m Model) Init() tea.Cmd {
	// Focus() in bubbles/v2 returns a Cmd that sets up the cursor — must run.
	return tea.Batch(
		m.input.Focus(),
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
		m.chat.AddToolProgress(msg.ToolUseID, msg.ToolName, msg.Status, label, msg.Metadata)

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
			m.focus = uiFocusEditor
			m.chat.Clear()
			m.chat.AddSystem("New session · " + common.ShortID(msg.ID))
			cmds = append(cmds, m.input.Focus()) // v2: Focus() returns a Cmd
		}

	case tui.SessionLoadedMsg:
		if msg.Err != nil {
			m.lastErr = msg.Err
		} else {
			m.activeSession = msg.ID
			m.state = stateChat
			m.focus = uiFocusEditor
			m.chat.Clear()
			m.chat.AddSystem("Resumed session · " + common.ShortID(msg.ID))
			cmds = append(cmds, m.input.Focus()) // v2: Focus() returns a Cmd
		}

	case tui.ModelListMsg:
		if msg.Err == nil {
			m.modelSelect.SetModels(msg.Models)
		}

	case tui.ModelChangedMsg:
		// Header will pick up new model string from workspace.ModelString()

	case tui.ErrMsg:
		m.lastErr = msg.Err

	case clearCopyNoticeMsg:
		m.copyNotice = ""

	case providerConfigLoadedMsg:
		m.configPanel.SetProviders(msg.providers)

	case cfgSaveResultMsg:
		if msg.err != nil {
			m.configPanel.SetError(msg.err.Error())
		} else {
			m.configPanel.SetSaved()
		}

	// v2 uses KeyPressMsg instead of KeyMsg
	case tea.KeyPressMsg:
		consumed, cmd := m.handleKey(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		// Non-consumed keys flow to the textarea so regular characters,
		// backspace, and cursor movement work normally.
		if !consumed && (m.state == stateChat || m.state == stateWelcome) {
			newInput, inputCmd := m.input.Update(msg)
			m.input = newInput
			cmds = append(cmds, inputCmd)
			m = m.resizeInput()
		}
		return m, tea.Batch(cmds...)

	case tea.MouseWheelMsg:
		// Mouse wheel scrolls chat regardless of focus state (no Tab required).
		if m.state == stateChat || m.state == stateWelcome {
			switch msg.Button {
			case tea.MouseWheelUp:
				m.chat.ScrollUp(3)
			case tea.MouseWheelDown:
				m.chat.ScrollDown(3)
			}
		}
		return m, tea.Batch(cmds...)
	}

	// Non-key messages (spinner, window resize, etc.) are also forwarded
	// to the textarea so blinking and focus work correctly.
	if m.state == stateChat || m.state == stateWelcome {
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
	case stateCommands:
		content = m.viewCommands()
	case stateProviderConfig:
		content = m.viewProviderConfig()
	case stateChat, statePermission:
		content = m.viewChat()
	default:
		content = m.viewChat()
	}

	v := tea.NewView(content)
	v.AltScreen = true
	if m.selectMode {
		// In select mode, release mouse capture so the terminal handles
		// native text selection. Mouse scroll is temporarily unavailable.
		v.MouseMode = tea.MouseModeNone
	} else {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

// ─── Key handling ─────────────────────────────────────────────────────────────

// handleKey processes a keypress. Returns (consumed, cmd):
//   - consumed=true  → key was handled; do NOT forward to textarea
//   - consumed=false → key was not handled; forward to textarea for normal input
func (m *Model) handleKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	k := msg.String()

	// ── Permission dialog (all keys consumed) ────────────────────────────
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
		return true, nil
	}

	// ── Model selection (all keys consumed) ─────────────────────────────
	if m.state == stateModelSelect {
		switch k {
		case "esc", "ctrl+m":
			m.state = m.prevChatState()
		case "left":
			// Navigate back to the commands palette if that's where we came from,
			// otherwise close to the chat/welcome state.
			if m.returnState == stateCommands {
				m.state = stateCommands
				m.commands.Open("")
			} else {
				m.state = m.prevChatState()
			}
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
		return true, nil
	}

	// ── Commands palette (all keys consumed) ────────────────────────────
	if m.state == stateCommands {
		switch k {
		case "esc", "ctrl+p":
			m.state = m.prevChatState()
		case "up", "k":
			m.commands.Up()
		case "down", "j":
			m.commands.Down()
		case "enter":
			var cmd tea.Cmd
			if sel := m.commands.Selected(); sel != nil {
				cmd = m.executeCommand(sel.ID)
			}
			if m.state == stateCommands {
				m.state = m.prevChatState()
			}
			return true, cmd
		case "backspace":
			m.commands.DeleteFilter()
		default:
			if len(k) == 1 {
				m.commands.TypeFilter(k)
			}
		}
		return true, nil
	}

	// ── Provider config panel (all keys consumed) ───────────────────────
	if m.state == stateProviderConfig {
		cp := m.configPanel
		if cp.IsEditing() {
			switch k {
			case "esc":
				m.state = m.prevChatState()
			case "left":
				cp.ExitEdit()
				// Reload provider status after editing.
				return true, m.loadProviderConfig()
			case "up", "k":
				cp.Up()
			case "down", "j":
				cp.Down()
			case "tab":
				cp.Down()
			case "enter":
				draft, _, fieldKey := cp.CurrentFieldDraft()
				if strings.TrimSpace(draft) == "" {
					return true, nil
				}
				providerID := cp.EditedProviderID()
				return true, func() tea.Msg {
					err := m.workspace.SaveProviderField(m.ctx, providerID, fieldKey, strings.TrimSpace(draft))
					if err != nil {
						return cfgSaveResultMsg{err: err}
					}
					return cfgSaveResultMsg{}
				}
			case "backspace":
				cp.DeleteChar()
			case "ctrl+v":
				cp.ToggleReveal()
			default:
				if len(k) == 1 {
					cp.TypeChar(k)
				}
			}
		} else {
			switch k {
			case "esc", "ctrl+,":
				m.state = m.prevChatState()
			case "up", "k":
				cp.Up()
			case "down", "j":
				cp.Down()
			case "enter":
				cp.EnterEdit()
			case "backspace":
				cp.DeleteFilter()
			default:
				if len(k) == 1 {
					cp.TypeFilter(k)
				}
			}
		}
		return true, nil
	}

	// ── Session browser (all keys consumed) ─────────────────────────────
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
				return true, m.loadSession(id)
			}
		case "d", "delete":
			id := m.sessions.DeleteSelected()
			if id != "" {
				return true, m.deleteSession(id)
			}
		case "backspace":
			m.sessions.DeleteFilter()
		default:
			if len(k) == 1 {
				m.sessions.TypeFilter(k)
			}
		}
		return true, nil
	}

	// ── Select mode toggle (ctrl+e) — works from any state ──────────────
	if k == "ctrl+e" {
		m.selectMode = !m.selectMode
		return true, nil
	}

	// ── Global shortcuts (always consumed) ──────────────────────────────
	switch k {
	case "ctrl+c":
		if m.busy {
			m.workspace.Cancel()
			return true, nil
		}
		m.cancel()
		return true, tea.Quit
	case "ctrl+q":
		m.cancel()
		return true, tea.Quit
	case "ctrl+p":
		if m.state != stateCommands {
			m.commands.Open("")
			m.state = stateCommands
		}
		return true, nil
	case "ctrl+,":
		if m.state != stateProviderConfig {
			m.state = stateProviderConfig
			return true, m.loadProviderConfig()
		}
		return true, nil
	case "ctrl+s":
		if m.state == stateChat || m.state == stateWelcome {
			m.state = stateSessions
			return true, m.loadSessions()
		}
	case "ctrl+n":
		return true, m.createSession()
	case "ctrl+m":
		if m.state != stateModelSelect {
			m.returnState = m.prevChatState()
			m.state = stateModelSelect
			m.modelSelect.ClearFilter()
			return true, m.listModels()
		}
	case "tab":
		// Tab toggles between editor focus (typing) and main focus (scrolling).
		if m.state == stateChat {
			if m.focus == uiFocusEditor {
				m.focus = uiFocusMain
				m.input.Blur()
			} else {
				m.focus = uiFocusEditor
				return true, m.input.Focus()
			}
			return true, nil
		}
	}

	// ── Chat / welcome: dispatch by focus state (crush pattern) ──────────
	if m.state == stateChat || m.state == stateWelcome {

		// When focus is on the chat list, arrow keys scroll rather than move cursor.
		if m.focus == uiFocusMain {
			switch k {
			case "up", "k":
				m.chat.ScrollUp(3)
				return true, nil
			case "down", "j":
				m.chat.ScrollDown(3)
				return true, nil
			case "pgup":
				m.chat.PageUp()
				return true, nil
			case "pgdown":
				m.chat.PageDown()
				return true, nil
			case "home":
				m.chat.GotoTop()
				return true, nil
			case "end":
				m.chat.GotoBottom()
				return true, nil
			case "n":
				return true, boolCmd(m.chat.SelectNextTool())
			case "p":
				return true, boolCmd(m.chat.SelectPrevTool())
			case "space":
				return true, boolCmd(m.chat.ToggleSelectedToolExpanded())
			case "o", "enter", "right":
				return true, boolCmd(m.chat.ToggleDetails())
			case "left", "esc":
				m.chat.CloseDetails()
				return true, nil
			}
			m.focus = uiFocusEditor
			return true, m.input.Focus()
		}
		// ── Editor focus (default) ────────────────────────────────────────

		// File completions popup intercepts keys while open.
		if m.completions.IsOpen() {
			switch k {
			case "esc":
				m.completions.Close()
			case "up":
				m.completions.Up()
			case "down":
				m.completions.Down()
			case "enter", "tab":
				if sel := m.completions.Selected(); sel != "" {
					query := m.completions.Query()
					val := m.input.Value()
					atIdx := strings.LastIndex(val, "@"+query)
					if atIdx >= 0 {
						m.input.SetValue(val[:atIdx] + sel + val[atIdx+len("@"+query):])
					}
					m.completions.Close()
				}
			case "backspace":
				m.completions.Backspace()
			default:
				if len(k) == 1 && k != "@" {
					m.completions.TypeChar(k)
				} else {
					m.completions.Close()
					return false, nil
				}
			}
			return true, nil
		}

		switch k {
		case "/":
			// Open the commands palette pre-filtered to slash commands when
			// the input is empty. If there's already text, let / go to textarea.
			if strings.TrimSpace(m.input.Value()) == "" {
				m.commands.Open("/")
				m.state = stateCommands
				return true, nil
			}
			return false, nil

		case "@":
			// Open completions AND let textarea receive @ to show it in the input.
			m.completions.Open(m.workspace.WorkingDir())
			// Fall through to textarea (consumed=false) so @ appears in input.
			return false, nil

		case "enter":
			text := strings.TrimSpace(m.input.Value())
			if text == "" || m.busy {
				return true, nil
			}
			if m.activeSession == "" {
				return true, tea.Batch(m.createSession(), func() tea.Msg {
					return pendingSubmitMsg{prompt: text}
				})
			}
			atts := m.attachments.List()
			_ = atts
			m.attachments.Reset()
			m.input.Reset()
			m.chat.AddUserMessage(text)
			m.workspace.Submit(m.ctx, text)
			return true, nil

		case "shift+enter", "alt+enter":
			// crush uses InsertRune('\n') directly — more reliable than Update(msg).
			m.input.InsertRune('\n')
			return true, nil

		case "ctrl+t":
			// Toggle thinking block collapse on the most recent assistant message.
			m.chat.ToggleThinking()
			return true, nil

		case "ctrl+u":
			// Copy last user message to clipboard.
			text := m.chat.GetLastUserText()
			if text != "" {
				return true, m.copyToClipboard(text, "Message copied")
			}
			return true, nil

		case "ctrl+a":
			return true, nil

		case "pgup":
			m.chat.PageUp()
			return true, nil
		case "pgdown":
			m.chat.PageDown()
			return true, nil
		case "home":
			m.chat.GotoTop()
			return true, nil
		case "end":
			m.chat.GotoBottom()
			return true, nil
		}
	}

	// Key was not handled — forward to the textarea.
	return false, nil
}

func (m *Model) executeCommand(id string) tea.Cmd {
	switch id {
	case "new-session":
		return m.createSession()
	case "sessions":
		m.state = stateSessions
		return m.loadSessions()
	case "model":
		m.returnState = stateCommands
		m.state = stateModelSelect
		m.modelSelect.ClearFilter()
		return m.listModels()
	case "thinking":
		m.chat.ToggleThinking()
		return nil
	case "select":
		m.selectMode = !m.selectMode
		return nil
	case "copy-msg":
		text := m.chat.GetLastUserText()
		if text != "" {
			return m.copyToClipboard(text, "Message copied")
		}
		return nil
	case "provider-config":
		m.state = stateProviderConfig
		return m.loadProviderConfig()
	case "clear":
		m.chat.Clear()
		if m.activeSession != "" {
			m.chat.AddSystem("Chat cleared")
		}
		return nil
	case "quit":
		m.cancel()
		return tea.Quit
	default:
		return nil
	}
}

// pendingSubmitMsg is used to queue a prompt while session creation is pending.
type pendingSubmitMsg struct{ prompt string }

// clearCopyNoticeMsg clears the transient "Copied!" footer message.
type clearCopyNoticeMsg struct{}

// cfgSaveResultMsg is sent after attempting to save a provider credential.
type cfgSaveResultMsg struct{ err error }

// providerConfigLoadedMsg carries a refreshed provider list.
type providerConfigLoadedMsg struct{ providers []tui.ProviderStatus }

// ─── Views ────────────────────────────────────────────────────────────────────

func (m Model) viewWelcome() string {
	// Braille logo rendered in orange primary colour.
	logoArt := lipgloss.NewStyle().Foreground(common.ColorPrimary).Render(common.NexusLogo)

	wordmark := m.styles.Logo.Render("◉ NEXUS")
	tagline := m.styles.HeaderModel.Render("One runtime. Any LLM. Any language.")

	hint := strings.Join([]string{
		m.styles.Key.Render("ctrl+n") + " " + m.styles.Desc.Render("new session"),
		m.styles.Key.Render("ctrl+s") + " " + m.styles.Desc.Render("sessions"),
		m.styles.Key.Render("ctrl+q") + " " + m.styles.Desc.Render("quit"),
	}, "  ")

	body := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height-2).
		Align(lipgloss.Center, lipgloss.Center).
		Render(logoArt + "\n" + wordmark + "\n\n" + tagline + "\n\n" + hint)

	return m.header() + "\n" + body
}

func (m Model) viewChat() string {
	inputView := m.inputView()
	chatH := m.height - headerHeight - footerHeight - lipgloss.Height(inputView)
	chatW := m.width
	var detailView string
	if m.chat.DetailsOpen() && m.width >= 110 {
		paneW := max(36, m.width/3)
		chatW = max(40, m.width-paneW-1)
		m.chat.SetSize(chatW, max(1, chatH))
		detailView = m.chat.DetailView(m.width-chatW-1, max(1, chatH))
	} else {
		m.chat.SetSize(chatW, max(1, chatH))
	}
	chatView := m.chat.View()
	body := chatView
	if detailView != "" {
		body = lipgloss.JoinHorizontal(lipgloss.Top, chatView, " ", detailView)
	}

	base := strings.Join([]string{
		m.header(),
		body,
		inputView,
		m.footer(),
	}, "\n")

	if m.state == statePermission && m.permission.HasPending() {
		overlay := m.permission.View()
		return common.OverlayOn(base, overlay, m.width, m.height)
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

func (m Model) header() string {
	logo := m.styles.Logo.Render("◉ NEXUS")
	sep := m.styles.HeaderSep.Render("  │  ")
	model := m.styles.HeaderModel.Render(m.workspace.ModelString())

	var status string
	if m.busy {
		status = m.spinner.View() + " " + m.styles.HeaderBusy.Render("working")
	} else if m.focus == uiFocusMain && m.state == stateChat {
		status = m.styles.HeaderBusy.Render("↕ chat") + "  " + m.styles.HeaderID.Render("n/p: tools · o: details")
	} else if m.activeSession != "" {
		status = m.styles.HeaderReady.Render("●") + " " + m.styles.HeaderID.Render(common.ShortID(m.activeSession))
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
	// Select mode banner takes priority.
	if m.selectMode {
		return lipgloss.NewStyle().
			Foreground(common.ColorPrimary).Bold(true).
			Render("SELECT MODE") +
			"  " +
			m.styles.Desc.Render("select text with mouse · copy with ctrl+c ·") +
			"  " +
			m.styles.Key.Render("ctrl+e") +
			" " +
			m.styles.Desc.Render("exit")
	}

	if m.copyNotice != "" {
		return m.styles.ToolDone.Render("✓ " + m.copyNotice)
	}

	var items []string
	if m.focus == uiFocusMain && m.state == stateChat {
		items = []string{
			m.styles.Key.Render("↑↓") + " " + m.styles.Desc.Render("scroll"),
			m.styles.Key.Render("n/p") + " " + m.styles.Desc.Render("select tool"),
			m.styles.Key.Render("space") + " " + m.styles.Desc.Render("expand"),
			m.styles.Key.Render("o") + " " + m.styles.Desc.Render("details"),
			m.styles.Key.Render("tab") + " " + m.styles.Desc.Render("back to input"),
		}
	} else {
		items = []string{
			m.styles.Key.Render("ctrl+p") + " " + m.styles.Desc.Render("commands"),
			m.styles.Key.Render("ctrl+n") + " " + m.styles.Desc.Render("new"),
			m.styles.Key.Render("ctrl+s") + " " + m.styles.Desc.Render("sessions"),
			m.styles.Key.Render("ctrl+e") + " " + m.styles.Desc.Render("select"),
			m.styles.Key.Render("tab") + " " + m.styles.Desc.Render("chat/tools"),
			m.styles.Key.Render("ctrl+c") + " " + m.styles.Desc.Render("cancel/quit"),
		}
	}
	return m.styles.Footer.Render(strings.Join(items, "  "))
}

// Select mode banner takes priority.
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
	m.commands.SetSize(m.width, m.height)
	m.configPanel.SetSize(m.width, m.height)
	m.chat.SetSize(m.width, max(1, m.height-headerHeight-footerHeight-inputMinH-inputPadding))
	return m
}

func (m Model) resizeInput() Model {
	lines := strings.Count(m.input.Value(), "\n") + 1
	h := common.Clamp(lines, inputMinH, inputMaxH)
	m.input.SetHeight(h)
	return m
}

func (m Model) prevChatState() uiState {
	if m.activeSession != "" {
		return stateChat
	}
	return stateWelcome
}

// ─── Clipboard ───────────────────────────────────────────────────────────────

// copyToClipboard copies text using OSC 52 (tea.SetClipboard) and the native
// clipboard (atotto/clipboard), then shows a transient notice in the footer.
// This mirrors crush's CopyToClipboard approach for maximum terminal compat.
func (m *Model) copyToClipboard(text, notice string) tea.Cmd {
	m.copyNotice = notice
	return tea.Sequence(
		tea.SetClipboard(text),
		func() tea.Msg {
			_ = clipboard.WriteAll(text)
			return nil
		},
		tea.Tick(2*time.Second, func(time.Time) tea.Msg {
			return clearCopyNoticeMsg{}
		}),
	)
}

// ─── Workspace commands ───────────────────────────────────────────────────────

func boolCmd(ok bool) tea.Cmd {
	if ok {
		return func() tea.Msg { return nil }
	}
	return nil
}

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

func (m Model) loadProviderConfig() tea.Cmd {
	return func() tea.Msg {
		providers := m.workspace.LoadProviderConfig(m.ctx)
		return providerConfigLoadedMsg{providers: providers}
	}
}

func (m Model) deleteSession(id string) tea.Cmd {
	return func() tea.Msg {
		_ = m.workspace.DeleteSession(m.ctx, id)
		m.workspace.ListSessions(m.ctx)
		return nil
	}
}

// ─── Utilities ────────────────────────────────────────────────────────────────

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
