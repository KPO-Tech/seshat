// Package model implements the BubbleTea TUI for nexus-engine, adapted from
// Charm's crush project architecture (BubbleTea state machine, workspace
// abstraction, draw cache, permission dialog, session browser).
package app

import (
	"context"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/EngineerProjects/nexus-engine/internal/tui"
	"github.com/EngineerProjects/nexus-engine/internal/tui/common"
	"github.com/EngineerProjects/nexus-engine/internal/tui/components"
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
	stateSearchConfig
)

// uiFocus mirrors crush's focus model: editor has the cursor / main lets
// the user scroll chat with arrow keys.
type uiFocus uint8

const (
	uiFocusEditor uiFocus = iota // textarea is active (default)
	uiFocusMain                  // chat list is scrollable with arrow keys
)

const (
	headerHeight  = 1
	contentTopGap = 1
	footerHeight  = 1
	statusHeight  = 1
	inputMinH     = 1
	inputMaxH     = 10
	inputPadding  = 1
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
	searchPanel      *components.SearchPanel
	completions      *components.FileCompletions
	skillCompletions *components.SkillCompletions
	attachments      *components.Attachments
	input            textarea.Model
	spinner          spinner.Model

	focus               uiFocus
	busy                bool
	cancelling          bool // true between ESC press and TurnDoneMsg arrival
	activeSession       string
	pendingPrompt       string // queued when Enter is pressed before session creation completes
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
		searchPanel:      components.NewSearchPanel(styles),
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
	case openSettingsMsg:
		m.returnState = stateWelcome
		m.state = stateProviderConfig
		return m, m.loadProviderConfig()

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
		wasOpen := m.chat.DetailsOpen()
		m.chat.AddToolProgress(msg.ToolUseID, msg.ToolName, msg.Status, label, msg.Metadata)
		if !wasOpen && m.chat.DetailsOpen() {
			m = m.relayout()
		}

	case tui.TurnStartMsg:
		m.busy = true
		m.lastTurnErr = ""
		m.chat.StartAssistantMessage()
		cmds = append(cmds, m.spinner.Tick)

	case tui.TurnDoneMsg:
		wasCancelling := m.cancelling
		m.busy = false
		m.cancelling = false
		m.lastInputTokens = msg.InputTokens
		m.lastOutputTokens = msg.OutputTokens
		m.lastStopReason = msg.StopReason
		m.sessionInputTokens += msg.InputTokens
		m.sessionOutputTokens += msg.OutputTokens
		m.lastTurnErr = ""
		m.chat.FinishAssistantMessage(msg.InputTokens, msg.OutputTokens, msg.StopReason)
		if msg.Err != nil && !wasCancelling &&
			msg.Err != context.Canceled && msg.Err != context.DeadlineExceeded {
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

	case pendingSubmitMsg:
		if m.activeSession != "" {
			m.chat.AddUserMessage(msg.prompt)
			m.workspace.Submit(m.ctx, msg.prompt)
		} else {
			m.pendingPrompt = msg.prompt
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
			m.chat.AddSystem("New session · " + common.ShortID(msg.ID))
			if prompt := m.pendingPrompt; prompt != "" {
				m.pendingPrompt = ""
				m.chat.AddUserMessage(prompt)
				m.workspace.Submit(m.ctx, prompt)
			}
			cmds = append(cmds, m.input.Focus()) // v2: Focus() returns a Cmd
		}

	case tui.SessionLoadedMsg:
		if msg.Err != nil {
			m.lastErr = msg.Err
			m.lastTurnErr = "session load failed: " + msg.Err.Error()
			m.state = stateChat
		} else {
			m.activeSession = msg.ID
			m.state = stateChat
			m.focus = uiFocusEditor
			m.chat.Clear()
			for _, entry := range msg.History {
				switch entry.Role {
				case "user":
					m.chat.AddUserMessage(entry.Text)
				case "assistant":
					m.chat.StartAssistantMessage()
					if entry.Thinking != "" {
						m.chat.AppendChunk(entry.Thinking, true)
					}
					if entry.Text != "" {
						m.chat.AppendChunk(entry.Text, false)
					}
					for _, tool := range entry.Tools {
						// Start from the persisted TUI metadata (content,
						// execution_duration_ms, lines_added, exit_code, …).
						// Always inject tool_input so detail renderers can
						// access file paths, commands, etc.
						meta := make(map[string]any, len(tool.Metadata)+1)
						for k, v := range tool.Metadata {
							meta[k] = v
						}
						meta["tool_input"] = tool.Input
						m.chat.AddToolProgress(tool.ID, tool.Name, "completed", "", meta)
					}
					m.chat.FinishAssistantMessage(entry.InputTokens, entry.OutputTokens, entry.StopReason)
				}
			}
			m.chat.AddSystem("↑ session resumed · " + common.ShortID(msg.ID))
			cmds = append(cmds, m.input.Focus()) // v2: Focus() returns a Cmd
		}

	case sessionDeleteResultMsg:
		if msg.err != nil {
			m.lastErr = msg.err
			m.lastTurnErr = "session delete failed: " + msg.err.Error()
		} else {
			m.lastTurnErr = ""
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

	case searchConfigLoadedMsg:
		m.searchPanel.SetConfig(msg.config)

	case searchKeySaveResultMsg:
		if msg.err != nil {
			m.searchPanel.SetError(msg.err.Error())
		} else {
			m.searchPanel.SetKeySaved()
		}

	case searchModeSaveResultMsg:
		if msg.err != nil {
			m.searchPanel.SetError(msg.err.Error())
		} else {
			m.searchPanel.SetModeSaved()
		}

	// v2 uses KeyPressMsg instead of KeyMsg
	case tea.KeyPressMsg:
		// Defensive: ensure completions are closed if we are not in a chat-related state.
		// This handles cases where a state transition happened (e.g. to config) while
		// a popup was open.
		if m.state != stateChat && m.state != stateWelcome {
			if m.completions.IsOpen() {
				m.completions.Close()
			}
			if m.skillCompletions.IsOpen() {
				m.skillCompletions.Close()
			}
		}

		consumed, cmd := m.handleKey(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		// Non-consumed keys flow to the textarea so regular characters,
		// backspace, and cursor movement work normally.
		// ONLY forward if in chat/welcome state AND a model is configured.
		// If no model is configured, the user MUST use global shortcuts (ctrl+p, etc.)
		// to set one up before they can type anything.
		canType := (m.state == stateChat || m.state == stateWelcome) && m.workspace.ModelString() != ""
		if !consumed && canType {
			newInput, inputCmd := m.input.Update(msg)
			m.input = newInput
			cmds = append(cmds, inputCmd)
			m = m.resizeInput()
			m.syncComposerAssist()
		} else if !consumed && m.state == stateWelcome && m.workspace.ModelString() == "" {
			// Explicitly close completions if user tries to type in empty welcome state.
			if m.completions.IsOpen() {
				m.completions.Close()
			}
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
		layout := m.currentChatLayout()
		if (m.state == stateChat || m.state == stateWelcome) && m.skillCompletions.IsOpen() {
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
		// Mouse wheel scrolls the active content under the pointer.
		// For the sidebar, only check the X coordinate — the sidebar occupies the
		// right portion of the screen from detailX onward. Using a Y-range check
		// was unreliable because minor layout-height differences caused misses.
		if m.state == stateChat || m.state == stateWelcome {
			if m.chat.DetailsOpen() && layout.detailW > 0 && msg.X >= layout.detailX {
				switch msg.Button {
				case tea.MouseWheelUp:
					m.chat.DetailScrollUp(3)
				case tea.MouseWheelDown:
					m.chat.DetailScrollDown(3)
				}
				return m, tea.Batch(cmds...)
			}
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
	} else {
		// Ensure completions are closed if we are not in a chat state.
		// This is defensive against state transitions while a popup was open.
		if m.completions.IsOpen() {
			m.completions.Close()
		}
		if m.skillCompletions.IsOpen() {
			m.skillCompletions.Close()
		}
	}

	return m, tea.Batch(cmds...)
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
