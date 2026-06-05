// Package model implements the BubbleTea TUI for nexus-engine, adapted from
// Charm's crush project architecture (BubbleTea state machine, workspace
// abstraction, draw cache, permission dialog, session browser).
package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
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
	statusHeight = 1
	inputMinH    = 1
	inputMaxH    = 10
	inputPadding = 1
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

	chat             *components.Chat
	sessions         *components.SessionList
	permission       *components.PermissionDialog
	modelSelect      *components.ModelPicker
	commands         *components.CommandPalette
	configPanel      *components.ConfigPanel
	completions      *components.FileCompletions
	skillCompletions *components.SkillCompletions
	attachments      *components.Attachments
	input            textarea.Model
	spinner          spinner.Model

	focus               uiFocus
	busy                bool
	activeSession       string
	lastErr             error
	permInput           string
	copyNotice          string  // transient "Copied!" message shown in footer
	returnState         uiState // state to restore when pressing ← from a sub-dialog
	lastInputTokens     int
	lastOutputTokens    int
	lastStopReason      string
	sessionInputTokens  int
	sessionOutputTokens int
	lastTurnErr         string
	skillCatalog        []tui.SkillInfo
	skillCatalogLoaded  bool
}

func New(ws tui.Workspace, ctx context.Context) Model {
	ctx, cancel := context.WithCancel(ctx)

	styles := common.DefaultStyles()
	keys := common.DefaultKeys()

	ta := textarea.New()
	ta.SetStyles(styles.Textarea)
	ta.Placeholder = "Ask Nexus...  /skill"
	ta.ShowLineNumbers = false
	ta.CharLimit = -1
	ta.SetVirtualCursor(true)
	ta.DynamicHeight = true
	ta.MinHeight = inputMinH
	ta.MaxHeight = inputMaxH
	ta.SetPromptFunc(4, editorPrompt(styles))
	ta.SetWidth(80)
	ta.SetHeight(inputMinH)
	// Don't call Focus() here — do it in Init() so the Cmd runs properly.

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(common.ColorYellow)

	return Model{
		workspace:        ws,
		ctx:              ctx,
		cancel:           cancel,
		state:            stateWelcome,
		focus:            uiFocusEditor,
		keys:             keys,
		styles:           styles,
		chat:             components.NewChat(styles, 80, 20),
		sessions:         components.NewSessionList(styles),
		permission:       components.NewPermissionDialog(styles),
		modelSelect:      components.NewModelPicker(styles),
		commands:         components.NewCommandPalette(styles),
		configPanel:      components.NewConfigPanel(styles),
		completions:      components.NewFileCompletions(styles, ws.WorkingDir()),
		skillCompletions: components.NewSkillCompletions(styles),
		attachments:      components.NewAttachments(styles),
		input:            ta,
		spinner:          sp,
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
		m.lastTurnErr = ""
		m.chat.StartAssistantMessage()
		cmds = append(cmds, m.spinner.Tick)

	case tui.TurnDoneMsg:
		m.busy = false
		m.lastInputTokens = msg.InputTokens
		m.lastOutputTokens = msg.OutputTokens
		m.lastStopReason = msg.StopReason
		m.sessionInputTokens += msg.InputTokens
		m.sessionOutputTokens += msg.OutputTokens
		m.lastTurnErr = ""
		m.chat.FinishAssistantMessage(msg.InputTokens, msg.OutputTokens, msg.StopReason)
		if msg.Err != nil {
			m.lastTurnErr = msg.Err.Error()
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
			m.sessionInputTokens = 0
			m.sessionOutputTokens = 0
			m.chat.Clear()
			m.sessionInputTokens = 0
			m.sessionOutputTokens = 0
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
			m.syncComposerAssist()
		}
		return m, tea.Batch(cmds...)

	case tea.MouseClickMsg:
		if cmd := m.handleMouseClick(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case tea.MouseMotionMsg:
		if m.handleMouseMotion(msg) {
			return m, tea.Batch(cmds...)
		}

	case tea.MouseReleaseMsg:
		if cmd := m.handleMouseRelease(msg); cmd != nil {
			cmds = append(cmds, cmd)
			return m, tea.Batch(cmds...)
		}
	case tea.MouseWheelMsg:
		if (m.state == stateChat || m.state == stateWelcome) && m.skillCompletions.IsOpen() {
			layout := m.currentChatLayout()
			if pointInRect(msg.X, msg.Y, layout.popupX, layout.popupY, layout.popupW, layout.popupH) {
				switch msg.Button {
				case tea.MouseWheelUp:
					m.skillCompletions.Scroll(-1)
				case tea.MouseWheelDown:
					m.skillCompletions.Scroll(1)
				}
				return m, tea.Batch(cmds...)
			}
		}
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
		m.syncComposerAssist()
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
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

type chatLayout struct {
	contentW int
	contentX int
	chatX    int
	chatY    int
	chatW    int
	chatH    int
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
	inputView := m.inputView()
	statusView := m.statusLine()
	contentW := m.contentWidth()
	chatH := m.height - headerHeight - footerHeight - lipgloss.Height(statusView) - lipgloss.Height(inputView)
	chatW := contentW
	inputW := max(12, contentW-2)
	inputX := max(0, (m.width-inputW)/2)
	popupW := 0
	popupH := 0
	popupX := inputX
	if m.chat.DetailsOpen() && contentW >= 110 {
		paneW := max(36, contentW/3)
		chatW = max(40, contentW-paneW-1)
	}
	contentX := max(0, (m.width-contentW)/2)
	chatY := headerHeight
	inputY := chatY + max(1, chatH) + lipgloss.Height(statusView)
	if m.skillCompletions.IsOpen() {
		popupW = m.skillCompletions.Width(max(24, contentW-4))
		popupH = m.skillCompletions.Height(max(24, contentW-4))
	}
	return chatLayout{
		contentW: contentW,
		contentX: contentX,
		chatX:    contentX,
		chatY:    chatY,
		chatW:    chatW,
		chatH:    max(1, chatH),
		inputX:   inputX,
		inputY:   inputY,
		inputW:   inputW,
		inputH:   lipgloss.Height(inputView),
		popupX:   popupX,
		popupY:   inputY,
		popupW:   popupW,
		popupH:   popupH,
	}
}

func pointInRect(x, y, rx, ry, rw, rh int) bool {
	return x >= rx && x < rx+rw && y >= ry && y < ry+rh
}

func clampMouse(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (m *Model) handleMouseClick(msg tea.MouseClickMsg) tea.Cmd {
	if m.state != stateChat {
		return nil
	}
	layout := m.currentChatLayout()
	if m.skillCompletions.IsOpen() && pointInRect(msg.X, msg.Y, layout.popupX, layout.popupY, layout.popupW, layout.popupH) {
		if msg.Button == tea.MouseLeft {
			row := msg.Y - layout.popupY - 1
			if sel := m.skillCompletions.ClickRow(row); sel != "" {
				m.input.SetValue(sel + " ")
				m.input.CursorEnd()
				m.skillCompletions.Close()
				m.focus = uiFocusEditor
				*m = m.resizeInput()
				return m.input.Focus()
			}
		}
		return nil
	}
	if pointInRect(msg.X, msg.Y, layout.inputX, layout.inputY+layout.popupH, layout.inputW, layout.inputH-layout.popupH) {
		m.focus = uiFocusEditor
		return m.input.Focus()
	}
	if !pointInRect(msg.X, msg.Y, layout.chatX, layout.chatY, layout.chatW, layout.chatH) {
		return nil
	}
	if msg.Button == tea.MouseRight {
		if text := m.chat.SelectedText(); text != "" {
			return m.copyToClipboard(text, "Selection copied")
		}
		return nil
	}
	if msg.Button != tea.MouseLeft {
		return nil
	}
	m.focus = uiFocusMain
	m.input.Blur()
	m.chat.HandleMouseDown(msg.X-layout.chatX, msg.Y-layout.chatY)
	return nil
}

func (m *Model) handleMouseMotion(msg tea.MouseMotionMsg) bool {
	if m.state != stateChat || !m.chat.HasMouseCapture() {
		return false
	}
	layout := m.currentChatLayout()
	relX := msg.X - layout.chatX
	relY := msg.Y - layout.chatY
	return m.chat.HandleMouseDrag(relX, relY)
}

func (m *Model) handleMouseRelease(msg tea.MouseReleaseMsg) tea.Cmd {
	if m.state != stateChat || !m.chat.HasMouseCapture() {
		return nil
	}
	layout := m.currentChatLayout()
	relX := msg.X - layout.chatX
	relY := msg.Y - layout.chatY
	if text := m.chat.HandleMouseUp(relX, relY); text != "" {
		return m.copyToClipboard(text, "Selection copied")
	}
	return nil
}

// ─── Key handling ─────────────────────────────────────────────────────────────

// handleKey processes a keypress. Returns (consumed, cmd):
//   - consumed=true  → key was handled; do NOT forward to textarea
//   - consumed=false → key was not handled; forward to textarea for normal input
func (m *Model) handleKey(msg tea.KeyPressMsg) (bool, tea.Cmd) {
	k := msg.String()
	stroke := msg.Keystroke()

	if stroke == "ctrl+shift+c" {
		if text := m.chat.SelectedText(); text != "" {
			return true, m.copyToClipboard(text, "Selection copied")
		}
		return true, nil
	}

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
			if m.returnState == stateCommands {
				m.refreshSettingsHubData()
				m.state = stateCommands
				m.commands.Open("")
			} else {
				m.state = m.prevChatState()
			}
		case "left":
			// Navigate back to the settings hub if that's where we came from,
			// otherwise close to the chat/welcome state.
			if m.returnState == stateCommands {
				m.refreshSettingsHubData()
				m.state = stateCommands
				m.commands.Open("")
			} else {
				m.state = m.prevChatState()
			}
		case "up":
			m.modelSelect.Up()
		case "down":
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

	// ── Settings hub (all keys consumed) ────────────────────────────────
	if m.state == stateCommands {
		switch k {
		case "esc", "ctrl+p":
			if !m.commands.Back() {
				m.state = m.prevChatState()
			}
		case "left":
			if !m.commands.Back() {
				m.state = m.prevChatState()
			}
		case "up":
			m.commands.Up()
		case "down":
			m.commands.Down()
		case "enter":
			return true, m.activateSettingsSelection()
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
			case "up":
				cp.Up()
			case "down":
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
				if m.returnState == stateCommands {
					m.refreshSettingsHubData()
					m.state = stateCommands
					m.commands.Open("")
				} else {
					m.state = m.prevChatState()
				}
			case "up":
				cp.Up()
			case "down":
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
		case "up":
			m.sessions.Up()
		case "down":
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
			m.refreshSettingsHubData()
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
			case "up":
				m.chat.ScrollUp(3)
				return true, nil
			case "down":
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

		// Slash-skill suggestions intercept keys while open.
		if m.skillCompletions.IsOpen() {
			switch k {
			case "esc":
				m.skillCompletions.Close()
				return true, nil
			case "up":
				m.skillCompletions.Up()
				return true, nil
			case "down":
				m.skillCompletions.Down()
				return true, nil
			case "enter", "tab":
				if sel := m.skillCompletions.Selected(); sel != "" {
					m.input.SetValue(sel + " ")
					m.input.CursorEnd()
					m.skillCompletions.Close()
					*m = m.resizeInput()
					return true, nil
				}
				m.skillCompletions.Close()
				return false, nil
			default:
				return false, nil
			}
		}

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
			// Slash is reserved for skills. Let the textarea receive it directly.
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
			*m = m.resizeInput()
			m.chat.AddUserMessage(text)
			m.workspace.Submit(m.ctx, text)
			m.syncComposerAssist()
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

func (m *Model) activateSettingsSelection() tea.Cmd {
	sel := m.commands.Selected()
	if sel == nil {
		return nil
	}
	switch sel.Kind {
	case components.PaletteSectionKind:
		m.commands.OpenSection(sel.ID)
		return nil
	case components.PaletteRouteKind:
		switch sel.ID {
		case "providers":
			m.returnState = stateCommands
			m.state = stateProviderConfig
			return m.loadProviderConfig()
		case "models":
			m.returnState = stateCommands
			m.state = stateModelSelect
			m.modelSelect.ClearFilter()
			return m.listModels()
		}
	case components.PaletteActionKind:
		cmd := m.executeCommand(sel.ID)
		if m.state == stateCommands {
			m.state = m.prevChatState()
		}
		return cmd
	case components.PaletteInfoKind:
		if strings.HasPrefix(sel.Name, "/") {
			return m.insertSkillIntoComposer(sel.Name)
		}
		return nil
	}
	return nil
}

func (m *Model) insertSkillIntoComposer(skill string) tea.Cmd {
	m.state = m.prevChatState()
	m.focus = uiFocusEditor
	m.input.SetValue(skill + " ")
	m.input.CursorEnd()
	*m = m.resizeInput()
	return m.input.Focus()
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
	case "copy-msg":
		text := m.chat.GetLastUserText()
		if text != "" {
			return m.copyToClipboard(text, "Message copied")
		}
		return nil
	case "provider-config":
		m.state = stateProviderConfig
		return m.loadProviderConfig()
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

func editorPrompt(styles common.Styles) func(textarea.PromptInfo) string {
	return func(info textarea.PromptInfo) string {
		if info.LineNumber == 0 {
			if info.Focused {
				return styles.InputPrompt.Render("> ")
			}
			return styles.InputHint.Render("> ")
		}
		return "  "
	}
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

	contentW := m.contentWidth()
	body := lipgloss.NewStyle().
		Width(contentW).
		Height(m.height-2).
		Align(lipgloss.Center, lipgloss.Center).
		Render(logoArt + "\n" + wordmark + "\n\n" + tagline + "\n\n" + hint)

	return m.header() + "\n" + common.CenterHorizontally(body, m.width)
}

func (m Model) viewChat() string {
	inputView := m.inputView()
	statusView := m.statusLine()
	contentW := m.contentWidth()
	chatH := m.height - headerHeight - footerHeight - lipgloss.Height(statusView) - lipgloss.Height(inputView)
	chatW := contentW
	var detailView string
	if m.chat.DetailsOpen() && contentW >= 110 {
		paneW := max(36, contentW/3)
		chatW = max(40, contentW-paneW-1)
		m.chat.SetSize(chatW, max(1, chatH))
		detailView = m.chat.DetailView(contentW-chatW-1, max(1, chatH))
	} else {
		m.chat.SetSize(chatW, max(1, chatH))
	}
	chatView := m.chat.View()
	body := chatView
	if detailView != "" {
		body = lipgloss.JoinHorizontal(lipgloss.Top, chatView, " ", detailView)
	}
	body = common.CenterHorizontally(lipgloss.NewStyle().Width(contentW).Render(body), m.width)

	base := strings.Join([]string{
		m.header(),
		body,
		statusView,
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
	contentW := m.contentWidth()
	logo := m.styles.Logo.Render("NEXUS")
	model := m.styles.HeaderPill.Render(m.workspace.ModelString())
	left := lipgloss.JoinHorizontal(lipgloss.Center, logo, " ", model)

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

func (m Model) statusLine() string {
	contentW := m.contentWidth()
	var line string
	switch {
	case m.busy:
		line = m.styles.Footer.Width(contentW).Render(m.styles.HeaderPillBusy.Render(m.spinner.View() + " working"))
	case m.lastTurnErr != "":
		line = m.styles.Footer.Width(contentW).Render(m.styles.ToolError.Render("failed") + "  " + m.styles.Desc.Render(truncateStatus(m.lastTurnErr, max(12, contentW/2))))
	default:
		line = m.styles.Footer.Width(contentW).Render(m.styles.Desc.Render("ready"))
	}
	return common.CenterHorizontally(line, m.width)
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
			m.styles.Key.Render("o") + " " + m.styles.Desc.Render("details"),
			m.styles.Key.Render("ctrl+p") + " " + m.styles.Desc.Render("settings"),
		}
	} else {
		leftItems = []string{
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

// Select mode banner takes priority.
func (m Model) inputView() string {
	contentW := m.contentWidth()
	inner := m.input.View()

	if attView := m.attachments.View(max(20, contentW-4)); attView != "" {
		inner = attView + "\n" + inner
	}

	box := m.styles.InputBorder.Width(max(12, contentW-2)).Render(inner)
	stackW := lipgloss.Width(box)
	if m.skillCompletions.IsOpen() {
		popup := m.skillCompletions.View(max(24, contentW-4))
		stack := lipgloss.NewStyle().Width(stackW).Render(popup) + "\n" + box
		return common.CenterHorizontally(stack, m.width)
	}
	if m.completions.IsOpen() {
		popup := m.completions.View(max(20, contentW-4))
		stack := lipgloss.NewStyle().Width(stackW).Render(popup) + "\n" + box
		return common.CenterHorizontally(stack, m.width)
	}
	return common.CenterHorizontally(box, m.width)
}

// ─── Layout ───────────────────────────────────────────────────────────────────

func (m Model) relayout() Model {
	contentW := m.contentWidth()
	inputW := contentW - 4
	if inputW < 10 {
		inputW = 10
	}
	m.input.SetWidth(inputW)
	m.sessions.SetSize(m.width, m.height)
	m.permission.SetSize(m.width, m.height)
	m.modelSelect.SetSize(m.width, m.height)
	m.commands.SetSize(m.width, m.height)
	m.configPanel.SetSize(m.width, m.height)
	m.chat.SetSize(contentW, max(1, m.height-headerHeight-footerHeight-statusHeight-inputMinH-inputPadding))
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
// This mirrors crush's CopyToClipboard approach, but avoids claiming success
// when the local session has no actual clipboard backend.
func (m *Model) copyToClipboard(text, notice string) tea.Cmd {
	m.copyNotice = copyNoticeForCapability(notice)
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

func copyNoticeForCapability(success string) string {
	return clipboardNotice(success, nativeClipboardLikelyAvailable(), terminalClipboardLikelyAvailable())
}

func clipboardNotice(success string, nativeAvailable, terminalAvailable bool) string {
	switch {
	case nativeAvailable:
		return success
	case terminalAvailable:
		return success + " (terminal clipboard requested)"
	default:
		return "Clipboard unavailable: install wl-clipboard or xclip"
	}
}

func nativeClipboardLikelyAvailable() bool {
	switch runtime.GOOS {
	case "windows", "darwin":
		return true
	}
	for _, name := range []string{"wl-copy", "xclip", "xsel", "pbcopy", "clip.exe", "powershell.exe"} {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}

func terminalClipboardLikelyAvailable() bool {
	term := strings.TrimSpace(os.Getenv("TERM"))
	return term != "" && term != "dumb"
}

// ─── Workspace commands ───────────────────────────────────────────────────────

func boolCmd(ok bool) tea.Cmd {
	if ok {
		return func() tea.Msg { return nil }
	}
	return nil
}

func (m *Model) syncComposerAssist() {
	if m.completions.IsOpen() {
		m.skillCompletions.Close()
		return
	}
	skills := m.loadSkillCatalog()
	m.skillCompletions.Sync(skills, m.input.Value())
}

func (m *Model) loadSkillCatalog() []tui.SkillInfo {
	if !m.skillCatalogLoaded {
		m.skillCatalog = m.workspace.LoadSkills(m.ctx)
		m.skillCatalogLoaded = true
	}
	return m.skillCatalog
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

func (m *Model) refreshSettingsHubData() {
	m.commands.SetSectionItems("tools", buildToolSettingsItems(m.workspace.LoadToolCatalog(m.ctx)))
	m.commands.SetSectionItems("mcp", buildMCPSettingsItems(m.workspace.LoadMCPServers(m.ctx)))
	m.skillCatalog = m.workspace.LoadSkills(m.ctx)
	m.skillCatalogLoaded = true
	m.commands.SetSectionItems("skills", buildSkillSettingsItems(m.skillCatalog))
}

func buildToolSettingsItems(items []tui.ToolInfo) []components.PaletteItem {
	if len(items) == 0 {
		return []components.PaletteItem{{
			Kind: components.PaletteInfoKind,
			ID:   "tools-empty",
			Name: "No tools found",
			Desc: "The current runtime did not expose any tools",
		}}
	}
	result := make([]components.PaletteItem, 0, len(items))
	for _, item := range items {
		desc := strings.TrimSpace(item.Description)
		if category := strings.TrimSpace(item.Category); category != "" {
			if desc != "" {
				desc = category + " · " + desc
			} else {
				desc = category
			}
		}
		result = append(result, components.PaletteItem{
			Kind: components.PaletteInfoKind,
			ID:   "tool-" + item.Name,
			Name: item.Name,
			Desc: desc,
		})
	}
	return result
}

func buildMCPSettingsItems(items []tui.MCPServerInfo) []components.PaletteItem {
	if len(items) == 0 {
		return []components.PaletteItem{{
			Kind: components.PaletteInfoKind,
			ID:   "mcp-empty",
			Name: "No MCP servers configured",
			Desc: "Add MCP servers in config to expose them here",
		}}
	}
	result := make([]components.PaletteItem, 0, len(items))
	for _, item := range items {
		desc := item.Status + " · " + strconv.Itoa(item.ToolsRegistered) + " tools"
		if item.Error != "" {
			desc += " · " + item.Error
		}
		result = append(result, components.PaletteItem{
			Kind: components.PaletteInfoKind,
			ID:   "mcp-" + item.Name,
			Name: item.Name,
			Desc: desc,
		})
	}
	return result
}

func buildSkillSettingsItems(items []tui.SkillInfo) []components.PaletteItem {
	if len(items) == 0 {
		return []components.PaletteItem{{
			Kind: components.PaletteInfoKind,
			ID:   "skills-empty",
			Name: "No skills found",
			Desc: "Add bundled, repo, or user skills to invoke them with /skill",
		}}
	}
	result := make([]components.PaletteItem, 0, len(items))
	for _, item := range items {
		desc := strings.TrimSpace(item.Description)
		if desc == "" {
			desc = strings.TrimSpace(item.WhenToUse)
		}
		if source := strings.TrimSpace(item.Source); source != "" {
			if desc != "" {
				desc = source + " · " + desc
			} else {
				desc = source
			}
		}
		result = append(result, components.PaletteItem{
			Kind: components.PaletteInfoKind,
			ID:   "skill-" + item.Name,
			Name: "/" + item.Name,
			Desc: desc,
		})
	}
	return result
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
